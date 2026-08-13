package fastcore

import "testing"

func FuzzReaderIntegerPrimitives(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0x81},
		{0xff},
		{0x00, 0xc0},
		{0x39, 0xc5},
		{0x10, 0x00, 0x00, 0x00, 0x80},
		{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x81},
		{0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0xff},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data)
		_, _ = r.ReadUint()
		assertReaderSane(t, r, len(data))

		r.Reset(data)
		_, _ = r.ReadInt()
		assertReaderSane(t, r, len(data))

		r.Reset(data)
		_, _, _ = r.ReadNullableUint()
		assertReaderSane(t, r, len(data))

		r.Reset(data)
		_, _, _ = r.ReadNullableInt()
		assertReaderSane(t, r, len(data))
	})
}

func FuzzReaderDecimalPrimitives(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0x80, 0x80},
		{0x81, 0x81},
		{0xfe, 0x39, 0xc5},
		{0x00, 0xc0, 0xff},
		{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x81},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data)
		_, _ = r.ReadScaledNumber()
		assertReaderSane(t, r, len(data))

		r.Reset(data)
		_, _, _ = r.ReadNullableScaledNumber()
		assertReaderSane(t, r, len(data))
	})
}

func FuzzReaderPMAP(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0xc0},
		{0x7f, 0xff},
		{0x00, 0x00, 0x80},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data)
		pm, err := r.ReadPMAP(nil)
		assertReaderSane(t, r, len(data))
		if err != nil {
			return
		}
		if got, want := pm.n+7*len(pm.ext), 7*r.Pos(); got != want {
			t.Fatalf("unread bits = %d, want %d (one 7-bit group per entity byte)", got, want)
		}
		for range len(data)*8 + 16 {
			_ = pm.Next()
		}
		if pm.BitsConsumed() != len(data)*8+16 {
			t.Fatalf("bits consumed = %d, want %d", pm.BitsConsumed(), len(data)*8+16)
		}
	})
}

func FuzzReaderASCII(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0x00, 0x80},
		{0x00, 0xc1},
		{0x76, 0x61, 0x6c, 0x75, 0xe5},
		{0x00, 0x00, 0x80},
		{0x7f, 0x7f, 0xff},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data)
		_, _ = r.ReadASCII()
		assertReaderSane(t, r, len(data))

		r.Reset(data)
		_, _, _ = r.ReadNullableASCII()
		assertReaderSane(t, r, len(data))
	})
}

func FuzzReaderByteVector(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0x81, 0x00},
		{0x85, 0x76, 0x61, 0x6c, 0x75, 0x65},
		{0x85, 0x76},
		{0x7f, 0xff},
		{0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0xff},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data)
		b, err := r.ReadByteVector()
		assertReaderSane(t, r, len(data))
		if err == nil && len(b) > len(data) {
			t.Fatalf("byte vector length %d exceeds input length %d", len(b), len(data))
		}

		r.Reset(data)
		b, _, err = r.ReadNullableByteVector()
		assertReaderSane(t, r, len(data))
		if err == nil && len(b) > len(data) {
			t.Fatalf("nullable byte vector length %d exceeds input length %d", len(b), len(data))
		}
	})
}

func FuzzReaderBitGroup(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		{0x80},
		{0xff},
		{0x7f, 0xff},
		{0x00, 0x00, 0x80},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data)
		br, err := r.ReadBitGroup(nil)
		assertReaderSane(t, r, len(data))
		if err != nil {
			return
		}
		for width := 0; width <= 65; width++ {
			copyReader := br
			_, _ = copyReader.ReadBits(width)
			copyReader = br
			_, _ = copyReader.ReadBitsSigned(width)
		}
	})
}

func assertReaderSane(t *testing.T, r *Reader, inputLen int) {
	t.Helper()
	if r.Pos() < 0 || r.Pos() > inputLen {
		t.Fatalf("reader position %d outside input length %d", r.Pos(), inputLen)
	}
	if r.Remaining() != inputLen-r.Pos() {
		t.Fatalf("remaining = %d, want %d", r.Remaining(), inputLen-r.Pos())
	}
}
