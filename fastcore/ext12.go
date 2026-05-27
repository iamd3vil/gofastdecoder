package fastcore

import "time"

// FAST 1.2 extension primitives.
//
// enum and set fields are encoded as ordinary SBIT-encoded (unsigned) integers,
// so they are decoded with DecodeUint and mapped to elements/flags by generated
// code. boolean is an unsigned integer constrained to 0/1 (nullable: 0=NULL,
// 1=false, 2=true — exactly the nullable-uint shift, so ReadNullableUint yields
// 0/1). timestamp is a signed integer count of time units since an epoch and is
// decoded with DecodeInt; the helpers below convert the count to a time.Time.
//
// bitGroup packs two or more small fixed-width fields into one SBIT-encoded
// entity, assigned most-significant-bit first across the 7 data bits of each
// byte, possibly spanning byte boundaries (the 'SAABCxxx' layout).

// Bool converts a decoded boolean field value (0/1) to a Go bool.
func Bool(v uint64) bool { return v != 0 }

// BitReader reads fixed-width bit fields from a single SBIT-encoded entity,
// most-significant-bit first, as used by a FAST 1.2 bit group.
type BitReader struct {
	data    []byte // 7-bit data groups (stop bits stripped)
	numBits int
	cursor  int
}

// ReadBitGroup reads one SBIT entity from the stream into dst (reused to avoid
// allocation) and returns a BitReader over its packed data bits.
func (r *Reader) ReadBitGroup(dst []byte) (BitReader, error) {
	dst = dst[:0]
	for {
		if r.pos >= len(r.buf) {
			return BitReader{}, ErrEndOfStream
		}
		b := r.buf[r.pos]
		r.pos++
		dst = append(dst, b&0x7f)
		if b&0x80 != 0 {
			break
		}
	}
	return BitReader{data: dst, numBits: len(dst) * 7}, nil
}

// ReadBits reads the next n bits (1..64) as an unsigned value, MSB first.
func (b *BitReader) ReadBits(n int) (uint64, error) {
	if n < 0 || n > 64 {
		return 0, ErrOverflow
	}
	if b.cursor+n > b.numBits {
		return 0, ErrEndOfStream
	}
	var v uint64
	for i := range n {
		bit := b.cursor + i
		group := bit / 7
		off := uint(6 - bit%7)
		v = (v << 1) | uint64(b.data[group]>>off&1)
	}
	b.cursor += n
	return v, nil
}

// ReadBitsSigned reads n bits as a two's-complement signed value (FAST 1.2
// int2..int7 within a bit group).
func (b *BitReader) ReadBitsSigned(n int) (int64, error) {
	u, err := b.ReadBits(n)
	if err != nil {
		return 0, err
	}
	if n > 0 && u&(1<<(uint(n)-1)) != 0 {
		return int64(u) - (1 << uint(n)), nil // sign-extend
	}
	return int64(u), nil
}

// BitsConsumed reports how many bits have been read.
func (b *BitReader) BitsConsumed() int { return b.cursor }

// Buffer returns the backing byte slice, so a caller that passed a reusable
// buffer to ReadBitGroup can store the (possibly grown) slice for reuse.
func (b *BitReader) Buffer() []byte { return b.data }

// TimeUnit is the granularity of a FAST 1.2 timestamp (§Timestamp Data Type).
type TimeUnit uint8

const (
	UnitDay TimeUnit = iota
	UnitSecond
	UnitMillisecond
	UnitMicrosecond
	UnitNanosecond
)

// TimestampUTC converts a unit count since the UNIX epoch to a UTC time.Time.
// (The "today" epoch / UTCTimeOnly case is a time-of-day offset and is handled
// by generated code using Duration.)
func TimestampUTC(count int64, u TimeUnit) time.Time {
	switch u {
	case UnitDay:
		return time.Unix(count*86400, 0).UTC()
	case UnitSecond:
		return time.Unix(count, 0).UTC()
	case UnitMillisecond:
		return time.Unix(0, count*int64(time.Millisecond)).UTC()
	case UnitMicrosecond:
		return time.Unix(0, count*int64(time.Microsecond)).UTC()
	default: // UnitNanosecond
		return time.Unix(0, count).UTC()
	}
}

// Duration returns the time.Duration represented by count units of u. For
// UnitDay this is count*24h.
func (u TimeUnit) Duration(count int64) time.Duration {
	switch u {
	case UnitDay:
		return time.Duration(count) * 24 * time.Hour
	case UnitSecond:
		return time.Duration(count) * time.Second
	case UnitMillisecond:
		return time.Duration(count) * time.Millisecond
	case UnitMicrosecond:
		return time.Duration(count) * time.Microsecond
	default:
		return time.Duration(count)
	}
}
