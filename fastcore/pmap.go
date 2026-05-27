package fastcore

// PMAP is a decoded presence map: a stop-bit entity whose 7-bit data groups
// form a sequence of bits consumed in field-entry order, most-significant bit
// first (§10.5). A presence map logically has an infinite suffix of zeroes, so
// reading past the transmitted bits yields false rather than an error.
type PMAP struct {
	data    []byte // raw entity bytes (each contributes its low 7 bits)
	numBits int    // 7 * len(data)
	cursor  int    // next bit index to return
}

// ReadPMAP reads a presence-map entity from the stream into dst (reused across
// messages to avoid allocation) and returns a PMAP positioned at its first bit.
func (r *Reader) ReadPMAP(dst []byte) (PMAP, error) {
	dst = dst[:0]
	for {
		if r.pos >= len(r.buf) {
			return PMAP{}, ErrEndOfStream
		}
		b := r.buf[r.pos]
		r.pos++
		dst = append(dst, b&0x7f)
		if b&0x80 != 0 {
			break
		}
	}
	return PMAP{data: dst, numBits: len(dst) * 7}, nil
}

// Next returns the next presence-map bit, advancing the cursor. Bits beyond the
// transmitted length read as false.
func (p *PMAP) Next() bool {
	i := p.cursor
	p.cursor++
	if i >= p.numBits {
		return false
	}
	group := i / 7
	off := uint(6 - i%7) // bit 0 is the most significant data bit of group 0
	return p.data[group]>>off&1 == 1
}

// BitsConsumed reports how many bits have been read via Next.
func (p *PMAP) BitsConsumed() int { return p.cursor }

// Buffer returns the backing byte slice, so a caller that passed a reusable
// buffer to ReadPMAP can store the (possibly grown) slice for reuse.
func (p *PMAP) Buffer() []byte { return p.data }
