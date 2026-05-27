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

// BenchmarkDecode confirms the decode hot path is allocation-free once the
// decoder's buffers are warm (the zero-alloc goal from PLAN.md).
func BenchmarkDecode(b *testing.B) {
	var dec TestDecoder
	r := &fastcore.Reader{}
	frame := []byte{0xF0, 0x81, 0x82, 0x83}
	var m Test
	dec.Decode(fastcore.NewReader(frame), &m) // warm the pmap buffer
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(frame)
		if err := dec.Decode(r, &m); err != nil {
			b.Fatal(err)
		}
	}
}

// TestZeroAlloc enforces the zero-allocation hot path as a gate (PLAN.md step 8).
func TestZeroAlloc(t *testing.T) {
	var dec TestDecoder
	r := &fastcore.Reader{}
	frame := []byte{0xF0, 0x81, 0x82, 0x83}
	var m Test
	dec.Decode(fastcore.NewReader(frame), &m) // warm buffers
	if n := testing.AllocsPerRun(100, func() {
		r.Reset(frame)
		_ = dec.Decode(r, &m)
	}); n != 0 {
		t.Errorf("decode allocates %.0f times/op, want 0", n)
	}
}

// TestRouterDispatch decodes framed messages through the generated Router: the
// presence map's bit 0 plus a global-dictionary copy carries the template id,
// and the remaining bits drive the template's fields. The second message omits
// both the template id and all field values, exercising copy-from-dictionary
// for the template id and every field across messages.
func TestRouterDispatch(t *testing.T) {
	var rt Router
	r := &fastcore.Reader{}

	// Message 1: PMAP 0xF8 = bits 0,1,2,3 set (tid present + 3 fields present);
	// tid=1 (0x81), then field values 10 (0x8A), 20 (0x94), 30 (0x9E).
	r.Reset([]byte{0xF8, 0x81, 0x8A, 0x94, 0x9E})
	msg, err := rt.Decode(r)
	if err != nil {
		t.Fatalf("message 1: %v", err)
	}
	m, ok := msg.(*Test)
	if !ok {
		t.Fatalf("message 1: got %T, want *Test", msg)
	}
	if m.Field1 != 10 || m.Field2 != 20 || m.Field3 != 30 {
		t.Fatalf("message 1: got (%d,%d,%d), want (10,20,30)", m.Field1, m.Field2, m.Field3)
	}

	// Message 2: PMAP 0x80 = no bits set; the template id copies to 1 and every
	// field copies its previous value.
	r.Reset([]byte{0x80})
	msg, err = rt.Decode(r)
	if err != nil {
		t.Fatalf("message 2: %v", err)
	}
	m, ok = msg.(*Test)
	if !ok {
		t.Fatalf("message 2: got %T, want *Test", msg)
	}
	if m.Field1 != 10 || m.Field2 != 20 || m.Field3 != 30 {
		t.Fatalf("message 2 (copy): got (%d,%d,%d), want (10,20,30)", m.Field1, m.Field2, m.Field3)
	}
}
