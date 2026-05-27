package fastcore

import "errors"

// Operator identifies a FAST field operator (§6.3).
type Operator uint8

const (
	OpNone Operator = iota
	OpConstant
	OpDefault
	OpCopy
	OpIncrement
	OpDelta
	OpTail
)

// State is the state of a previous value held in a dictionary (§6.3.1).
type State uint8

const (
	// Undefined: no previous value has been established yet.
	Undefined State = iota
	// Empty: the previous value is absent (only reachable for optional fields).
	Empty
	// Assigned: a previous value is present.
	Assigned
)

// Dynamic and reportable errors from the operator semantics (§6.3, Appendix 4).
var (
	// ErrD5 — operator needs an initial value for an undefined mandatory field.
	ErrD5 = errors.New("fastcore: [ERR D5] missing initial value for undefined mandatory field")
	// ErrD6 — mandatory field resolved to empty, or delta on an empty previous value.
	ErrD6 = errors.New("fastcore: [ERR D6] mandatory field is empty")
	// ErrD7 — subtraction length exceeds the base length (delta), or empty tail on a mandatory field.
	ErrD7 = errors.New("fastcore: [ERR D7] subtraction/tail length exceeds base")
)

// IntWidth is the declared width of an integer field, used so the increment
// operator wraps at the type maximum rather than at 64 bits (§6.3.6).
type IntWidth uint8

const (
	W64 IntWidth = 64 // int64 / uInt64 (and timestamp); no wrap before 2^64
	W32 IntWidth = 32 // int32 / uInt32
)

// wrapUint wraps an incremented unsigned value to the field width.
func wrapUint(v uint64, w IntWidth) uint64 {
	if w == W32 {
		return v & 0xFFFFFFFF
	}
	return v
}

// wrapInt wraps an incremented signed value to the field width (int32 max+1
// becomes int32 min).
func wrapInt(v int64, w IntWidth) int64 {
	if w == W32 {
		return int64(int32(v))
	}
	return v
}

// UintSlot is a dictionary entry for an unsigned-integer field.
type UintSlot struct {
	State State
	Val   uint64
}

// IntSlot is a dictionary entry for a signed-integer field.
type IntSlot struct {
	State State
	Val   int64
}

// DecimalSlot is a dictionary entry for a decimal field, preserving the
// exponent/mantissa layout the delta operator needs (§6.3.7.2 note).
type DecimalSlot struct {
	State State
	Mant  int64
	Exp   int32
}

// BytesSlot is a dictionary entry for a string or byte-vector field. The stored
// slice is owned by the slot (copied on assignment) so it survives buffer reuse.
type BytesSlot struct {
	State State
	Val   []byte
}

// set copies b into the slot and marks it Assigned, reusing capacity.
func (s *BytesSlot) set(b []byte) {
	s.Val = append(s.Val[:0], b...)
	s.State = Assigned
}
