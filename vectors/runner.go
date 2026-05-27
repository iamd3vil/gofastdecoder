package vectors

import (
	"fmt"
	"testing"
)

// Decoder is the contract a candidate implementation (fastcore) satisfies to be
// driven by the operator corpus. Given a fully-specified vector — operator,
// type, presence, initial value, and prior previous-value state — it decodes
// the vector's wire bytes and reports the outcome.
//
// Implementations should construct a field instruction from the vector, seed
// the previous value per PrevState/PrevValue, decode the PMAP then the field,
// and return what was decoded. A dynamic FAST error must be returned as a
// non-nil error (the harness matches it against Expect.Error).
type Decoder interface {
	Decode(v OperatorVector) (Outcome, error)
}

// Outcome is what a Decoder reports for one vector.
type Outcome struct {
	Value            *Value // decoded value, or nil if Absent
	Absent           bool   // field decoded as absent
	PmapBitsConsumed int    // number of presence-map bits the field consumed (0 or 1)
}

// Run drives every vector in the file through d and reports mismatches via t.
// fastcore's test suite calls this once its decoder implements Decoder.
func Run(t *testing.T, f *OperatorFile, d Decoder) {
	t.Helper()
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			out, err := d.Decode(v)

			if v.Expect.Error {
				if err == nil {
					t.Fatalf("expected a dynamic error, got value=%v absent=%v", out.Value, out.Absent)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantBit := 0
			if v.HasPmapBit {
				wantBit = 1
			}
			if out.PmapBitsConsumed != wantBit {
				t.Errorf("pmap bits consumed: got %d, want %d", out.PmapBitsConsumed, wantBit)
			}

			if v.Expect.Absent {
				if !out.Absent {
					t.Errorf("expected absent, got value %v", out.Value)
				}
				return
			}
			if out.Absent {
				t.Fatalf("expected value %v, got absent", v.Expect.Value)
			}
			if msg, ok := valuesEqual(v.Type, v.Expect.Value, out.Value); !ok {
				t.Errorf("decoded value mismatch: %s", msg)
			}
		})
	}
}

// valuesEqual compares an expected and actual Value for the given field type.
// It returns a human-readable message when they differ.
func valuesEqual(typ string, want, got *Value) (string, bool) {
	if want == nil || got == nil {
		return fmt.Sprintf("nil value (want=%v got=%v)", want, got), want == got
	}
	switch typ {
	case "decimal":
		if want.Decimal == nil || got.Decimal == nil {
			return "decimal field but a value is not decimal-shaped", false
		}
		if want.Decimal.Mantissa != got.Decimal.Mantissa || want.Decimal.Exponent != got.Decimal.Exponent {
			return fmt.Sprintf("want %sx10^%d, got %sx10^%d",
				want.Decimal.Mantissa, want.Decimal.Exponent,
				got.Decimal.Mantissa, got.Decimal.Exponent), false
		}
		return "", true
	default: // uint64, ascii, unicode — scalar string compare
		if want.Scalar == nil || got.Scalar == nil {
			return "scalar field but a value is not scalar-shaped", false
		}
		if *want.Scalar != *got.Scalar {
			return fmt.Sprintf("want %q, got %q", *want.Scalar, *got.Scalar), false
		}
		return "", true
	}
}
