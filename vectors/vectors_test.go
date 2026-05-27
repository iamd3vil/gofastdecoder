package vectors

import (
	"path/filepath"
	"testing"
)

// operatorVectorPath is the operator corpus relative to this package directory.
const operatorVectorPath = "../testdata/vectors/operator_decode.json"

// TestOperatorCorpusWellFormed guards the on-disk corpus: it must parse and
// pass internal consistency checks. This runs without a decoder so the corpus
// stays trustworthy independent of fastcore's progress.
func TestOperatorCorpusWellFormed(t *testing.T) {
	f, err := LoadOperatorFile(filepath.Clean(operatorVectorPath))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(f.Vectors) == 0 {
		t.Fatal("no vectors loaded")
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	t.Logf("loaded %d operator vectors", len(f.Vectors))
}

// TestOperatorCoverage asserts the corpus exercises every operator, so a
// regression that silently drops cases is caught.
func TestOperatorCoverage(t *testing.T) {
	f, err := LoadOperatorFile(filepath.Clean(operatorVectorPath))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{"none", "constant", "default", "copy", "increment", "delta", "tail"}
	got := make(map[string]int)
	for _, v := range f.Vectors {
		got[v.Operator]++
	}
	for _, op := range want {
		if got[op] == 0 {
			t.Errorf("no vectors for operator %q", op)
		}
	}
}
