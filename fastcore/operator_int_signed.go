package fastcore

// Signed-integer field operators (§6.3), mirroring DecodeUint. Used for int32/
// int64 fields and as the underlying representation of timestamp fields, whose
// value is a signed count of time units since an epoch (FAST 1.2).

// DecodeInt decodes a signed integer field under op. width wraps the increment
// operator at the type maximum (§6.3.6).
func DecodeInt(r *Reader, pm *PMAP, op Operator, width IntWidth, optional, hasInitial bool, initial int64, slot *IntSlot) (val int64, present bool, err error) {
	switch op {
	case OpNone:
		if optional {
			v, null, err := r.ReadNullableInt()
			if err != nil || null {
				return 0, false, err
			}
			return v, true, nil
		}
		v, err := r.ReadInt()
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
		if pm.Next() {
			if optional {
				v, null, err := r.ReadNullableInt()
				if err != nil || null {
					return 0, false, err
				}
				return v, true, nil
			}
			v, err := r.ReadInt()
			return v, err == nil, err
		}
		if hasInitial {
			return initial, true, nil
		}
		if optional {
			return 0, false, nil
		}
		return 0, false, ErrD5

	case OpCopy, OpIncrement:
		if pm.Next() {
			if optional {
				v, null, err := r.ReadNullableInt()
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
			v, err := r.ReadInt()
			if err != nil {
				return 0, false, err
			}
			slot.State, slot.Val = Assigned, v
			return v, true, nil
		}
		switch slot.State {
		case Assigned:
			if op == OpIncrement {
				slot.Val = wrapInt(slot.Val+1, width)
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
			return 0, false, err
		}
		base, err := intDeltaBase(slot, hasInitial, initial)
		if err != nil {
			return 0, false, err
		}
		v := base + d
		slot.State, slot.Val = Assigned, v
		return v, true, nil
	}
	return 0, false, errUnsupportedOperator(op)
}

func intDeltaBase(slot *IntSlot, hasInitial bool, initial int64) (int64, error) {
	switch slot.State {
	case Assigned:
		return slot.Val, nil
	case Empty:
		return 0, ErrD6
	default:
		if hasInitial {
			return initial, nil
		}
		return 0, nil
	}
}
