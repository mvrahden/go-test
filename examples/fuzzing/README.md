# fuzzing — Wire Protocol Fuzzing, Crash to Regression Test

This example shows a fuzz-found bug becoming a committed regression test.
The subject is a message broker's binary frame codec: the code every service with a custom
protocol has, and the classic place where adversarial input causes CVEs.

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
| `FuzzTopicMatches` | two pass-through string arguments, symmetry property |
| `FuzzHeaderRoundTrip` | a struct beside a pass-through string, **round-trip** property |

`BeforeEach` rebuilds `s.codec` before every execution, not only before every top-level test:
fuzz targets replay `BeforeEach`/`AfterEach` around each execution, the same as any other test.

## From crash to regression test

`Frame` packed `Version` and `Kind` into one byte to save space on the wire.
`Kind` needs three bits today, so the trick looked safe.
Neither field was bounded to its share of the byte, so a `Version >= 32` or a `Kind >= 8` silently
corrupted the other field and broke `FuzzFrameRoundTrip`'s round-trip property.
`gotest fuzz` found it in 11 executions, under a second:

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

`gotest fuzz triage` re-runs the crasher and prints the `Frame` the target reassembled from it, not
the raw per-leaf corpus values:

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

`Version` and `Kind` were then given a byte each, and `go run ./cmd/gotest ./examples/fuzzing -v`
went green. The promoted seed now replays as `FuzzFrameRoundTrip/seed#1` on every run: a permanent
regression test for the packed-byte bug. (The fuzzer reached it through `Version: 48` overflowing
the five bits it shared with `Kind`, not through `Kind >= 8`; the byte gives out from either
direction.)

## What the fuzzer can and cannot take

`Frame` exercises every struct shape gotest's fuzzing supports: a named basic (`Kind`, whose
promoted literals render as `Kind(3)`), a nested struct slice (`[]Header`), a pointer-to-struct
(`*TraceID`), `[]byte` and `string`. Go's engine accepts only fifteen primitive types, and `Frame`
is not one of them, so `gotest generate` fans it into one engine argument per leaf field and
reassembles the `Frame` before each execution. `Frame` comes to eight leaves:

| Field | Leaves |
|---|---|
| `Version`, `Kind` | one little-endian `[]byte` each |
| `Topic` | one `string`, passed through |
| `Payload` | one `[]byte`, passed through |
| `Headers` | one packed `[]byte` |
| `Trace` | a `bool` nil-flag, then `Hi` and `Lo` |

Each leaf is something the mutator moves on its own, which is what puts a boundary value like
`Version: 48` one mutation away. The mapping rules are in ARCHITECTURE.md, "Code Generation".

Every position of the callback fans on its own: `FuzzHeaderRoundTrip` mixes a `Header` with a plain
`string`. It takes two arguments because the property is about two independent values; when values
belong together, a named struct says so better than a wider tuple, the way `FuzzFrameRoundTrip`
uses `Frame`.

Some shapes are rejected at generation time rather than silently mis-encoded:

- **Unexported fields** — outside the package they can't be set; inside, setting them bypasses the
  invariants a constructor enforces. Fuzz the constructor's input instead, or declare a local
  wrapper struct.
- **`map`** — no canonical encoding for key order. Fuzz a slice of key/value pairs and build the map
  in the callback.
- Interfaces, channels, funcs, recursive types, and `time.Time`-shaped opaque structs are rejected
  for the same reason: there is no honest value to synthesize for them.

A rejection is a generation-time error naming the offending field, not a runtime surprise.

Two details a careful reader may check:

- A nil `Headers`/`Payload` and an empty one are not distinguishable after a round trip, by
  convention parity: `Decode` collapses a zero-length read to `nil`, and so does the fan on its way
  in. The engine does not preserve the distinction either; a nil `[]byte` seed comes back as
  `[]byte{}` after a trip through the corpus format. Both sides agree, under replay and under
  `-fuzz` alike.
- `go run ./cmd/gotest lint ./examples/fuzzing/...` reports nothing only because
  `FuzzNormalizeTopicIdempotent` carries a `//nolint:fuzz-seed` directive. Its seeds are harvested
  from `TestNormalizeTopicTable`'s literals (see the Targets table), so the missing `f.Add` is
  intentional. Suppressing a rule you can justify, with the reason in the source, is the intended
  workflow.
