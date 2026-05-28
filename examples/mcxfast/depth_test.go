package mcxfast

import (
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
)

var (
	depthIncrementalFrame = buildDepthIncrementalFrame()
	depthSnapshotFrame    = buildDepthSnapshotFrame()
)

func TestDepthIncrementalDecode(t *testing.T) {
	var dec DepthIncrementalDecoder
	var m DepthIncremental
	if err := dec.Decode(fastcore.NewReader(depthIncrementalFrame), &m); err != nil {
		t.Fatal(err)
	}
	if m.MsgType != "X" || m.MsgSeqNum != 1 || m.SenderCompID != 10 || m.MarketSegmentID != 2 {
		t.Fatalf("header = (%q,%d,%d,%d), want (X,1,10,2)", m.MsgType, m.MsgSeqNum, m.SenderCompID, m.MarketSegmentID)
	}
	if len(m.MDIncGrp) != 4 {
		t.Fatalf("len(MDIncGrp) = %d, want 4", len(m.MDIncGrp))
	}
	last := m.MDIncGrp[3]
	if last.SecurityID != 1003 || !last.HasMDEntryPx || last.MDEntryPx.Mant != 4006 || last.MDEntryPx.Exp != -8 || last.MDPriceLevel != 4 {
		t.Fatalf("last incremental entry = %+v, want security 1003, px 4006x10^-8, level 4", last)
	}
}

func TestDepthSnapshotDecode(t *testing.T) {
	var dec DepthSnapshotDecoder
	var m DepthSnapshot
	if err := dec.Decode(fastcore.NewReader(depthSnapshotFrame), &m); err != nil {
		t.Fatal(err)
	}
	if m.MsgType != "W" || !m.HasMsgSeqNum || m.MsgSeqNum != 1 || m.SenderCompID != 10 {
		t.Fatalf("header = (%q,%v,%d,%d), want (W,true,1,10)", m.MsgType, m.HasMsgSeqNum, m.MsgSeqNum, m.SenderCompID)
	}
	if m.SecurityID != 1000 || len(m.MDSshGrp) != 4 {
		t.Fatalf("security/entries = (%d,%d), want (1000,4)", m.SecurityID, len(m.MDSshGrp))
	}
	last := m.MDSshGrp[3]
	if last.MDEntryType != MDEntryTypeOffer || !last.HasMDEntryPx || last.MDEntryPx.Mant != 4006 || last.MDEntryPx.Exp != -8 || last.MDPriceLevel != 4 {
		t.Fatalf("last snapshot entry = %+v, want offer, px 4006x10^-8, level 4", last)
	}
}

func BenchmarkDepthIncrementalDecode(b *testing.B) {
	var dec DepthIncrementalDecoder
	r := &fastcore.Reader{}
	var m DepthIncremental
	if err := dec.Decode(fastcore.NewReader(depthIncrementalFrame), &m); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(depthIncrementalFrame)
		if err := dec.Decode(r, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDepthSnapshotDecode(b *testing.B) {
	var dec DepthSnapshotDecoder
	r := &fastcore.Reader{}
	var m DepthSnapshot
	if err := dec.Decode(fastcore.NewReader(depthSnapshotFrame), &m); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(depthSnapshotFrame)
		if err := dec.Decode(r, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func buildDepthIncrementalFrame() []byte {
	var out []byte
	out = append(out, pmap(true, true, true)...)
	out = appendUint(out, 1)  // MsgSeqNum
	out = appendUint(out, 10) // SenderCompID
	out = appendUint(out, 2)  // MarketSegmentID
	out = appendUint(out, 4)  // NoMDEntries
	for i := 0; i < 4; i++ {
		out = append(out, pmap(false, true, true, true, true, false)...)
		out = appendUint(out, uint64(i%2))        // MDUpdateAction
		out = appendUint(out, uint64(i%2))        // MDEntryType
		out = appendInt(out, int64(1000+i))       // SecurityID
		out = appendNullableInt(out, -2, false)   // MDEntryPx exponent delta
		out = appendInt(out, int64(1000+i))       // MDEntryPx mantissa delta
		out = appendNullableInt(out, 0, false)    // MDEntrySize exponent delta
		out = appendInt(out, int64(10+i))         // MDEntrySize mantissa delta
		out = appendNullableInt(out, 1, false)    // NumberOfOrders delta
		out = appendNullableUint(out, uint64(i+1), false)
		out = appendNullableInt(out, int64(100000+i), false) // MDEntryTime
		out = appendNullableUint(out, 0, true)               // PotentialSecurityTradingEvent
		out = appendNullableUint(out, 0, true)               // QuoteCondition
		out = appendNullableInt(out, 0, true)                // TotalBuyQuantity
		out = appendNullableInt(out, 0, true)                // TotalSellQuantity
	}
	return out
}

func buildDepthSnapshotFrame() []byte {
	var out []byte
	out = append(out, pmap(true, true, true, true, true, true, false, true)...)
	out = appendNullableUint(out, 1, false) // MsgSeqNum
	out = appendUint(out, 10)               // SenderCompID
	out = appendNullableUint(out, 1, false) // LastMsgSeqNumProcessed
	out = appendNullableUint(out, 0, false) // RefreshIndicator
	out = appendUint(out, 2)                // MarketSegmentID
	out = appendInt(out, 1000)              // SecurityID delta
	out = appendUint(out, 0)                // ProductComplex
	out = appendNullableUint(out, 0, true)  // TESSecurityStatus
	out = appendInt(out, 100000)            // LastUpdateTime delta
	out = appendNullableInt(out, 0, false)  // TotalBuyQuantity exponent delta
	out = appendInt(out, 100)               // TotalBuyQuantity mantissa delta
	out = appendNullableInt(out, 0, false)  // TotalSellQuantity exponent delta
	out = appendInt(out, 120)               // TotalSellQuantity mantissa delta
	out = appendUint(out, 4)                // NoMDEntries
	for i := 0; i < 4; i++ {
		out = append(out, pmap(
			false, true, false, false, false, false, false,
			false, false, false, false, false, false, false,
			false, false, false, false, true, false, false,
		)...)
		out = appendUint(out, uint64(i%2))      // MDEntryType
		out = appendNullableInt(out, -2, false) // MDEntryPx exponent delta
		out = appendInt(out, int64(1000+i))     // MDEntryPx mantissa delta
		out = appendNullableInt(out, 0, false)  // MDEntrySize exponent delta
		out = appendInt(out, int64(10+i))       // MDEntrySize mantissa delta
		out = appendNullableInt(out, 1, false)  // NumberOfOrders delta
		out = appendNullableUint(out, uint64(i+1), false)
		out = appendNullableInt(out, 0, true) // MDEntryTime
		out = appendNullableInt(out, 0, true) // TotalTradedValue
		out = appendNullableInt(out, 0, true) // AverageTradedPrice
	}
	return out
}

func pmap(bits ...bool) []byte {
	if len(bits) == 0 {
		return []byte{0x80}
	}
	n := (len(bits) + 6) / 7
	out := make([]byte, n)
	for i, bit := range bits {
		if bit {
			out[i/7] |= 1 << uint(6-i%7)
		}
	}
	out[len(out)-1] |= 0x80
	return out
}

func appendNullableUint(out []byte, v uint64, null bool) []byte {
	if null {
		return append(out, 0x80)
	}
	return appendUint(out, v+1)
}

func appendUint(out []byte, v uint64) []byte {
	var groups [10]byte
	i := len(groups)
	for {
		i--
		groups[i] = byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			break
		}
	}
	for ; i < len(groups); i++ {
		b := groups[i]
		if i == len(groups)-1 {
			b |= 0x80
		}
		out = append(out, b)
	}
	return out
}

func appendNullableInt(out []byte, v int64, null bool) []byte {
	if null {
		return append(out, 0x80)
	}
	if v >= 0 {
		return appendInt(out, v+1)
	}
	return appendInt(out, v)
}

func appendInt(out []byte, v int64) []byte {
	var groups [10]byte
	i := len(groups)
	for {
		i--
		groups[i] = byte(v & 0x7f)
		sign := groups[i]&0x40 != 0
		v >>= 7
		if (v == 0 && !sign) || (v == -1 && sign) {
			break
		}
	}
	for ; i < len(groups); i++ {
		b := groups[i]
		if i == len(groups)-1 {
			b |= 0x80
		}
		out = append(out, b)
	}
	return out
}
