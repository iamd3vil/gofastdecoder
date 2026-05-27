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
	g := &generator{imports: map[string]bool{
		"github.com/iamd3vil/gofastdecoder/fastcore": true,
	}}

	blocks := make([]string, 0, len(s.Templates))
	for _, t := range s.Templates {
		blk, err := g.templateBlock(t)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", t.Name, err)
		}
		blocks = append(blocks, blk)
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
	topDecoder string     // receiver type name for the current template's methods
	methods    []string   // rendered method bodies for the current template
}

type slotDecl struct{ Name, Type string }

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
	Slots []slotDecl
}

// templateBlock renders the structs, decoder, and decode methods for one template.
func (g *generator) templateBlock(t *ast.Template) (string, error) {
	name := exported(t.Name)
	g.slots = g.slots[:0]
	g.methods = g.methods[:0]
	g.topDecoder = name

	var structs strings.Builder
	if err := g.renderStructs(&structs, name, t.Instructions); err != nil {
		return "", err
	}
	if err := g.renderMethod(name, t.Instructions); err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString(structs.String())
	if err := codeTemplates.ExecuteTemplate(&out, "decoder", decoderView{Name: name, Slots: g.slots}); err != nil {
		return "", err
	}
	out.WriteString("\n")
	for _, m := range g.methods {
		out.WriteString(m)
		out.WriteString("\n")
	}
	if err := codeTemplates.ExecuteTemplate(&out, "decodeEntry", struct{ Name string }{name}); err != nil {
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
			gt, err := goType(x)
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
// sequences/groups, whose methods are appended to g.methods).
func (g *generator) renderMethod(slotPrefix string, instrs []ast.Instruction) error {
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

	var body strings.Builder
	if err := codeTemplates.ExecuteTemplate(&body, "method", struct {
		Recv, Name, Struct string
		Steps              []string
	}{Recv: g.topDecoder, Name: "decode" + slotPrefix, Struct: slotPrefix, Steps: steps}); err != nil {
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
		return s, func() error { return g.renderMethod(slotPrefix+gname, x.Instructions) }, err
	case *ast.Sequence:
		return g.sequenceStep(slotPrefix, x)
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
	lenSlot := g.addSlot(slotPrefix+sname+"Len", lenField)
	op, _ := operatorExpr(lenField.Op.Kind)
	hasInit, initExpr, err := intInitial(lenField)
	if err != nil {
		return "", nil, err
	}
	out, err := render("sequence", struct {
		Field, Elem, Op, Init, LenSlot string
		Optional, HasInit              bool
	}{Field: sname, Elem: elem, Op: op, Init: initExpr, LenSlot: lenSlot, Optional: s.Presence == ast.Optional, HasInit: hasInit})
	return out, func() error { return g.renderMethod(elem, s.Instructions) }, err
}

// fieldStep renders the decode statement for a scalar/string/binary field.
func (g *generator) fieldStep(slotPrefix string, f *ast.Field) (string, error) {
	fname := exported(f.Name)
	optional := f.Presence == ast.Optional

	switch f.Type {
	case ast.Int32, ast.Int64, ast.Timestamp:
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
		slot := g.addSlot(slotPrefix+fname, f)
		op, _ := operatorExpr(f.Op.Kind)
		hasInit, initExpr, err := uintInitial(f)
		if err != nil {
			return "", err
		}
		return render("fieldUint", struct {
			Field, Op, Width, Init, Slot string
			Optional, HasInit, IsBool    bool
		}{Field: fname, Op: op, Width: widthExpr(f.Type), Init: initExpr, Slot: slot,
			Optional: optional, HasInit: hasInit, IsBool: f.Type == ast.Boolean})

	case ast.Decimal:
		if f.Exponent != nil || f.Mantissa != nil {
			return "", fmt.Errorf("field %q: decimal with individual exponent/mantissa operators not yet supported", f.Name)
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
	}
	return "", fmt.Errorf("field %q: type not yet supported by emitter", f.Name)
}

func (g *generator) bytesFieldStep(slotPrefix string, f *ast.Field, optional bool) (string, error) {
	fname := exported(f.Name)
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

func goType(f *ast.Field) (string, error) {
	switch f.Type {
	case ast.Int32, ast.Int64:
		return "int64", nil
	case ast.UInt32, ast.UInt64, ast.Enum, ast.Set:
		return "uint64", nil
	case ast.Boolean:
		return "bool", nil
	case ast.Decimal:
		if f.Exponent != nil || f.Mantissa != nil {
			return "", fmt.Errorf("field %q: decimal with individual operators not yet supported", f.Name)
		}
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
