package fastcore

// ASCII strings are stop-bit entities whose data bytes are 7-bit characters
// (§10.6.3). A leading zero-preamble (a data byte of 0x00) distinguishes the
// empty string from NULL and guards against overlong encodings. Byte vectors
// (§10.6.5) and Unicode strings (§10.6.4, a UTF-8 byte vector) use a length
// preamble followed by raw 8-bit bytes.

// ErrOverlong is reported for an encoding that carries a redundant zero-preamble
// (a reportable error, §10.6.3 [ERR R9]).
var ErrOverlong = stringError("fastcore: overlong string encoding")

type stringError string

func (e stringError) Error() string { return string(e) }

// readASCIIBytes reads one ASCII entity and returns its decoded value bytes.
// The result is backed by r.scratch and only valid until the next ASCII read
// on r; callers that retain it must copy.
func (r *Reader) readASCIIBytes(nullable bool) (b []byte, null bool, err error) {
	buf, pos := r.buf, r.pos
	start := pos
	for {
		if pos >= len(buf) {
			r.pos = pos
			return nil, false, ErrEndOfStream
		}
		if buf[pos]&0x80 != 0 {
			pos++
			break
		}
		pos++
	}
	r.pos = pos
	// Only the final byte carries the stop bit, so copy and mask just that one.
	n := pos - start
	if cap(r.scratch) < n {
		r.scratch = make([]byte, n)
	}
	data := r.scratch[:n]
	copy(data, buf[start:pos])
	data[n-1] &= 0x7f
	return interpretASCII(data, nullable)
}

// ReadASCII reads a mandatory ASCII string.
func (r *Reader) ReadASCII() (string, error) {
	b, _, err := r.readASCIIBytes(false)
	return string(b), err
}

// ReadNullableASCII reads an optional ASCII string. A single all-zero entity
// (byte 0x80) is NULL.
func (r *Reader) ReadNullableASCII() (s string, null bool, err error) {
	b, null, err := r.readASCIIBytes(true)
	return string(b), null, err
}

// interpretASCII strips the zero-preamble from entity data bytes (§10.6.3),
// returning the value bytes. When nullable, a lone zero byte means NULL.
func interpretASCII(data []byte, nullable bool) (b []byte, null bool, err error) {
	if nullable {
		if len(data) == 1 && data[0] == 0 {
			return nil, true, nil // NULL
		}
		if data[0] == 0 {
			// A nullable zero-preamble is only legal when the value it guards
			// needs it: an empty string or a NUL-leading string. If the rest
			// starts with a non-zero byte, the preamble was unnecessary and the
			// encoding is overlong (§10.6.3 [ERR R9]).
			rest := data[1:]
			if rest[0] != 0 {
				return nil, false, ErrOverlong
			}
			data = rest
		}
		// else: no preamble — a plain non-nullable string follows.
	}
	if len(data) == 1 && data[0] == 0 {
		return data[:0], false, nil // empty string
	}
	if len(data) >= 2 && data[0] == 0 {
		// Zero-preamble followed by content: leading "\0" chars are real, but a
		// non-zero byte right after the preamble makes the encoding overlong.
		if data[1] != 0 {
			return nil, false, ErrOverlong
		}
		data = data[1:]
	}
	return data, false, nil
}

// ReadByteVector reads a mandatory byte vector: an unsigned length preamble
// followed by that many raw bytes (§10.6.5). The returned slice aliases the
// input buffer (zero-copy).
func (r *Reader) ReadByteVector() ([]byte, error) {
	n, err := r.readEntityUnsigned()
	if err != nil {
		return nil, err
	}
	return r.takeRaw(int(n))
}

// ReadNullableByteVector reads an optional byte vector; a NULL length preamble
// means NULL.
func (r *Reader) ReadNullableByteVector() (b []byte, null bool, err error) {
	n, isNull, err := r.ReadNullableUint()
	if err != nil {
		return nil, false, err
	}
	if isNull {
		return nil, true, nil
	}
	b, err = r.takeRaw(int(n))
	return b, false, err
}

func (r *Reader) takeRaw(n int) ([]byte, error) {
	if n < 0 || n > len(r.buf)-r.pos {
		return nil, ErrEndOfStream
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}
