// Package vectors loads the language-neutral FAST decode test corpus from
// testdata/vectors and runs it against an implementation of the decoder.
//
// The corpus is transcribed from objectcomputing/mFAST (BSD-3-Clause); see
// THIRD_PARTY_NOTICES.md at the repo root. The Go types here decouple the
// fastcore/fastgen implementation from the on-disk format, so the vectors stay
// valid even as the decoder API evolves.
package vectors

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
)

// OperatorFile is the top-level shape of operator_decode.json.
type OperatorFile struct {
	Source  string           `json:"source"`
	Format  string           `json:"format"`
	Vectors []OperatorVector `json:"vectors"`
}

// OperatorVector is one decode case for a single field with a single operator.
//
// The wire bytes in Input begin with the presence-map byte(s); a runner is
// expected to decode the PMAP first (exactly as mFAST's decode_mref does) and
// then decode the field, asserting Expect and the pmap-bit consumption implied
// by HasPmapBit.
type OperatorVector struct {
	Name       string `json:"name"`
	Operator   string `json:"operator"`   // none|constant|default|copy|increment|delta|tail
	Type       string `json:"type"`       // uint64|decimal|ascii|unicode (extendable)
	Presence   string `json:"presence"`   // mandatory|optional
	Initial    *Value `json:"initial"`    // initial value from the template, or null
	PrevState  string `json:"prevState"`  // ""|undefined|empty ; mutually exclusive with PrevValue
	PrevValue  *Value `json:"prevValue"`  // assigned previous value, if any
	Input      string `json:"input"`      // hex of the wire bytes, pmap byte first
	HasPmapBit bool   `json:"hasPmapBit"` // whether the field consumes a pmap bit
	Expect     Expect `json:"expect"`
	PrevAfter  string `json:"prevAfter"` // change|preserve
}

// Bytes decodes the Input hex into raw wire bytes.
func (v OperatorVector) Bytes() ([]byte, error) {
	b, err := hex.DecodeString(v.Input)
	if err != nil {
		return nil, fmt.Errorf("vector %q: bad input hex %q: %w", v.Name, v.Input, err)
	}
	return b, nil
}

// Expect is the asserted outcome of decoding a vector.
type Expect struct {
	Value  *Value `json:"value"`  // present value, if the field decodes to a value
	Absent bool   `json:"absent"` // field is absent (optional null / empty)
	Error  bool   `json:"error"`  // decoding must report a dynamic error
}

// Value is a polymorphic FAST value: an integer/string scalar or a decimal.
// Exactly one representation is populated by the JSON decoder.
type Value struct {
	// Scalar holds a uint64/int as a base-10 string (uint64 can exceed int64),
	// or an ascii/unicode string literal, depending on the vector's Type.
	Scalar *string
	// Decimal holds a scaled number when set.
	Decimal *Decimal
}

// Decimal is a FAST scaled number: Mantissa * 10^Exponent. Mantissa is a
// base-10 string to preserve full int64 range without ambiguity.
type Decimal struct {
	Mantissa string `json:"mantissa"`
	Exponent int    `json:"exponent"`
}

// UnmarshalJSON accepts either a JSON string (scalar) or an object {mantissa,exponent}.
func (val *Value) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		val.Scalar = &s
		return nil
	}
	var d Decimal
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("value is neither a string scalar nor a {mantissa,exponent} decimal: %w", err)
	}
	val.Decimal = &d
	return nil
}

// LoadOperatorFile reads and parses an operator vector file.
func LoadOperatorFile(path string) (*OperatorFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f OperatorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// Validate checks the corpus is internally well-formed: unique names, known
// enum values, decodable hex, and consistent expectations. It does not require
// a decoder, so it can guard the corpus on its own.
func (f *OperatorFile) Validate() error {
	seen := make(map[string]bool, len(f.Vectors))
	for i, v := range f.Vectors {
		if v.Name == "" {
			return fmt.Errorf("vector[%d]: empty name", i)
		}
		if seen[v.Name] {
			return fmt.Errorf("duplicate vector name %q", v.Name)
		}
		seen[v.Name] = true

		if !knownOperator[v.Operator] {
			return fmt.Errorf("vector %q: unknown operator %q", v.Name, v.Operator)
		}
		if v.Presence != "mandatory" && v.Presence != "optional" {
			return fmt.Errorf("vector %q: bad presence %q", v.Name, v.Presence)
		}
		if v.PrevState != "" && v.PrevState != "undefined" && v.PrevState != "empty" {
			return fmt.Errorf("vector %q: bad prevState %q", v.Name, v.PrevState)
		}
		if v.PrevState != "" && v.PrevValue != nil {
			return fmt.Errorf("vector %q: prevState and prevValue are mutually exclusive", v.Name)
		}
		if v.PrevAfter != "change" && v.PrevAfter != "preserve" {
			return fmt.Errorf("vector %q: bad prevAfter %q", v.Name, v.PrevAfter)
		}
		if _, err := v.Bytes(); err != nil {
			return err
		}
		// Exactly one expectation kind.
		kinds := 0
		if v.Expect.Value != nil {
			kinds++
		}
		if v.Expect.Absent {
			kinds++
		}
		if v.Expect.Error {
			kinds++
		}
		if kinds != 1 {
			return fmt.Errorf("vector %q: expect must set exactly one of value/absent/error (got %d)", v.Name, kinds)
		}
		// A decimal type implies decimal-shaped values where present.
		if err := checkValueShape(v.Name, v.Type, v.Initial); err != nil {
			return err
		}
		if err := checkValueShape(v.Name, v.Type, v.Expect.Value); err != nil {
			return err
		}
	}
	return nil
}

var knownOperator = map[string]bool{
	"none": true, "constant": true, "default": true,
	"copy": true, "increment": true, "delta": true, "tail": true,
}

func checkValueShape(name, typ string, v *Value) error {
	if v == nil {
		return nil
	}
	switch typ {
	case "decimal":
		if v.Decimal == nil {
			return fmt.Errorf("vector %q: decimal type requires {mantissa,exponent} value", name)
		}
		if _, ok := new(big.Int).SetString(v.Decimal.Mantissa, 10); !ok {
			return fmt.Errorf("vector %q: bad decimal mantissa %q", name, v.Decimal.Mantissa)
		}
	case "uint64":
		if v.Scalar == nil {
			return fmt.Errorf("vector %q: uint64 type requires a string scalar value", name)
		}
		if _, ok := new(big.Int).SetString(*v.Scalar, 10); !ok {
			return fmt.Errorf("vector %q: bad integer scalar %q", name, *v.Scalar)
		}
	case "ascii", "unicode":
		if v.Scalar == nil {
			return fmt.Errorf("vector %q: %s type requires a string scalar value", name, typ)
		}
	default:
		return fmt.Errorf("vector %q: unknown type %q", name, typ)
	}
	return nil
}
