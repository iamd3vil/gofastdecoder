package fastcore

// Decimal (scaled number) operators (§6.3.7.2, §10.6.2). A scaled number is a
// signed exponent followed by a signed mantissa; its delta carries an exponent
// delta and a mantissa delta that are added to the base componentwise.

// DecodeDecimalDelta decodes a decimal field under the delta operator.
// When optional, a NULL exponent delta means the field is absent and the
// previous value is left untouched (§10.7.2).
func DecodeDecimalDelta(r *Reader, optional, hasInitial bool, initMant int64, initExp int32, slot *DecimalSlot) (mant int64, exp int32, present bool, err error) {
	var dExp int64
	if optional {
		var null bool
		dExp, null, err = r.ReadNullableInt()
		if err != nil || null {
			return 0, 0, false, err
		}
	} else {
		dExp, err = r.ReadInt()
		if err != nil {
			return 0, 0, false, err
		}
	}
	dMant, err := r.ReadInt() // mantissa delta is always non-nullable here
	if err != nil {
		return 0, 0, false, err
	}

	baseMant, baseExp, err := decimalDeltaBase(slot, hasInitial, initMant, initExp)
	if err != nil {
		return 0, 0, false, err
	}
	mant = baseMant + dMant
	exp = baseExp + int32(dExp)
	slot.State, slot.Mant, slot.Exp = Assigned, mant, exp
	return mant, exp, true, nil
}

// decimalDeltaBase resolves the base value for a decimal delta. The default
// base is mantissa 0, exponent 0 (§6.3.7.2).
func decimalDeltaBase(slot *DecimalSlot, hasInitial bool, initMant int64, initExp int32) (mant int64, exp int32, err error) {
	switch slot.State {
	case Assigned:
		return slot.Mant, slot.Exp, nil
	case Empty:
		return 0, 0, ErrD6
	default: // Undefined
		if hasInitial {
			return initMant, initExp, nil
		}
		return 0, 0, nil
	}
}
