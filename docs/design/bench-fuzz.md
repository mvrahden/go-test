# Benchmarking & Fuzzing

> Status: Part 1 (benchmarking core) shipped; Parts 2-4 remain proposals.
> See the README's "Benchmarking" section and ARCHITECTURE.md for what
> `gotest bench` actually does today. The rest of this document — baselines,
> gates, editor integration, and all of fuzzing — is still a design proposal.

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

Today, `-bench`/`-fuzz` flags are classified in `internal/gotestrunner/args.go` and
forwarded to the test binary — but generated suites never emit `Benchmark*`/`Fuzz*`
functions, so those flags do nothing useful.

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

## Part 2: Fuzzing

### Authoring model

`Fuzz*` methods on suites, one generated top-level `Fuzz` function per method
(required — Go's fuzzing engine targets exactly one `FuzzX` symbol per run):

```go
func (s *ParserTestSuite) FuzzParse(f *gotest.F) {
    f.Add("{}")                            // explicit seeds
    f.Fuzz(func(t *gotest.T, input string) {
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

Generated: `func FuzzParserTestSuite_Parse(f *testing.F)` with `BeforeAll` once per
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
