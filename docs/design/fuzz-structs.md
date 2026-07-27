# Struct Fuzzing

> Status: **Phase A implemented** — supersedes Part 4's "typed fuzzing via generated decoders" sketch in
> [bench-fuzz.md](bench-fuzz.md), which was deferred during implementation because its mechanism
> did not survive contact with the codegen model. Phases B–D remain proposals.

Go's fuzzing engine accepts exactly fifteen argument types.
Everything real takes structs.

```go
// what users want to write
func (s *UserServiceTestSuite) FuzzCreate(f *gotest.F) {
    f.Add(CreateUserRequest{Email: "a@b.c", Age: 30})
    gotest.Fuzz(f, func(t *gotest.T, req CreateUserRequest) {
        _, err := s.svc.Create(req)
        gotest.NotPanics(t, ...)
    })
}
```

Today this compiles and then panics at run time: `testing: unsupported type for fuzzing`.
The workaround — hand-rolling a byte splitter in every target — is exactly the boilerplate
gotest exists to delete.

---

## Constraints

These are derived from the architecture, not chosen. A design that breaks one is disqualified.

1. **Generated code is additive.** The overlay injects new files; it never replaces a user's file.
   What the user reads is what compiles. (`-overlay` *can* replace files — see
   [Rejected: overlay rewrite](#rejected-overlay-rewrite) for why it must not.)
2. **The fuzz call site lives in user source.** `gotest.Fuzz(f, cb)` is written in a file gotest
   does not own. Whatever makes struct callbacks work must reach that call site through a value
   the generated wrapper already controls — and `*gotest.F` is the only such value.
3. **No reflection over user data, no runtime magic.** Discovery and dispatch stay static.
4. **No generated file enters the user's tree.** Nothing gotest emits may need committing.
5. **A fuzz execution that rejects its input is a wasted execution.** Coverage-guided fuzzing
   pays for every `t.Skip()` with zero learning. Rejection rate is a first-order design metric,
   not a detail.

## Verified facts

Confirmed empirically against Go 1.24 before designing (a scratch module, not this repo):

| Question | Answer |
|---|---|
| Does `x.(Codec[A])` work when `A` is a type parameter? | **Yes** — matches on identical instantiation, misses cleanly otherwise. It is a type assertion, not `reflect`. |
| Does `go vet` block a struct type argument to a *generic* wrapper? | **No** — vet checks direct `(*testing.F).Fuzz` calls only; the generic body hides instantiation. Failure is at run time. |
| What is the failure without a codec? | `testing: unsupported type for fuzzing <T>` (panic, at run time) |
| Does `f.Add(structValue)` reach our code first? | **No** — `testing.F.Add` panics with `unsupported type to Add <T>`. `gotest.F.Add` must intercept before forwarding. |

Fact 2 is why the static gate matters: nothing in the toolchain catches this for us.
Fact 4 is a hard requirement on the design, not an optimization.

---

## Mechanism: codecs travel on `*gotest.F`

The generated wrapper constructs the `*gotest.F` that the user's method receives. That is the
seam. Codegen attaches a decoder/encoder pair for every non-native type the package fuzzes;
`gotest.Fuzz[A]` looks for one and reroutes to a native `[]byte` target when it finds a match.

```go
// pkg/gotest — runtime side, ~40 lines total
type Codec[A any] struct {
    Decode func([]byte) A        // total: every byte string yields a value
    Encode func(A) []byte
}

func Fuzz[A any](f *F, fn func(*T, A)) {
    for _, c := range f.codecs {
        if codec, ok := c.(Codec[A]); ok {          // type assertion, not reflect
            f.f.Fuzz(func(t *testing.T, raw []byte) {
                f.run(t, func(tt *T) { fn(tt, codec.Decode(raw)) })
            })
            return
        }
    }
    f.f.Fuzz(func(t *testing.T, a A) {              // native path, unchanged
        f.run(t, func(tt *T) { fn(tt, a) })
    })
}
```

`F.Add` mirrors it, encoding any argument a codec claims (fact 4 above makes this mandatory):

```go
func (f *F) Add(args ...any) {
    for i, a := range args {
        for _, c := range f.codecs {
            if b, ok := c.(anyCodec).encodeAny(a); ok { args[i] = b; break }
        }
    }
    f.f.Add(args...)
}
```

Generated wrapper, unchanged in shape from what ships today apart from the codec list:

```go
func FuzzUserServiceTestSuite_FuzzCreate(f *testing.F) {
    // ... fixtures, guard, BeforeAll (as today) ...
    s.FuzzCreate(gotest.NewF(f, s.BeforeEach, s.AfterEach,
        gotest.Codec[CreateUserRequest]{
            Decode: ƒ_fuzzdec_CreateUserRequest,
            Encode: ƒ_fuzzenc_CreateUserRequest,
        },
    ))
}
```

**The user-facing API does not change.** `gotest.Fuzz` with a struct callback starts working;
nothing else moves. Every part of the mechanism is internal, which is the property that lets us
replace it later without a deprecation cycle.

### Honest accounting of the type assertion

`c.(Codec[A])` is a runtime type check. It is not the `reflect` package, does not inspect user
data, and costs one interface comparison per fuzz *target* (not per execution — the lookup happens
once, before `f.f.Fuzz`). It is the same language construct as a type switch. The alternative that
avoids it entirely (rewriting user source) is strictly worse on every other axis. This is the
minimum honest price for closing the gap, and it is worth naming in the docs rather than
pretending the feature is free.

### Detection

Codegen finds targets from type information, not call shape: walk the package's test files for
calls to `gotest.Fuzz`/`Fuzz2`/`Fuzz3`, read the instantiated type arguments from
`types.Info.Instances`, and emit a codec for every argument outside the native set. The callback
does **not** need to be an inline literal (the earlier sketch required this) — a method value or
named function works, because we read the instantiation, never the body.

Codecs are generated per package and attached to every `F` in that package. One `Fuzz` call per
target means the list is one or two entries; a linear scan is cheaper than any map.

---

## Wire format: a total, consuming reader

The deferred design used a length-prefixed, field-index-tagged format plus a committed
`.gotest-fieldmap.json` to keep indexes stable. Both fall away once constraint 5 is taken
seriously.

**Format.** Fields are decoded in declaration order from a byte cursor. Exhaustion is not an
error — it yields zero values. Nothing ever rejects.

| Type | Encoding |
|---|---|
| `bool` | 1 byte, `!= 0` |
| `intN`/`uintN` | N/8 bytes, little-endian, fixed width |
| `float32`/`float64` | 4/8 bytes via `Float64frombits` (NaN and Inf reachable — desirable) |
| `string`, `[]byte` | 2-byte length, clamped to remaining, then bytes |
| slice, array | 1-byte count, clamped; then elements |
| pointer | 1 byte nil flag, then value |
| nested struct | inlined, fields in declaration order |

Fixed-width integers rather than varints: a single byte flip perturbs exactly one field, which is
the mutation locality that makes coverage guidance converge.

**Why not tagged.** Tags buy field-reorder tolerance and cost mutation efficiency — every length
prefix is a desync point, and a desynced parse either rejects (wasted execution) or shifts every
subsequent field anyway. We would be paying the fuzzer's budget for a corpus property we can get
somewhere better.

**Where corpus durability actually comes from.** The artifacts worth keeping are seeds and
crashers, and `gotest fuzz promote` already turns crashers into source-level `f.Add(...)` calls.
With `F.Add` encoding typed values, a promoted crasher becomes
`f.Add(CreateUserRequest{Email: "a@\x00", Age: -1})` — a Go literal that re-encodes correctly under
*any* future format. The durable artifact is source, not bytes. The on-disk corpus becomes a cache,
which is what Go's own cached corpus already is.

Two properties fall out for free:

- **Appending a field is corpus-safe.** New trailing fields read from an exhausted cursor and
  decode as zero; every existing field keeps its value. No tags needed for the common evolution.
- **Minimization produces better reproducers.** Go shrinks a crasher toward smaller inputs, which
  here means zero-filled trailing fields — a minimal failing struct rather than a minimal blob.

Reordering or inserting fields *does* reinterpret cached corpus entries. That is the honest cost,
it is documented, and promote is the mitigation. Codec output is versioned
(`ƒ_fuzzdec_v1_<Type>`) so a future format change moves every generated identifier rather than
quietly redefining the old ones.

**In Phase A this mitigation does not yet reach struct targets.** `triage`/`promote` decode a corpus
entry through Go's native corpus format, where a rerouted struct target's entry is a plain
`[]byte(...)`. Promotion therefore splices a *byte literal* into user source — it replays correctly
today (`F.Add` passes an unclaimed `[]byte` straight through to the rerouted target), but it is
format-bound in exactly the way a Go literal is not. Versioning the generated identifiers cannot
protect a blob living in a hand-owned file: under a future v2 format it would decode to a different
value with no error. So for struct targets the durable artifact is source only once **Phase C**
lands and promotion emits `f.Add(CreateUserRequest{...})`. Until then, treat a promoted struct
crasher as a cache entry that happens to live in source.

---

## What is deliberately excluded

Refusing generates a clear error at generation time; permitting generates code that lies. Start
strict — loosening is backward compatible, tightening is not.

| Excluded | Why | What to do instead |
|---|---|---|
| Structs with **unexported fields** | Outside its package we cannot set them; inside, setting them bypasses the invariants a constructor enforces, manufacturing impossible states and false-positive crashes | Fuzz the constructor's input, or declare a local wrapper struct |
| `map` | Encoding requires a deterministic key order; decoding requires a canonical form | Fuzz a slice of pairs and build the map in the callback |
| Interfaces, channels, funcs | No sensible inhabitant to synthesize | — |
| Recursive types | Unbounded decode | Depth-limited variant if ever demanded |
| `time.Time` and friends | Opaque internals (an unexported-fields case) | Fuzz an `int64` and convert in the callback |

Each rejection names the offending field and suggests the alternative, e.g.
`struct CreateUserRequest field mu (sync.Mutex) is not fuzzable — fuzz the constructor input instead`.

---

## Testing strategy

The failure mode that matters is *silently generating wrong code*, so the tests target that:

1. **Round-trip property** — `Decode(Encode(v)) == v` across a table covering every supported kind,
   nesting, pointers, slices, and boundary values.
2. **Totality by fuzzing our own decoder** — a fuzz target over random bytes asserting the
   generated decoder never panics and always returns a value. gotest fuzzing gotest's fuzz support
   is the honest way to prove constraint 5 holds.
3. **Compile-the-output** — generated decoders are type-checked in a scratch module, reusing the
   harness `scaffold --fuzz` already uses. Substring assertions cannot catch a non-compiling
   template; this branch already shipped that bug once.
4. **End-to-end** — a struct-typed fuzz target in `examples/`, seeds replayed under a plain
   `gotest ./...` run.

---

## Rejected alternatives

### Rejected: global codec registry keyed by `reflect.Type`

Generated `init()` functions register codecs in a package-level map; `Fuzz[A]` looks up
`reflect.TypeFor[A]()`. Same user-facing behavior, but it imports `reflect`, introduces global
mutable state and init-order coupling, and gains nothing the `F`-scoped list does not already
provide. This was the mechanism ruled in mid-implementation, and reviewing it in writing is what
prompted the deferral.

### Rejected: overlay rewrite

`-overlay` can replace a user's test file, so codegen could splice the call site into a
`FuzzBytes` form and eliminate runtime dispatch entirely — the "purest" codegen answer.
Rejected because it breaks constraint 1 in the way that matters: the code that compiles would no
longer be the code the user reads. Debuggers, coverage attribution, and `gopls` all diverge, and
gotest permanently owns faithfully reproducing user source. A byte-range splice (rather than AST
reprinting) makes it *safer*, not safe. The codec design preserves the option to adopt this later
without a user-visible change, which is the strongest argument for shipping the codec design now.

### Rejected: reflection-based value filler

A `gotest.Consume[T]` helper in the style of `go-fuzz-headers` reflects over `T` at run time,
per execution. This violates constraint 3 at the worst point — reflection over user data in the
hot path — and is slower than generated code by construction.

---

## Phasing

**Phase A — core. Shipped.** Codec plumbing on `F` (`Fuzz` dispatch, `Add` encoding); byte reader/writer
primitives in `pkg/gotestruntime` so emitted decoders stay small; type-argument detection and
validation with actionable errors; decoder/encoder emission; template injection; the four test
layers above.

Two decisions the implementation added to what is written above. The `fuzzCodec` interface `F` carries
has an unexported `encodeAny` method, so `Codec[A]` is the only type that can ever satisfy it — the
mechanism is closed to outside implementations, which is what lets it be replaced without a deprecation
cycle. And `Fuzz2`/`Fuzz3` stay native-types-only: a codec per argument position would need one closure
shape per native/non-native combination (four for `Fuzz2`, eight for `Fuzz3`), and wrapping the arguments
in a single struct is both cheaper and better for mutation locality. Codegen rejects the non-native case
with exactly that advice. Loosening this later is backward compatible.

**Phase B — ergonomics.** Typed seeds (`f.Add(T{...})`) documented; struct composite literals in
the table-test harvester; `f.Add` seed/target type-mismatch reported against the target instead of
panicking from stdlib.

**Phase C — readable crashers.** This is also what makes promote's output durable for struct
targets rather than format-bound (see "Where corpus durability actually comes from" above).
`triage`/`promote` currently handle primitives only; struct
decoding must happen inside the test binary, where the types exist. A generated, normally-skipped
helper target that decodes a corpus file and prints a Go literal on demand keeps the decoder
single-sourced instead of reimplementing it in the CLI.

**Phase D — on demand.** Maps with canonical ordering, depth-limited recursion, user-registered
codecs for opaque types.

Phase A is self-contained and shippable: struct fuzzing works end to end, crashers are byte blobs
until Phase C.

---

## What we would still regret

- **Reordering fields reinterprets the cached corpus.** Mitigated by promote-to-source and by
  codec versioning; not eliminated. If real usage shows this hurts, tags can be added *behind the
  same API* — the format is internal.
- **The type assertion.** If gotest ever wants a literally zero-runtime-dispatch story, the
  overlay rewrite is the escape hatch, and adopting it requires no user change.
- **Strictness on unexported fields will annoy someone.** That is the correct direction to be
  wrong in: a rejection is a conversation, a false-positive crasher is a lost afternoon.
- **The fixed-width prefixes truncate.** A seed string or `[]byte` over 65535 bytes, or a slice over
  255 elements, is clipped on encode, so `Decode(Encode(v)) == v` holds only within those bounds.
  An empty slice and a nil slice are also indistinguishable after a round trip, and a slice of
  zero-width elements decodes as empty. These are the price of the mutation locality a fixed-width
  prefix buys; they are asserted in the round-trip tests rather than papered over.
