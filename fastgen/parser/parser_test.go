// Package parser_test contains tests for the FAST template XML parser.
// It verifies that every fixture in testdata/mfast/templates/ parses without
// error and checks specific structural properties of the produced AST.
package parser_test

import (
	"path/filepath"
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastgen/ast"
	"github.com/iamd3vil/gofastdecoder/fastgen/parser"
)

// ---------------------------------------------------------------------------
// Fixture round-trip tests
// ---------------------------------------------------------------------------

// TestParseAllFixtures parses every file in testdata/mfast/templates/ and
// asserts that parsing succeeds without error.
func TestParseAllFixtures(t *testing.T) {
	t.Helper()
	fixtures, err := filepath.Glob("../../testdata/mfast/templates/*.xml")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixture files found – check path")
	}
	for _, f := range fixtures {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			schema, err := parser.ParseFile(f)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", f, err)
			}
			if schema == nil {
				t.Fatalf("ParseFile(%s): returned nil schema", f)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Focused structural tests
// ---------------------------------------------------------------------------

// TestSimple1CopyOperator verifies that simple1.xml produces three uInt32
// fields each with a Copy operator, and that ID values are parsed correctly.
func TestSimple1CopyOperator(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple1.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schema.Templates) != 1 {
		t.Fatalf("want 1 template, got %d", len(schema.Templates))
	}
	tmpl := schema.Templates[0]
	if tmpl.Name != "Test" {
		t.Errorf("template name: want %q got %q", "Test", tmpl.Name)
	}
	if !tmpl.HasID || tmpl.ID != 1 {
		t.Errorf("template id: want 1 got %d (hasID=%v)", tmpl.ID, tmpl.HasID)
	}
	if len(tmpl.Instructions) != 3 {
		t.Fatalf("want 3 instructions, got %d", len(tmpl.Instructions))
	}
	for i, instr := range tmpl.Instructions {
		f, ok := instr.(*ast.Field)
		if !ok {
			t.Fatalf("instruction[%d]: want *ast.Field, got %T", i, instr)
		}
		if f.Type != ast.UInt32 {
			t.Errorf("field[%d] type: want UInt32 got %v", i, f.Type)
		}
		if f.Op.Kind != ast.Copy {
			t.Errorf("field[%d] op: want Copy got %v", i, f.Op.Kind)
		}
	}
}

// TestSequenceWithLengthField verifies that a sequence with an explicit
// <length> produces an ast.Sequence with the correct Length field.
func TestSequenceWithLengthField(t *testing.T) {
	// simple12.xml has <length name="num_elements" id="12"/> in sequence_1.
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple12.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var seq *ast.Sequence
	for _, tmpl := range schema.Templates {
		for _, instr := range tmpl.Instructions {
			if s, ok := instr.(*ast.Sequence); ok {
				if s.Name == "sequence_1" {
					seq = s
					break
				}
			}
		}
	}
	if seq == nil {
		t.Fatal("sequence_1 not found")
	}
	if seq.Length == nil {
		t.Fatal("sequence_1.Length is nil")
	}
	if seq.Length.Name != "num_elements" {
		t.Errorf("length name: want %q got %q", "num_elements", seq.Length.Name)
	}
	if !seq.Length.HasID || seq.Length.ID != 12 {
		t.Errorf("length id: want 12 got %d (hasID=%v)", seq.Length.ID, seq.Length.HasID)
	}
	if seq.Length.Type != ast.UInt32 {
		t.Errorf("length type: want UInt32 got %v", seq.Length.Type)
	}
}

// TestSequenceImplicitLength verifies that a sequence without an explicit
// <length> gets a synthetic length field with an implicit name.
func TestSequenceImplicitLength(t *testing.T) {
	const xmlDoc = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.1">
  <template name="T" id="1">
    <sequence name="mySeq">
      <uInt32 name="x"/>
    </sequence>
  </template>
</templates>`
	schema, err := parser.ParseBytes([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seq := schema.Templates[0].Instructions[0].(*ast.Sequence)
	if seq.Length == nil {
		t.Fatal("implicit length is nil")
	}
	if seq.Length.Name == "" {
		t.Error("implicit length name is empty")
	}
	if seq.Length.Type != ast.UInt32 {
		t.Errorf("implicit length type: want UInt32 got %v", seq.Length.Type)
	}
}

// TestDecimalIndividualOperators parses test2.xml and verifies that
// the MDEntrySize decimal field uses individual exponent/mantissa operators.
func TestDecimalIndividualOperators(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/test2.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var mdEntrySize *ast.Field
	for _, tmpl := range schema.Templates {
		for _, instr := range tmpl.Instructions {
			seq, ok := instr.(*ast.Sequence)
			if !ok {
				continue
			}
			for _, sub := range seq.Instructions {
				f, ok := sub.(*ast.Field)
				if ok && f.Name == "MDEntrySize" {
					mdEntrySize = f
				}
			}
		}
	}
	if mdEntrySize == nil {
		t.Fatal("MDEntrySize field not found in test2.xml")
	}
	if mdEntrySize.Type != ast.Decimal {
		t.Errorf("type: want Decimal got %v", mdEntrySize.Type)
	}
	if mdEntrySize.Exponent == nil {
		t.Fatal("Exponent op is nil")
	}
	if mdEntrySize.Exponent.Kind != ast.Copy {
		t.Errorf("exponent op: want Copy got %v", mdEntrySize.Exponent.Kind)
	}
	if !mdEntrySize.Exponent.HasInitial || mdEntrySize.Exponent.Initial != "-2" {
		t.Errorf("exponent initial: want -2 got %q (hasInitial=%v)",
			mdEntrySize.Exponent.Initial, mdEntrySize.Exponent.HasInitial)
	}
	if mdEntrySize.Mantissa == nil {
		t.Fatal("Mantissa op is nil")
	}
	if mdEntrySize.Mantissa.Kind != ast.Delta {
		t.Errorf("mantissa op: want Delta got %v", mdEntrySize.Mantissa.Kind)
	}
	// With individual ops the top-level Op should be NoOp.
	if mdEntrySize.Op.Kind != ast.NoOp {
		t.Errorf("top-level op: want NoOp got %v", mdEntrySize.Op.Kind)
	}
}

// TestDesugaringEquivalence verifies that the legacy form
// <uInt32 name="x"><copy/></uInt32>
// produces an identical AST to the FAST 1.2 new form
// <field name="x"><uInt32><copy/></uInt32></field>.
func TestDesugaringEquivalence(t *testing.T) {
	const legacy = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.1">
  <template name="T" id="1">
    <uInt32 name="x"><copy/></uInt32>
  </template>
</templates>`

	const newForm = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.1">
  <template name="T" id="1">
    <field name="x"><uInt32><copy/></uInt32></field>
  </template>
</templates>`

	sLegacy, err := parser.ParseBytes([]byte(legacy))
	if err != nil {
		t.Fatalf("parse legacy: %v", err)
	}
	sNew, err := parser.ParseBytes([]byte(newForm))
	if err != nil {
		t.Fatalf("parse new form: %v", err)
	}

	fl := sLegacy.Templates[0].Instructions[0].(*ast.Field)
	fn := sNew.Templates[0].Instructions[0].(*ast.Field)

	if fl.Name != fn.Name {
		t.Errorf("name: legacy=%q new=%q", fl.Name, fn.Name)
	}
	if fl.Type != fn.Type {
		t.Errorf("type: legacy=%v new=%v", fl.Type, fn.Type)
	}
	if fl.Op.Kind != fn.Op.Kind {
		t.Errorf("op.Kind: legacy=%v new=%v", fl.Op.Kind, fn.Op.Kind)
	}
}

// TestDefineAndTypeResolution verifies that a <define>/<type> reference
// is fully inlined into the template.
func TestDefineAndTypeResolution(t *testing.T) {
	const xmlDoc = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.2">
  <define name="MyStr">
    <string><copy/></string>
  </define>
  <template name="T" id="1">
    <field name="greeting"><type name="MyStr"/></field>
  </template>
</templates>`

	schema, err := parser.ParseBytes([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schema.Templates) != 1 {
		t.Fatalf("want 1 template, got %d", len(schema.Templates))
	}
	f, ok := schema.Templates[0].Instructions[0].(*ast.Field)
	if !ok {
		t.Fatalf("want *ast.Field, got %T", schema.Templates[0].Instructions[0])
	}
	if f.Name != "greeting" {
		t.Errorf("field name: want %q got %q", "greeting", f.Name)
	}
	if f.Type != ast.ASCIIString {
		t.Errorf("type: want ASCIIString got %v", f.Type)
	}
	if f.Op.Kind != ast.Copy {
		t.Errorf("op: want Copy got %v", f.Op.Kind)
	}
}

// TestEnumElementValues verifies that enum elements get the correct encoded
// values (0,1,2,… when no explicit value= attr).
func TestEnumElementValues(t *testing.T) {
	const xmlDoc = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.2">
  <template name="T" id="1">
    <enum name="MatchType">
      <element name="M1"/>
      <element name="M2"/>
      <element name="M3"/>
      <copy/>
    </enum>
  </template>
</templates>`

	schema, err := parser.ParseBytes([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	f, ok := schema.Templates[0].Instructions[0].(*ast.Field)
	if !ok {
		t.Fatalf("want *ast.Field, got %T", schema.Templates[0].Instructions[0])
	}
	if f.Type != ast.Enum {
		t.Errorf("type: want Enum got %v", f.Type)
	}
	want := []ast.Element{
		{Name: "M1", Label: "M1", Value: 0},
		{Name: "M2", Label: "M2", Value: 1},
		{Name: "M3", Label: "M3", Value: 2},
	}
	if len(f.Elements) != len(want) {
		t.Fatalf("elements: want %d got %d", len(want), len(f.Elements))
	}
	for i, w := range want {
		if f.Elements[i] != w {
			t.Errorf("element[%d]: want %+v got %+v", i, w, f.Elements[i])
		}
	}
	if f.Op.Kind != ast.Copy {
		t.Errorf("op: want Copy got %v", f.Op.Kind)
	}
}

// TestEnumExplicitValues verifies that explicitly-assigned element values
// (as in simple16.xml DiscreteEnum) are respected.
func TestEnumExplicitValues(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple16.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// DiscreteEnum: One=1, Three=3, Five=5
	// It is inlined into template Test_1 field "discrete".
	var discrete *ast.Field
	for _, tmpl := range schema.Templates {
		for _, instr := range tmpl.Instructions {
			f, ok := instr.(*ast.Field)
			if ok && f.Name == "discrete" {
				discrete = f
				break
			}
		}
	}
	if discrete == nil {
		t.Fatal("discrete field not found")
	}
	if discrete.Type != ast.Enum {
		t.Errorf("type: want Enum got %v", discrete.Type)
	}
	want := []struct {
		name string
		val  int64
	}{
		{"One", 1}, {"Three", 3}, {"Five", 5},
	}
	if len(discrete.Elements) != len(want) {
		t.Fatalf("elements: want %d got %d", len(want), len(discrete.Elements))
	}
	for i, w := range want {
		if discrete.Elements[i].Name != w.name || discrete.Elements[i].Value != w.val {
			t.Errorf("element[%d]: want {%s %d} got %+v", i, w.name, w.val, discrete.Elements[i])
		}
	}
}

// TestSetElementValues verifies that set elements get power-of-two values.
func TestSetElementValues(t *testing.T) {
	const xmlDoc = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.2">
  <template name="T" id="1">
    <set name="Flags">
      <element name="A"/>
      <element name="B"/>
      <element name="C"/>
    </set>
  </template>
</templates>`

	schema, err := parser.ParseBytes([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	f := schema.Templates[0].Instructions[0].(*ast.Field)
	if f.Type != ast.Set {
		t.Errorf("type: want Set got %v", f.Type)
	}
	want := []ast.Element{
		{Name: "A", Label: "A", Value: 1},
		{Name: "B", Label: "B", Value: 2},
		{Name: "C", Label: "C", Value: 4},
	}
	for i, w := range want {
		if f.Elements[i] != w {
			t.Errorf("element[%d]: want %+v got %+v", i, w, f.Elements[i])
		}
	}
}

// TestTypeWithOperatorOverride verifies that an operator specified inside
// <type> overrides the define's own operator.
func TestTypeWithOperatorOverride(t *testing.T) {
	// simple16.xml Test_3: <type name="DiscreteEnum"><copy/></type>
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple16.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found *ast.Field
	for _, tmpl := range schema.Templates {
		if tmpl.Name != "Test_3" {
			continue
		}
		for _, instr := range tmpl.Instructions {
			if f, ok := instr.(*ast.Field); ok && f.Name == "discrete" {
				found = f
			}
		}
	}
	if found == nil {
		t.Fatal("Test_3.discrete not found")
	}
	if found.Op.Kind != ast.Copy {
		t.Errorf("op: want Copy got %v", found.Op.Kind)
	}
}

// TestBooleanField verifies that a <boolean> field is parsed correctly.
func TestBooleanField(t *testing.T) {
	const xmlDoc = `<?xml version="1.0"?>
<templates xmlns="http://www.fixprotocol.org/ns/fast/td/1.2">
  <template name="T" id="1">
    <boolean name="active"/>
  </template>
</templates>`

	schema, err := parser.ParseBytes([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	f := schema.Templates[0].Instructions[0].(*ast.Field)
	if f.Type != ast.Boolean {
		t.Errorf("type: want Boolean got %v", f.Type)
	}
	if f.Name != "active" {
		t.Errorf("name: want active got %q", f.Name)
	}
}

// TestTimestampField verifies that a <timestamp> field carries Unit and Epoch.
func TestTimestampField(t *testing.T) {
	// simple17.xml has <timestamp name="TransactTime" unit="nanosecond" …>
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple17.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var ts *ast.Field
	for _, tmpl := range schema.Templates {
		for _, instr := range tmpl.Instructions {
			if f, ok := instr.(*ast.Field); ok && f.Name == "TransactTime" {
				ts = f
			}
		}
	}
	if ts == nil {
		t.Fatal("TransactTime not found")
	}
	if ts.Type != ast.Timestamp {
		t.Errorf("type: want Timestamp got %v", ts.Type)
	}
	if ts.Unit != "nanosecond" {
		t.Errorf("unit: want nanosecond got %q", ts.Unit)
	}
	if ts.Presence != ast.Optional {
		t.Errorf("presence: want Optional got %v", ts.Presence)
	}
}

// TestGroupInstruction verifies that a <group> produces an ast.Group with
// the correct instructions.
func TestGroupInstruction(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple2.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tmpl := schema.Templates[0]
	if len(tmpl.Instructions) != 2 {
		t.Fatalf("want 2 instructions, got %d", len(tmpl.Instructions))
	}
	grp, ok := tmpl.Instructions[1].(*ast.Group)
	if !ok {
		t.Fatalf("want *ast.Group, got %T", tmpl.Instructions[1])
	}
	if grp.Name != "group1" {
		t.Errorf("group name: want group1 got %q", grp.Name)
	}
	if grp.Presence != ast.Optional {
		t.Errorf("presence: want Optional got %v", grp.Presence)
	}
	if len(grp.Instructions) != 2 {
		t.Errorf("group instructions: want 2 got %d", len(grp.Instructions))
	}
}

// TestOptionalSequenceInheritsPresence verifies that an optional sequence's
// synthetic length field inherits the Optional presence.
func TestOptionalSequenceInheritsPresence(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple3.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seq, ok := schema.Templates[0].Instructions[1].(*ast.Sequence)
	if !ok {
		t.Fatalf("want *ast.Sequence at index 1")
	}
	if seq.Presence != ast.Optional {
		t.Errorf("seq presence: want Optional got %v", seq.Presence)
	}
	if seq.Length.Presence != ast.Optional {
		t.Errorf("length presence: want Optional got %v", seq.Length.Presence)
	}
}

// TestConstantOperator verifies that a constant operator carries its value.
func TestConstantOperator(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/test1.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	f := schema.Templates[0].Instructions[0].(*ast.Field)
	if f.Op.Kind != ast.Constant {
		t.Errorf("op: want Constant got %v", f.Op.Kind)
	}
	if !f.Op.HasInitial || f.Op.Initial != "X" {
		t.Errorf("initial: want X got %q (hasInitial=%v)", f.Op.Initial, f.Op.HasInitial)
	}
}

// TestUnicodeStringCharset verifies that charset="unicode" maps to UnicodeString.
func TestUnicodeStringCharset(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/test2.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var secID *ast.Field
	for _, tmpl := range schema.Templates {
		for _, instr := range tmpl.Instructions {
			seq, ok := instr.(*ast.Sequence)
			if !ok {
				continue
			}
			for _, sub := range seq.Instructions {
				if f, ok := sub.(*ast.Field); ok && f.Name == "SecurityID" {
					secID = f
				}
			}
		}
	}
	if secID == nil {
		t.Fatal("SecurityID not found")
	}
	if secID.Type != ast.UnicodeString {
		t.Errorf("type: want UnicodeString got %v", secID.Type)
	}
}

// TestSchemaNsAttributes verifies that the <templates> ns/templateNs/dictionary
// attributes are captured in the Schema.
func TestSchemaNsAttributes(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple1.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if schema.Namespace != "http://www.fixprotocol.org/ns/fix" {
		t.Errorf("ns: got %q", schema.Namespace)
	}
	if schema.TemplateNs != "http://www.fixprotocol.org/ns/templates/sample" {
		t.Errorf("templateNs: got %q", schema.TemplateNs)
	}
}

// TestLengthWithOperator verifies that a <length> element with an operator
// is parsed into the length field's Op.
func TestLengthWithOperator(t *testing.T) {
	// simple10.xml has <length id="110"><constant value="1"/></length>
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple10.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seq := schema.Templates[0].Instructions[0].(*ast.Sequence)
	if seq.Length.Op.Kind != ast.Constant {
		t.Errorf("length op: want Constant got %v", seq.Length.Op.Kind)
	}
	if !seq.Length.Op.HasInitial || seq.Length.Op.Initial != "1" {
		t.Errorf("length initial: want 1 got %q", seq.Length.Op.Initial)
	}
}

// TestTypeRefOnSet tests the define/type pattern for a Set type (simple19.xml).
func TestTypeRefOnSet(t *testing.T) {
	schema, err := parser.ParseFile("../../testdata/mfast/templates/simple19.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var tc *ast.Field
	for _, tmpl := range schema.Templates {
		for _, instr := range tmpl.Instructions {
			if f, ok := instr.(*ast.Field); ok && f.Name == "TradeCondition" {
				tc = f
				break
			}
		}
	}
	if tc == nil {
		t.Fatal("TradeCondition field not found")
	}
	if tc.Type != ast.Set {
		t.Errorf("type: want Set got %v", tc.Type)
	}
	// TradeConditionSet has 14 elements.
	if len(tc.Elements) != 14 {
		t.Errorf("elements: want 14 got %d", len(tc.Elements))
	}
	// First element "U" should have value 1.
	if tc.Elements[0].Value != 1 {
		t.Errorf("element[0].Value: want 1 got %d", tc.Elements[0].Value)
	}
}
