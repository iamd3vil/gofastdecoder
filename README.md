# gofastdecoder

A Go decoder for the **FAST** protocol (FIX Adapted for STreaming) — the full
**FAST 1.1** base plus the **FAST 1.2** extension types. XML templates are
compiled to Go source by a code generator; the generated decoders call a small
runtime core on a zero-allocation hot path.

Decode-only (no encoder).

## Layout

| Package | Role |
|---|---|
| `fastcore` | Runtime library: stop-bit transport, presence map, the field-operator state machine, and the FAST 1.2 primitives. Knows nothing about specific templates. |
| `fastgen` | Code generator. `parser` parses template XML into `ast`; `gen` emits Go source; the `fastgen` command wires them for `go:generate`. |
| `vectors` | Test harness: loads a language-neutral decode corpus and drives any `fastcore`-style decoder. |
| `testdata` | mFAST templates (verbatim, BSD-3-Clause) and the transcribed vector corpus. |
| `examples` | Generated decoders + round-trip and benchmark tests. |

`fastcore` owns all primitive reads and operator state; `fastgen` emits one
struct + decoder per template that issues a flat sequence of `fastcore` calls —
no reflection or per-field interface dispatch at runtime.

## Usage

Generate a decoder from a template file:

```go
//go:generate go run github.com/iamd3vil/gofastdecoder/fastgen -in templates.xml -out msgs_gen.go -pkg myfeed
```

Decode messages (the decoder is reusable; its dictionary state persists across
messages, which is what the copy/increment/delta/tail operators rely on):

```go
var dec myfeed.QuoteDecoder
r := fastcore.NewReader(frame)      // or r.Reset(frame) to reuse
var m myfeed.Quote
if err := dec.Decode(r, &m); err != nil { /* ... */ }
```

The decode path is allocation-free once buffers are warm (enforced by
`examples/simple.TestZeroAlloc`).

## Coverage

**Implemented and tested**

- Transport (§10): stop-bit unsigned/signed integers, nullable representations,
  presence map, ASCII/Unicode strings, byte vectors.
- All field operators (§6.3) — none, constant, default, copy, increment, delta,
  tail — with full previous-value state (undefined/empty/assigned) and the
  D5/D6/D7 dynamic errors. Verified by a 53-case corpus transcribed from mFAST
  covering uint64, decimal, ascii, and unicode.
- Code generation for scalar fields of every base type plus the 1.2 types
  (boolean, timestamp, enum, set), decimals with individual exponent/mantissa
  operators, bit groups, sequences, and groups. 22 of the 25 mFAST fixtures
  generate valid Go (the other 3 use the unsupported constructs below).
- FAST 1.2 primitives: bit-group bit reader, timestamp→`time.Time`, boolean.
- A generated `Router` that reads the message presence map, decodes the
  template id (global-dictionary copy on PMAP bit 0), and dispatches to the
  matching template, returning a `Message` the caller type-switches on.
- Static `<templateRef name="T"/>`: the referenced template is inlined,
  including references whose target is defined in a different file (pass each
  file with a repeated `-in`; `parser.ParseFiles` merges and resolves them).

**Not yet implemented**

- Dynamic `<templateRef/>` (template id carried in the stream — nested dynamic
  dispatch). Reported by the emitter rather than silently dropped.
- Bit groups with an operator (e.g. `copy`) or optional sub-fields.
- An encoder.

See `PLAN.md` for the full build sequence and status.

## Specs

- `FAST-Specification-1-x-1.pdf` — FAST 1.1 base.
- `FAST-1-2-Extension-v10.pdf` — FAST 1.2 extensions.

## Licensing

MIT (see `LICENSE`). Test material derived from mFAST is BSD-3-Clause; see
`THIRD_PARTY_NOTICES.md`.
