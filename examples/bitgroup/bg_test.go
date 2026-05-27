package bitgroup

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

// TestBitGroup decodes a message whose only field is a bit group packing four
// sub-fields into one SBIT byte. The message presence map is empty (0x80) since
// the bit group has no operator; the bit-group byte 0xDD has data bits 1011101:
//
//	updateAction (2 bits) = 10 = 2
//	entryType    (1 bit)  = 1
//	endOfBook    (1 bit)  = 1 -> true
//	priceLevel   (3 bits) = 101 = 5
func TestBitGroup(t *testing.T) {
	var dec LevelDecoder
	r := fastcore.NewReader([]byte{0x80, 0xDD})
	var m Level
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if m.UpdateAction != 2 {
		t.Errorf("UpdateAction = %d, want 2", m.UpdateAction)
	}
	if m.EntryType != 1 {
		t.Errorf("EntryType = %d, want 1", m.EntryType)
	}
	if !m.EndOfBook {
		t.Errorf("EndOfBook = false, want true")
	}
	if m.PriceLevel != 5 {
		t.Errorf("PriceLevel = %d, want 5", m.PriceLevel)
	}
}
