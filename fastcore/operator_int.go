package fastcore

// Unsigned-integer field operators (§6.3). DecodeUint applies one operator to
// an unsigned field, reading from r/pm and threading state through slot.
//
// Returns present=false when the field resolves to absent (optional NULL or
// empty previous value). hasInitial distinguishes "initial value 0" from "no
// initial value", which the copy/increment/delta/default rules treat very
// differently.
//
// Signed fields use DecodeInt with the same structure; the only differences are
// the wire read (signed two's complement) and increment wrap behavior.

// DecodeUint decodes an unsigned integer field under op. width is the field's
// declared integer width, used only to wrap the increment operator at the type
// maximum (§6.3.6).
func DecodeUint(r *Reader, pm *PMAP, op Operator, width IntWidth, optional, hasInitial bool, initial uint64, slot *UintSlot) (val uint64, present bool, err error) {
	switch op {
	case OpNone:
		if optional {
			v, null, err := r.ReadNullableUint()
			if err != nil || null {
				return 0, false, err
			}
			return v, true, nil
		}
		v, err := r.ReadUint()
		return v, err == nil, err

	case OpConstant:
		if !optional {
			return initial, true, nil
		}
		if pm.Next() {
			return initial, true, nil
		}
		return 0, false, nil

	case OpDefault:
		if pm.Next() { // value present in stream
			if optional {
				v, null, err := r.ReadNullableUint()
				if err != nil || null {
					return 0, false, err // NULL leaves previous value unchanged (§10.5.1)
				}
				return v, true, nil
			}
			v, err := r.ReadUint()
			return v, err == nil, err
		}
		// Not present: use the initial value.
		if hasInitial {
			return initial, true, nil
		}
		if optional {
			return 0, false, nil
		}
		return 0, false, ErrD5

	case OpCopy, OpIncrement:
		if pm.Next() { // value present -> becomes the new previous value
			if optional {
				v, null, err := r.ReadNullableUint()
				if err != nil {
					return 0, false, err
				}
				if null {
					slot.State = Empty
					return 0, false, nil
				}
				slot.State, slot.Val = Assigned, v
				return v, true, nil
			}
			v, err := r.ReadUint()
			if err != nil {
				return 0, false, err
			}
			slot.State, slot.Val = Assigned, v
			return v, true, nil
		}
		// Not present: resolve from previous-value state.
		switch slot.State {
		case Assigned:
			if op == OpIncrement {
				slot.Val = wrapUint(slot.Val+1, width)
			}
			return slot.Val, true, nil
		case Undefined:
			if hasInitial {
				slot.State, slot.Val = Assigned, initial
				return initial, true, nil
			}
			if optional {
				slot.State = Empty
				return 0, false, nil
			}
			return 0, false, ErrD5
		default: // Empty
			if optional {
				return 0, false, nil
			}
			return 0, false, ErrD6
		}

	case OpDelta:
		d, null, err := r.readDelta(optional)
		if err != nil || null {
			return 0, false, err // optional NULL: previous value left untouched
		}
		base, err := uintDeltaBase(slot, hasInitial, initial)
		if err != nil {
			return 0, false, err
		}
		v := uint64(int64(base) + d)
		slot.State, slot.Val = Assigned, v
		return v, true, nil
	}
	return 0, false, errUnsupportedOperator(op)
}

// uintDeltaBase resolves the base value for an unsigned delta (§6.3.7).
func uintDeltaBase(slot *UintSlot, hasInitial bool, initial uint64) (uint64, error) {
	switch slot.State {
	case Assigned:
		return slot.Val, nil
	case Empty:
		return 0, ErrD6
	default: // Undefined
		if hasInitial {
			return initial, nil
		}
		return 0, nil // type-dependent default base for integers is 0
	}
}

// readDelta reads an integer delta value (§10.7.1), nullable when optional.
func (r *Reader) readDelta(optional bool) (d int64, null bool, err error) {
	if optional {
		return r.ReadNullableInt()
	}
	d, err = r.ReadInt()
	return d, false, err
}

type unsupportedOperatorError Operator

func (e unsupportedOperatorError) Error() string {
	return "fastcore: operator not applicable to this field type"
}

func errUnsupportedOperator(op Operator) error { return unsupportedOperatorError(op) }
