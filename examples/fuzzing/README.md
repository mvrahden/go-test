# fuzzing — Wire Protocol Fuzzing, Crash to Regression Test

A message broker's binary frame codec — the thing every service that speaks a custom protocol has,
and the classic place adversarial input causes CVEs. Fuzzing a codec is not a toy; it is the single
most common real use of fuzzing in Go.

## Structure

- **frame.go** — `Frame`/`Header`/`TraceID`/`Kind`, `Encode`/`Decode`, and a `Codec` that layers a
  broker payload-size limit on top
- **topic.go** — `NormalizeTopic`, `TopicMatches`
- **suite_test.go** — `FrameCodecTestSuite`

## Targets

| Target | Feature |
|---|---|
| `TestNormalizeTopicTable` | `gotest.Each` table test; harvested into `f.Add` seeds for `FuzzNormalizeTopicIdempotent` |
| `TestEncodeDecodeExamples` | explicit round-trip examples |
| `FuzzFrameRoundTrip` | struct-typed target, typed `f.Add(Frame{...})` seed, **round-trip** property |
| `FuzzDecodeNeverPanics` | native `[]byte` target, **invariant** property with a real oracle: never panics, and anything that decodes must re-encode and decode again to the same value |
| `FuzzNormalizeTopicIdempotent` | native `string` target, **idempotence** property |
| `FuzzTopicMatches` | `gotest.Fuzz2` over two native strings, symmetry property |

`BeforeEach` rebuilds `s.codec` before every execution, not just before every top-level test — fuzz
targets replay `BeforeEach`/`AfterEach` around each individual execution, the same as any other test.

## From crash to regression test

`Frame` packed `Version` and `Kind` into a single byte to save space on the wire — a realistic
trick that looked safe because `Kind` only ever needs 3 bits today. Neither field was actually
bounded to fit its share of the byte, so a `Version >= 32` or a `Kind >= 8` silently corrupted the
other field, breaking `FuzzFrameRoundTrip`'s round-trip property. `gotest fuzz` found it in 11
executions, under a second:

```
$ go run ./cmd/gotest fuzz ./examples/fuzzing --for=60s
[FuzzFrameCodecTestSuite_FuzzTopicMatches] fuzz: elapsed: 0s, gathering baseline coverage: 0/145 completed
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] fuzz: elapsed: 0s, gathering baseline coverage: 0/1 completed
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] fuzz: elapsed: 0s, gathering baseline coverage: 1/1 completed, now fuzzing with 6 workers
[FuzzFrameCodecTestSuite_FuzzNormalizeTopicIdempotent] fuzz: elapsed: 0s, gathering baseline coverage: 0/78 completed
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] fuzz: elapsed: 0s, execs: 11 (425/sec), new interesting: 0 (total: 1)
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] --- FAIL: FuzzFrameCodecTestSuite_FuzzFrameRoundTrip (0.03s)
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]     --- FAIL: FuzzFrameCodecTestSuite_FuzzFrameRoundTrip (0.00s)
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]         suite_test.go:86: value.go:369: Equal failed:
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]               expected: fuzzing.Frame{Version:0x30, Kind:0x0, Topic:"", Headers:[]fuzzing.Header(nil), Payload:[]uint8(nil), Trace:(*fuzzing.TraceID)(nil)}
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]               actual:   fuzzing.Frame{Version:0x10, Kind:0x0, Topic:"", Headers:[]fuzzing.Header(nil), Payload:[]uint8(nil), Trace:(*fuzzing.TraceID)(nil)}
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]     Failing input written to testdata/fuzz/FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528ddfad69eb5
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]     To re-run:
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]     go test -run=FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528ddfad69eb5
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] FAIL
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] exit status 1
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] FAIL	github.com/mvrahden/go-test/examples/fuzzing	0.033s
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] new crasher: .../examples/fuzzing/testdata/fuzz/FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528ddfad69eb5
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] inspect it with `gotest fuzz triage`, then `gotest fuzz promote` to keep it as a typed seed
```

`gotest fuzz triage` decodes the crasher back through the generated codec and prints the actual
`Frame` value, not a raw byte blob:

```
$ go run ./cmd/gotest fuzz triage ./examples/fuzzing
FuzzFrameCodecTestSuite_FuzzFrameRoundTrip: 1 crasher
  file:  examples/fuzzing/testdata/fuzz/FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528ddfad69eb5
  input: Frame{Version: 48, Kind: Kind(0), Topic: "", Headers: nil, Payload: nil, Trace: nil}
  cause: --- FAIL: FuzzFrameCodecTestSuite_FuzzFrameRoundTrip (0.00s)
```

`gotest fuzz promote` splices that decoded value into the target as a typed `f.Add(...)` seed and
deletes the crasher file:

```
$ go run ./cmd/gotest fuzz promote ./examples/fuzzing
promoted FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528ddfad69eb5 -> f.Add(Frame{Version: 48, Kind: Kind(0), Topic: "", Headers: nil, Payload: nil, Trace: nil}) in examples/fuzzing/suite_test.go:83
```

`Version` and `Kind` were then given their own byte each, and `go run ./cmd/gotest ./examples/fuzzing -v`
went green — the promoted seed now replays as `FuzzFrameRoundTrip/seed#1` on every run, a permanent
regression test for the packed-byte bug. (The fuzzer reached the bug via `Version: 48` overflowing
the 5 bits it shared with `Kind`, rather than via `Kind >= 8` — the same flaw, the byte just gives
out from either direction.)

## What the fuzzer can and cannot take

`Frame` exercises every struct shape gotest's fuzzing supports: a named basic (`Kind`, whose
promoted literals render as `Kind(3)`), a nested struct slice (`[]Header`), a pointer-to-struct
(`*TraceID`), `[]byte`, and `string`. `gotest generate` turns each non-native argument type into a
total decoder/encoder pair so `FuzzFrameRoundTrip` compiles and runs at all — Go's own fuzzing
engine only accepts fifteen primitive types, and `Frame` isn't one of them.

`FuzzTopicMatches` uses `gotest.Fuzz2` instead, and deliberately stays on two plain `string`
arguments: multi-argument fuzz targets (`Fuzz2`, `Fuzz3`) support Go's native fuzzable types only,
never struct arguments. Structured input belongs in a single struct-typed `gotest.Fuzz` target, the
way `FuzzFrameRoundTrip` uses `Frame`.

Some shapes are rejected at generation time rather than silently mis-encoded:

- **Unexported fields** — outside the package they can't be set; inside, setting them bypasses the
  invariants a constructor enforces. Fuzz the constructor's input instead, or declare a local
  wrapper struct.
- **`map`** — no canonical encoding for key order. Fuzz a slice of key/value pairs and build the map
  in the callback.
- Interfaces, channels, funcs, recursive types, and `time.Time`-shaped opaque structs are rejected
  for the same reason: there is no honest value to synthesize for them.

A rejection is a generation-time error naming the offending field, not a runtime surprise — see
`docs/design/fuzz-structs.md` for the full table and the reasoning behind it.

A careful reader may wonder whether a nil `Headers`/`Payload` and a genuinely empty one are
distinguishable after a round trip: they aren't, by convention parity — `Decode` collapses a
zero-length read to `nil`, exactly like the generated codec that manufactures `FuzzFrameRoundTrip`'s
seed values, so the two sides never disagree on which one to produce.

`go run ./cmd/gotest lint ./examples/fuzzing/...` reports nothing and exits 0 — but only because
`FuzzNormalizeTopicIdempotent` carries a `//nolint:fuzz-seed` directive. Without it the `fuzz-seed`
rule would flag that target for having no explicit `f.Add` seed of its own, which is intentional
here, not an oversight: its seeds are harvested from `TestNormalizeTopicTable`'s literals (see the
Targets table above). Suppressing a rule you can justify — and leaving the reason in the source —
is the intended workflow.
