package fastcore

import (
	"testing"
	"time"
)

func TestBitReaderSingleByte(t *testing.T) {
	// Spec example: fields A(2 bits), B(1), C(1) in one SBIT byte 'SAABCxxx'.
	// A=2 (10), B=1, C=0 -> data bits 1010000 = 0x50, SBIT byte = 0xD0.
	r := NewReader([]byte{0xD0})
	bg, err := r.ReadBitGroup(nil)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := bg.ReadBits(2); a != 2 {
		t.Errorf("A = %d, want 2", a)
	}
	if b, _ := bg.ReadBits(1); b != 1 {
		t.Errorf("B = %d, want 1", b)
	}
	if c, _ := bg.ReadBits(1); c != 0 {
		t.Errorf("C = %d, want 0", c)
	}
}

func TestBitReaderSpanningBytes(t *testing.T) {
	// Two SBIT bytes => 14 data bits. Read a 5-bit then a 6-bit field that
	// spans the byte boundary, then a 3-bit field.
	// data bits: byte0 = 1111100 (0x7C -> SBIT 0x7C), byte1 = 1110110 (0x76 -> SBIT 0xF6)
	// concatenated: 1111100 1110110
	// A(5)=11111=31; B(6)=001110=14; C(3)=110=6
	r := NewReader([]byte{0x7C, 0xF6})
	bg, err := r.ReadBitGroup(nil)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := bg.ReadBits(5); a != 31 {
		t.Errorf("A = %d, want 31", a)
	}
	if b, _ := bg.ReadBits(6); b != 14 {
		t.Errorf("B = %d, want 14", b)
	}
	if c, _ := bg.ReadBits(3); c != 6 {
		t.Errorf("C = %d, want 6", c)
	}
}

func TestBitReaderSigned(t *testing.T) {
	// 3-bit field 101 = -3 in two's complement. Top 3 data bits = 101,
	// so data byte = 1010000 = 0x50, SBIT byte = 0xD0.
	r := NewReader([]byte{0xD0})
	bg, _ := r.ReadBitGroup(nil)
	v, err := bg.ReadBitsSigned(3)
	if err != nil {
		t.Fatal(err)
	}
	if v != -3 {
		t.Errorf("signed 3-bit = %d, want -3", v)
	}
}

func TestTimestampUTC(t *testing.T) {
	// 1 day since epoch = 1970-01-02.
	got := TimestampUTC(1, UnitDay)
	want := time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("UnitDay 1 = %v, want %v", got, want)
	}
	// 1500000000 seconds.
	if got := TimestampUTC(1500000000, UnitSecond); got.Unix() != 1500000000 {
		t.Errorf("UnitSecond = %v", got)
	}
	if d := UnitMillisecond.Duration(1500); d != 1500*time.Millisecond {
		t.Errorf("Duration = %v", d)
	}
}

func TestDecodeIntSigned(t *testing.T) {
	// delta operator on a signed field, mandatory, initial -5, delta +2 -> -3.
	// pmap 0xC0, delta 0x82 (=+2).
	r := NewReader([]byte{0xC0, 0x82})
	pm, _ := r.ReadPMAP(nil)
	var slot IntSlot
	v, present, err := DecodeInt(r, &pm, OpDelta, false, true, -5, &slot)
	if err != nil || !present || v != -3 {
		t.Fatalf("DecodeInt delta = %d present=%v err=%v, want -3", v, present, err)
	}
}
