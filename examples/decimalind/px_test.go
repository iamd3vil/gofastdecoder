package decimalind

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

// TestDecimalIndividual decodes a decimal whose exponent uses copy and mantissa
// uses delta. PMAP 0xC0 sets bit 0 (the exponent copy bit); the exponent -2
// (0xFE) is read, then the mantissa delta +5 (0x85) combines with base 0.
// Result: 5 x 10^-2.
func TestDecimalIndividual(t *testing.T) {
	var dec PxDecoder
	r := fastcore.NewReader([]byte{0xC0, 0xFE, 0x85})
	var m Px
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if m.Size.Mant != 5 || m.Size.Exp != -2 {
		t.Errorf("Size = %dx10^%d, want 5x10^-2", m.Size.Mant, m.Size.Exp)
	}
}
