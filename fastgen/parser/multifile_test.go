package parser

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastgen/ast"
)

// TestParseFilesCrossRef verifies that a static templateRef whose target lives
// in a different file is resolved when both files are parsed together. test2.xml
// references SampleInfo (defined in test1.xml); after ParseFiles no unresolved
// SampleInfo reference should remain.
func TestParseFilesCrossRef(t *testing.T) {
	// Single file: the cross-file reference is left unresolved.
	single, err := ParseFile("../../testdata/mfast/templates/test2.xml")
	if err != nil {
		t.Fatalf("single parse: %v", err)
	}
	if !hasRef(single, "SampleInfo") {
		t.Fatal("expected an unresolved SampleInfo ref when test2 is parsed alone")
	}

	// Both files: the reference resolves by inlining.
	merged, err := ParseFiles(
		"../../testdata/mfast/templates/test1.xml",
		"../../testdata/mfast/templates/test2.xml",
	)
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if hasRef(merged, "SampleInfo") {
		t.Error("SampleInfo reference still unresolved after merging test1 + test2")
	}
}

// hasRef reports whether any template in s still contains an unresolved static
// reference to the named template.
func hasRef(s *ast.Schema, name string) bool {
	var walk func([]ast.Instruction) bool
	walk = func(instrs []ast.Instruction) bool {
		for _, in := range instrs {
			switch x := in.(type) {
			case *ast.TemplateRef:
				if x.Name == name {
					return true
				}
			case *ast.Group:
				if walk(x.Instructions) {
					return true
				}
			case *ast.Sequence:
				if walk(x.Instructions) {
					return true
				}
			}
		}
		return false
	}
	for _, t := range s.Templates {
		if walk(t.Instructions) {
			return true
		}
	}
	return false
}
