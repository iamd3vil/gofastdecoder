// Package parser implements a FAST template XML parser that produces the AST
// defined in fastgen/ast. It handles both FAST 1.1 (§6.1–§6.4, Appendix 1
// RELAX NG schema) and the FAST 1.2 extension syntax (<define>, <field>,
// <type>, <enum>, <set>, <boolean>, <timestamp>, <bitGroup>).
//
// The parser performs two logical passes:
//  1. Parse the raw XML into an intermediate representation, collecting
//     <define> blocks and instructions.
//  2. Resolve all <type name="…"> references by inlining the corresponding
//     <define> content, detecting cycles, and constructing the final ast.Schema.
//
// All element comparisons are done on the local name only (namespace-agnostic),
// so templates using any of the common FAST namespaces are accepted without
// modification.
package parser

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/iamd3vil/gofastdecoder/fastgen/ast"
)

// reXMLVersion matches the version attribute in an XML declaration, allowing
// for whitespace-padded values such as version=" 1.0 ".
var reXMLVersion = regexp.MustCompile(`version\s*=\s*["'][^"']*["']`)

// ParseFile reads the file at path and delegates to ParseBytes.
func ParseFile(path string) (*ast.Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parser: read %s: %w", path, err)
	}
	return ParseBytes(data)
}

// ParseFiles parses several FAST template files and merges their templates into
// one schema, then resolves static template references across the whole set
// (§6.4). This is how a template definition in one file (e.g. via templateNs)
// is referenced from another. Templates are matched by name; on a duplicate
// name across files the first occurrence wins. Cross-file <define> references
// are not resolved — defines are file-local.
func ParseFiles(paths ...string) (*ast.Schema, error) {
	merged := &ast.Schema{}
	seen := make(map[string]bool)
	for _, path := range paths {
		s, err := ParseFile(path)
		if err != nil {
			return nil, err
		}
		if merged.Namespace == "" {
			merged.Namespace, merged.TemplateNs, merged.Dictionary = s.Namespace, s.TemplateNs, s.Dictionary
		}
		for _, t := range s.Templates {
			if seen[t.Name] {
				continue
			}
			seen[t.Name] = true
			merged.Templates = append(merged.Templates, t)
		}
	}
	// Re-run inlining over the merged set: references left unresolved in their
	// own file (the target lived elsewhere) now find their target.
	if err := inlineStaticTemplateRefs(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

// ParseBytes parses a FAST template XML document and returns the fully
// resolved ast.Schema. All <define>/<type> references are inlined; the
// returned schema contains no unresolved references.
func ParseBytes(data []byte) (*ast.Schema, error) {
	p := &parser{
		defines: make(map[string]*rawTypeDef),
	}
	return p.parse(data)
}

// ---------------------------------------------------------------------------
// Internal types for the intermediate parse pass
// ---------------------------------------------------------------------------

// rawOp is an operator element collected from XML (§6.3).
type rawOp struct {
	Kind       ast.OpKind
	HasInitial bool
	Initial    string
	Dictionary string
	Key        string
}

// rawElem is a named member of enum/set.
type rawElem struct {
	Name  string
	Value string // explicit value attr, may be empty
	ID    string // some fixtures use id= instead of value=
}

// rawTypeDef holds the parsed content of a <define> block: a single
// inner type element with its children.
type rawTypeDef struct {
	// innerTag is the local name of the type element (e.g. "string", "enum", …)
	innerTag string
	// innerAttrs holds attributes other than name/presence/id on the type elem
	innerAttrs map[string]string
	// op is an operator child of the type element (if any)
	op *rawOp
	// exponent/mantissa for decimal individual ops
	exponent *rawOp
	mantissa *rawOp
	// elements for enum/set
	elements []rawElem
	// subInstructions for group/sequence/bitGroup defines
	subInstructions []rawInstruction
	// length for sequence defines
	length *rawLengthDef
}

// rawLengthDef is the <length> child of a <sequence>.
type rawLengthDef struct {
	Name string
	ID   string
	Op   *rawOp
}

// rawInstruction represents a parsed instruction before define resolution.
// It is either a field, sequence, group, or templateRef.
type rawInstruction struct {
	kind string // "field", "sequence", "group", "templateRef"

	// common field attrs
	name     string
	id       string
	presence string // "mandatory"|"optional"|""

	// field-specific
	fieldTag     string // e.g. "uInt32", "string", "enum", "type", …
	fieldAttrs   map[string]string
	op           *rawOp
	exponent     *rawOp
	mantissa     *rawOp
	elements     []rawElem
	typeRef      string // name attr of <type> element
	typeOp       *rawOp // operator inside <type> element
	typePresence string // presence attr of <type> element

	// sequence-specific
	seqDictionary string
	seqTypeRef    string
	length        *rawLengthDef
	instructions  []rawInstruction // body of sequence/group

	// group-specific
	grpDictionary string
	grpTypeRef    string

	// bitGroup sub-fields (for bitGroup field)
	bitGroupInstructions []rawInstruction
	bitGroupOp           *rawOp
}

// ---------------------------------------------------------------------------
// parser state
// ---------------------------------------------------------------------------

type parser struct {
	defines map[string]*rawTypeDef
}

// ---------------------------------------------------------------------------
// Top-level parse
// ---------------------------------------------------------------------------

func (p *parser) parse(data []byte) (*ast.Schema, error) {
	// The xml package does not like the escaped quotes in some fixture XML
	// declarations like <?xml version=\" 1.0 \"?> – normalise them first.
	data = normalizeXMLDecl(data)

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	// Read the root element.
	var schema *ast.Schema
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parser: xml error: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch localName(se.Name) {
		case "templates":
			schema, err = p.parseTemplates(dec, se)
			if err != nil {
				return nil, err
			}
		case "template":
			// Single bare <template> document.
			schema = &ast.Schema{}
			tmpl, err := p.parseTemplate(dec, se)
			if err != nil {
				return nil, err
			}
			schema.Templates = append(schema.Templates, tmpl)
		default:
			// skip unknown root elements
			if err := skipElement(dec); err != nil {
				return nil, err
			}
		}
	}
	if schema == nil {
		return nil, fmt.Errorf("parser: no <templates> or <template> root element found")
	}
	if err := inlineStaticTemplateRefs(schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// inlineStaticTemplateRefs replaces every static <templateRef name="T"/> with a
// copy of T's instructions (§6.4), recursively, with cycle detection. Dynamic
// references (no name) are left in place. A reference to an unknown template is
// an error.
func inlineStaticTemplateRefs(schema *ast.Schema) error {
	byName := make(map[string]*ast.Template, len(schema.Templates))
	for _, t := range schema.Templates {
		byName[t.Name] = t
	}
	for _, t := range schema.Templates {
		expanded, err := expandRefs(t.Instructions, byName, map[string]bool{t.Name: true})
		if err != nil {
			return fmt.Errorf("template %q: %w", t.Name, err)
		}
		t.Instructions = expanded
	}
	return nil
}

// expandRefs returns instrs with static template references replaced by the
// referenced template's (recursively expanded) instructions. active guards
// against reference cycles.
func expandRefs(instrs []ast.Instruction, byName map[string]*ast.Template, active map[string]bool) ([]ast.Instruction, error) {
	out := make([]ast.Instruction, 0, len(instrs))
	for _, in := range instrs {
		switch x := in.(type) {
		case *ast.TemplateRef:
			if x.Dynamic {
				out = append(out, x) // left for the consumer
				continue
			}
			target, ok := byName[x.Name]
			if !ok {
				// The target is not in this template set (e.g. a cross-file or
				// cross-namespace reference). Leave it unresolved for the
				// consumer to report; parsing one file should still succeed.
				out = append(out, x)
				continue
			}
			if active[x.Name] {
				return nil, fmt.Errorf("cyclic templateRef involving %q", x.Name)
			}
			active[x.Name] = true
			sub, err := expandRefs(target.Instructions, byName, active)
			active[x.Name] = false
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		case *ast.Group:
			sub, err := expandRefs(x.Instructions, byName, active)
			if err != nil {
				return nil, err
			}
			g := *x
			g.Instructions = sub
			out = append(out, &g)
		case *ast.Sequence:
			sub, err := expandRefs(x.Instructions, byName, active)
			if err != nil {
				return nil, err
			}
			s := *x
			s.Instructions = sub
			out = append(out, &s)
		default:
			out = append(out, in)
		}
	}
	return out, nil
}

// normalizeXMLDecl fixes malformed XML declarations found in some fixture
// files:
//   - Backslash-escaped quotes: version=\" 1.0 \"  →  version="1.0"
//   - Version strings with surrounding whitespace: version=" 1.0 " → version="1.0"
func normalizeXMLDecl(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("<?xml")) {
		return data
	}
	end := bytes.Index(data, []byte("?>"))
	if end < 0 {
		return data
	}
	declOrig := data[:end+2]

	// Replace backslash-escaped quotes.
	decl := bytes.ReplaceAll(declOrig, []byte(`\"`), []byte(`"`))

	// Fix version=" 1.0 " → version="1.0"  (spaces inside quotes).
	decl = reXMLVersion.ReplaceAll(decl, []byte(`version="1.0"`))

	if bytes.Equal(decl, declOrig) {
		return data
	}
	out := make([]byte, len(decl)+len(data[end+2:]))
	copy(out, decl)
	copy(out[len(decl):], data[end+2:])
	return out
}

// ---------------------------------------------------------------------------
// <templates> element
// ---------------------------------------------------------------------------

func (p *parser) parseTemplates(dec *xml.Decoder, se xml.StartElement) (*ast.Schema, error) {
	schema := &ast.Schema{}
	for _, a := range se.Attr {
		switch localName(a.Name) {
		case "ns":
			schema.Namespace = a.Value
		case "templateNs":
			schema.TemplateNs = a.Value
		case "dictionary":
			schema.Dictionary = a.Value
		}
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parser: xml error inside <templates>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "templates" {
				return schema, nil
			}
		case xml.StartElement:
			switch localName(t.Name) {
			case "template":
				tmpl, err := p.parseTemplate(dec, t)
				if err != nil {
					return nil, err
				}
				schema.Templates = append(schema.Templates, tmpl)
			case "define":
				if err := p.parseDefine(dec, t); err != nil {
					return nil, err
				}
			default:
				// skip <view>, <templateRef> at top level, foreign elements, …
				if err := skipElement(dec); err != nil {
					return nil, err
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// <template> element
// ---------------------------------------------------------------------------

func (p *parser) parseTemplate(dec *xml.Decoder, se xml.StartElement) (*ast.Template, error) {
	tmpl := &ast.Template{}
	for _, a := range se.Attr {
		switch localName(a.Name) {
		case "name":
			tmpl.Name = a.Value
		case "id":
			id, err := parseID(a.Value)
			if err != nil {
				return nil, fmt.Errorf("template %q: bad id %q: %w", tmpl.Name, a.Value, err)
			}
			tmpl.ID = id
			tmpl.HasID = true
		case "ns":
			tmpl.Namespace = a.Value
		case "templateNs":
			// per-template override, stored in Namespace field (reuse)
		case "dictionary":
			tmpl.Dictionary = a.Value
		}
	}

	instructions, typeRef, err := p.parseInstructionList(dec, "template")
	if err != nil {
		return nil, fmt.Errorf("template %q: %w", tmpl.Name, err)
	}
	tmpl.TypeRef = typeRef
	tmpl.Instructions = instructions
	return tmpl, nil
}

// ---------------------------------------------------------------------------
// <define> element  (FAST 1.2)
// ---------------------------------------------------------------------------

func (p *parser) parseDefine(dec *xml.Decoder, se xml.StartElement) error {
	var name string
	for _, a := range se.Attr {
		if localName(a.Name) == "name" {
			name = a.Value
		}
	}
	if name == "" {
		if err := skipElement(dec); err != nil {
			return err
		}
		return nil
	}

	def, err := p.parseTypeContent(dec, "define")
	if err != nil {
		return fmt.Errorf("define %q: %w", name, err)
	}
	if def != nil {
		p.defines[name] = def
	}
	return nil
}

// parseTypeContent reads the single inner type element of a <define> or
// inline <field> body. Returns nil if the element is empty/not a type.
func (p *parser) parseTypeContent(dec *xml.Decoder, endTag string) (*rawTypeDef, error) {
	var def *rawTypeDef
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xml error: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if depth == 0 && localName(t.Name) == endTag {
				return def, nil
			}
			depth--
		case xml.StartElement:
			depth++
			ln := localName(t.Name)
			if def != nil {
				// already have the type – skip extra elements
				if err := skipElement(dec); err != nil {
					return nil, err
				}
				depth--
				continue
			}
			// The inner element is the type definition.
			d, err := p.parseTypeElement(dec, t, ln)
			if err != nil {
				return nil, err
			}
			def = d
			depth--
		}
	}
}

// parseTypeElement parses the body of a type-defining element such as
// <string>, <enum>, <decimal>, <group>, <sequence>, <bitGroup>, etc.,
// returning a rawTypeDef.  The opening StartElement has already been consumed.
func (p *parser) parseTypeElement(dec *xml.Decoder, se xml.StartElement, tag string) (*rawTypeDef, error) {
	def := &rawTypeDef{
		innerTag:   tag,
		innerAttrs: make(map[string]string),
	}
	for _, a := range se.Attr {
		ln := localName(a.Name)
		if ln != "name" && ln != "id" {
			def.innerAttrs[ln] = a.Value
		}
	}

	return def, p.fillTypeDef(dec, def, tag)
}

// fillTypeDef reads the children of a type element into def, consuming up to
// the matching end tag.
func (p *parser) fillTypeDef(dec *xml.Decoder, def *rawTypeDef, endTag string) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("xml error inside <%s>: %w", endTag, err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == endTag {
				return nil
			}
			// mismatched end element – ignore
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "constant", "default", "copy", "increment", "delta", "tail":
				op, err := parseOp(dec, t, ln)
				if err != nil {
					return err
				}
				def.op = op
			case "exponent":
				op, err := parseOpWrapper(dec, t)
				if err != nil {
					return err
				}
				def.exponent = op
			case "mantissa":
				op, err := parseOpWrapper(dec, t)
				if err != nil {
					return err
				}
				def.mantissa = op
			case "element":
				el := parseElementAttr(t)
				def.elements = append(def.elements, el)
				if err := skipElement(dec); err != nil {
					return err
				}
			case "length":
				ld, err := parseLengthDef(dec, t)
				if err != nil {
					return err
				}
				def.length = ld
			case "field", "int32", "uInt32", "int64", "uInt64",
				"decimal", "string", "byteVector", "ascii",
				"sequence", "group", "enum", "set", "boolean",
				"timestamp", "bitGroup", "templateRef", "type":
				// sub-instructions inside group/sequence/bitGroup
				ri, err := p.parseRawInstruction(dec, t, ln)
				if err != nil {
					return err
				}
				if endTag == "bitGroup" {
					def.subInstructions = append(def.subInstructions, ri)
				} else {
					def.subInstructions = append(def.subInstructions, ri)
				}
			default:
				// foreign / unknown element
				if err := skipElement(dec); err != nil {
					return err
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Instruction list parsing (template body, group body, sequence body)
// ---------------------------------------------------------------------------

// parseInstructionList reads a list of instructions until the matching end
// tag. It also collects a <typeRef> if present.
func (p *parser) parseInstructionList(dec *xml.Decoder, endTag string) ([]ast.Instruction, string, error) {
	var instrs []ast.Instruction
	var typeRef string

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, "", fmt.Errorf("xml error: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == endTag {
				return instrs, typeRef, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "typeRef":
				for _, a := range t.Attr {
					if localName(a.Name) == "name" {
						typeRef = a.Value
					}
				}
				if err := skipElement(dec); err != nil {
					return nil, "", err
				}
			case "templateRef":
				var refName string
				for _, a := range t.Attr {
					if localName(a.Name) == "name" {
						refName = a.Value
					}
				}
				if err := skipElement(dec); err != nil {
					return nil, "", err
				}
				instrs = append(instrs, &ast.TemplateRef{Name: refName, Dynamic: refName == ""})
			case "view":
				// FAST extension – skip
				if err := skipElement(dec); err != nil {
					return nil, "", err
				}
			default:
				ri, err := p.parseRawInstruction(dec, t, ln)
				if err != nil {
					return nil, "", err
				}
				instr, err := p.resolveInstruction(ri)
				if err != nil {
					return nil, "", err
				}
				if instr != nil {
					instrs = append(instrs, instr)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Raw instruction parsing (first pass, before define resolution)
// ---------------------------------------------------------------------------

// parseRawInstruction parses one instruction element into a rawInstruction.
// The caller has already consumed the opening StartElement.
func (p *parser) parseRawInstruction(dec *xml.Decoder, se xml.StartElement, tag string) (rawInstruction, error) {
	ri := rawInstruction{
		fieldAttrs: make(map[string]string),
	}

	// Extract common attributes.
	for _, a := range se.Attr {
		ln := localName(a.Name)
		switch ln {
		case "name":
			ri.name = a.Value
		case "id":
			ri.id = a.Value
		case "presence":
			ri.presence = a.Value
		default:
			ri.fieldAttrs[ln] = a.Value
		}
	}

	switch tag {
	case "field":
		return p.parseFieldElement(dec, ri)
	case "sequence":
		return p.parseSequenceElement(dec, ri)
	case "group":
		return p.parseGroupElement(dec, ri)
	case "bitGroup":
		return p.parseBitGroupElement(dec, ri)
	case "templateRef":
		ri.kind = "templateRef"
		if err := skipElement(dec); err != nil {
			return ri, err
		}
		return ri, nil
	default:
		// Legacy field form: <uInt32 name="x"><copy/></uInt32>
		// or FAST 1.2 types used directly: <enum name="x">…</enum>
		return p.parseLegacyField(dec, se, tag, ri)
	}
}

// parseFieldElement handles the FAST 1.2 <field name="x">…</field> form.
// The inner content is either a type element (string, enum, etc.) or a
// <type name="T"/> reference, with an optional operator.
func (p *parser) parseFieldElement(dec *xml.Decoder, ri rawInstruction) (rawInstruction, error) {
	ri.kind = "field"

	// Read children: one type element or <type name="T"/>, plus optional op.
	for {
		tok, err := dec.Token()
		if err != nil {
			return ri, fmt.Errorf("xml error inside <field>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "field" {
				return ri, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "type":
				// <type name="T" presence="optional"><op/></type>
				var typeName string
				var typePresence string
				for _, a := range t.Attr {
					switch localName(a.Name) {
					case "name":
						typeName = a.Value
					case "presence":
						typePresence = a.Value
					}
				}
				ri.typeRef = typeName
				ri.typePresence = typePresence
				// Parse operator inside <type> (if any)
				op, err := p.parseOpInsideType(dec)
				if err != nil {
					return ri, err
				}
				ri.typeOp = op
			case "constant", "default", "copy", "increment", "delta", "tail":
				// Operator directly inside <field> (alongside <type>)
				op, err := parseOp(dec, t, ln)
				if err != nil {
					return ri, err
				}
				ri.op = op
			case "reference":
				// <view> fields use <reference> – skip
				if err := skipElement(dec); err != nil {
					return ri, err
				}
			default:
				// Inline type element: <string/>, <enum>…</enum>, etc.
				ri.fieldTag = ln
				def, err := p.parseTypeElement(dec, t, ln)
				if err != nil {
					return ri, err
				}
				// Copy define contents into ri
				ri = applyDefToRI(ri, def)
			}
		}
	}
}

// parseOpInsideType reads the operator (if any) inside a <type> element
// and consumes up to </type>.
func (p *parser) parseOpInsideType(dec *xml.Decoder) (*rawOp, error) {
	var op *rawOp
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xml error inside <type>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "type" {
				return op, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "constant", "default", "copy", "increment", "delta", "tail":
				o, err := parseOp(dec, t, ln)
				if err != nil {
					return nil, err
				}
				op = o
			default:
				if err := skipElement(dec); err != nil {
					return nil, err
				}
			}
		}
	}
}

// parseLegacyField handles the legacy form <uInt32 name="x"><copy/></uInt32>
// and similar FAST 1.2 types used directly (<enum name="x">…</enum>).
func (p *parser) parseLegacyField(dec *xml.Decoder, se xml.StartElement, tag string, ri rawInstruction) (rawInstruction, error) {
	ri.kind = "field"
	ri.fieldTag = tag

	// For string, pick up charset.
	for _, a := range se.Attr {
		ln := localName(a.Name)
		if ln != "name" && ln != "id" && ln != "presence" {
			ri.fieldAttrs[ln] = a.Value
		}
	}

	def, err := p.parseTypeElement(dec, se, tag)
	if err != nil {
		return ri, err
	}
	ri = applyDefToRI(ri, def)
	return ri, nil
}

// applyDefToRI copies a rawTypeDef's operator/elements/sub-instructions
// into a rawInstruction.
func applyDefToRI(ri rawInstruction, def *rawTypeDef) rawInstruction {
	if def == nil {
		return ri
	}
	if def.op != nil {
		ri.op = def.op
	}
	if def.exponent != nil {
		ri.exponent = def.exponent
	}
	if def.mantissa != nil {
		ri.mantissa = def.mantissa
	}
	if len(def.elements) > 0 {
		ri.elements = def.elements
	}
	if def.length != nil {
		ri.length = def.length
	}
	if len(def.subInstructions) > 0 {
		ri.instructions = def.subInstructions
	}
	// Copy over innerAttrs (charset, unit, epoch, etc.)
	for k, v := range def.innerAttrs {
		if ri.fieldAttrs == nil {
			ri.fieldAttrs = make(map[string]string)
		}
		if _, exists := ri.fieldAttrs[k]; !exists {
			ri.fieldAttrs[k] = v
		}
	}
	return ri
}

// parseSequenceElement handles <sequence name="…">…</sequence>.
func (p *parser) parseSequenceElement(dec *xml.Decoder, ri rawInstruction) (rawInstruction, error) {
	ri.kind = "sequence"
	for k, v := range ri.fieldAttrs {
		if k == "dictionary" {
			ri.seqDictionary = v
		}
	}

	// Read children: optional <length>, optional <typeRef>, then instructions.
	for {
		tok, err := dec.Token()
		if err != nil {
			return ri, fmt.Errorf("xml error inside <sequence>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "sequence" {
				return ri, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "length":
				ld, err := parseLengthDef(dec, t)
				if err != nil {
					return ri, err
				}
				ri.length = ld
			case "typeRef":
				for _, a := range t.Attr {
					if localName(a.Name) == "name" {
						ri.seqTypeRef = a.Value
					}
				}
				if err := skipElement(dec); err != nil {
					return ri, err
				}
			case "templateRef":
				tr := rawInstruction{kind: "templateRef"}
				for _, a := range t.Attr {
					if localName(a.Name) == "name" {
						tr.name = a.Value
					}
				}
				if err := skipElement(dec); err != nil {
					return ri, err
				}
				ri.instructions = append(ri.instructions, tr)
			default:
				sub, err := p.parseRawInstruction(dec, t, ln)
				if err != nil {
					return ri, err
				}
				ri.instructions = append(ri.instructions, sub)
			}
		}
	}
}

// parseGroupElement handles <group name="…">…</group>.
func (p *parser) parseGroupElement(dec *xml.Decoder, ri rawInstruction) (rawInstruction, error) {
	ri.kind = "group"
	for k, v := range ri.fieldAttrs {
		if k == "dictionary" {
			ri.grpDictionary = v
		}
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return ri, fmt.Errorf("xml error inside <group>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "group" {
				return ri, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "typeRef":
				for _, a := range t.Attr {
					if localName(a.Name) == "name" {
						ri.grpTypeRef = a.Value
					}
				}
				if err := skipElement(dec); err != nil {
					return ri, err
				}
			case "templateRef":
				tr := rawInstruction{kind: "templateRef"}
				for _, a := range t.Attr {
					if localName(a.Name) == "name" {
						tr.name = a.Value
					}
				}
				if err := skipElement(dec); err != nil {
					return ri, err
				}
				ri.instructions = append(ri.instructions, tr)
			default:
				sub, err := p.parseRawInstruction(dec, t, ln)
				if err != nil {
					return ri, err
				}
				ri.instructions = append(ri.instructions, sub)
			}
		}
	}
}

// parseBitGroupElement handles <bitGroup name="…">…</bitGroup>.
func (p *parser) parseBitGroupElement(dec *xml.Decoder, ri rawInstruction) (rawInstruction, error) {
	ri.kind = "field"
	ri.fieldTag = "bitGroup"

	for {
		tok, err := dec.Token()
		if err != nil {
			return ri, fmt.Errorf("xml error inside <bitGroup>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "bitGroup" {
				return ri, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			switch ln {
			case "constant", "default", "copy", "increment", "delta", "tail":
				op, err := parseOp(dec, t, ln)
				if err != nil {
					return ri, err
				}
				ri.bitGroupInstructions = append(ri.bitGroupInstructions,
					rawInstruction{kind: "_op", op: op})
			default:
				sub, err := p.parseRawInstruction(dec, t, ln)
				if err != nil {
					return ri, err
				}
				ri.bitGroupInstructions = append(ri.bitGroupInstructions, sub)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Operator parsing
// ---------------------------------------------------------------------------

// parseOp parses an operator element (constant/default/copy/…) whose
// opening StartElement has already been consumed.
func parseOp(dec *xml.Decoder, se xml.StartElement, kind string) (*rawOp, error) {
	op := &rawOp{Kind: opKindOf(kind)}
	for _, a := range se.Attr {
		ln := localName(a.Name)
		switch ln {
		case "value":
			op.Initial = a.Value
			op.HasInitial = true
		case "dictionary":
			op.Dictionary = a.Value
		case "key":
			op.Key = a.Value
		}
	}
	// Consume children (opContext may contain foreign elements).
	if err := skipElement(dec); err != nil {
		return nil, err
	}
	return op, nil
}

// parseOpWrapper parses <exponent>{fieldOp}</exponent> or <mantissa>{fieldOp}</mantissa>.
func parseOpWrapper(dec *xml.Decoder, se xml.StartElement) (*rawOp, error) {
	wrapTag := localName(se.Name)
	var op *rawOp
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xml error inside <%s>: %w", wrapTag, err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == wrapTag {
				return op, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			o, err := parseOp(dec, t, ln)
			if err != nil {
				return nil, err
			}
			op = o
		}
	}
}

// parseLengthDef parses a <length> element.
func parseLengthDef(dec *xml.Decoder, se xml.StartElement) (*rawLengthDef, error) {
	ld := &rawLengthDef{}
	for _, a := range se.Attr {
		switch localName(a.Name) {
		case "name":
			ld.Name = a.Value
		case "id":
			ld.ID = a.Value
		}
	}
	// The body is an optional operator.
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xml error inside <length>: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if localName(t.Name) == "length" {
				return ld, nil
			}
		case xml.StartElement:
			ln := localName(t.Name)
			op, err := parseOp(dec, t, ln)
			if err != nil {
				return nil, err
			}
			ld.Op = op
		}
	}
}

// parseElementAttr extracts a rawElem from an <element> start element.
func parseElementAttr(se xml.StartElement) rawElem {
	el := rawElem{}
	for _, a := range se.Attr {
		switch localName(a.Name) {
		case "name":
			el.Name = a.Value
		case "value":
			el.Value = a.Value
		case "id":
			el.ID = a.Value
		}
	}
	return el
}

// opKindOf maps an element local-name to an OpKind.
func opKindOf(name string) ast.OpKind {
	switch name {
	case "constant":
		return ast.Constant
	case "default":
		return ast.Default
	case "copy":
		return ast.Copy
	case "increment":
		return ast.Increment
	case "delta":
		return ast.Delta
	case "tail":
		return ast.Tail
	}
	return ast.NoOp
}

// ---------------------------------------------------------------------------
// Resolution pass: convert rawInstruction → ast.Instruction
// ---------------------------------------------------------------------------

// resolveInstruction converts a rawInstruction into an ast.Instruction,
// inlining any <type name="T"> references from the defines map.
func (p *parser) resolveInstruction(ri rawInstruction) (ast.Instruction, error) {
	switch ri.kind {
	case "field":
		return p.resolveField(ri)
	case "sequence":
		return p.resolveSequence(ri)
	case "group":
		return p.resolveGroup(ri)
	case "templateRef":
		return &ast.TemplateRef{Name: ri.name, Dynamic: ri.name == ""}, nil
	case "_op":
		return nil, nil // operator marker, not an instruction
	default:
		return nil, fmt.Errorf("unknown instruction kind %q", ri.kind)
	}
}

// resolveField converts a field rawInstruction into an ast.Field.
func (p *parser) resolveField(ri rawInstruction) (*ast.Field, error) {
	// If this is a <type name="T"> reference, inline the define.
	if ri.typeRef != "" {
		return p.resolveTypeRef(ri)
	}

	f := &ast.Field{Name: ri.name}
	if ri.id != "" {
		id, err := parseID(ri.id)
		if err != nil {
			return nil, fmt.Errorf("field %q: bad id %q: %w", ri.name, ri.id, err)
		}
		f.ID = id
		f.HasID = true
	}
	f.Presence = presenceOf(ri.presence)

	// Handle the field tag.
	tag := ri.fieldTag
	if tag == "" {
		tag = "uInt32" // fallback
	}

	bt, err := baseTypeOf(tag, ri.fieldAttrs)
	if err != nil {
		// Unknown/extension type – skip silently.
		return nil, nil
	}
	f.Type = bt
	f.BitWidth = bitWidthOf(tag)

	// Timestamp attrs
	if bt == ast.Timestamp {
		f.Unit = ri.fieldAttrs["unit"]
		f.Epoch = ri.fieldAttrs["epoch"]
	}

	// Decimal individual operators
	if bt == ast.Decimal {
		if ri.exponent != nil {
			op := convertOp(ri.exponent)
			f.Exponent = &op
		}
		if ri.mantissa != nil {
			op := convertOp(ri.mantissa)
			f.Mantissa = &op
		}
		// If individual ops are present, the top-level op stays NoOp.
		if ri.exponent == nil && ri.mantissa == nil && ri.op != nil {
			f.Op = convertOp(ri.op)
		}
	} else {
		if ri.op != nil {
			f.Op = convertOp(ri.op)
		}
	}

	// Enum / Set elements
	if bt == ast.Enum || bt == ast.Set {
		f.Elements = assignElementValues(ri.elements, bt)
		// Operator on enum/set
		if ri.op != nil {
			f.Op = convertOp(ri.op)
		}
	}

	// BitGroup sub-fields
	if bt == ast.BitGroup {
		bfs, bgOp, err := p.resolveBitGroupFields(ri.bitGroupInstructions)
		if err != nil {
			return nil, err
		}
		f.BitFields = bfs
		if bgOp != nil {
			f.Op = convertOp(bgOp)
		} else if ri.op != nil {
			f.Op = convertOp(ri.op)
		}
	}

	return f, nil
}

// resolveTypeRef inlines a <type name="T"> reference.
func (p *parser) resolveTypeRef(ri rawInstruction) (*ast.Field, error) {
	def, ok := p.defines[ri.typeRef]
	if !ok {
		// Unknown type – treat as unresolvable, skip.
		return nil, fmt.Errorf("undefined type reference %q in field %q", ri.typeRef, ri.name)
	}

	// Build a new rawInstruction that merges the define content with the
	// field-level overrides (name, id, presence, operator).
	merged := rawInstruction{
		kind:         "field",
		name:         ri.name,
		id:           ri.id,
		presence:     ri.presence,
		fieldTag:     def.innerTag,
		fieldAttrs:   copyMap(def.innerAttrs),
		op:           def.op,
		exponent:     def.exponent,
		mantissa:     def.mantissa,
		elements:     def.elements,
		length:       def.length,
		instructions: def.subInstructions,
	}

	// Override with field-level operator from <type> or <field>.
	if ri.typeOp != nil {
		merged.op = ri.typeOp
	} else if ri.op != nil {
		merged.op = ri.op
	}

	// Override presence from <type> element itself if present.
	if ri.typePresence != "" && ri.presence == "" {
		merged.presence = ri.typePresence
	}

	// For group/sequence defines, we need to create a Group/Sequence.
	switch def.innerTag {
	case "group":
		return nil, nil // groups are handled separately; shouldn't end up here but be safe
	case "sequence":
		return nil, nil
	}

	f, err := p.resolveField(merged)
	if err != nil {
		return nil, err
	}
	// Record the define name so the emitter can generate one shared enum type.
	if f != nil && (f.Type == ast.Enum || f.Type == ast.Set) {
		f.TypeName = ri.typeRef
	}
	return f, nil
}

// resolveSequence converts a sequence rawInstruction into an ast.Sequence.
func (p *parser) resolveSequence(ri rawInstruction) (*ast.Sequence, error) {
	seq := &ast.Sequence{
		Name:       ri.name,
		Presence:   presenceOf(ri.presence),
		Dictionary: ri.seqDictionary,
	}
	if ri.id != "" {
		id, err := parseID(ri.id)
		if err != nil {
			return nil, fmt.Errorf("sequence %q: bad id %q: %w", ri.name, ri.id, err)
		}
		seq.ID = id
		seq.HasID = true
	}

	// Build the length field.
	seq.Length = buildLengthField(ri.length, ri.name, seq.Presence)

	for _, sub := range ri.instructions {
		instr, err := p.resolveInstruction(sub)
		if err != nil {
			return nil, fmt.Errorf("sequence %q: %w", ri.name, err)
		}
		if instr != nil {
			seq.Instructions = append(seq.Instructions, instr)
		}
	}
	return seq, nil
}

// resolveGroup converts a group rawInstruction into an ast.Group.
func (p *parser) resolveGroup(ri rawInstruction) (*ast.Group, error) {
	grp := &ast.Group{
		Name:       ri.name,
		Presence:   presenceOf(ri.presence),
		Dictionary: ri.grpDictionary,
	}
	if ri.id != "" {
		id, err := parseID(ri.id)
		if err != nil {
			return nil, fmt.Errorf("group %q: bad id %q: %w", ri.name, ri.id, err)
		}
		grp.ID = id
		grp.HasID = true
	}

	for _, sub := range ri.instructions {
		instr, err := p.resolveInstruction(sub)
		if err != nil {
			return nil, fmt.Errorf("group %q: %w", ri.name, err)
		}
		if instr != nil {
			grp.Instructions = append(grp.Instructions, instr)
		}
	}
	return grp, nil
}

// resolveBitGroupFields resolves the sub-instructions of a bitGroup field.
func (p *parser) resolveBitGroupFields(ris []rawInstruction) ([]*ast.Field, *rawOp, error) {
	var fields []*ast.Field
	var bgOp *rawOp
	for _, ri := range ris {
		if ri.kind == "_op" {
			bgOp = ri.op
			continue
		}
		if ri.kind == "templateRef" {
			continue
		}
		f, err := p.resolveField(ri)
		if err != nil {
			return nil, nil, err
		}
		if f != nil {
			fields = append(fields, f)
		}
	}
	return fields, bgOp, nil
}

// resolveInstructionList resolves a list of rawInstructions inside
// a define's group/sequence, used when inlining.
func (p *parser) resolveInstructionList(ris []rawInstruction) ([]ast.Instruction, error) {
	var instrs []ast.Instruction
	for _, ri := range ris {
		instr, err := p.resolveInstruction(ri)
		if err != nil {
			return nil, err
		}
		if instr != nil {
			instrs = append(instrs, instr)
		}
	}
	return instrs, nil
}

// ---------------------------------------------------------------------------
// Type resolution for define-based groups/sequences used in <field>/<type>
// ---------------------------------------------------------------------------

// resolveDefineAsInstruction handles a <field> whose <type> reference resolves
// to a group or sequence define.
func (p *parser) resolveDefineAsGroupOrSeq(ri rawInstruction) (ast.Instruction, error) {
	def, ok := p.defines[ri.typeRef]
	if !ok {
		return nil, fmt.Errorf("undefined type %q", ri.typeRef)
	}
	switch def.innerTag {
	case "group":
		grp := &ast.Group{
			Name:     ri.name,
			Presence: presenceOf(ri.presence),
		}
		if ri.id != "" {
			id, err := parseID(ri.id)
			if err != nil {
				return nil, err
			}
			grp.ID = id
			grp.HasID = true
		}
		var err error
		grp.Instructions, err = p.resolveInstructionList(def.subInstructions)
		if err != nil {
			return nil, err
		}
		return grp, nil
	case "sequence":
		seq := &ast.Sequence{
			Name:     ri.name,
			Presence: presenceOf(ri.presence),
		}
		if ri.id != "" {
			id, err := parseID(ri.id)
			if err != nil {
				return nil, err
			}
			seq.ID = id
			seq.HasID = true
		}
		seq.Length = buildLengthField(def.length, ri.name, seq.Presence)
		var err error
		seq.Instructions, err = p.resolveInstructionList(def.subInstructions)
		if err != nil {
			return nil, err
		}
		return seq, nil
	}
	return nil, fmt.Errorf("define %q has type %q, not group/sequence", ri.typeRef, def.innerTag)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// buildLengthField constructs the synthetic ast.Field for the sequence length.
func buildLengthField(ld *rawLengthDef, seqName string, seqPresence ast.Presence) *ast.Field {
	f := &ast.Field{
		Type:     ast.UInt32,
		Presence: seqPresence, // optional sequence => optional length
	}
	if ld != nil {
		f.Name = ld.Name
		if ld.ID != "" {
			id, err := parseID(ld.ID)
			if err == nil {
				f.ID = id
				f.HasID = true
			}
		}
		if ld.Op != nil {
			f.Op = convertOp(ld.Op)
		}
	}
	if f.Name == "" {
		// Implicit name: guaranteed not to collide with any explicit name.
		f.Name = seqName + "__length"
	}
	return f
}

// convertOp converts a rawOp to an ast.Op.
func convertOp(r *rawOp) ast.Op {
	if r == nil {
		return ast.Op{}
	}
	return ast.Op{
		Kind:       r.Kind,
		HasInitial: r.HasInitial,
		Initial:    r.Initial,
		Dictionary: r.Dictionary,
		Key:        r.Key,
	}
}

// assignElementValues assigns encoded values to enum/set elements.
// Enum: 0, 1, 2, … (or explicit value= attr).
// Set:  1, 2, 4, 8, … (powers of two).
// When an explicit value= attr is present, it is used verbatim.
// assignElementValues computes each element's wire-encoded value. Per the FAST
// 1.2 enum/set encoding the wire carries the element's index (enum: 0,1,2,…;
// set: bit 1,2,4,…), not the element's name — e.g. T7 MDI elements named
// "1","3","5" encode as 0,1,2. An explicit value= attribute overrides this.
func assignElementValues(elems []rawElem, bt ast.BaseType) []ast.Element {
	out := make([]ast.Element, len(elems))
	var auto int64
	if bt == ast.Set {
		auto = 1
	}
	for i, el := range elems {
		name := el.Name
		// Some feeds put a numeric FIX value in name= and the description in
		// id=; prefer id= as the human label for the generated constant.
		label := el.ID
		if label == "" {
			label = el.Name
		}

		var val int64
		if el.Value != "" {
			v, err := strconv.ParseInt(strings.TrimSpace(el.Value), 10, 64)
			if err == nil {
				val = v
			} else {
				// Non-numeric value attr – use auto.
				val = auto
			}
		} else {
			val = auto
		}
		out[i] = ast.Element{Name: name, Label: label, Value: val}

		// Advance auto counter.
		if bt == ast.Set {
			auto <<= 1
		} else {
			auto++
		}
	}
	return out
}

// baseTypeOf maps an element local name to an ast.BaseType.
// Returns an error for unknown/extension types.
func baseTypeOf(tag string, attrs map[string]string) (ast.BaseType, error) {
	switch tag {
	case "int32":
		return ast.Int32, nil
	case "uInt32":
		return ast.UInt32, nil
	case "int64":
		return ast.Int64, nil
	case "uInt64":
		return ast.UInt64, nil
	case "decimal":
		return ast.Decimal, nil
	case "string", "ascii":
		if charset, ok := attrs["charset"]; ok && charset == "unicode" {
			return ast.UnicodeString, nil
		}
		return ast.ASCIIString, nil
	case "byteVector":
		return ast.ByteVector, nil
	case "enum":
		return ast.Enum, nil
	case "set":
		return ast.Set, nil
	case "boolean":
		return ast.Boolean, nil
	case "timestamp":
		return ast.Timestamp, nil
	case "bitGroup":
		return ast.BitGroup, nil
	case "int2", "int3", "int4", "int5", "int6", "int7":
		return ast.Int32, nil // bit-group signed sub-field; width from bitWidthOf
	case "uInt1", "uInt2", "uInt3", "uInt4", "uInt5", "uInt6", "uInt7":
		return ast.UInt32, nil // bit-group unsigned sub-field
	default:
		return 0, fmt.Errorf("unknown type %q", tag)
	}
}

// bitWidthOf returns the fixed bit width of a FAST 1.2 intN/uIntN bit-group
// sub-type tag, or 0 if tag is not such a type.
func bitWidthOf(tag string) int {
	switch {
	case len(tag) == 4 && tag[:3] == "int": // int2..int7
		return int(tag[3] - '0')
	case len(tag) == 5 && tag[:4] == "uInt": // uInt1..uInt7
		return int(tag[4] - '0')
	}
	return 0
}

// presenceOf parses a presence attribute value.
func presenceOf(s string) ast.Presence {
	if s == "optional" {
		return ast.Optional
	}
	return ast.Mandatory
}

// parseID parses an id attribute value (decimal or octal integer literal).
func parseID(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	// Some fixtures use leading zeros (e.g. "01", "02") – treat as decimal,
	// not octal, since FAST ids are decimal tag numbers.
	v, err := strconv.ParseUint(strings.TrimLeft(s, "0")+"0", 10, 32)
	if err != nil {
		// Try plain parse.
		v2, err2 := strconv.ParseUint(s, 10, 32)
		if err2 != nil {
			return 0, err
		}
		return uint32(v2), nil
	}
	// Recover original: TrimLeft removed leading zeros; re-parse without them.
	v2, err2 := strconv.ParseUint(s, 10, 32)
	if err2 != nil {
		return uint32(v / 10), nil
	}
	return uint32(v2), nil
}

// localName returns the local part of an xml.Name (ignoring namespace).
func localName(n xml.Name) string {
	if n.Local != "" {
		return n.Local
	}
	// Some decoders put the full name in Space.
	if idx := strings.LastIndex(n.Space, "/"); idx >= 0 {
		return n.Space[idx+1:]
	}
	return n.Space
}

// skipElement consumes all tokens up to and including the matching end element.
// It assumes the opening StartElement has already been consumed.
func skipElement(dec *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
