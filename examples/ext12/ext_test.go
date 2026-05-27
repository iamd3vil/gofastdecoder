package ext12

import (
	"testing"
	"time"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

// TestExtDecode decodes a message with FAST 1.2 extension types. All three
// fields are mandatory with no operator, so they consume no presence-map bits;
// the PMAP byte 0x80 is empty.
//
//	flag (boolean): 0x81 -> 1 -> true
//	when (timestamp, seconds): 0x00 0xE4 -> 100 -> 1970-01-01T00:01:40Z
//	kind (enum):    0x82 -> 2
func TestExtDecode(t *testing.T) {
	var dec ExtDecoder
	r := fastcore.NewReader([]byte{0x80, 0x81, 0x00, 0xE4, 0x82})
	var m Ext
	if err := dec.Decode(r, &m); err != nil {
		t.Fatal(err)
	}
	if !m.Flag {
		t.Errorf("Flag = false, want true")
	}
	if want := time.Unix(100, 0).UTC(); !m.When.Equal(want) {
		t.Errorf("When = %v, want %v", m.When, want)
	}
	if m.Kind != ExtKindC { // typed enum constant; encoded value 2
		t.Errorf("Kind = %d, want %d (ExtKindC)", m.Kind, ExtKindC)
	}
}
