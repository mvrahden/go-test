# Benchmarking & Fuzzing

> Status: Parts 1–3 (benchmarking core, baselines/gates/editor integration,
> fuzz core) shipped. Of Part 4 (the fuzz AST layer), seed harvesting,
> crash triage/promote, `scaffold --fuzz`, and the three fuzz lint rules
> (determinism/no-oracle/seed) have also shipped, primitives-only; only
> struct decoders (the typed-fuzzing codec layer) remain a proposal, now
> designed separately in [fuzz-structs.md](fuzz-structs.md). See "Shipped deviations (Part 4)"
> below, the README's "Benchmarking" and "Fuzzing" sections, and
> ARCHITECTURE.md for what `gotest bench`/`gotest fuzz` actually do today.

## Shipped deviations (Part 1)

Where the implementation diverged from this proposal:

- **Per-method lifecycle timer fencing.** The generated wrapper stops the
  benchmark timer before `BeforeEach`, starts and resets it immediately
  before the benchmark method runs, and stops it again before `AfterEach` —
  not just "outside the loop" as implied above, but an explicit
  `StopTimer`/`BeforeEach`/`StartTimer`/`ResetTimer`/method/`StopTimer`/`AfterEach`
  sequence per benchmark method.
- **Fixture per-method hooks × benchmarks is a hard rejection, not a
  restriction to work around later.** A fixture with `BeforeEach`/`AfterEach`
  bound to a suite that has `Benchmark*` methods fails at generation time
  ("per-method fixture hooks are not supported for benchmarks"). Only
  fixture `BeforeAll`/`AfterAll` (once per process) are supported for
  benchmark suites.
- **`-run` and `-bench` compose with AND semantics**, both matched against
  the same `Benchmark<SuiteName>` wrapper name (`-run` scopes by suite,
  `-bench` by benchmark function name) — not the stdlib model where `-run`
  and `-bench` are independent regexes matched against different things.
  A `-run Test<Suite>`-style value matches nothing in bench mode.
- **No `gotest.EachBench` yet.** Input-scaling tables (the
  `BenchmarkParseScaling` idiom below) are not implemented — Part 1 shipped
  single-point benchmarks only.

Tests are a specification. That framing extends beyond correctness:

- **Tests** specify *correctness* — "it returns ErrDuplicate".
- **Benchmarks** specify *performance* — "Parse handles 10k rows under 2µs/op".
- **Fuzz targets** specify *robustness* — "Parse never panics, for any input".

The goal is not "also run `go test -bench`". It is making performance and robustness
first-class parts of the behavioral spec, with the same lifecycle, isolation, and
tooling story that suites already have.

This is no longer a forward-looking claim: `-bench`/`-fuzz` flags have been
classified in `internal/gotestrunner/args.go` since before this doc was
written, and generated suites now do emit `Benchmark*`/`Fuzz*` functions —
see the README's "Benchmarking" and "Fuzzing" sections and ARCHITECTURE.md
for what ships today.

## Why gotest is structurally positioned for this

Three advantages nothing else in the Go ecosystem has here:

1. **The runner owns scheduling.** `go test -bench` happily runs benchmarks while
   other packages' tests thrash the CPU — numbers are garbage by default. gotest
   controls all subprocesses, so it can serialize benchmarks on a quiet machine
   automatically while still parallelizing everything else. Similarly,
   `go test -fuzz` can only fuzz **one target per invocation** — gotest can
   orchestrate a whole module's fuzz targets across cores with a time budget.
2. **AST analysis sees the whole test corpus.** Table tests are a hand-curated
   seed corpus that nobody feeds to the fuzzer. gotest can.
3. **Codegen erases stdlib limitations.** `testing.F.Fuzz` only accepts primitives.
   A generated decoder makes struct fuzzing work with zero reflection and zero
   runtime dependencies.

---

## Part 1: Benchmarking

### Authoring model — methods on existing suites

No new suite kind. The collector's method-classification pass (Pass 3) grows one
classifier: `^(X_|F_)?Benchmark.+$` with receiver param `*gotest.B` (or `*testing.B`):

```go
type ParserTestSuite struct {
    Setup *E2ESetupFixture
    p     *Parser
}

func (s *ParserTestSuite) BeforeEach(t *gotest.T) { s.p = NewParser(s.Setup.Pool) }

func (s *ParserTestSuite) BenchmarkParse(b *gotest.B) {
    doc := loadTestdata("large.json")
    for b.Loop() {
        s.p.Parse(doc)
    }
}
```

This inherits everything: fixtures hydrate before the benchmark, `BeforeEach` gives
fresh state per benchmark method (outside timing), `F_`/`X_` focus/exclude carries
over, and the generated wrapper is exactly what a careful developer writes by hand:

```go
func BenchmarkParserTestSuite(b *testing.B) {
    s := &ƒƒ_GOTEST_ParserTestSuite{...}
    ƒ_setupFixtures(b)                    // fixture DAG, outside timing
    s.BeforeAll(...)
    b.Run("BenchmarkParse", func(b *testing.B) {
        s.BeforeEach(...)                 // fresh state, timer not started
        defer s.AfterEach(...)
        s.BenchmarkParse(wrap(b))         // user's b.Loop() bounds measurement
    })
}
```

Since gotest requires Go 1.24+, standardize on `b.Loop()` — it excludes setup before
the loop from timing automatically, and it gives the lint analyzer a crisp rule:

- **A `Benchmark*` method that never calls `b.Loop()` (or reads `b.N`) is a
  measurement bug** — the single most common benchmark mistake in the wild.
- Second rule: sub-benchmark registration or allocation-heavy setup inside the
  loop body.

### Input scaling — `gotest.EachBench`

Extend the existing table idiom rather than inventing one:

```go
func (s *ParserTestSuite) BenchmarkParseScaling(b *gotest.B) {
    for b, size := range gotest.EachBench(b, []int{100, 10_000, 1_000_000}) {
        data := gen(size)
        for b.Loop() {
            s.p.Parse(data)
        }
    }
}
```

Because the sizes are AST-visible literals, the spec view can render a scaling
profile (ns/op vs n) — and flag when a change bends the curve from n·log n toward
n², which a single-point benchmark can never show.

### Execution — `gotest bench`

`gotest ./...` keeps skipping benchmarks (semantics unchanged). New subcommand:

```bash
gotest bench ./...                     # all benchmarks
gotest bench ./pkg/parser -run Parse   # filtered
```

What the runner does differently in bench mode:

- **Serial by default.** One benchmark suite process at a time; `SuiteConfig.Parallel`
  is ignored; streaming is off — compile everything first, *then* run, so `go test -c`
  never competes with a running benchmark for CPU.
- **Process-per-suite = clean heap per suite.** GC pressure from one benchmark cannot
  pollute another's numbers. This is a methodological advantage, not just an
  implementation detail — document it as such.
- **`-benchmem` on by default**, `-count` forwarded, output parsed from the existing
  test2json capture path (test2json emits bench events; the `spec`/`summary` parsers
  grow a bench event type in `internal/protocol`).

### Baselines and regression gates

The highest sustained value, and the tie-in to the planned `gotest stats` command:

```bash
gotest bench ./... --save=.gotest/bench-baseline.json
gotest bench ./... --against=.gotest/bench-baseline.json --gate=10%
```

- `--save` writes structured results (ns/op, B/op, allocs/op, per-count samples).
- `--against` runs benchstat-style significance testing (`golang.org/x/perf` or a
  small internal implementation) — deltas are reported only when statistically
  meaningful, so CI does not cry wolf on noise.
- `--gate` fails CI on significant regressions beyond the threshold. The GitHub
  Action (`action.yml`) grows a mode that posts the delta table as a PR comment.
- `gotest spec --bench` renders the performance section of the spec:

```
Parser
  ✓ Parse            1.8µs/op   24 B/op   3 allocs/op   (▲ +2.1% vs baseline)
  ✓ ParseScaling     n=100: 0.2µs  n=10k: 21µs  n=1M: 2.4ms
```

### Editor integration

The VS Code extension already has CodeLens, discovery JSON, and an output parser.
Benchmarks slot in:

- A `run bench` lens on `Benchmark*` methods; the result rendered as an inline
  decoration (`1.82µs/op  24 B/op  3 allocs/op  Δ+3%` vs the last local run),
  history kept in the existing `testResultStore`.
- `gotest watch --bench ./pkg/x` re-runs on save and prints deltas against the
  previous run — a tight edit-measure loop is the thing `go test -bench` workflows
  most painfully lack.
- Later refinement: discovery has full type info, so map changed functions →
  benchmarks that (transitively) call them, and re-run only affected benchmarks
  on save.

---

## Shipped deviations (Part 3)

Where the implementation diverged from this proposal (or filled in something
it left open):

- **`*gotest.F`-only parameter — no `*testing.F` fallback.** Unlike
  benchmarks, where a `Benchmark*` method may take either `*gotest.B` or
  bare `*testing.B`, a `Fuzz*` method must take `*gotest.F`; the collector
  rejects `*testing.F` outright (`"fuzz method %s must accept exactly one
  parameter of type *gotest.F"`). Fuzz lifecycle interposition depends on
  `gotest.F`'s `beforeEach`/`afterEach` wiring, which the stdlib type has no
  way to carry.
- **`Fuzz`/`Fuzz2`/`Fuzz3` — a hard arity cap of three.** The generic
  adapters in `pkg/gotest/f.go` cover one, two, or three fuzzed arguments;
  there is no variadic or reflection-based form. A fourth argument needs a
  fourth adapter to be added by hand — not a runtime limitation, just what
  shipped.
- **Generated-name convention: full method name kept.** The wrapper is
  `Fuzz<SuiteIdentifier>_<MethodName>`, e.g. `FuzzParserTestSuite_FuzzParse`
  — not `FuzzParserTestSuite_Parse` as an earlier sketch of this section
  showed before this section was reconciled to match. Every generated name
  in this document now uses the shipped convention.
- **Per-execution lifecycle is unconditional — no `FuzzConfig` escape
  hatch.** The "provide the escape hatch from day one" bullet under
  "Lifecycle semantics decision" below did not ship: `SuiteConfig{Fuzz:
  gotest.FuzzConfig{PerExecutionLifecycle: false}}` does not exist.
  `BeforeEach`/`AfterEach` run before/after every single execution — seed
  replay and every generated input alike — with no opt-out. Revisit if a
  suite's `BeforeEach` doing real I/O turns out to bottleneck fuzzing
  throughput in practice.
- **Seed replay covers every run subcommand, not just the plain runner.**
  `gotest ./...`/`gotest test`, `gotest spec`, `gotest watch`, and `gotest
  summary` all replay each fuzz method's seed corpus (`f.Add` seeds plus
  anything already under `testdata/fuzz/`) as ordinary subtests
  (`Fuzz<Suite>_<Method>/seed#0`). A user-supplied `-run` filter is never
  widened to include fuzz funcs — it wins outright, and a seed replays only
  if the generated name happens to match the filter on its own.
- **Top-level stdlib `func FuzzXxx(*testing.F)` funcs are invisible to
  gotest.** Only `Fuzz*` methods on a `*TestSuite` struct are discovered.
  A bare fuzz function not attached to any suite runs only via `go test`
  directly — `gotest ./...`, `gotest fuzz`, and every other subcommand
  silently skip it. No AST discovery work was done for this case; it is a
  known, documented limitation, not a bug.
- **The orchestrator shares one global `--timeout` across all targets, and
  skips rather than blocks.** `gotest fuzz` has no per-target timeout
  independent of `--for`; when there are more targets than `--jobs`, later
  waves may not acquire a slot before the shared deadline expires, in which
  case gotest prints `[<Func>] skipped: global timeout reached before this
  target started` for each one rather than letting them start anyway and
  run past the deadline. `--for` (which floors each target's slice at 10s)
  is the documented way to give every target an explicit, bounded budget
  instead of relying on `--timeout` alone.

## Part 2: Fuzzing

### Authoring model

`Fuzz*` methods on suites, one generated top-level `Fuzz` function per method
(required — Go's fuzzing engine targets exactly one `FuzzX` symbol per run):

```go
func (s *ParserTestSuite) FuzzParse(f *gotest.F) {
    f.Add("{}")                            // explicit seeds
    gotest.Fuzz(f, func(t *gotest.T, input string) {
        doc, err := s.p.Parse(input)
        if err != nil {
            return                         // rejecting invalid input is fine
        }
        out, err := doc.Marshal()          // property: round-trip
        gotest.NoError(t, err)
        gotest.JSONEq(t, input, out)
    })
}
```

(`gotest.Fuzz(f, func(t *gotest.T, input string) {...})` — the shipped API is a
package-level generic adapter taking `*gotest.F`, not a `Fuzz` method on
`*gotest.F` itself; `Fuzz2`/`Fuzz3` are the two/three-argument forms, capped
at three. See "Shipped deviations (Part 3)" above and the README's "Fuzzing"
section.)

Generated: `func FuzzParserTestSuite_FuzzParse(f *testing.F)` — the full method name
is kept, not stripped of its `Fuzz` prefix — with `BeforeAll` once per
process and `BeforeEach`/`AfterEach` interposed around each execution.

**Lifecycle semantics decision.** Per-execution lifecycle is correct for isolation,
but fuzzing executes millions of times. Run `BeforeEach`/`AfterEach` per execution
by default (isolation is the brand promise), and:

- lint-warn when the suite's `BeforeEach` touches a fixture with I/O, suggesting a
  fuzz-specific suite, and
- provide the escape hatch from day one:
  `SuiteConfig{Fuzz: gotest.FuzzConfig{PerExecutionLifecycle: false}}`.

Two behaviors fall out for free and matter a lot:

- **Seed replay in normal runs.** Under plain `gotest ./...`, generated `Fuzz`
  functions execute their seed corpus as ordinary tests (stock `go test` semantics)
  — every past crasher becomes a permanent regression test with zero extra work.
- **Contract suites fuzz every implementation.** A `Fuzz*` method on a generic
  contract suite generates a fuzz target per implementation — write the property
  once, fuzz the Postgres and in-memory backends alike.

### The AST features

#### 1. Typed fuzzing via generated decoders

Stock `f.Fuzz` rejects structs. The generator has full type info, so when the
callback takes `req CreateUserRequest`, emit a deterministic, reflection-free
decoder and rewrite the target to native form:

```go
f.Fuzz(func(t *testing.T, ƒ_raw []byte) {
    req, ok := ƒ_decode_CreateUserRequest(ƒ_raw)  // generated, byte-reader per field
    if !ok {
        t.Skip()
    }
    ...
})
```

**Corpus stability is a compatibility contract.** Decoding field-by-field in
declared order silently re-interprets the corpus when someone reorders fields.
Emit a length-prefixed, field-index-tagged encoding so reordering and appending
fields keep old corpus entries meaningful; `gotest lint` flags field *removals*
as corpus-invalidating.

#### 2. Seed harvesting from table tests

The collector already parses `gotest.Each` tables and `It` bodies. Extract literal
arguments that flow into the function-under-fuzz (call-site analysis within the
package's tests) and inject them as `f.Add(...)` seeds in the generated wrapper.

Users' table tests are a curated, valid-input corpus that stock fuzzing never
sees — coverage-guided mutation starting from real inputs finds interesting states
far faster than starting from `""`. On by default, opt-out flag.

#### 3. Crash triage that produces spec entries, not hex files

Today a crasher is an opaque `testdata/fuzz/FuzzX/582528ddfad69eb5` file. With the
generated decoder available in both directions:

```bash
gotest fuzz triage
# FuzzParse: 1 new crasher
#   input: CreateUserRequest{Email: "a@\x00", Age: -1}
#   panic: index out of range in normalizeEmail (email.go:42)

gotest fuzz promote
```

`promote` uses the existing `internal/refactor` infrastructure to append a named
regression case to the suite's table (or scaffold an
`It("does not panic on NUL byte in email")` block) — the crasher becomes a
readable, permanent part of the behavioral spec.

#### 4. Inverse-pair scaffolding

`gotest scaffold --fuzz ./pkg/codec.Encode` — the AST can spot symmetric pairs
(`Marshal`/`Unmarshal`, `Parse`/`Format`, `Encode`/`Decode`) by name and signature
and generate a round-trip property skeleton instead of an empty body.

Companion lint rule: a fuzz body with no assertions and no error propagation only
detects panics — suggest a property.

#### 5. Determinism lint

Coverage-guided fuzzing and corpus replay both silently degrade if the target reads
`time.Now()`, `rand.*`, or map-iteration order into its behavior. AST detection of
nondeterminism inside fuzz bodies is a lint no other Go tool offers and directly
protects fuzzing ROI.

### Orchestration — `gotest fuzz`

The scheduling gap in stock tooling: one target per `go test -fuzz` invocation,
manual babysitting. gotest already has the process-pool machinery:

```bash
gotest fuzz ./... --for=10m       # whole module, budgeted
gotest fuzz ./pkg/parser --for=2h # overnight, one package
```

- Discover all targets, schedule one subprocess per target across the core budget
  (each Go fuzz process additionally uses internal workers via `-parallel`; the
  runner divides cores rather than oversubscribing).
- Track accumulated fuzz time per target in the cache dir; prioritize targets that
  are under-fuzzed, have recent crashers, or whose transitive callees changed since
  the last run (git diff + the type graph discovery already builds). "Fuzz what
  changed" for a few minutes pre-merge is a dramatically better spend than
  round-robin.
- `gotest stats` grows a fuzz-health section: corpus size, total fuzz time,
  time-since-last-crasher per target.

## Shipped deviations (Part 4)

Seed harvesting, crash triage/promote, `scaffold --fuzz`, and the three fuzz
lint rules all shipped — every one of them scoped to Go's natively fuzzable
primitive types (string, `[]byte`, bool, and the int/uint/float variants).
Only struct decoders (feature 1 above) did not ship; see below.

- **Seed harvesting is primitives-only, test-file-scoped, and
  conservative-literal.** `HarvestSeeds` (`internal/gotestast/seeds.go`)
  only matches call-sites whose non-`*gotest.T` arguments are basic types —
  struct-typed fuzz callbacks are out of scope, since only primitives have
  `f.Add` codecs. It scans `_test.go` sources only, never production files.
  Within test files it only lifts literal shapes: a basic literal, a
  unary-minus of one, `true`/`false`, or a single-arg conversion of one of
  those — never arbitrary expressions — matched via two shapes: direct
  literal call-sites and `gotest.Each` table rows. It runs on by default;
  disable per-run with `--no-harvest`, or persistently with `fuzz: harvest:
  false` in project config (the CLI flag wins if both are set).
- **Triage and promote are primitives-only, matching harvesting's scope.**
  `gotest fuzz triage` re-runs each package's crasher files
  (`testdata/fuzz/<Func>/...`) as ordinary subtests and reports
  input/cause/status; `gotest fuzz promote` does the same discovery, then
  splices each crasher's arguments into the suite's `Fuzz*` method as a new
  `f.Add(...)` seed via AST-level source editing (`internal/refactor`),
  deleting the crasher file only once the splice succeeds. Both are
  restricted to Go's native primitive corpus types — no struct decoding,
  since no encode/decode codecs exist for them.
- **`gotest scaffold --fuzz` generates one of three skeleton shapes**,
  chosen by what it can determine about the target function:
  1. a round-trip property test, when the target's parameter is natively
     fuzzable and a same-package inverse function (`Marshal`/`Unmarshal`,
     `Encode`/`Decode`, ...) is found;
  2. a crash-safety skeleton, when the parameter is fuzzable but no inverse
     pair was found — it calls the target and leaves a `// TODO: assert an
     invariant beyond "doesn't crash"` comment;
  3. a not-natively-fuzzable TODO stub, when the parameter type isn't one
     of Go's native fuzzable types — struct-typed targets land here today,
     since generating a fuzz call for them would panic at runtime; the body
     is comment-only, explaining that struct fuzzing isn't supported yet.
- **Three fuzz lint rules shipped** (`internal/lint/fuzz.go`): `fuzz-
  determinism` flags fuzz callbacks (and their same-package callees) that
  read `time.Now`, `math/rand`/`math/rand/v2`, or `os.Getenv`; `fuzz-no-
  oracle` flags a fuzz callback whose body never asserts anything through
  its `*gotest.T` parameter (only catching panics, not a real property);
  `fuzz-seed` flags a `Fuzz*` method with no `f.Add` call (harvesting may
  still backfill seeds at generate time regardless).
- **Struct decoders (Task 20) remain a proposal, not shipped.** The design
  below (deterministic, reflection-free, field-index-tagged codecs; a
  `gotest.FuzzBytes` primitive) was reviewed and found to compromise two
  project principles as specced — a reflect-based type registry breaks
  zero-reflection, and a committed field-map file breaks the
  no-committed-generated-files rule — so it was deferred rather than shipped
  as designed. [fuzz-structs.md](fuzz-structs.md) supersedes the sketch below
  with a mechanism that keeps both principles: codecs travel on `*gotest.F`
  (a type assertion, no `reflect`, no globals) and the wire format is a total
  consuming reader with no committed field map. Until that ships, typed fuzzing
  via generated decoders is out of scope for `gotest fuzz`, `triage`,
  `promote`, and `scaffold --fuzz` alike.

---

## Command surface

| Command | Semantics |
|---|---|
| `gotest ./...` | unchanged; fuzz seeds replay as tests, benchmarks skipped |
| `gotest bench` | benchmarks only: serial, isolated, `-benchmem`, baseline save/compare/gate |
| `gotest fuzz --for` | budgeted multi-target orchestration, smart prioritization |
| `gotest fuzz triage` / `promote` | decode crashers, codegen regression cases |
| `gotest spec --bench` | performance + robustness sections in the spec |
| `gotest watch --bench` | re-run on save with deltas |

## Phasing

1. **Bench core** — collector classifier, wrapper codegen, `gotest bench` with
   serial scheduling, spec rendering. Smallest slice, immediately useful,
   exercises every layer once.
2. **Baselines + gates + Action + VS Code lenses** — turns phase 1 into a CI
   product; aligns with the stats/census work.
3. **Fuzz core** — primitive-typed targets, lifecycle wiring, seed replay,
   `gotest fuzz` orchestrator.
4. **Fuzz AST layer** — struct decoders, seed harvesting, triage/promote,
   scaffolding, determinism lint. The differentiating features, built on a
   proven base.

## Risks & open questions

- **Corpus-encoding stability** for struct fuzzing: the tagged encoding solves
  reorder/append, but it is a compatibility contract — version it explicitly.
- **Per-execution lifecycle cost** in fuzz mode: the `FuzzConfig` escape hatch
  must exist from day one.
- **Bench output through the batcher**: the test2json parsing and package-batching
  paths are currently PASS/FAIL-shaped; bench events need their own protocol type
  and rendering.
- **`gotest.B` / `gotest.F` API surface**: keep them thin wrappers (mirroring
  `gotest.T`) so the zero-runtime-cost principle holds.
