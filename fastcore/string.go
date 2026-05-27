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

// readASCIIEntity collects one stop-bit entity and returns its data bytes
// (low 7 bits of each byte), without interpreting preambles.
func (r *Reader) readASCIIEntity() ([]byte, error) {
	start := r.pos
	for {
		if r.pos >= len(r.buf) {
			return nil, ErrEndOfStream
		}
		b := r.buf[r.pos]
		r.pos++
		if b&0x80 != 0 {
			break
		}
	}
	out := make([]byte, 0, r.pos-start)
	for i := start; i < r.pos; i++ {
		out = append(out, r.buf[i]&0x7f)
	}
	return out, nil
}

// ReadASCII reads a mandatory ASCII string. The returned slice is freshly
// allocated (callers that need to retain it across reads get a stable copy).
func (r *Reader) ReadASCII() (string, error) {
	data, err := r.readASCIIEntity()
	if err != nil {
		return "", err
	}
	s, _, err := interpretASCII(data, false)
	return s, err
}

// ReadNullableASCII reads an optional ASCII string. A single all-zero entity
// (byte 0x80) is NULL.
func (r *Reader) ReadNullableASCII() (s string, null bool, err error) {
	data, err := r.readASCIIEntity()
	if err != nil {
		return "", false, err
	}
	return interpretASCII(data, true)
}

// interpretASCII turns entity data bytes into a string, handling the
// zero-preamble rules (§10.6.3). When nullable, a leading zero-preamble that
// leaves nothing behind means NULL.
func interpretASCII(data []byte, nullable bool) (s string, null bool, err error) {
	if nullable {
		if len(data) == 1 && data[0] == 0 {
			return "", true, nil // NULL
		}
		if data[0] == 0 {
			// A nullable zero-preamble is only legal when the value it guards
			// needs it: an empty string or a NUL-leading string. If the rest
			// starts with a non-zero byte, the preamble was unnecessary and the
			// encoding is overlong (§10.6.3 [ERR R9]).
			rest := data[1:]
			if rest[0] != 0 {
				return "", false, ErrOverlong
			}
			data = rest
		}
		// else: no preamble — a plain non-nullable string follows.
	}
	if len(data) == 1 && data[0] == 0 {
		return "", false, nil // empty string
	}
	if len(data) >= 2 && data[0] == 0 {
		// Zero-preamble followed by content: leading "\0" chars are real, but a
		// non-zero byte right after the preamble makes the encoding overlong.
		if data[1] != 0 {
			return "", false, ErrOverlong
		}
		data = data[1:]
	}
	return string(data), false, nil
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
	if n < 0 || r.pos+n > len(r.buf) {
		return nil, ErrEndOfStream
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}
