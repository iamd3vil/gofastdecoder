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

// TestOptionalDecimalIndividual covers the optional-decimal path where the
// mantissa's presence-map bit is present only when the exponent is present
// (§10.5.1) — the highest-risk path in individual-operator decimals.
func TestOptionalDecimalIndividual(t *testing.T) {
	var dec OptPxDecoder
	r := &fastcore.Reader{}

	// msg1: PMAP 0xE0 (exp bit + mant bit set); exponent -2 (0xFE, nullable
	// signed), mantissa 5 (0x85, mandatory). Both present -> 5x10^-2.
	r.Reset([]byte{0xE0, 0xFE, 0x85})
	var m OptPx
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if !m.HasSize || m.Size.Mant != 5 || m.Size.Exp != -2 {
		t.Fatalf("msg1: Has=%v Size=%dx10^%d, want present 5x10^-2", m.HasSize, m.Size.Mant, m.Size.Exp)
	}
	if r.Remaining() != 0 {
		t.Errorf("msg1: %d bytes left", r.Remaining())
	}

	// msg2: PMAP 0x80 (both copy bits clear); exponent and mantissa copy their
	// previous values -> still 5x10^-2, no bytes consumed beyond the PMAP.
	r.Reset([]byte{0x80})
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if !m.HasSize || m.Size.Mant != 5 || m.Size.Exp != -2 {
		t.Fatalf("msg2 (copy): Has=%v Size=%dx10^%d, want 5x10^-2", m.HasSize, m.Size.Mant, m.Size.Exp)
	}

	// msg3: PMAP 0xC0 (exp bit set), exponent NULL (0x80) -> decimal absent. The
	// mantissa is not in the stream and its PMAP bit is not consumed, so the
	// reader ends exactly after the null exponent.
	r.Reset([]byte{0xC0, 0x80})
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if m.HasSize {
		t.Errorf("msg3: HasSize = true, want false (null exponent)")
	}
	if r.Remaining() != 0 {
		t.Errorf("msg3: %d bytes left — mantissa bit/value wrongly consumed?", r.Remaining())
	}
}

func BenchmarkDecimalIndividual(b *testing.B) {
	var dec PxDecoder
	r := &fastcore.Reader{}
	frame := []byte{0xC0, 0xFE, 0x85}
	var m Px
	if err := dec.Decode(fastcore.NewReader(frame), &m); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(frame)
		if err := dec.Decode(r, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOptionalDecimalCopy(b *testing.B) {
	var dec OptPxDecoder
	r := &fastcore.Reader{}
	var m OptPx
	if err := dec.Decode(fastcore.NewReader([]byte{0xE0, 0xFE, 0x85}), &m); err != nil {
		b.Fatal(err)
	}
	frame := []byte{0x80}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(frame)
		if err := dec.Decode(r, &m); err != nil {
			b.Fatal(err)
		}
	}
}
