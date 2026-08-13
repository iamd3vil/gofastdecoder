package fastcore

// Signed-integer field operators (§6.3), mirroring DecodeUint. Used for int32/
// int64 fields and as the underlying representation of timestamp fields, whose
// value is a signed count of time units since an epoch (FAST 1.2).

// DecodeInt decodes a signed integer field under op. width wraps the increment
// operator at the type maximum (§6.3.6). Generated code calls the per-operator
// functions directly; this dispatcher remains for callers with a runtime op.
func DecodeInt(r *Reader, pm *PMAP, op Operator, width IntWidth, optional, hasInitial bool, initial int64, slot *IntSlot) (val int64, present bool, err error) {
	switch op {
	case OpNone:
		return DecodeIntNone(r, optional)

	case OpConstant:
		return DecodeIntConstant(pm, optional, initial)

	case OpDefault:
		return DecodeIntDefault(r, pm, optional, hasInitial, initial)

	case OpCopy:
		return DecodeIntCopy(r, pm, optional, hasInitial, initial, slot)

	case OpIncrement:
		return DecodeIntIncrement(r, pm, width, optional, hasInitial, initial, slot)

	case OpDelta:
		return DecodeIntDelta(r, optional, hasInitial, initial, slot)
	}
	return 0, false, errUnsupportedOperator(op)
}

func DecodeIntConstant(pm *PMAP, optional bool, initial int64) (int64, bool, error) {
	if !optional || pm.Next() {
		return initial, true, nil
	}
	return 0, false, nil
}

func DecodeIntNone(r *Reader, optional bool) (int64, bool, error) {
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

func DecodeIntDefault(r *Reader, pm *PMAP, optional, hasInitial bool, initial int64) (int64, bool, error) {
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
}

func DecodeIntCopy(r *Reader, pm *PMAP, optional, hasInitial bool, initial int64, slot *IntSlot) (int64, bool, error) {
	if pm.Next() {
		return readIntPrevious(r, optional, slot)
	}
	return resolveIntPrevious(optional, hasInitial, initial, slot)
}

func DecodeIntIncrement(r *Reader, pm *PMAP, width IntWidth, optional, hasInitial bool, initial int64, slot *IntSlot) (int64, bool, error) {
	if pm.Next() {
		return readIntPrevious(r, optional, slot)
	}
	if slot.State == Assigned {
		slot.Val = wrapInt(slot.Val+1, width)
		return slot.Val, true, nil
	}
	return resolveIntPrevious(optional, hasInitial, initial, slot)
}

func DecodeIntDelta(r *Reader, optional, hasInitial bool, initial int64, slot *IntSlot) (int64, bool, error) {
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

func readIntPrevious(r *Reader, optional bool, slot *IntSlot) (int64, bool, error) {
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

func resolveIntPrevious(optional, hasInitial bool, initial int64, slot *IntSlot) (int64, bool, error) {
	switch slot.State {
	case Assigned:
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
