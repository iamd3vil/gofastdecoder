package rich

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

var marketDataFrame = []byte{
	0x80,                   // top-level PMAP
	0x81,                   // seqNum = 1
	0xE4,                   // sendingTime = 100
	0x41, 0x41, 0x50, 0xCC, // symbol = AAPL
	0xFE, 0xAA, // price = 42 x 10^-2
	0x80,                         // stats PMAP
	0x85,                         // bidCount = 5
	0x84, 0xDE, 0xAD, 0xBE, 0xEF, // payload = 4 bytes
	0x84,             // entries length = 4
	0x80, 0x81, 0xC2, 0xFE, 0xAA, 0x8A, // entry 1
	0x80, 0x82, 0xC1, 0xFE, 0xAB, 0x94, // entry 2
	0x80, 0x81, 0xC2, 0xFE, 0xAC, 0x9E, // entry 3
	0x80, 0x82, 0xC1, 0xFE, 0xAD, 0xA8, // entry 4
}

func TestMarketDataDecode(t *testing.T) {
	var dec MarketDataDecoder
	var m MarketData
	if err := dec.Decode(fastcore.NewReader(marketDataFrame), &m); err != nil {
		t.Fatal(err)
	}
	if m.SeqNum != 1 || m.SendingTime != 100 || m.Symbol != "AAPL" {
		t.Fatalf("header = (%d,%d,%q), want (1,100,AAPL)", m.SeqNum, m.SendingTime, m.Symbol)
	}
	if m.Price.Mant != 42 || m.Price.Exp != -2 {
		t.Fatalf("Price = %dx10^%d, want 42x10^-2", m.Price.Mant, m.Price.Exp)
	}
	if m.Stats.BidCount != 5 || string(m.Stats.Payload) != "\xde\xad\xbe\xef" {
		t.Fatalf("Stats = (%d,%x), want (5,deadbeef)", m.Stats.BidCount, m.Stats.Payload)
	}
	if len(m.Entries) != 4 {
		t.Fatalf("len(Entries) = %d, want 4", len(m.Entries))
	}
	if m.Entries[3].EntryPx.Mant != 45 || m.Entries[3].Size != 40 {
		t.Fatalf("entry 4 = %+v, want px mantissa 45 and size 40", m.Entries[3])
	}
}

func BenchmarkMarketDataDecode(b *testing.B) {
	var dec MarketDataDecoder
	r := &fastcore.Reader{}
	var m MarketData
	if err := dec.Decode(fastcore.NewReader(marketDataFrame), &m); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(marketDataFrame)
		if err := dec.Decode(r, &m); err != nil {
			b.Fatal(err)
		}
	}
}
