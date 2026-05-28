package multifile

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

// TestCrossFileRef decodes a Quote whose body statically references Header from
// a different template file; Header's seqNum field is inlined and decoded in
// the same segment. PMAP 0xE0 sets bits 0 and 1 (seqNum increment present,
// price copy present); seqNum=100 (0xE4), price=50 (0xB2).
func TestCrossFileRef(t *testing.T) {
	var dec QuoteDecoder
	r := fastcore.NewReader([]byte{0xE0, 0xE4, 0xB2})
	var m Quote
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if m.SeqNum != 100 {
		t.Errorf("SeqNum = %d, want 100", m.SeqNum)
	}
	if m.Price != 50 {
		t.Errorf("Price = %d, want 50", m.Price)
	}
}

func BenchmarkCrossFileTemplateRef(b *testing.B) {
	var dec QuoteDecoder
	r := &fastcore.Reader{}
	frame := []byte{0xE0, 0xE4, 0xB2}
	var m Quote
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
