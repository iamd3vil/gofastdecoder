package fastcore

// Decimal (scaled number) operators (§6.3.7.2, §10.6.2). A scaled number is a
// signed exponent followed by a signed mantissa; its delta carries an exponent
// delta and a mantissa delta that are added to the base componentwise.

// Decimal is a scaled number: Mant * 10^Exp (§10.6.2). Generated code uses it
// as the Go representation of decimal fields.
type Decimal struct {
	Mant int64
	Exp  int32
}

// ReadScaledNumber reads a mandatory scaled number: a signed exponent followed
// by a signed mantissa (§10.6.2).
func (r *Reader) ReadScaledNumber() (Decimal, error) {
	exp, err := r.ReadInt()
	if err != nil {
		return Decimal{}, err
	}
	mant, err := r.ReadInt()
	if err != nil {
		return Decimal{}, err
	}
	return Decimal{Mant: mant, Exp: int32(exp)}, nil
}

// ReadNullableScaledNumber reads an optional scaled number; a NULL exponent
// means NULL and the mantissa is then absent (§10.6.2).
func (r *Reader) ReadNullableScaledNumber() (d Decimal, null bool, err error) {
	exp, null, err := r.ReadNullableInt()
	if err != nil || null {
		return Decimal{}, null, err
	}
	mant, err := r.ReadInt()
	if err != nil {
		return Decimal{}, false, err
	}
	return Decimal{Mant: mant, Exp: int32(exp)}, false, nil
}

// DecodeDecimal decodes a decimal field treated as a single value (no
// individual exponent/mantissa operators) under op. Delta is handled by
// DecodeDecimalDelta; this covers none/constant/default/copy.
func DecodeDecimal(r *Reader, pm *PMAP, op Operator, optional, hasInitial bool, initial Decimal, slot *DecimalSlot) (val Decimal, present bool, err error) {
	switch op {
	case OpNone:
		if optional {
			d, null, err := r.ReadNullableScaledNumber()
			if err != nil || null {
				return Decimal{}, false, err
			}
			return d, true, nil
		}
		d, err := r.ReadScaledNumber()
		return d, err == nil, err

	case OpConstant:
		if !optional || pm.Next() {
			return initial, true, nil
		}
		return Decimal{}, false, nil

	case OpDefault:
		if pm.Next() {
			if optional {
				d, null, err := r.ReadNullableScaledNumber()
				if err != nil || null {
					return Decimal{}, false, err
				}
				return d, true, nil
			}
			d, err := r.ReadScaledNumber()
			return d, err == nil, err
		}
		if hasInitial {
			return initial, true, nil
		}
		if optional {
			return Decimal{}, false, nil
		}
		return Decimal{}, false, ErrD5

	case OpCopy:
		if pm.Next() {
			if optional {
				d, null, err := r.ReadNullableScaledNumber()
				if err != nil {
					return Decimal{}, false, err
				}
				if null {
					slot.State = Empty
					return Decimal{}, false, nil
				}
				slot.State, slot.Mant, slot.Exp = Assigned, d.Mant, d.Exp
				return d, true, nil
			}
			d, err := r.ReadScaledNumber()
			if err != nil {
				return Decimal{}, false, err
			}
			slot.State, slot.Mant, slot.Exp = Assigned, d.Mant, d.Exp
			return d, true, nil
		}
		switch slot.State {
		case Assigned:
			return Decimal{Mant: slot.Mant, Exp: slot.Exp}, true, nil
		case Undefined:
			if hasInitial {
				slot.State, slot.Mant, slot.Exp = Assigned, initial.Mant, initial.Exp
				return initial, true, nil
			}
			if optional {
				slot.State = Empty
				return Decimal{}, false, nil
			}
			return Decimal{}, false, ErrD5
		default:
			if optional {
				return Decimal{}, false, nil
			}
			return Decimal{}, false, ErrD6
		}
	}
	return Decimal{}, false, errUnsupportedOperator(op)
}

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
