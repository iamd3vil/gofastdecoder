package fastcore

// String/byte-vector operators: delta (§6.3.7.3/.4) and tail (§6.3.8). These
// work on byte sequences — for ASCII the bytes are 7-bit characters, for
// Unicode/byte-vector they are raw 8-bit bytes. The combine logic is identical
// across them; only the wire representation of the appended part differs.

// stringBase resolves the base value for delta: assigned -> previous;
// undefined -> initial or the empty default; empty -> ERR D6.
func stringBase(slot *BytesSlot, hasInitial bool, initial []byte) ([]byte, error) {
	switch slot.State {
	case Assigned:
		return slot.Val, nil
	case Empty:
		return nil, ErrD6
	default:
		if hasInitial {
			return initial, nil
		}
		return nil, nil
	}
}

// applyStringDelta combines a subtraction length and an appended part with a
// base (§6.3.7.3): a non-negative subtraction removes from the back and appends
// to the back; a negative subtraction (excess-1 encoded) removes from the front
// and prepends to the front.
func applyStringDelta(base []byte, subLen int64, part []byte) ([]byte, error) {
	if subLen >= 0 {
		if subLen > int64(len(base)) {
			return nil, ErrD7
		}
		kept := base[:int64(len(base))-subLen]
		out := make([]byte, 0, len(kept)+len(part))
		out = append(out, kept...)
		out = append(out, part...)
		return out, nil
	}
	remove := -subLen - 1 // excess-1: -1 encodes "prepend, remove nothing"
	if remove > int64(len(base)) {
		return nil, ErrD7
	}
	kept := base[remove:]
	out := make([]byte, 0, len(part)+len(kept))
	out = append(out, part...)
	out = append(out, kept...)
	return out, nil
}

// DecodeASCIIDelta decodes an ASCII string field under the delta operator.
func DecodeASCIIDelta(r *Reader, optional, hasInitial bool, initial []byte, slot *BytesSlot) (val []byte, present bool, err error) {
	subLen, null, err := r.readDelta(optional)
	if err != nil || null {
		return nil, false, err // optional NULL: previous value untouched
	}
	part, err := r.ReadASCII()
	if err != nil {
		return nil, false, err
	}
	base, err := stringBase(slot, hasInitial, initial)
	if err != nil {
		return nil, false, err
	}
	val, err = applyStringDelta(base, subLen, []byte(part))
	if err != nil {
		return nil, false, err
	}
	slot.set(val)
	return val, true, nil
}

// DecodeUnicodeDelta decodes a Unicode string field under the delta operator.
// The appended part is a byte vector of UTF-8 bytes rather than an ASCII entity.
func DecodeUnicodeDelta(r *Reader, optional, hasInitial bool, initial []byte, slot *BytesSlot) (val []byte, present bool, err error) {
	subLen, null, err := r.readDelta(optional)
	if err != nil || null {
		return nil, false, err
	}
	part, err := r.ReadByteVector()
	if err != nil {
		return nil, false, err
	}
	base, err := stringBase(slot, hasInitial, initial)
	if err != nil {
		return nil, false, err
	}
	val, err = applyStringDelta(base, subLen, part)
	if err != nil {
		return nil, false, err
	}
	slot.set(val)
	return val, true, nil
}

// tailBase resolves the base value for the tail operator: assigned -> previous;
// undefined/empty -> initial or the empty default (§6.3.8).
func tailBase(slot *BytesSlot, hasInitial bool, initial []byte) []byte {
	if slot.State == Assigned {
		return slot.Val
	}
	if hasInitial {
		return initial
	}
	return nil
}

// applyTail combines a tail value with a base (§6.3.8.1): remove len(tail)
// bytes from the back of base and append tail; if the tail is longer than the
// base, the result is the tail.
func applyTail(base, tail []byte) []byte {
	if len(tail) >= len(base) {
		return append([]byte(nil), tail...)
	}
	kept := base[:len(base)-len(tail)]
	out := make([]byte, 0, len(base))
	out = append(out, kept...)
	out = append(out, tail...)
	return out
}

// DecodeASCIITail decodes an ASCII string field under the tail operator.
func DecodeASCIITail(r *Reader, pm *PMAP, optional, hasInitial bool, initial []byte, slot *BytesSlot) (val []byte, present bool, err error) {
	if pm.Next() { // tail value present in stream
		var part string
		if optional {
			s, null, err := r.ReadNullableASCII()
			if err != nil {
				return nil, false, err
			}
			if null {
				slot.State = Empty
				return nil, false, nil
			}
			part = s
		} else {
			s, err := r.ReadASCII()
			if err != nil {
				return nil, false, err
			}
			part = s
		}
		val = applyTail(tailBase(slot, hasInitial, initial), []byte(part))
		slot.set(val)
		return val, true, nil
	}
	// Tail value not present: resolve from the previous-value state.
	switch slot.State {
	case Assigned:
		return slot.Val, true, nil
	case Undefined:
		if hasInitial {
			slot.set(initial)
			return slot.Val, true, nil
		}
		if optional {
			slot.State = Empty
			return nil, false, nil
		}
		return nil, false, ErrD6
	default: // Empty
		if optional {
			return nil, false, nil
		}
		return nil, false, ErrD7
	}
}
