package templateref

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

// TestStaticTemplateRef decodes a Msg whose body statically references Common;
// the referenced template's seqNum field is inlined and decoded in the same
// segment. PMAP 0xE0 sets bits 0 and 1 (msgID copy present, seqNum increment
// present); msgID=10 (0x8A), seqNum=1 (0x81).
func TestStaticTemplateRef(t *testing.T) {
	var dec MsgDecoder
	r := fastcore.NewReader([]byte{0xE0, 0x8A, 0x81})
	var m Msg
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if m.MsgID != 10 {
		t.Errorf("MsgID = %d, want 10", m.MsgID)
	}
	if m.SeqNum != 1 {
		t.Errorf("SeqNum = %d, want 1", m.SeqNum)
	}
}

// TestTemplateRefInGroup exercises static templateRef inlining inside a group
// (a nested segment with its own presence map).
func TestTemplateRefInGroup(t *testing.T) {
	var dec WrapDecoder
	// message PMAP 0xC0 (msgID copy present), msgID 10 (0x8A); then the
	// mandatory group reads its own PMAP 0xC0 (seqNum increment present),
	// seqNum 1 (0x81).
	r := fastcore.NewReader([]byte{0xC0, 0x8A, 0xC0, 0x81})
	var m Wrap
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if m.MsgID != 10 {
		t.Errorf("MsgID = %d, want 10", m.MsgID)
	}
	if m.Hdr.SeqNum != 1 {
		t.Errorf("Hdr.SeqNum = %d, want 1", m.Hdr.SeqNum)
	}
}

func BenchmarkTemplateRefInGroup(b *testing.B) {
	var dec WrapDecoder
	r := &fastcore.Reader{}
	frame := []byte{0xC0, 0x8A, 0xC0, 0x81}
	var m Wrap
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
