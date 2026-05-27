# Decode test vectors

Language-neutral FAST decode test cases, consumed by the Go harness in
`/vectors`. Transcribed from mFAST (BSD-3-Clause) — see
[`/THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md).

## `operator_decode.json`

Each entry asserts the decoding of a **single field** with a **single field
operator**, isolating the FAST 1.1 operator + dictionary semantics (§6.3 of the
spec). The wire bytes begin with the presence-map byte(s); a runner decodes the
PMAP first, then the field — exactly as mFAST's `decode_mref` does.

### Schema (per vector)

| field        | type            | meaning |
|--------------|-----------------|---------|
| `name`       | string          | unique, human-readable case id |
| `operator`   | string          | `none`,`constant`,`default`,`copy`,`increment`,`delta`,`tail` |
| `type`       | string          | `uint64`,`decimal`,`ascii`,`unicode` (extendable) |
| `presence`   | string          | `mandatory` or `optional` |
| `initial`    | value\|null     | template initial value, or `null` if none |
| `prevState`  | string          | prior previous-value state: omitted/`undefined`, or `empty` |
| `prevValue`  | value           | assigned previous value (mutually exclusive with `prevState`) |
| `input`      | hex string      | wire bytes, **PMAP byte first** |
| `hasPmapBit` | bool            | whether the field consumes a presence-map bit |
| `expect`     | object          | exactly one of `value`, `absent:true`, `error:true` |
| `prevAfter`  | string          | `change` (previous value updated) or `preserve` |

### Value encoding

A **value** is either:
- a JSON string — for `uint64` (base-10, may exceed int64), `ascii`, `unicode`; or
- an object `{"mantissa": "<base-10>", "exponent": <int>}` — for `decimal`
  (= mantissa × 10^exponent).

### Notes on the wire encoding the vectors rely on

- Integers use stop-bit base-128; the high bit (`0x80`) marks the final byte.
- Optional integers/deltas use the nullable representation (non-negative values
  shifted +1; `0`/`0x80` is NULL).
- ASCII string delta carries a signed *subtraction length*: non-negative removes
  from the **tail** and appends the delta; negative removes from the **front**
  (count = −len − 1) and prepends. Unicode delta is byte-length-prefixed.
- `tail` replaces the trailing characters of the base with the transmitted tail.

These are the exact behaviors the vectors encode; the `fastcore` implementation
is expected to reproduce them and is verified by `vectors.Run`.
