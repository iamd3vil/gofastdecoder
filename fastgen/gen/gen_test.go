package gen_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastgen/gen"
	fastparser "github.com/iamd3vil/gofastdecoder/fastgen/parser"
)

// TestGenerateFixtures generates code for every mFAST template fixture and
// verifies that whatever is produced is syntactically valid Go. Some fixtures
// use constructs the emitter does not yet support (bitGroup, decimal with
// individual operators, vendor types); those are expected to error and are
// logged, not failed — but the ones that succeed must parse as valid Go.
func TestGenerateFixtures(t *testing.T) {
	files, err := filepath.Glob("../../testdata/mfast/templates/*.xml")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob fixtures: %v", err)
	}
	var ok, skipped int
	for _, f := range files {
		schema, err := fastparser.ParseFile(f)
		if err != nil {
			t.Errorf("%s: parse: %v", filepath.Base(f), err)
			continue
		}
		src, err := gen.Generate(schema, "generated")
		if err != nil {
			t.Logf("SKIP %s: %v", filepath.Base(f), err)
			skipped++
			continue
		}
		// Must be valid Go.
		if _, err := parser.ParseFile(token.NewFileSet(), filepath.Base(f)+".go", src, parser.AllErrors); err != nil {
			t.Errorf("%s: generated invalid Go: %v\n%s", filepath.Base(f), err, src)
			continue
		}
		ok++
	}
	t.Logf("generated %d/%d fixtures cleanly (%d skipped for unsupported constructs)", ok, len(files), skipped)
	if ok == 0 {
		t.Fatal("no fixtures generated cleanly")
	}
}

// TestGenerateSimple1 checks the shape of generated code for a known template.
func TestGenerateSimple1(t *testing.T) {
	schema, err := fastparser.ParseFile("../../testdata/mfast/templates/simple1.xml")
	if err != nil {
		t.Fatal(err)
	}
	src, err := gen.Generate(schema, "generated")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	s := string(src)
	for _, want := range []string{
		"type Test struct {",
		"Field1 uint64",
		"type TestDecoder struct {",
		"func (d *TestDecoder) Decode(r *fastcore.Reader, m *Test) error",
		"fastcore.DecodeUintCopy(r, &pm",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("generated code missing %q\n---\n%s", want, s)
		}
	}
}
