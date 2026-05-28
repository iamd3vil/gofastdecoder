package mcxfast

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

const fuzzMaxEntityValue = 256

func FuzzDepthIncrementalDecoder(f *testing.F) {
	f.Add(buildFuzzDepthIncrementalFrame())
	f.Add(depthIncrementalFrame)
	f.Add(depthIncrementalFrame[:1])
	f.Add([]byte{0x80})
	f.Add([]byte{0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		if !withinFuzzEntityBudget(data) {
			t.Skip()
		}
		var dec DepthIncrementalDecoder
		var m DepthIncremental
		r := fastcore.NewReader(data)
		_ = dec.Decode(r, &m)
		assertReaderSane(t, r, len(data))
	})
}

func FuzzDepthSnapshotDecoder(f *testing.F) {
	f.Add(buildFuzzDepthSnapshotFrame())
	f.Add(depthSnapshotFrame)
	f.Add(depthSnapshotFrame[:1])
	f.Add([]byte{0x80})
	f.Add([]byte{0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		if !withinFuzzEntityBudget(data) {
			t.Skip()
		}
		var dec DepthSnapshotDecoder
		var m DepthSnapshot
		r := fastcore.NewReader(data)
		_ = dec.Decode(r, &m)
		assertReaderSane(t, r, len(data))
	})
}

func FuzzRouterDecode(f *testing.F) {
	for _, seed := range [][]byte{
		buildFuzzDepthIncrementalRouterFrame(),
		buildFuzzDepthSnapshotRouterFrame(),
		{0x80},
		{0xc0, 0xe6},
		{0xc0, 0xe5},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if !withinFuzzEntityBudget(data) {
			t.Skip()
		}
		var rt Router
		rt.ResetID = 120
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

func buildFuzzDepthIncrementalFrame() []byte {
	var out []byte
	out = append(out, pmap(true, true, true)...)
	out = appendUint(out, 1) // MsgSeqNum
	out = appendUint(out, 1) // SenderCompID
	out = appendUint(out, 1) // MarketSegmentID
	out = appendUint(out, 1) // NoMDEntries
	out = append(out, pmap(false, true, true, true, true, false)...)
	out = appendUint(out, 0)                // MDUpdateAction
	out = appendUint(out, 0)                // MDEntryType
	out = appendInt(out, 1)                 // SecurityID
	out = appendNullableInt(out, -2, false) // MDEntryPx exponent delta
	out = appendInt(out, 1)                 // MDEntryPx mantissa delta
	out = appendNullableInt(out, 0, false)  // MDEntrySize exponent delta
	out = appendInt(out, 1)                 // MDEntrySize mantissa delta
	out = appendNullableInt(out, 1, false)  // NumberOfOrders delta
	out = appendNullableUint(out, 1, false)
	out = appendNullableInt(out, 1, false) // MDEntryTime
	out = appendNullableUint(out, 0, true) // PotentialSecurityTradingEvent
	out = appendNullableUint(out, 0, true) // QuoteCondition
	out = appendNullableInt(out, 0, true)  // TotalBuyQuantity
	out = appendNullableInt(out, 0, true)  // TotalSellQuantity
	return out
}

func buildFuzzDepthSnapshotFrame() []byte {
	var out []byte
	out = append(out, pmap(true, true, true, true, true, true, false, true)...)
	out = appendNullableUint(out, 1, false) // MsgSeqNum
	out = appendUint(out, 1)                // SenderCompID
	out = appendNullableUint(out, 1, false) // LastMsgSeqNumProcessed
	out = appendNullableUint(out, 0, false) // RefreshIndicator
	out = appendUint(out, 1)                // MarketSegmentID
	out = appendInt(out, 1)                 // SecurityID delta
	out = appendUint(out, 0)                // ProductComplex
	out = appendNullableUint(out, 0, true)  // TESSecurityStatus
	out = appendInt(out, 1)                 // LastUpdateTime delta
	out = appendNullableInt(out, 0, false)  // TotalBuyQuantity exponent delta
	out = appendInt(out, 1)                 // TotalBuyQuantity mantissa delta
	out = appendNullableInt(out, 0, false)  // TotalSellQuantity exponent delta
	out = appendInt(out, 1)                 // TotalSellQuantity mantissa delta
	out = appendUint(out, 0)                // NoMDEntries
	return out
}

func buildFuzzDepthIncrementalRouterFrame() []byte {
	payload := buildFuzzDepthIncrementalFrame()
	out := pmap(true, true, true, true)
	out = appendUint(out, 102)
	out = append(out, payload[pmapLen(payload):]...)
	return out
}

func buildFuzzDepthSnapshotRouterFrame() []byte {
	payload := buildFuzzDepthSnapshotFrame()
	out := pmap(true, true, true, true, true, true, true, false, true)
	out = appendUint(out, 101)
	out = append(out, payload[pmapLen(payload):]...)
	return out
}

func withinFuzzEntityBudget(data []byte) bool {
	var v uint64
	for i, b := range data {
		v = (v << 7) | uint64(b&0x7f)
		if v > fuzzMaxEntityValue {
			return false
		}
		if b&0x80 != 0 {
			v = 0
			continue
		}
		if i == len(data)-1 {
			return true
		}
	}
	return true
}

func pmapLen(data []byte) int {
	for i, b := range data {
		if b&0x80 != 0 {
			return i + 1
		}
	}
	return len(data)
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
