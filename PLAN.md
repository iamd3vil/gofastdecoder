# gofastdecoder — implementation plan

A Go decoder for the **FAST** protocol: full **FAST 1.1** base plus the
**FAST 1.2** extensions. Decode-only (no encoder, for now). XML templates are
compiled to Go source by a code generator; the generated code calls a small
runtime core on a zero-allocation hot path.

## Specs

- `FAST-Specification-1-x-1.pdf` — FAST 1.1 base (transport, PMAP, operators,
  dictionaries, types, sequences/groups). The bulk of the work.
- `FAST-1-2-Extension-v10.pdf` — 1.2 extensions: named type defs
  (`<define>`/`<type>`/`<field>` + desugaring), `enum`, `set`, `bitGroup`,
  `timestamp`, `boolean`.

FAST 1.0 is **not** needed: 1.1 is the unified base everyone implements against,
and 1.2 extends 1.1.

## Architecture

```
fastcore/   runtime library — wire primitives + operator/dictionary state.
            Knows nothing about specific templates. Small, inlinable methods.
fastgen/    XML -> Go source generator (CLI, used via //go:generate).
            parser/ -> ast/ (desugar + named-type resolution) -> gen/ (emit).
vectors/    [DONE] test harness: loads the neutral vector corpus and drives a
            Decoder implementation (fastcore satisfies vectors.Decoder).
testdata/   [DONE] mFAST templates (verbatim) + transcribed vector corpus.
examples/   a real template set with //go:generate + decode/bench tests.
```

**Responsibility split (as requested):** `fastcore` owns all primitive reads
and operator state; `fastgen` reads XML and emits one struct + `Decode` method
per template/group/sequence, issuing a flat sequence of `fastcore` calls — no
reflection, no per-field interface dispatch at runtime.

## Zero-alloc strategy

- Decode in place into a caller-owned struct: `func (m *Msg) Decode(r *fastcore.Reader) error`.
- Strings/byteVectors: reusable scratch buffers on the `Reader`, plus a
  zero-copy accessor returning sub-slices of the input.
- Dictionaries preallocated per template; reset (not realloc) between messages.
- Sequences decode into caller-provided, grow-only slices (reset `len`).
- `go test -bench -benchmem` as a CI gate asserting `0 allocs/op` on the hot path.

## Test harness (already in place)

- `testdata/mfast/templates/*.xml` — 25 mFAST templates (verbatim, BSD-3-Clause).
- `testdata/mfast/upstream/*` — mFAST test sources kept for audit/reference.
- `testdata/vectors/operator_decode.json` — **53 operator vectors** transcribed
  from mFAST `decoder_operator_test.cpp`, covering every operator
  (none/constant/default/copy/increment/delta/tail) across uint64, decimal,
  ascii, and unicode, including the dynamic-error and prev-value-state cases.
- `vectors/` — Go loader, corpus validator (passes today), and `vectors.Run`,
  the contract `fastcore` will implement (`vectors.Decoder`).

This means the hardest layer (operators + dictionaries) has a spec-conformant
oracle *before* a line of `fastcore` is written.

## Build sequence (each step verifiable)

1. **[DONE] `fastcore` transport** — stop-bit reader, PMAP cursor, nullable
   int/uint/ascii/unicode/byteVector, decimal.
2. **[DONE] `fastcore` operators + dictionaries** — operator state machine wired
   to `vectors.Decoder`; the 53-vector corpus is green. **Correctness milestone.**
3. **[DONE] `fastgen` parser + AST** — parses 1.1 + 1.2 XML, applies the 1.2
   desugaring rule, resolves `<define>`/`<type>`; all 25 fixtures parse.
   _Gap:_ `<templateRef>`, `<view>`, and vendor types are skipped.
4. **[DONE] `fastgen` emitter** — generates structs + decoders for scalars,
   sequences, and groups (view-model + `text/template`); 24/25 fixtures emit
   valid Go; `examples/simple` round-trips and `go:generate` is reproducible.
5. **[DONE] 1.2 types** — `enum`, `set`, `boolean`, `timestamp` (→ `time.Time`)
   in both `fastcore` and `fastgen`; verified end-to-end by `examples/ext12`.
   _Remaining:_ transcribe mFAST feature vectors (`enum_*`/`set_*`/`timestamp_*`)
   for byte-level message verification; typed Go enums (currently `uint64`).
6. **[DONE] `bitGroup`** — parser maps `intN/uIntN` sub-types with widths;
   emitter flattens sub-fields and unpacks them via `fastcore.BitReader`
   (`examples/bitgroup`).
7. **[DONE] Structural decode** — sequences and groups; message-level
   template-id dispatch via a generated `Router` (`examples/simple`
   `TestRouterDispatch`). Static `<templateRef>` inlined (`examples/templateref`);
   decimal individual operators (`examples/decimalind`).
8. **[DONE] Example + benchmarks** — `0 allocs/op` gated by `TestZeroAlloc`.
9. **[DONE] Docs** — `README.md`.

### Remaining work
- Dynamic `<templateRef/>` (template id in the stream): nested dynamic dispatch.
- Bit groups with an operator (e.g. `copy`) or optional sub-fields.
- mFAST feature/structural vector transcription for byte-level verification.
- An encoder (if ever needed).

## Pending inputs (non-blocking)

- A real exchange `templates.xml` + captured packets for `testdata/` (turns the
  toy templates into real-world coverage; pins decimal-delta / string-delta edge
  cases). Until then, the mFAST corpus + spec Appendix-3 examples are the oracle.

## Open defaults (speak up to change)

- **Decode-only** (no encoder).
- **One FAST version per generated file.**
- Module path `github.com/iamd3vil/gofastdecoder`.
