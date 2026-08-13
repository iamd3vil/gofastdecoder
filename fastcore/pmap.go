package fastcore

// PMAP is a decoded presence map: a stop-bit entity whose 7-bit data groups
// form a sequence of bits consumed in field-entry order, most-significant bit
// first (§10.5). A presence map logically has an infinite suffix of zeroes, so
// reading past the transmitted bits yields false rather than an error.
//
// The first nine groups (63 bits — more than any realistic template needs) are
// packed into a single uint64 shift register so Next is a shift and a test;
// longer maps spill the remaining groups into ext and are loaded on demand.
type PMAP struct {
	bits   uint64 // unread bits, most significant first (next bit is bit 63)
	n      int    // number of unread bits currently in bits
	ext    []byte // 7-bit groups beyond the first nine, loaded as bits drains
	extPos int    // next ext group to load
	cursor int    // total bits consumed via Next
}

// ReadPMAP reads a presence-map entity from the stream. dst is retained as
// scratch for maps too long for the shift register (reused across messages to
// avoid allocation); retrieve it with Buffer.
func (r *Reader) ReadPMAP(dst []byte) (PMAP, error) {
	buf, pos := r.buf, r.pos
	if pos < len(buf) {
		if b := buf[pos]; b&0x80 != 0 { // single-group map
			r.pos = pos + 1
			return PMAP{bits: uint64(b&0x7f) << 57, n: 7, ext: dst[:0]}, nil
		}
	}

	var bits uint64
	n := 0
	dst = dst[:0]
	for {
		if pos >= len(buf) {
			r.pos = pos
			return PMAP{}, ErrEndOfStream
		}
		b := buf[pos]
		pos++
		if n < 63 {
			bits |= uint64(b&0x7f) << (57 - n)
			n += 7
		} else {
			dst = append(dst, b&0x7f)
		}
		if b&0x80 != 0 {
			break
		}
	}
	r.pos = pos
	return PMAP{bits: bits, n: n, ext: dst}, nil
}

// Next returns the next presence-map bit, advancing the cursor. Bits beyond the
// transmitted length read as false.
func (p *PMAP) Next() bool {
	p.cursor++
	if p.n == 0 {
		if p.extPos >= len(p.ext) {
			return false
		}
		p.bits = uint64(p.ext[p.extPos]) << 57
		p.extPos++
		p.n = 7
	}
	bit := p.bits >> 63
	p.bits <<= 1
	p.n--
	return bit == 1
}

// BitsConsumed reports how many bits have been read via Next.
func (p *PMAP) BitsConsumed() int { return p.cursor }

// Buffer returns the scratch slice passed to ReadPMAP (possibly grown), so the
// caller can store it for reuse.
func (p *PMAP) Buffer() []byte { return p.ext }
