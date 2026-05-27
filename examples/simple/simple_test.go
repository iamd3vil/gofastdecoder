package simple

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

// TestRoundTrip decodes two consecutive messages of the simple1 template
// (three mandatory uInt32 copy fields) and verifies both the decoded values
// and that the copy operator's dictionary state persists across messages.
func TestRoundTrip(t *testing.T) {
	var dec TestDecoder
	r := &fastcore.Reader{}

	// Message 1: PMAP 0xF0 (bits 0,1,2 set => all three fields present),
	// then mandatory uint values 1 (0x81), 2 (0x82), 3 (0x83).
	r.Reset([]byte{0xF0, 0x81, 0x82, 0x83})
	var m Test
	if err := dec.Decode(r, &m); err != nil {
		t.Fatalf("message 1: %v", err)
	}
	if m.Field1 != 1 || m.Field2 != 2 || m.Field3 != 3 {
		t.Fatalf("message 1: got (%d,%d,%d), want (1,2,3)", m.Field1, m.Field2, m.Field3)
	}

	// Message 2: PMAP 0x80 (no bits set) => every field copies its previous
	// value from the dictionary, so the values repeat without appearing on the
	// wire.
	r.Reset([]byte{0x80})
	var m2 Test
	if err := dec.Decode(r, &m2); err != nil {
		t.Fatalf("message 2: %v", err)
	}
	if m2.Field1 != 1 || m2.Field2 != 2 || m2.Field3 != 3 {
		t.Fatalf("message 2 (copy from dict): got (%d,%d,%d), want (1,2,3)", m2.Field1, m2.Field2, m2.Field3)
	}
}
