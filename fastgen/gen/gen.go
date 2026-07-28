// Package gen emits Go source from a parsed FAST template AST. For each
// template it generates a message struct (one Go field per template field) and
// a stateful decoder type holding the operator dictionaries; the decoder's
// Decode method reads a presence map and decodes each field via fastcore calls.
//
// Generated decoders are reusable across messages: the dictionary slots persist
// (that is what the copy/increment/delta/tail operators read), and the caller
// owns the message struct that fields decode into.
//
// Layout lives in templates.go; this file makes the decisions (operator/type
// dispatch, slot collection, initial-value conversion) and fills the views.
package gen

import (
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"

	"github.com/iamd3vil/gofastdecoder/fastgen/ast"
)

// Generate produces formatted Go source for schema s in package pkg.
func Generate(s *ast.Schema, pkg string) ([]byte, error) {
	g := &generator{
		imports:  map[string]bool{"github.com/iamd3vil/gofastdecoder/fastcore": true},
		enumSeen: map[string]bool{},
	}

	blocks := make([]string, 0, len(s.Templates)+2)
	var routerTemplates []routerEntry
	for _, t := range s.Templates {
		blk, err := g.templateBlock(t)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", t.Name, err)
		}
		blocks = append(blocks, blk)
		routerTemplates = append(routerTemplates, routerEntry{Name: exported(t.Name), ID: t.ID})
	}

	// Typed enum/set declarations (collected while emitting fields), before
	// their first use.
	if len(g.enums) > 0 {
		enumBlock, err := render("enums", struct{ Enums []enumView }{buildEnumViews(g.enums)})
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, enumBlock)
	}

	// A Router over all templates: dispatches on the template id (PMAP bit 0).
	if len(routerTemplates) > 0 {
		g.imports["fmt"] = true
		router, err := render("router", struct{ Templates []routerEntry }{routerTemplates})
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, router)
	}

	imports := make([]string, 0, len(g.imports))
	for imp := range g.imports {
		imports = append(imports, imp)
	}
	sort.Strings(imports)

	var out strings.Builder
	if err := codeTemplates.ExecuteTemplate(&out, "file", fileView{
		Pkg: pkg, Imports: imports, Blocks: blocks,
	}); err != nil {
		return nil, err
	}

	src, err := format.Source([]byte(out.String()))
	if err != nil {
		return []byte(out.String()), fmt.Errorf("gofmt generated source: %w", err)
	}
	return src, nil
}

type generator struct {
	imports    map[string]bool
	slots      []slotDecl // accumulated for the template currently being emitted
	pmaps      []string   // nested-segment PMAP buffer field names for the current template
	topDecoder string     // receiver type name for the current template's methods
	methods    []string   // rendered method bodies for the current template
	enums      []enumType // enum/set types to emit, in first-seen order
	enumSeen   map[string]bool
}

type slotDecl struct{ Name, Type string }

// enumType is a generated Go named type for an enum or set field.
type enumType struct {
	Name     string
	Elements []ast.Element
}

// enumView is the rendered form: a type name plus fully-formed const declaration
// lines (e.g. "ColorsRed Colors = 0").
type enumView struct {
	Name   string
	Consts []string
}

// buildEnumViews turns collected enum types into render views, naming each
// constant <Type><Label> and de-duplicating names within a type.
func buildEnumViews(enums []enumType) []enumView {
	views := make([]enumView, 0, len(enums))
	for _, e := range enums {
		seen := make(map[string]bool, len(e.Elements))
		consts := make([]string, 0, len(e.Elements))
		for _, el := range e.Elements {
			cname := e.Name + exported(el.Label)
			if seen[cname] {
				cname = fmt.Sprintf("%s_%d", cname, el.Value) // disambiguate clashes
			}
			seen[cname] = true
			consts = append(consts, fmt.Sprintf("%s %s = %d", cname, e.Name, el.Value))
		}
		views = append(views, enumView{Name: e.Name, Consts: consts})
	}
	return views
}

// routerEntry is one template the generated Router can dispatch to.
type routerEntry struct {
	Name string
	ID   uint32
}

// --- views passed to templates.go ---

type fileView struct {
	Pkg     string
	Imports []string
	Blocks  []string
}
type structView struct {
	Name   string
	Fields []structFieldView
}
type structFieldView struct {
	Name, Type string
	Optional   bool
}
type decoderView struct {
	Name  string
	Pmaps []string
	Slots []slotDecl
}

// templateBlock renders the structs, decoder, and decode methods for one template.
func (g *generator) templateBlock(t *ast.Template) (string, error) {
	name := exported(t.Name)
	g.slots = g.slots[:0]
	g.pmaps = g.pmaps[:0]
	g.methods = g.methods[:0]
	g.topDecoder = name

	var structs strings.Builder
	if err := g.renderStructs(&structs, name, t.Instructions); err != nil {
		return "", err
	}
	if err := g.renderMethod(name, t.Instructions, true, ""); err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString(structs.String())
	if err := codeTemplates.ExecuteTemplate(&out, "decoder", decoderView{Name: name, Pmaps: g.pmaps, Slots: g.slots}); err != nil {
		return "", err
	}
	out.WriteString("\n")
	for _, m := range g.methods {
		out.WriteString(m)
		out.WriteString("\n")
	}
	if err := codeTemplates.ExecuteTemplate(&out, "decodeEntry", struct {
		Name  string
		Slots []slotDecl
	}{Name: name, Slots: g.slots}); err != nil {
		return "", err
	}
	return out.String(), nil
}

// renderStructs emits the struct type for instrs (recursing into nested
// element/group types first) into w.
func (g *generator) renderStructs(w *strings.Builder, name string, instrs []ast.Instruction) error {
	for _, in := range instrs {
		switch x := in.(type) {
		case *ast.Sequence:
			if err := g.renderStructs(w, name+exported(x.Name)+"Elem", x.Instructions); err != nil {
				return err
			}
		case *ast.Group:
			if err := g.renderStructs(w, name+exported(x.Name), x.Instructions); err != nil {
				return err
			}
		}
	}

	sv := structView{Name: name}
	for _, in := range instrs {
		switch x := in.(type) {
		case *ast.Field:
			if x.Type == ast.BitGroup {
				// Bit-group sub-fields are flattened into the parent struct.
				for _, bf := range x.BitFields {
					gt, err := goType(bf)
					if err != nil {
						return err
					}
					sv.Fields = append(sv.Fields, structFieldView{Name: exported(bf.Name), Type: gt})
				}
				continue
			}
			gt, err := g.goFieldType(x, name)
			if err != nil {
				return err
			}
			if x.Type == ast.Timestamp {
				g.imports["time"] = true
			}
			sv.Fields = append(sv.Fields, structFieldView{Name: exported(x.Name), Type: gt, Optional: x.Presence == ast.Optional})
		case *ast.Sequence:
			sv.Fields = append(sv.Fields, structFieldView{Name: exported(x.Name), Type: "[]" + name + exported(x.Name) + "Elem"})
		case *ast.Group:
			sv.Fields = append(sv.Fields, structFieldView{Name: exported(x.Name), Type: name + exported(x.Name), Optional: x.Presence == ast.Optional})
		}
	}
	if err := codeTemplates.ExecuteTemplate(w, "struct", sv); err != nil {
		return err
	}
	w.WriteString("\n")
	return nil
}

// renderMethod renders one segment's decode method (recursing into nested
// sequences/groups, whose methods are appended to g.methods). A top-level
// template body receives its presence map from the caller (Router or Decode);
// a nested segment reads its own into the dedicated buffer field.
func (g *generator) renderMethod(slotPrefix string, instrs []ast.Instruction, topLevel bool, bufName string) error {
	steps := make([]string, 0, len(instrs))
	var nested []func() error
	for _, in := range instrs {
		step, sub, err := g.step(slotPrefix, in)
		if err != nil {
			return err
		}
		steps = append(steps, step)
		if sub != nil {
			nested = append(nested, sub)
		}
	}

	resets := optionalHasFields(instrs)

	var body strings.Builder
	var err error
	if topLevel {
		err = codeTemplates.ExecuteTemplate(&body, "methodTop", struct {
			Recv, Name, Struct string
			Resets, Steps      []string
		}{Recv: g.topDecoder, Name: "decode" + slotPrefix, Struct: slotPrefix, Resets: resets, Steps: steps})
	} else {
		err = codeTemplates.ExecuteTemplate(&body, "methodNested", struct {
			Recv, Name, Struct, Buf string
			Resets, Steps           []string
		}{Recv: g.topDecoder, Name: "decode" + slotPrefix, Struct: slotPrefix, Buf: bufName, Resets: resets, Steps: steps})
	}
	if err != nil {
		return err
	}
	g.methods = append(g.methods, body.String())

	for _, fn := range nested {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

// optionalHasFields returns the exported Has-flag field names for the optional
// fields and groups directly in instrs. The decode method clears these at the
// start so a reused message struct does not report a field as present when it
// is absent in the current message.
func optionalHasFields(instrs []ast.Instruction) []string {
	var out []string
	for _, in := range instrs {
		switch x := in.(type) {
		case *ast.Field:
			if x.Presence == ast.Optional && x.Type != ast.BitGroup {
				out = append(out, "Has"+exported(x.Name))
			}
		case *ast.Group:
			if x.Presence == ast.Optional {
				out = append(out, "Has"+exported(x.Name))
			}
		}
	}
	return out
}

// nestedMethod registers a PMAP buffer field for a nested segment and renders
// its method.
func (g *generator) nestedMethod(slotPrefix string, instrs []ast.Instruction) error {
	buf := "pmap_" + sanitize(slotPrefix)
	g.pmaps = append(g.pmaps, buf)
	return g.renderMethod(slotPrefix, instrs, false, buf)
}

// step renders the decode statement for one instruction, returning the rendered
// text and (for sequences/groups) a closure that renders the nested method.
func (g *generator) step(slotPrefix string, in ast.Instruction) (string, func() error, error) {
	switch x := in.(type) {
	case *ast.Field:
		s, err := g.fieldStep(slotPrefix, x)
		return s, nil, err
	case *ast.Group:
		gname := exported(x.Name)
		s, err := render("group", struct {
			Field, Method string
			Optional      bool
		}{Field: gname, Method: slotPrefix + gname, Optional: x.Presence == ast.Optional})
		return s, func() error { return g.nestedMethod(slotPrefix+gname, x.Instructions) }, err
	case *ast.Sequence:
		return g.sequenceStep(slotPrefix, x)
	case *ast.TemplateRef:
		// In-file static refs are inlined by the parser. A dynamic ref or an
		// unresolved (cross-file) static ref reaches here unhandled.
		if x.Dynamic {
			return "", nil, fmt.Errorf("dynamic templateRef not yet supported")
		}
		return "", nil, fmt.Errorf("unresolved templateRef %q (target not in this template set)", x.Name)
	}
	return "", nil, fmt.Errorf("unsupported instruction %T", in)
}

func (g *generator) sequenceStep(slotPrefix string, s *ast.Sequence) (string, func() error, error) {
	sname := exported(s.Name)
	elem := slotPrefix + sname + "Elem"
	lenField := s.Length
	if lenField == nil {
		lenField = &ast.Field{Name: s.Name + "Length", Type: ast.UInt32, Presence: s.Presence}
	}
	if lenField.Op.Kind == ast.Constant {
		hasInit, initExpr, err := uintInitial(lenField)
		if err != nil {
			return "", nil, err
		}
		if !hasInit {
			return "", nil, fmt.Errorf("sequence %q: constant length without a value", s.Name)
		}
		out, err := render("sequenceConstant", struct {
			Field, Elem, Len string
			Optional         bool
		}{Field: sname, Elem: elem, Len: initExpr, Optional: s.Presence == ast.Optional})
		return out, func() error { return g.nestedMethod(elem, s.Instructions) }, err
	}
	lenSlot := g.addSlot(slotPrefix+sname+"Len", lenField)
	hasInit, initExpr, err := intInitial(lenField)
	if err != nil {
		return "", nil, err
	}
	// The rendered sequence rejects an out-of-range length with a formatted
	// error.
	g.imports["fmt"] = true
	out, err := render("sequence", struct {
		Field, Elem, Call string
	}{Field: sname, Elem: elem, Call: uintDecodeCall(lenField.Op.Kind, lenField.Type, s.Presence == ast.Optional, hasInit, initExpr, lenSlot)})
	return out, func() error { return g.nestedMethod(elem, s.Instructions) }, err
}

// fieldStep renders the decode statement for a scalar/string/binary field.
func (g *generator) fieldStep(slotPrefix string, f *ast.Field) (string, error) {
	fname := exported(f.Name)
	optional := f.Presence == ast.Optional

	switch f.Type {
	case ast.Int32, ast.Int64, ast.Timestamp:
		if f.Op.Kind == ast.Constant {
			hasInit, initExpr, err := intInitial(f)
			if err != nil {
				return "", err
			}
			if !hasInit {
				return "", fmt.Errorf("field %q: constant without a value", f.Name)
			}
			assign := initExpr
			if f.Type == ast.Timestamp {
				assign = fmt.Sprintf("fastcore.TimestampUTC(%s, %s)", initExpr, unitExpr(f.Unit))
			}
			return render("fieldConstant", struct {
				Field, Assign string
				Optional      bool
			}{Field: fname, Assign: assign, Optional: optional})
		}
		slot := g.addSlot(slotPrefix+fname, f)
		op, _ := operatorExpr(f.Op.Kind)
		hasInit, initExpr, err := intInitial(f)
		if err != nil {
			return "", err
		}
		return render("fieldInt", struct {
			Field, Op, Width, Init, Slot, Unit string
			Optional, HasInit, IsTimestamp     bool
		}{Field: fname, Op: op, Width: widthExpr(f.Type), Init: initExpr, Slot: slot, Unit: unitExpr(f.Unit),
			Optional: optional, HasInit: hasInit, IsTimestamp: f.Type == ast.Timestamp})

	case ast.UInt32, ast.UInt64, ast.Enum, ast.Set, ast.Boolean:
		hasInit, initExpr, err := uintInitial(f)
		if err != nil {
			return "", err
		}
		assign := "v"
		switch f.Type {
		case ast.Boolean:
			assign = "fastcore.Bool(v)"
		case ast.Enum, ast.Set:
			assign = g.enumTypeName(f, slotPrefix) + "(v)"
		}
		if f.Op.Kind == ast.Constant {
			if !hasInit {
				return "", fmt.Errorf("field %q: constant without a value", f.Name)
			}
			assign = initExpr
			switch f.Type {
			case ast.Boolean:
				assign = "fastcore.Bool(" + initExpr + ")"
			case ast.Enum, ast.Set:
				assign = g.enumTypeName(f, slotPrefix) + "(" + initExpr + ")"
			}
			return render("fieldConstant", struct {
				Field, Assign string
				Optional      bool
			}{Field: fname, Assign: assign, Optional: optional})
		}
		slot := g.addSlot(slotPrefix+fname, f)
		return render("fieldUint", struct {
			Field, Call, Assign string
			Optional            bool
		}{Field: fname, Call: uintDecodeCall(f.Op.Kind, f.Type, optional, hasInit, initExpr, slot), Assign: assign,
			Optional: optional})

	case ast.Decimal:
		if f.Op.Kind == ast.Constant {
			hasInit, m, e, err := decimalInitial(f)
			if err != nil {
				return "", err
			}
			if !hasInit {
				return "", fmt.Errorf("field %q: constant without a value", f.Name)
			}
			return render("fieldConstant", struct {
				Field, Assign string
				Optional      bool
			}{Field: fname, Assign: fmt.Sprintf("fastcore.Decimal{Mant: %s, Exp: %s}", m, e), Optional: optional})
		}
		if f.Exponent != nil || f.Mantissa != nil {
			return g.decimalIndividualStep(slotPrefix, f, optional)
		}
		slot := g.addSlot(slotPrefix+fname, f)
		hasInit, m, e, err := decimalInitial(f)
		if err != nil {
			return "", err
		}
		if f.Op.Kind == ast.Delta {
			return render("fieldDecimalDelta", struct {
				Field, Mant, Exp, Slot string
				Optional, HasInit      bool
			}{Field: fname, Mant: m, Exp: e, Slot: slot, Optional: optional, HasInit: hasInit})
		}
		op, _ := operatorExpr(f.Op.Kind)
		return render("fieldDecimal", struct {
			Field, Op, Mant, Exp, Slot string
			Optional, HasInit          bool
		}{Field: fname, Op: op, Mant: m, Exp: e, Slot: slot, Optional: optional, HasInit: hasInit})

	case ast.ASCIIString, ast.UnicodeString, ast.ByteVector:
		return g.bytesFieldStep(slotPrefix, f, optional)

	case ast.BitGroup:
		return g.bitGroupStep(slotPrefix, f)
	}
	return "", fmt.Errorf("field %q: type not yet supported by emitter", f.Name)
}

// decimalIndividualStep renders a decimal whose exponent and mantissa carry
// separate operators (§6.2.2). They decode as two integer fields: the exponent
// is int32 and optional iff the decimal is optional; the mantissa is int64 and
// mandatory, present only when the exponent is present (§10.5.1).
func (g *generator) decimalIndividualStep(slotPrefix string, f *ast.Field, optional bool) (string, error) {
	fname := exported(f.Name)
	expSlot := g.addSlot(slotPrefix+fname+"Exp", &ast.Field{Type: ast.Int32})
	mantSlot := g.addSlot(slotPrefix+fname+"Mant", &ast.Field{Type: ast.Int64})

	expOp, expHas, expInit := opIntParts(f.Exponent)
	mantOp, mantHas, mantInit := opIntParts(f.Mantissa)
	return render("fieldDecimalIndividual", struct {
		Field, ExpOp, ExpInit, ExpSlot, MantOp, MantInit, MantSlot string
		Optional, ExpHasInit, MantHasInit                          bool
	}{
		Field: fname,
		ExpOp: expOp, ExpInit: expInit, ExpSlot: expSlot, ExpHasInit: expHas,
		MantOp: mantOp, MantInit: mantInit, MantSlot: mantSlot, MantHasInit: mantHas,
		Optional: optional,
	})
}

// opIntParts returns the operator expression, has-initial flag, and integer
// initial expression for an optional decimal component operator (nil = none).
func opIntParts(op *ast.Op) (opExpr string, hasInit bool, initExpr string) {
	if op == nil {
		return "fastcore.OpNone", false, "0"
	}
	expr, _ := operatorExpr(op.Kind)
	if !op.HasInitial {
		return expr, false, "0"
	}
	n, err := strconv.ParseInt(strings.TrimSpace(op.Initial), 10, 64)
	if err != nil {
		return expr, false, "0"
	}
	return expr, true, strconv.FormatInt(n, 10)
}

// bitGroupStep renders the decode for a bit group: read the SBIT entity, then
// unpack each fixed-width sub-field. Only mandatory sub-fields and a bit group
// without an operator are supported for now.
func (g *generator) bitGroupStep(slotPrefix string, f *ast.Field) (string, error) {
	if f.Op.Kind != ast.NoOp {
		return "", fmt.Errorf("field %q: bitGroup with an operator not yet supported", f.Name)
	}
	buf := "bg_" + sanitize(slotPrefix+exported(f.Name))
	g.pmaps = append(g.pmaps, buf)

	lines := make([]string, 0, len(f.BitFields))
	for _, bf := range f.BitFields {
		if bf.Presence == ast.Optional {
			return "", fmt.Errorf("field %q: optional bit-group sub-field %q not yet supported", f.Name, bf.Name)
		}
		w := bitFieldWidth(bf)
		if w <= 0 {
			return "", fmt.Errorf("field %q: cannot determine bit width of sub-field %q", f.Name, bf.Name)
		}
		var tmpl string
		switch bf.Type {
		case ast.Int32, ast.Int64:
			tmpl = "bitfieldInt"
		case ast.Boolean:
			tmpl = "bitfieldBool"
		default: // UInt32/UInt64/Enum/Set
			tmpl = "bitfieldUint"
		}
		line, err := render(tmpl, struct {
			Field string
			Width int
		}{Field: exported(bf.Name), Width: w})
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	return render("bitgroup", struct {
		Buf    string
		Fields []string
	}{Buf: buf, Fields: lines})
}

// bitFieldWidth returns the number of bits a bit-group sub-field occupies: the
// explicit width for int2..int7 / uInt1..uInt7, or a derived width for enum
// (ceil(log2(n))), set (n), and boolean (1) — all mandatory (§FAST 1.2 Bit Group).
func bitFieldWidth(f *ast.Field) int {
	if f.BitWidth > 0 {
		return f.BitWidth
	}
	switch f.Type {
	case ast.Boolean:
		return 1
	case ast.Enum:
		return bitsForCount(len(f.Elements))
	case ast.Set:
		return len(f.Elements)
	}
	return 0
}

// bitsForCount returns ceil(log2(n)), with a minimum of 1.
func bitsForCount(n int) int {
	w := 1
	for (1 << w) < n {
		w++
	}
	return w
}

func (g *generator) bytesFieldStep(slotPrefix string, f *ast.Field, optional bool) (string, error) {
	fname := exported(f.Name)
	if f.Op.Kind == ast.Constant && f.Type != ast.ByteVector {
		if !f.Op.HasInitial {
			return "", fmt.Errorf("field %q: constant without a value", f.Name)
		}
		return render("fieldConstant", struct {
			Field, Assign string
			Optional      bool
		}{Field: fname, Assign: strconv.Quote(f.Op.Initial), Optional: optional})
	}
	slot := g.addSlot(slotPrefix+fname, f)
	assign := "string(v)"
	if f.Type == ast.ByteVector {
		assign = "v"
	}
	initExpr := bytesInitial(f)

	switch f.Op.Kind {
	case ast.Delta:
		fn := "DecodeASCIIDelta"
		if f.Type != ast.ASCIIString {
			fn = "DecodeUnicodeDelta"
		}
		return render("fieldBytesDelta", struct {
			Field, Fn, Init, Slot, Assign string
			Optional, HasInit             bool
		}{Field: fname, Fn: fn, Init: initExpr, Slot: slot, Assign: assign, Optional: optional, HasInit: f.Op.HasInitial})
	case ast.Tail:
		if f.Type != ast.ASCIIString {
			return "", fmt.Errorf("field %q: tail on non-ascii not yet supported", f.Name)
		}
		return render("fieldBytesTail", struct {
			Field, Init, Slot, Assign string
			Optional, HasInit         bool
		}{Field: fname, Init: initExpr, Slot: slot, Assign: assign, Optional: optional, HasInit: f.Op.HasInitial})
	default:
		op, _ := operatorExpr(f.Op.Kind)
		kind := "fastcore.ASCIIKind"
		if f.Type != ast.ASCIIString {
			kind = "fastcore.ByteVectorKind"
		}
		return render("fieldBytes", struct {
			Field, Op, Kind, Init, Slot, Assign string
			Optional, HasInit                   bool
		}{Field: fname, Op: op, Kind: kind, Init: initExpr, Slot: slot, Assign: assign, Optional: optional, HasInit: f.Op.HasInitial})
	}
}

// render executes a named template into a string.
func render(name string, data any) (string, error) {
	var b strings.Builder
	if err := codeTemplates.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// addSlot records a dictionary slot for a field and returns its decoder field name.
func (g *generator) addSlot(path string, f *ast.Field) string {
	name := "s_" + sanitize(path)
	g.slots = append(g.slots, slotDecl{Name: name, Type: slotType(f)})
	return name
}

// --- type / initial-value mapping helpers ---

// enumTypeName returns the Go type name for an enum/set field and registers the
// type for emission (deduped by name). A field resolved from a named <define>
// shares one type across templates; an anonymous enum gets a per-field type
// named after the struct and field.
func (g *generator) enumTypeName(f *ast.Field, structName string) string {
	var name string
	if f.TypeName != "" {
		name = exported(f.TypeName)
	} else {
		name = structName + exported(f.Name)
	}
	if !g.enumSeen[name] {
		g.enumSeen[name] = true
		g.enums = append(g.enums, enumType{Name: name, Elements: f.Elements})
	}
	return name
}

// goFieldType is goType but resolves enum/set fields to their generated named
// type (registering it). structName scopes anonymous enum types.
func (g *generator) goFieldType(f *ast.Field, structName string) (string, error) {
	if f.Type == ast.Enum || f.Type == ast.Set {
		return g.enumTypeName(f, structName), nil
	}
	return goType(f)
}

func goType(f *ast.Field) (string, error) {
	switch f.Type {
	case ast.Int32, ast.Int64:
		return "int64", nil
	case ast.UInt32, ast.UInt64, ast.Enum, ast.Set:
		return "uint64", nil
	case ast.Boolean:
		return "bool", nil
	case ast.Decimal:
		return "fastcore.Decimal", nil
	case ast.ASCIIString, ast.UnicodeString:
		return "string", nil
	case ast.ByteVector:
		return "[]byte", nil
	case ast.Timestamp:
		return "time.Time", nil
	case ast.BitGroup:
		return "", fmt.Errorf("field %q: bitGroup not yet supported by emitter", f.Name)
	}
	return "", fmt.Errorf("field %q: unknown type", f.Name)
}

func slotType(f *ast.Field) string {
	switch f.Type {
	case ast.Int32, ast.Int64, ast.Timestamp:
		return "fastcore.IntSlot"
	case ast.UInt32, ast.UInt64, ast.Enum, ast.Set, ast.Boolean:
		return "fastcore.UintSlot"
	case ast.Decimal:
		return "fastcore.DecimalSlot"
	default:
		return "fastcore.BytesSlot"
	}
}

func operatorExpr(k ast.OpKind) (string, bool) {
	switch k {
	case ast.Constant:
		return "fastcore.OpConstant", true
	case ast.Default:
		return "fastcore.OpDefault", true
	case ast.Copy:
		return "fastcore.OpCopy", true
	case ast.Increment:
		return "fastcore.OpIncrement", true
	case ast.Delta:
		return "fastcore.OpDelta", true
	case ast.Tail:
		return "fastcore.OpTail", true
	default:
		return "fastcore.OpNone", false
	}
}

func uintDecodeCall(op ast.OpKind, typ ast.BaseType, optional, hasInitial bool, initial, slot string) string {
	opt := strconv.FormatBool(optional)
	init := strconv.FormatBool(hasInitial)
	switch op {
	case ast.Constant:
		return fmt.Sprintf("fastcore.DecodeUintConstant(&pm, %s, %s)", opt, initial)
	case ast.Default:
		return fmt.Sprintf("fastcore.DecodeUintDefault(r, &pm, %s, %s, %s)", opt, init, initial)
	case ast.Copy:
		return fmt.Sprintf("fastcore.DecodeUintCopy(r, &pm, %s, %s, %s, &d.%s)", opt, init, initial, slot)
	case ast.Increment:
		return fmt.Sprintf("fastcore.DecodeUintIncrement(r, &pm, %s, %s, %s, %s, &d.%s)", widthExpr(typ), opt, init, initial, slot)
	case ast.Delta:
		return fmt.Sprintf("fastcore.DecodeUintDelta(r, %s, %s, %s, &d.%s)", opt, init, initial, slot)
	default:
		return fmt.Sprintf("fastcore.DecodeUintNone(r, %s)", opt)
	}
}

// widthExpr returns the fastcore.IntWidth for a field type, so the generated
// increment wraps at the right boundary (int32/uInt32 are 32-bit).
func widthExpr(t ast.BaseType) string {
	if t == ast.Int32 || t == ast.UInt32 {
		return "fastcore.W32"
	}
	return "fastcore.W64"
}

func unitExpr(unit string) string {
	switch unit {
	case "day":
		return "fastcore.UnitDay"
	case "second":
		return "fastcore.UnitSecond"
	case "microsecond":
		return "fastcore.UnitMicrosecond"
	case "nanosecond":
		return "fastcore.UnitNanosecond"
	default:
		return "fastcore.UnitMillisecond"
	}
}

func intInitial(f *ast.Field) (bool, string, error) {
	if !f.Op.HasInitial {
		return false, "0", nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(f.Op.Initial), 10, 64)
	if err != nil {
		return false, "0", fmt.Errorf("field %q: bad integer initial %q: %w", f.Name, f.Op.Initial, err)
	}
	return true, strconv.FormatInt(n, 10), nil
}

func uintInitial(f *ast.Field) (bool, string, error) {
	if !f.Op.HasInitial {
		return false, "0", nil
	}
	s := strings.TrimSpace(f.Op.Initial)
	if f.Type == ast.Boolean {
		if s == "true" {
			return true, "1", nil
		}
		return true, "0", nil
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return false, "0", fmt.Errorf("field %q: bad unsigned initial %q: %w", f.Name, f.Op.Initial, err)
	}
	return true, strconv.FormatUint(n, 10), nil
}

func decimalInitial(f *ast.Field) (has bool, mant, exp string, err error) {
	if !f.Op.HasInitial {
		return false, "0", "0", nil
	}
	m, e, err := parseDecimal(strings.TrimSpace(f.Op.Initial))
	if err != nil {
		return false, "0", "0", fmt.Errorf("field %q: %w", f.Name, err)
	}
	return true, strconv.FormatInt(m, 10), strconv.FormatInt(int64(e), 10), nil
}

// parseDecimal converts a decimal literal to a normalized mantissa/exponent
// (§6.3.2: drop trailing zero digits so mant % 10 != 0).
func parseDecimal(s string) (mant int64, exp int32, err error) {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "+"), "-")
	intPart, fracPart, _ := strings.Cut(s, ".")
	digits := intPart + fracPart
	if digits == "" {
		return 0, 0, fmt.Errorf("bad decimal %q", s)
	}
	m, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad decimal %q: %w", s, err)
	}
	exp = -int32(len(fracPart))
	for m != 0 && m%10 == 0 {
		m /= 10
		exp++
	}
	if neg {
		m = -m
	}
	return m, exp, nil
}

func bytesInitial(f *ast.Field) string {
	if !f.Op.HasInitial {
		return "nil"
	}
	return "[]byte(" + strconv.Quote(f.Op.Initial) + ")"
}

// --- identifier helpers ---

func exported(name string) string {
	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '_' || r == '-' || r == ' ' || r == '.' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(toUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "X"
	}
	if c := s[0]; c >= '0' && c <= '9' {
		return "X" + s
	}
	return s
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}
