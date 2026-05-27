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
