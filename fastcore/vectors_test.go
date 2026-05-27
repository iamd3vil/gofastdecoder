package fastcore_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/iamd3vil/gofastdecoder/fastcore"
	"github.com/iamd3vil/gofastdecoder/vectors"
)

// corpusDecoder adapts fastcore to the vectors.Decoder contract: it builds a
// field instruction from a vector, seeds the previous value, reads the presence
// map then the field, and reports the outcome.
type corpusDecoder struct{}

func (corpusDecoder) Decode(v vectors.OperatorVector) (vectors.Outcome, error) {
	raw, err := v.Bytes()
	if err != nil {
		return vectors.Outcome{}, err
	}
	r := fastcore.NewReader(raw)
	pm, err := r.ReadPMAP(nil)
	if err != nil {
		return vectors.Outcome{}, err
	}
	optional := v.Presence == "optional"
	op, err := operator(v.Operator)
	if err != nil {
		return vectors.Outcome{}, err
	}

	switch v.Type {
	case "uint64":
		return decodeUint(r, &pm, v, op, optional)
	case "decimal":
		return decodeDecimal(r, &pm, v, op, optional)
	case "ascii", "unicode":
		return decodeString(r, &pm, v, op, optional)
	default:
		return vectors.Outcome{}, fmt.Errorf("unsupported type %q", v.Type)
	}
}

func decodeUint(r *fastcore.Reader, pm *fastcore.PMAP, v vectors.OperatorVector, op fastcore.Operator, optional bool) (vectors.Outcome, error) {
	var slot fastcore.UintSlot
	if err := seedUint(&slot, v); err != nil {
		return vectors.Outcome{}, err
	}
	hasInitial, initial, err := uintInitial(v)
	if err != nil {
		return vectors.Outcome{}, err
	}
	val, present, err := fastcore.DecodeUint(r, pm, op, fastcore.W64, optional, hasInitial, initial, &slot)
	if err != nil {
		return vectors.Outcome{}, err
	}
	return uintOutcome(val, present, pm), nil
}

func decodeDecimal(r *fastcore.Reader, pm *fastcore.PMAP, v vectors.OperatorVector, op fastcore.Operator, optional bool) (vectors.Outcome, error) {
	if op != fastcore.OpDelta {
		return vectors.Outcome{}, fmt.Errorf("decimal operator %q not yet supported", v.Operator)
	}
	var slot fastcore.DecimalSlot
	var mant int64
	var exp int32
	hasInitial := v.Initial != nil
	if hasInitial {
		m, e, err := decimalValue(v.Initial)
		if err != nil {
			return vectors.Outcome{}, err
		}
		mant, exp = m, e
	}
	gotM, gotE, present, err := fastcore.DecodeDecimalDelta(r, optional, hasInitial, mant, exp, &slot)
	if err != nil {
		return vectors.Outcome{}, err
	}
	out := vectors.Outcome{Absent: !present, PmapBitsConsumed: pm.BitsConsumed()}
	if present {
		out.Value = &vectors.Value{Decimal: &vectors.Decimal{Mantissa: strconv.FormatInt(gotM, 10), Exponent: int(gotE)}}
	}
	return out, nil
}

func decodeString(r *fastcore.Reader, pm *fastcore.PMAP, v vectors.OperatorVector, op fastcore.Operator, optional bool) (vectors.Outcome, error) {
	var slot fastcore.BytesSlot
	if v.PrevState == "empty" {
		slot.State = fastcore.Empty
	} else if v.PrevValue != nil {
		slot.State = fastcore.Assigned
		slot.Val = []byte(*v.PrevValue.Scalar)
	}
	hasInitial := v.Initial != nil
	var initial []byte
	if hasInitial {
		initial = []byte(*v.Initial.Scalar)
	}

	var val []byte
	var present bool
	var err error
	switch {
	case op == fastcore.OpDelta && v.Type == "ascii":
		val, present, err = fastcore.DecodeASCIIDelta(r, optional, hasInitial, initial, &slot)
	case op == fastcore.OpDelta && v.Type == "unicode":
		val, present, err = fastcore.DecodeUnicodeDelta(r, optional, hasInitial, initial, &slot)
	case op == fastcore.OpTail && v.Type == "ascii":
		val, present, err = fastcore.DecodeASCIITail(r, pm, optional, hasInitial, initial, &slot)
	default:
		return vectors.Outcome{}, fmt.Errorf("%s operator %q not yet supported", v.Type, v.Operator)
	}
	if err != nil {
		return vectors.Outcome{}, err
	}
	out := vectors.Outcome{Absent: !present, PmapBitsConsumed: pm.BitsConsumed()}
	if present {
		s := string(val)
		out.Value = &vectors.Value{Scalar: &s}
	}
	return out, nil
}

// --- helpers ---

func operator(s string) (fastcore.Operator, error) {
	switch s {
	case "none":
		return fastcore.OpNone, nil
	case "constant":
		return fastcore.OpConstant, nil
	case "default":
		return fastcore.OpDefault, nil
	case "copy":
		return fastcore.OpCopy, nil
	case "increment":
		return fastcore.OpIncrement, nil
	case "delta":
		return fastcore.OpDelta, nil
	case "tail":
		return fastcore.OpTail, nil
	}
	return 0, fmt.Errorf("unknown operator %q", s)
}

func uintInitial(v vectors.OperatorVector) (bool, uint64, error) {
	if v.Initial == nil {
		return false, 0, nil
	}
	n, err := strconv.ParseUint(*v.Initial.Scalar, 10, 64)
	return true, n, err
}

func seedUint(slot *fastcore.UintSlot, v vectors.OperatorVector) error {
	switch {
	case v.PrevState == "empty":
		slot.State = fastcore.Empty
	case v.PrevValue != nil:
		n, err := strconv.ParseUint(*v.PrevValue.Scalar, 10, 64)
		if err != nil {
			return err
		}
		slot.State, slot.Val = fastcore.Assigned, n
	}
	return nil
}

func uintOutcome(val uint64, present bool, pm *fastcore.PMAP) vectors.Outcome {
	out := vectors.Outcome{Absent: !present, PmapBitsConsumed: pm.BitsConsumed()}
	if present {
		s := strconv.FormatUint(val, 10)
		out.Value = &vectors.Value{Scalar: &s}
	}
	return out
}

func decimalValue(v *vectors.Value) (int64, int32, error) {
	if v.Decimal == nil {
		return 0, 0, fmt.Errorf("expected decimal value")
	}
	m, err := strconv.ParseInt(v.Decimal.Mantissa, 10, 64)
	return m, int32(v.Decimal.Exponent), err
}

// TestOperatorCorpus runs the full mFAST-derived operator corpus through fastcore.
func TestOperatorCorpus(t *testing.T) {
	f, err := vectors.LoadOperatorFile(filepath.Clean("../testdata/vectors/operator_decode.json"))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	vectors.Run(t, f, corpusDecoder{})
}
