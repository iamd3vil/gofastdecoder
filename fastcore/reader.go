// Package fastcore is the runtime library for decoding FAST 1.1/1.2 streams.
//
// It provides the wire-level primitives (stop-bit entities, presence map,
// nullable representations) and the field-operator state machine that generated
// decoders call. It knows nothing about specific templates.
//
// Wire encoding (FAST 1.1 §10): integers are big-endian stop-bit entities — the
// high bit (0x80) of each byte marks the final byte; the low 7 bits are data.
// Nullable integers shift non-negative values by +1, reserving the all-zero
// entity (byte 0x80) for NULL.
package fastcore

import "errors"

// ErrEndOfStream is returned when a read runs past the end of the buffer.
var ErrEndOfStream = errors.New("fastcore: unexpected end of stream")

// ErrOverflow is returned when a stop-bit entity has more bytes than the target
// integer width can hold.
var ErrOverflow = errors.New("fastcore: integer entity overflow")

// Reader consumes a FAST stream from an in-memory buffer. It is reset between
// messages with Reset and never allocates while reading scalars; string and
// byte-vector reads return sub-slices of the input (zero-copy).
type Reader struct {
	buf     []byte
	pos     int
	scratch []byte // reused by ASCII reads for decoded (stop-bit-stripped) bytes
}

// NewReader returns a Reader over buf.
func NewReader(buf []byte) *Reader { return &Reader{buf: buf} }

// Reset points the Reader at buf and rewinds to the start.
func (r *Reader) Reset(buf []byte) {
	r.buf = buf
	r.pos = 0
}

// Pos reports the current byte offset, for diagnostics and tests.
func (r *Reader) Pos() int { return r.pos }

// Remaining reports how many bytes are left unread.
func (r *Reader) Remaining() int { return len(r.buf) - r.pos }

// readEntityUnsigned decodes one stop-bit entity as an unsigned integer.
// It enforces a ceiling of 64 significant bits (10 groups of 7 bits, with the
// 10th group only contributing its lowest bit) — anything wider overflows.
func (r *Reader) readEntityUnsigned() (uint64, error) {
	buf, pos := r.buf, r.pos
	if pos >= len(buf) {
		return 0, ErrEndOfStream
	}
	b := buf[pos]
	pos++
	if b&0x80 != 0 {
		r.pos = pos
		return uint64(b & 0x7f), nil
	}

	// Groups 2..9 accumulate at most 63 bits and cannot overflow, so this loop
	// carries no overflow checks; only the rare 10th-plus group needs them.
	val := uint64(b & 0x7f)
	end := min(pos+8, len(buf))
	for pos < end {
		b = buf[pos]
		pos++
		val = (val << 7) | uint64(b&0x7f)
		if b&0x80 != 0 {
			r.pos = pos
			return val, nil
		}
	}
	groups := 9
	for pos < len(buf) {
		b = buf[pos]
		pos++
		groups++
		if groups > 10 || val > (^uint64(0))>>7 {
			r.pos = pos
			return 0, ErrOverflow
		}
		val = (val << 7) | uint64(b&0x7f)
		if b&0x80 != 0 {
			r.pos = pos
			return val, nil
		}
	}
	r.pos = pos
	return 0, ErrEndOfStream
}

// readEntitySigned decodes one stop-bit entity as a two's-complement signed
// integer. The most significant data bit of the first byte is the sign bit
// (§10.6.1.1), so the accumulator is seeded to all-ones when it is set.
func (r *Reader) readEntitySigned() (int64, error) {
	buf, pos := r.buf, r.pos
	if pos >= len(buf) {
		return 0, ErrEndOfStream
	}
	b := buf[pos]
	pos++
	// Sign-extend the 7 data bits of the first group: shift them to the top of
	// a byte (dropping the stop bit), then arithmetic-shift back down.
	val := int64(int8(b<<1)) >> 1
	if b&0x80 != 0 {
		r.pos = pos
		return val, nil
	}

	// Groups 2..9 grow the magnitude to at most 62 bits and cannot overflow,
	// so this loop carries no overflow checks; only the rare 10th-plus group
	// needs them.
	end := min(pos+8, len(buf))
	for pos < end {
		b = buf[pos]
		pos++
		val = (val << 7) | int64(b&0x7f)
		if b&0x80 != 0 {
			r.pos = pos
			return val, nil
		}
	}
	groups := 9
	for pos < len(buf) {
		b = buf[pos]
		pos++
		groups++
		if groups > 10 {
			r.pos = pos
			return 0, ErrOverflow
		}
		shifted := val << 7
		if shifted>>7 != val { // significant bits lost -> would not fit int64
			r.pos = pos
			return 0, ErrOverflow
		}
		val = shifted | int64(b&0x7f)
		if b&0x80 != 0 {
			r.pos = pos
			return val, nil
		}
	}
	r.pos = pos
	return 0, ErrEndOfStream
}

// ReadUint reads a mandatory unsigned integer field (§10.6.1.2).
func (r *Reader) ReadUint() (uint64, error) { return r.readEntityUnsigned() }

// ReadInt reads a mandatory signed integer field (§10.6.1.1).
func (r *Reader) ReadInt() (int64, error) { return r.readEntitySigned() }

// ReadNullableUint reads an optional unsigned integer field. The all-zero
// entity is NULL; otherwise the transmitted value is decremented by 1 to undo
// the nullable shift (§10.6.1).
func (r *Reader) ReadNullableUint() (val uint64, null bool, err error) {
	e, err := r.readEntityUnsigned()
	if err != nil {
		return 0, false, err
	}
	if e == 0 {
		return 0, true, nil
	}
	return e - 1, false, nil
}

// ReadNullableInt reads an optional signed integer field. The all-zero entity
// is NULL; positive entities (which encoded a non-negative value) are
// decremented by 1, negatives are left unchanged (§10.6.1).
func (r *Reader) ReadNullableInt() (val int64, null bool, err error) {
	e, err := r.readEntitySigned()
	if err != nil {
		return 0, false, err
	}
	if e == 0 {
		return 0, true, nil
	}
	if e > 0 {
		return e - 1, false, nil
	}
	return e, false, nil
}
