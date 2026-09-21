---
title: "Fuzzing Go Structs: Past the Fifteen Types the Engine Accepts"
date: 2026-09-18
description: "Go's fuzzing engine takes fifteen primitive types, none of them a struct. How gotest fans a struct into engine arguments and keeps seeds readable."
tags: ["Deep Dive"]
keywords: ["go fuzzing struct", "go fuzz complex types", "structure aware fuzzing go", "go test fuzz struct input"]
cta_text: "Fuzz your own types with gotest."
---

Go's fuzzing engine accepts fifteen argument types: `string`, `[]byte`, `bool`, `int` and `uint` with their sized forms, and the two floats. That list is the entire contract of `f.Fuzz`. Your domain types are not on it.

So the first time you try to fuzz the thing you actually want to fuzz — a wire frame, a request payload, a config struct — you hit a wall that has only two doors. Either you fuzz a `[]byte` and decode it into your type by hand, or you do not fuzz that type at all. Most codebases pick the second door quietly.

The first door is the one the ecosystem documents. You write a consumer: take the fuzzer's bytes, slice off four for a version, read a length prefix, take that many bytes for a topic, and so on. Libraries exist to make this less tedious. It works, and it costs more than it looks.

## What a byte consumer costs

**Your seeds stop being readable.** A seed is supposed to be the interesting input you already know about — the empty string, the boundary version, the header with a colon in the name. Behind a consumer, that knowledge has to be written as the byte encoding your consumer happens to expect. Nobody reviews those.

**Your crashes come back as bytes.** The fuzzer finds something and writes a corpus file. What you get is the encoded form. To learn what the value *was*, you run the consumer in your head, or add a print statement and re-run.

**A refactor silently reinterprets the corpus.** Add a field in the middle of the struct, or swap two fields, and every stored corpus entry now decodes to a different value than the one that was interesting. Nothing fails. Your regression corpus quietly stops testing what it was saved to test.

**The consumer is code with bugs.** It is parsing logic, written under no test, sitting between the fuzzer and the code under test. When it is wrong, the fuzzer explores a space that does not correspond to reality.

The third cost is not unique to hand-written consumers, and it is worth being straight about: gotest's own corpus files are positional too, so a field reorder reinterprets them the same way. What escapes it is the *seed* — a typed literal in source, which is where gotest puts anything worth keeping. [From Crasher to Regression Test]({{< ref "/blog/fuzz-crasher-triage" >}}) covers that trade in full.

None of this is the engine's fault. Fifteen types is a reasonable contract for a mutator that works on bytes. The question is who builds the bridge from those bytes to your type, and when.

## Fanning the struct at generation time

gotest builds that bridge when it generates the test harness, not when the fuzzer runs.

A fuzz target is a method on a suite. Its callback takes the type you care about:

```go {title="frame_suite_test.go"}
func (s *FrameCodecTestSuite) FuzzFrameRoundTrip(f *gotest.F) {
    f.Add(Frame{
        Version: 1,
        Kind:    KindControl,
        Topic:   "orders.created",
        Headers: []Header{{Name: "content-type", Value: "application/json"}},
        Payload: []byte(`{"id":1}`),
        Trace:   &TraceID{Hi: 1, Lo: 2},
    })
    f.Fuzz(func(t *gotest.T, in Frame) {
        out, err := Decode(Encode(in))
        gotest.NoError(t, err)
        gotest.Equal(t, in, out) // property: round trip
    })
}
```

`Frame` is not one of the fifteen types. At generation time, gotest walks it and emits one engine argument per leaf field, then reassembles a `Frame` from those arguments before each execution. For this type it comes to eight leaves:

| Field | Leaves |
|---|---|
| `Version`, `Kind` | one little-endian `[]byte` each |
| `Topic` | one `string`, passed through |
| `Payload` | one `[]byte`, passed through |
| `Headers` | one packed `[]byte` |
| `Trace` | a `bool` nil-flag, then `Hi` and `Lo` |

The generated wrapper is an ordinary `go test` fuzz target with eight primitive parameters. The engine never learns that a struct exists; your callback never learns that it does not.

## Why the timing is the point

Doing the fan at generation time, rather than in a consumer at run time, changes three things.

**Rejections happen when you generate.** Some shapes have no honest byte encoding, and gotest refuses them with an error naming the field instead of synthesizing something plausible:

- **Unexported fields.** Outside the package they cannot be set; inside, setting them bypasses the invariants the constructor enforces. Fuzz the constructor's input, or declare a local wrapper struct.
- **Maps.** There is no canonical key order, so no stable encoding. Fuzz a slice of key/value pairs and build the map in the callback.
- **Interfaces, channels, funcs, recursive types, and opaque structs like `time.Time`.** There is no honest value to synthesize.

You learn this the moment you write the target, not at 3am when a CI fuzz session behaves strangely.

**Each leaf mutates independently.** This is the part that decides whether fuzzing finds anything. The engine's mutator works per argument: it flips bits in one, replaces another with a boundary value, leaves the rest alone. Because `Version` is its own argument rather than bytes four through five of a blob, a boundary value for `Version` is one mutation away rather than one lucky sequence away.

**Seeds and crashes stay typed.** `f.Add(Frame{...})` takes the value, not an encoding of it. When a crash is triaged, what comes back is the literal:

```text
input: Frame{Version: 48, Kind: Kind(0), Topic: "", Headers: nil, Payload: nil, Trace: nil}
```

That is a line you can paste into a test.

## A real find

The example this post draws on is a broker frame codec. `Frame` packed `Version` and `Kind` into a single byte to save space on the wire — `Kind` needs three bits today, so the trick looked safe. Neither field was bounded to its share of the byte.

`gotest fuzz` found it in eleven executions, in under a second:

{{< terminal title="gotest fuzz ./examples/fuzzing --for=60s" >}}
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] fuzz: elapsed: 0s, execs: 11 (425/sec), new interesting: 0 (total: 1)
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] <span class="t-fail">--- FAIL: FuzzFrameCodecTestSuite_FuzzFrameRoundTrip (0.03s)</span>
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]         suite_test.go:86: Equal failed:
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]               expected: fuzzing.Frame{Version:0x30, Kind:0x0, ...}
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip]               actual:   fuzzing.Frame{Version:0x10, Kind:0x0, ...}
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] new crasher: testdata/fuzz/FuzzFrameCodecTestSuite_FuzzFrameRoundTrip/582528ddfad69eb5
[FuzzFrameCodecTestSuite_FuzzFrameRoundTrip] inspect it with `gotest fuzz triage`, then `gotest fuzz promote` to keep it as a typed seed
{{< /terminal >}}

`Version: 48` overflowed the five bits it shared with `Kind`, and the decoded frame came back as `Version: 16`. The property that caught it is one line: encode, decode, compare. No oracle to write, no expected output to maintain.

Note which field the fuzzer reached. In the fanned form `Version` is its own fixed-width `[]byte` argument rather than bytes four and five of a blob, so "try 48 here" is a single mutation instead of a lucky coincidence of layout.

## Seeds you can read, and seeds you get for free

A seed is a `f.Add` call with a value of the callback's type. Three things then hold.

**Seeds replay on every ordinary run.** `gotest ./...` runs each seed as a subtest of the target. You do not need `gotest fuzz` for that, and you do not need CI time budgeted for fuzzing to keep the regression value of everything you have found so far.

**Table tests become seeds.** Seed harvesting is on by default: for targets whose callback takes basic types, literal arguments in table rows and call sites across your `_test.go` files are mined at generation time and added as extra `f.Add` seeds. A table test you already wrote for `NormalizeTopic` seeds the fuzz target for `NormalizeTopic` without being mentioned twice. Turn it off per run with `--no-harvest`, or in `.gotest.yml` with `fuzz.harvest: false`.

**A crash becomes a seed in one command.** `gotest fuzz promote` splices the decoded value into the target as an `f.Add(...)` after the last existing seed and deletes the corpus file. The regression lives in source, reviewable in the pull request that fixes the bug.

## Choosing the shape of a target

One thing the fan makes easy is worth resisting: fanning everything into one enormous struct because you can.

Each callback position fans on its own, so a target can mix a struct with a plain argument — a `Header` beside a `string`, say. Use separate arguments when the property is about independent values, and a named struct when the values belong together. `FuzzFrameRoundTrip` takes a `Frame` because a frame is one thing. A target that takes a frame *and* a codec limit takes two arguments, because those vary independently.

The property in the callback is what you are really designing. Round trips (`Decode(Encode(x)) == x`), idempotence (`f(f(x)) == f(x)`), invariants (never panics, output always valid), and symmetry (`match(a, b) == match(b, a)`) all give the fuzzer something to falsify without you predicting a single output value. The `fuzz-no-oracle` lint rule exists because a target with no property — one that calls the code and asserts nothing — only finds panics.

## What this changes

Fuzzing in Go has a reputation for being for parsers and byte-slice APIs. That reputation is mostly an artifact of the fifteen-type list: those are the APIs whose inputs the engine happens to speak.

When the bridge to your types is generated instead of hand-written, the set of things worth fuzzing widens to anything with a property you can state in one line — validators, normalizers, codecs, state machines, permission checks. The seeds stay reviewable, the crashes come back as Go literals, and the corpus does not rot the next time someone adds a field.

## Further reading

For what to do with what the fuzzer finds, [From Crasher to Regression Test]({{< ref "/blog/fuzz-crasher-triage" >}}) covers `triage`, `promote`, and why a re-run that produced no verdict is reported as unverified. For the suite model these targets live in — fixtures, hooks, and why `BeforeEach` runs around every fuzz execution — start with [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}) and [Go Test Lifecycle]({{< ref "/blog/go-test-lifecycle" >}}). The flags, session rules and rejection list are in the [Fuzzing reference](/reference/#fuzzing).
