package simple

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

func FuzzTestDecoder(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0xf0, 0x81, 0x82, 0x83},
		{0x80},
		{0xf0, 0x81},
		{0x7f, 0xff},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var dec TestDecoder
		var m Test
		r := fastcore.NewReader(data)
		_ = dec.Decode(r, &m)
		assertReaderSane(t, r, len(data))
	})
}

func FuzzRouterDecode(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0xc0, 0x81},
		{0xf8, 0x81, 0x8a, 0x94, 0x9e},
		{0x80},
		{0xc0, 0xe3},
		{0xf8, 0x81, 0x8a},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var rt Router
		rt.ResetID = 99
		r := fastcore.NewReader(data)
		for r.Remaining() > 0 {
			before := r.Pos()
			_, err := rt.Decode(r)
			assertReaderSane(t, r, len(data))
			if err != nil {
				return
			}
			if r.Pos() <= before {
				t.Fatalf("router decode did not advance: before=%d after=%d", before, r.Pos())
			}
		}
	})
}

func FuzzRouterStatefulDecode(f *testing.F) {
	for _, seed := range [][]byte{
		{0xf8, 0x81, 0x8a, 0x94, 0x9e, 0x80},
		{0xf8, 0x81, 0x8a, 0x94, 0x9e, 0xc0, 0xe3, 0xf8, 0x81, 0xa8, 0xb2, 0xbc},
		{0xf8, 0x81, 0x8a, 0x80},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var rt Router
		rt.ResetID = 99
		r := fastcore.NewReader(data)
		for r.Remaining() > 0 {
			before := r.Pos()
			_, err := rt.Decode(r)
			assertReaderSane(t, r, len(data))
			if err != nil {
				return
			}
			if r.Pos() <= before {
				t.Fatalf("router decode did not advance: before=%d after=%d", before, r.Pos())
			}
		}
	})
}

func assertReaderSane(t *testing.T, r *fastcore.Reader, inputLen int) {
	t.Helper()
	if r.Pos() < 0 || r.Pos() > inputLen {
		t.Fatalf("reader position %d outside input length %d", r.Pos(), inputLen)
	}
	if r.Remaining() != inputLen-r.Pos() {
		t.Fatalf("remaining = %d, want %d", r.Remaining(), inputLen-r.Pos())
	}
}
