package fastcore

import "testing"

func TestReadEntityUnsigned(t *testing.T) {
	cases := []struct {
		in   []byte
		want uint64
	}{
		{[]byte{0x80}, 0},          // single zero group
		{[]byte{0x81}, 1},          // 1
		{[]byte{0x82}, 2},          // 2
		{[]byte{0x39, 0xC5}, 7365}, // (57<<7)|69
		{[]byte{0x10, 0x00, 0x00, 0x00, 0x80}, 4294967296}, // 2^32, spec §10.6.1 note
	}
	for _, c := range cases {
		r := NewReader(c.in)
		got, err := r.ReadUint()
		if err != nil {
			t.Fatalf("ReadUint(% x): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ReadUint(% x) = %d, want %d", c.in, got, c.want)
		}
		if r.Remaining() != 0 {
			t.Errorf("ReadUint(% x): %d bytes left", c.in, r.Remaining())
		}
	}
}

func TestReadEntitySigned(t *testing.T) {
	cases := []struct {
		in   []byte
		want int64
	}{
		{[]byte{0x82}, 2},          // +2
		{[]byte{0xFE}, -2},         // 1111110 -> sign-extended = -2
		{[]byte{0xFF}, -1},         // -1
		{[]byte{0xC0}, -64},        // 1000000 -> -64
		{[]byte{0x00, 0xC0}, 64},   // spec §10.6.1.1 note: 64 needs leading zero group
		{[]byte{0x3F, 0xFF}, 8191}, // (63<<7)|127
	}
	for _, c := range cases {
		r := NewReader(c.in)
		got, err := r.ReadInt()
		if err != nil {
			t.Fatalf("ReadInt(% x): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ReadInt(% x) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNullableInt(t *testing.T) {
	// NULL is the all-zero entity (0x80).
	r := NewReader([]byte{0x80})
	if _, null, err := r.ReadNullableUint(); err != nil || !null {
		t.Errorf("nullable uint NULL: null=%v err=%v", null, err)
	}
	// 0x81 -> transmitted 1 -> value 0 (shift undone).
	r = NewReader([]byte{0x81})
	if v, null, err := r.ReadNullableUint(); err != nil || null || v != 0 {
		t.Errorf("nullable uint 0: v=%d null=%v err=%v", v, null, err)
	}
	// signed negative is not shifted: 0xFE -> -2.
	r = NewReader([]byte{0xFE})
	if v, null, err := r.ReadNullableInt(); err != nil || null || v != -2 {
		t.Errorf("nullable int -2: v=%d null=%v err=%v", v, null, err)
	}
}

func TestPMAP(t *testing.T) {
	// 0xC0 = stop bit set, data 1000000 -> first bit 1, rest 0.
	r := NewReader([]byte{0xC0})
	pm, err := r.ReadPMAP(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !pm.Next() {
		t.Error("bit 0 should be set")
	}
	for i := 1; i < 10; i++ {
		if pm.Next() {
			t.Errorf("bit %d should be clear (incl. infinite zero suffix)", i)
		}
	}
	if pm.BitsConsumed() != 10 {
		t.Errorf("BitsConsumed = %d, want 10", pm.BitsConsumed())
	}
}

func TestReadASCII(t *testing.T) {
	// "value": 0x76 0x61 0x6C 0x75 0xE5 (last byte has stop bit).
	r := NewReader([]byte{0x76, 0x61, 0x6C, 0x75, 0xE5})
	if s, err := r.ReadASCII(); err != nil || s != "value" {
		t.Errorf("ReadASCII = %q err=%v, want \"value\"", s, err)
	}
	// empty string: single 0x80.
	r = NewReader([]byte{0x80})
	if s, err := r.ReadASCII(); err != nil || s != "" {
		t.Errorf("ReadASCII empty = %q err=%v", s, err)
	}
	// nullable NULL: single 0x80.
	r = NewReader([]byte{0x80})
	if s, null, err := r.ReadNullableASCII(); err != nil || !null || s != "" {
		t.Errorf("ReadNullableASCII NULL: s=%q null=%v err=%v", s, null, err)
	}
}

func TestReadByteVector(t *testing.T) {
	// length 5 (0x85) then raw "value".
	r := NewReader([]byte{0x85, 0x76, 0x61, 0x6C, 0x75, 0x65})
	b, err := r.ReadByteVector()
	if err != nil || string(b) != "value" {
		t.Errorf("ReadByteVector = %q err=%v", b, err)
	}
}

// Regression tests for bugs found in review.

func TestSignedOverflowGuard(t *testing.T) {
	// More than 10 continuation groups cannot fit int64 -> ErrOverflow,
	// rather than silently wrapping (review bug 1).
	in := []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x81}
	r := NewReader(in)
	if _, err := r.ReadInt(); err != ErrOverflow {
		t.Errorf("ReadInt(11 groups) err = %v, want ErrOverflow", err)
	}
}

func TestIncrementWidthWrap(t *testing.T) {
	// uInt32 at its max increments (no pmap bit) to 0, not 2^32 (review bug 2).
	r := NewReader([]byte{0x80}) // pmap: increment bit clear
	pm, _ := r.ReadPMAP(nil)
	slot := UintSlot{State: Assigned, Val: 0xFFFFFFFF}
	v, present, err := DecodeUint(r, &pm, OpIncrement, W32, false, true, 0, &slot)
	if err != nil || !present || v != 0 {
		t.Errorf("uInt32 increment wrap = %d present=%v err=%v, want 0", v, present, err)
	}
	// int32 max wraps to int32 min.
	r = NewReader([]byte{0x80})
	pm, _ = r.ReadPMAP(nil)
	islot := IntSlot{State: Assigned, Val: 0x7FFFFFFF}
	iv, _, err := DecodeInt(r, &pm, OpIncrement, W32, false, true, 0, &islot)
	if err != nil || iv != -0x80000000 {
		t.Errorf("int32 increment wrap = %d err=%v, want -2147483648", iv, err)
	}
}

func TestOverlongNullableASCII(t *testing.T) {
	// Nullable entity 0x00 0x41 is an overlong "A" (review bug 3): the nullable
	// preamble was unnecessary for a non-NUL-leading string.
	r := NewReader([]byte{0x00, 0xC1}) // data bytes 0x00, 0x41
	if _, _, err := r.ReadNullableASCII(); err != ErrOverlong {
		t.Errorf("ReadNullableASCII overlong err = %v, want ErrOverlong", err)
	}
}
