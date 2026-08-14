# gotest CLI: Architecture Deep Dive

## High-Level Pipeline

```
 User invokes: gotest ./...
                  │
                  ▼
 ┌─────────────────────────────────────────────────────────────┐
 │  1. DISCOVERY                                               │
 │     packages.Load(./...) → AST Inspector → Suites/Fixtures  │
 └──────────────────────────┬──────────────────────────────────┘
                            │  []*LoadResult (Ptest + Pxtest per pkg)
                            ▼
 ┌─────────────────────────────────────────────────────────────┐
 │  2. CODE GENERATION                                         │
 │     Collector → Resolver → Renderer → overlay.json          │
 │     (Renderer works directly with ResolvedFixture)          │
 └──────────────────────────┬──────────────────────────────────┘
                            │  overlayResult (tmpDir, fixture info)
                            ▼
 ┌─────────────────────────────────────────────────────────────┐
 │  3. PREPARE (concurrent)                                    │
 │                                                             │
 │  ┌─────────────────────┐    ┌────────────────────────────┐  │
 │  │ CompilePackages-    │    │ startSharedFixtures        │  │
 │  │ Stream              │    │   go build → start → read  │  │
 │  │   go test -c per    │    │   JSON state from stdout   │  │
 │  │   pkg (NumCPU par)  │    │   → write state.json       │  │
 │  └────────┬────────────┘    └────────────┬───────────────┘  │
 │           │ <-chan CompileResult         │ *SharedFixtureProcess
 └───────────┼──────────────────────────────┼──────────────────┘
             │    (overlap: exec starts     │
             │     as packages compile)     │
             ▼                              │
 ┌─────────────────────────────────────────────────────────────┐
 │  4. EXECUTION                                               │
 │     Per-suite subprocesses, 2×GOMAXPROCS concurrency        │
 │     Suites needing shared fixtures block on resolveFixture- │
 │     Env(); others start immediately with base env           │
 └──────────────────────────┬──────────────────────────────────┘
                            │  exit code (worst of all suites)
                            ▼
 ┌─────────────────────────────────────────────────────────────┐
 │  5. TEARDOWN                                                │
 │     SIGTERM → shared fixture subprocess                     │
 │     → AfterAll per fixture (reverse order) → exit           │
 └─────────────────────────────────────────────────────────────┘
```

---

## 1. Discovery

Discovery is fully static -- AST-based, no reflection at runtime.

### Package Loading

```
cli.go:runTest() / discover.go:runDiscover()
  │
  ├─ LoadPackages(patterns, buildFlags)        [exec path]
  │    mode: NeedModule | NeedSyntax | NeedName | NeedTypes
  │           | NeedTypesInfo | NeedImports | NeedDeps
  │
  └─ LoadPackagesForDiscovery(patterns, ...)   [discover path]
       mode: same but NeedFiles instead of NeedDeps
       (avoids type-checking the full transitive dep graph)
```

`packages.Load()` with `Tests: true` returns both internal-test (`pkg_test.go`
in the same package) and external-test (`pkg_test.go` in `pkg_test` package)
packages. These are grouped into `LoadResult` structs:

```
LoadResult {
    PkgDir    "/path/to/pkg"
    PkgPath   "github.com/example/repo/auth"
    Ptest     *packages.Package   ← internal test package (same pkg name)
    Pxtest    *packages.Package   ← external test package (pkg_test name)
}
```

### Suite Detection (AST)

The `collector.CollectSuiteSpecs(pkg)` orchestrates a multi-pass AST traversal
using `go/ast/inspector.Inspector.Preorder()`:

```
Pass 1: Find Suites
  ├─ Walk GenDecl nodes
  ├─ Match: ^(?!ƒƒ_GOTEST_|_)(?:X_|F_)?.+TestSuite$
  │         (excludes generated wrappers and leading underscores)
  ├─ Must be a struct type (resolve through aliases for generics)
  └─ Result: []*TestSuiteSpec

Pass 2: Find Fixtures
  ├─ Walk GenDecl nodes
  ├─ Match suffix: *SharedFixture → SharedFixture kind
  │                *Fixture        → PackageFixture kind
  └─ Result: []*FixtureSpec

Pass 3: Find Suite Methods (per suite)
  ├─ Walk FuncDecl nodes
  ├─ Match pointer receiver to suite type
  ├─ Classify by name:
  │   ├─ BeforeAll / AfterAll / BeforeEach / AfterEach
  │   ├─ SuiteConfig / SuiteGuard
  │   └─ ^(X_|F_)?Test.+$ → test case method
  ├─ Validate signatures (param types, return types)
  └─ Parse SuiteConfig AST for Parallel: true

Pass 4: Validate Context Consistency
  ├─ If BeforeEach returns a context type:
  │   all test methods MUST accept it as 2nd param
  │   AfterEach MUST accept it as 2nd param
  └─ Parallel: true requires returning BeforeEach

Pass 5: Find Fixture Methods (per fixture)
  ├─ Lifecycle: BeforeAll, AfterAll, BeforeEach, AfterEach
  ├─ Config: FixtureConfig() or SharedFixtureConfig()
  ├─ Hydrate/Dehydrate (shared fixtures only)
  └─ Validate signatures: all must be (ctx context.Context) error

Pass 6: Validate Fixture Consistency
  └─ Hydrate and Dehydrate must appear together (or not at all)
```

### Focus/Exclude System

```
Name prefix → behavior:
  F_FooTestSuite     → focused  (only focused suites run)
  X_BarTestSuite     → excluded (always skipped)
  F_TestCreateUser   → focused method within suite
  X_TestDeleteUser   → excluded method within suite

ReduceToEffectiveSet() logic:
  1. If any suite has F_ prefix → run ONLY focused suites, skip rest
  2. Remove all X_ prefixed suites
  3. Within each surviving suite, apply same logic to methods
```

---

## 2. Code Generation

The resolver walks the type graph starting from discovered suites,
building a fixture tree and collecting shared fixture references.

### Fixture Resolution

```
Resolve(targetPkg, suites, localFixtures)
  │
  for each suite:
  │
  ├─ Inspect struct fields:
  │   ├─ *FooSharedFixture → build SharedFixtureRef
  │   │   ├─ Validate: not in internal/ package
  │   │   ├─ Register in sharedSeen map (deduped by pkgPath.Name)
  │   │   └─ Classify transfer vs local fields via Hydrate AST
  │   │
  │   └─ *BarFixture → resolveFixture(named)
  │       ├─ Check resolved cache (avoid re-resolving)
  │       ├─ Cycle detection via resolving map
  │       ├─ Lookup method set: BeforeAll (required), AfterAll,
  │       │   BeforeEach, AfterEach, Hydrate, Dehydrate, Config
  │       └─ Recurse into fixture struct fields for:
  │           ├─ Parent fixtures (zero or more, builds DAG)
  │           └─ SharedFixture references
  │
  ├─ Suite gets linked: fixture.ChildSuites ← append(suite)
  └─ Suite categorized: FixtureBound or Standalone
```

Fixtures form a **DAG** (directed acyclic graph) via embedding: a fixture may
have multiple parents, and suites may reference multiple fixtures. The same
fixture type is deduplicated by identity so each fixture is set up exactly once.

The resolver produces both `RootFixtures` (entry points with no parents) and
`AllFixtures` (every fixture in topological order). The renderer uses
`AllFixtures` to emit a flat list with `DependsOn` edges for the fixture
DAG initializer.

### What Gets Generated

For **standalone suites** (no fixture):

```go
// Generated wrapper struct
type ƒƒ_GOTEST_FooTestSuite struct { FooTestSuite }

// Generated test function
func TestFooTestSuite(t *testing.T) {
    s := &ƒƒ_GOTEST_FooTestSuite{}
    // optional: read shared state from GOTEST_SHARED_STATE_FILE
    // apply SuiteConfig, deadlines
    s.BeforeAll(setupT)
    t.Cleanup(func() { s.AfterAll(teardownT) })

    t.Run("TestCreateUser", func(it *testing.T) {
        s.BeforeEach(ttt)
        defer s.AfterEach(ttt)
        s.TestCreateUser(ttt)
    })
}
```

For **fixture-bound suites** (have a fixture), a lazy initializer wrapping
`sync.Once` is generated. Each `TestX` function calls it — the DAG runs
at most once per test binary:

```go
var ƒ_fixtureOnce gotestruntime.FixtureOnce
var ƒ_fixtureDAG *gotestruntime.FixtureDAG

func ƒ_setupFixtures(t *testing.T) {
    if err := ƒ_fixtureOnce.Do(func() error {
        // Fixtures is a flat list with DependsOn edges forming a DAG.
        // 1. Read shared fixture state (if needed)
        // 2. DAG wavefront setup via SetupFixtureDAG:
        //    no-dependency fixtures start concurrently;
        //    each fixture waits for its DependsOn set
        // 3. BeforeAll on each fixture (retries, timeout)
        // 4. Compute teardown budget → write budget file
        var err error
        ƒ_fixtureDAG, err = gotestruntime.SetupFixtureDAG(ctx, cfg)
        return err
    }); err != nil {
        t.Fatalf("fixture setup: %v", err)
    }
    t.Cleanup(func() { ƒ_fixtureDAG.Teardown() })
}

func TestQueryTestSuite(t *testing.T) {
    ƒ_setupFixtures(t)      // idempotent — DAG setup runs at most once
    s := &ƒƒ_GOTEST_QueryTestSuite{...}
    // ... suite lifecycle (same as standalone) ...
}
```

No `TestMain` is generated — both `package foo` and `package foo_test`
can define fixture-bound suites without conflict. Teardown runs via
`t.Cleanup` (reverse-wavefront: leaves first, roots last).

For suites with `Fuzz*` methods, codegen emits one top-level `FuzzX`
function **per method** — not one wrapper with sub-tests like `Benchmark<Suite>`
above. Go's fuzzing engine targets exactly one `FuzzX` symbol per invocation,
so there is no shared enclosing scope to sub-test into; each generated
function independently repeats fixture setup and `BeforeAll`:

```go
func FuzzFooTestSuite_FuzzParse(f *testing.F) {
    ƒ_setupFixtures(f)                    // only if fixture-bound
    s := &ƒƒ_GOTEST_FooTestSuite{...}
    ƒlifecycleT := gotest.NewTFromTB(f)
    f.Cleanup(func() { s.AfterAll(gotest.NewTFromTB(f)) })
    s.BeforeAll(ƒlifecycleT)
    s.FuzzParse(gotest.NewF(f, s.BeforeEach, s.AfterEach))
}
```

Naming: `Fuzz<SuiteIdentifier>_<MethodName>`, full method name kept (e.g.
`FuzzParserTestSuite_FuzzParse`, not `FuzzParserTestSuite_Parse`) — the
`gotest.NewF(f, s.BeforeEach, s.AfterEach)` call is what wires per-execution
lifecycle interposition into the generic `gotest.Fuzz`/`Fuzz2`/`Fuzz3`
adapters (`pkg/gotest/f.go`): `BeforeEach` runs immediately before each
execution's body, `AfterEach` is deferred to run immediately after,
for every seed replay and every generated input — not once for the whole
target.

When seed harvesting is enabled (`gotestast.HarvestSeeds`, on by default —
see the README's "Seed harvesting" section), the renderer additionally
emits one `f.Add(...)` line per harvested literal directly in the wrapper,
after `BeforeAll` and before the user method call, so harvested seeds
replay through the exact same `f.Add` path as hand-written ones.

A suite must be named `*TestSuite` for its `Fuzz*` methods to be collected,
same as benchmarks. `ValidateContextConsistency` rejects a fuzz suite with a
returning `BeforeEach`, any lifecycle hook still typed `*testing.T`, and (via
the resolver) a fixture with per-method `BeforeEach`/`AfterEach` bound to it —
mirroring the three benchmark rejections above with fuzz-specific error text.
Unlike benchmarks, a `Fuzz*` method's parameter type is checked more strictly:
only `*gotest.F` is accepted, never `*testing.F` — the collector rejects the
stdlib type outright (`detectParamF` in `internal/gotestast/spec.go`, no
`usesStdlibT` escape hatch the way `detectParamB` has one for `*testing.B`).

The overlay filesystem (`-overlay=path/overlay.json`) injects generated files
without modifying source. Go's compiler reads virtual paths from the overlay.

---

## 3. Fixtures: Lifecycle Models

### PackageFixture Lifecycle

```
┌──────────────────── PER PACKAGE (lazy fixture init) ────────────────┐
│                                                                      │
│  DAG wavefront setup (channel-based parallel scheduling):            │
│                                                                      │
│  wave 0 (no dependencies):                                           │
│  ┌─ FixtureA.BeforeAll(ctx) ─┐  ┌─ FixtureB.BeforeAll(ctx) ─┐     │
│  │  timeout: Config.Timeout  │  │  timeout: Config.Timeout   │     │
│  │  retries: Config.Retries  │  │  retries: Config.Retries   │     │
│  └───────────┬───────────────┘  └───────────┬────────────────┘     │
│              │ done(ch)                      │ done(ch)              │
│              └──────────┬────────────────────┘                      │
│                         ▼                                            │
│  wave 1 (depends on A and B):                                        │
│  ┌─ FixtureC.BeforeAll(ctx) ────────────────────────────────────┐   │
│  │  blocks until all DependsOn channels signal                  │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌──── PER SUITE (each TestXxxTestSuite function) ──────────────┐   │
│  │                                                               │   │
│  │  Suite.BeforeAll(setupT)    ← deadline: SuiteConfig.SetupTimeout │
│  │                                                               │   │
│  │  ┌──── PER TEST METHOD (t.Run subtests) ──────────────────┐  │   │
│  │  │                                                         │  │   │
│  │  │  Fixture.BeforeEach(ctx)  ← if fixture has it          │  │   │
│  │  │  Suite.BeforeEach(ttt)    ← if suite has it            │  │   │
│  │  │  Suite.TestXxx(ttt)       ← deadline: SuiteConfig.Timeout│ │   │
│  │  │  Suite.AfterEach(ttt)     ← deferred                   │  │   │
│  │  │  Fixture.AfterEach(ctx)   ← deferred, if fixture has it│  │   │
│  │  │                                                         │  │   │
│  │  └─────────────────────────────────────────────────────────┘  │   │
│  │                                                               │   │
│  │  Suite.AfterAll(teardownT)  ← deferred via t.Cleanup         │   │
│  │                                                               │   │
│  └───────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  t.Cleanup: reverse-wavefront teardown (idempotent via sync.Once)    │
│    wave 0: FixtureC.AfterAll(ctx)  ← leaves first                   │
│    wave 1: FixtureA.AfterAll(ctx), FixtureB.AfterAll(ctx)  ← conc.  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### SharedFixture Lifecycle

SharedFixtures live in a **separate subprocess** that outlives all test
binaries. They form a DAG via pointer fields to other SharedFixture types.
State is transferred via JSON serialization, streamed as each fixture completes.

```
┌── SETUP SUBPROCESS ──────────────────────────────────────────────────┐
│                                                                       │
│  DAG-ordered BeforeAll (parents before children, independent conc.):  │
│                                                                       │
│  wave 0 (no dependencies):                                            │
│  ┌─ PostgresSF.BeforeAll(ctx) ─┐  ┌─ RedisSF.BeforeAll(ctx) ─┐      │
│  └───────────┬─────────────────┘  └──────────┬────────────────┘      │
│              │ emit JSON state               │ emit JSON state        │
│              └──────────┬────────────────────┘                        │
│                         ▼                                              │
│  wave 1 (depends on Postgres):                                        │
│  ┌─ SchemaSF.BeforeAll(ctx) ────────────────────────────────────┐     │
│  └──────────┬───────────────────────────────────────────────────┘     │
│             │ emit JSON state                                         │
│                                                                       │
│  Each fixture's state emitted to stdout immediately after BeforeAll   │
│  Include _teardownBudget = sum of SupervisorBudget(Timeout) + 30s     │
│  (teardown below is sequential, so the members' budgets add; the      │
│  budget rides both _done forms — a failed setup still has succeeded   │
│  siblings to release under it)                                        │
│                                                                       │
│  ═══════════ block on SIGTERM/SIGINT ═══════════                      │
│                                                                       │
│  for each SharedFixture (reverse DAG order):                          │
│    sf.AfterAll(ctx)      timeout: SharedFixtureConfig.Timeout         │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘

         │ stdout: JSON state       │ per-suite state.json files
         ▼                          ▼

┌── TEST PROCESS (per suite binary) ───────────────────────────────────┐
│                                                                       │
│  Read GOTEST_SHARED_STATE_FILE (contains only this suite's entries)   │
│  json.Unmarshal → SharedFixture struct (transfer fields only)         │
│  sf.Hydrate(ctx)    ← reconstruct local fields (e.g., DB handles)    │
│  t.Cleanup → sf.Dehydrate(ctx)   ← release local resources           │
│                                                                       │
│  Suite runs with shared fixture pointer available on its struct       │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

**Transfer vs Local fields:**
- Transfer fields: exported, JSON-serializable, sent across processes.
  These should be stable connection parameters (host, port, credentials),
  not ephemeral handles or runtime state.
- Local fields: assigned inside `Hydrate()`, reconstructed in each test
  process (e.g., `*sql.DB` handles that can't serialize)

**Design intent:** the state file is a connection-parameter snapshot, not a
general data bus. `Hydrate()` turns those parameters into live connections;
`Dehydrate()` releases them. This keeps fixture state deterministic across
the N test processes that read the same snapshot.

The `ClassifyLocalFieldsRaw()` function performs AST analysis on the Hydrate
method body to determine which exported fields are assigned (and therefore
"local"). It also follows one level of receiver method calls
(e.g., `f.connect()` → inspects `connect` body for assignments).

---

## 4. Process Parallelism

The system has **four levels of parallelism**, each with distinct mechanisms:

```
┌─────────────────────────────────────────────────────────────────────┐
│ Level 1: Package COMPILATION            semaphore: runtime.NumCPU()│
│                                                                     │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐                                  │
│  │ go  │ │ go  │ │ go  │ │ go  │  ← goroutines, each runs         │
│  │test │ │test │ │test │ │test │    `go test -c -overlay=...`      │
│  │ -c  │ │ -c  │ │ -c  │ │ -c  │                                  │
│  │pkg1 │ │pkg2 │ │pkg3 │ │pkg4 │                                  │
│  └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘                                  │
│     │       │       │       │                                       │
│     └───────┴───────┴───────┘                                       │
│               │                                                     │
│               ▼  CompileResult streamed to channel                  │
│                  (execution starts as each package finishes)        │
├─────────────────────────────────────────────────────────────────────┤
│ Level 2: Shared fixture SETUP           concurrent with compilation│
│                                                                     │
│  ┌──────────────────────────────┐                                   │
│  │ setup subprocess             │  ← single process                 │
│  │  sf0.BeforeAll() ──┐        │    but fixture BeforeAll calls     │
│  │  sf1.BeforeAll() ──┤ (conc) │    run concurrently inside it      │
│  │  sf2.BeforeAll() ──┘        │                                    │
│  └──────────────────────────────┘                                   │
├─────────────────────────────────────────────────────────────────────┤
│ Level 3: Suite EXECUTION           semaphore: 2×GOMAXPROCS(0)      │
│                                                                     │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐           │
│  │suite │ │suite │ │suite │ │suite │ │suite │ │suite │  ← OS      │
│  │proc  │ │proc  │ │proc  │ │proc  │ │proc  │ │proc  │    procs   │
│  │(bin  │ │(bin  │ │(bin  │ │(bin  │ │(bin  │ │(bin  │            │
│  │ -run │ │ -run │ │ -run │ │ -run │ │ -run │ │ -run │            │
│  │ ^A$) │ │ ^B$) │ │ ^C$) │ │ ^D$) │ │ ^E$) │ │ ^F$) │           │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘           │
│  Each suite = separate OS subprocess with own process group         │
│  One compiled binary may serve multiple suites (diff -test.run)     │
├─────────────────────────────────────────────────────────────────────┤
│ Level 4: Within-suite test PARALLELISM     (optional, in-process)  │
│                                                                     │
│  Enabled by: SuiteConfig{ Parallel: true }                          │
│  Requires:   BeforeEach returns per-test context struct             │
│  Mechanism:  it.Parallel(); AfterAll via t.Cleanup                  │
│  Concurrency governed by `-test.parallel` (default: GOMAXPROCS)     │
│                                                                     │
│  ┌─ Suite subprocess ──────────────────────────────────────────┐    │
│  │ BeforeAll()                                                  │    │
│  │ t.Run("TestA", func() { it.Parallel(); ctx := BeforeEach(); │    │
│  │                          TestA(t, ctx); AfterEach(t, ctx) }) │    │
│  │ t.Run("TestB", func() { it.Parallel(); ctx := BeforeEach(); │    │
│  │                          TestB(t, ctx); AfterEach(t, ctx) }) │    │
│  │ (testing waits for all parallel subtests before cleanup)     │    │
│  │ AfterAll()                                                   │    │
│  └──────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

### Fuzz mode: subprocess-per-target, not binary reuse

`gotest fuzz` (`internal/gotestrunner/fuzzrun.go`) does not fit into Levels
1–3 above at all — it deliberately does **not** reuse the compiled suite
binaries `BuildSuiteTargets`/`CompilePackagesStream` produce for every other
subcommand. The reason is a `cmd/go` constraint, not a gotest design choice:
native fuzz instrumentation (the coverage counters `go test -fuzz` needs to
drive its mutation engine) is woven in by `cmd/go` at `go test` invocation
time, only when `-fuzz` is present on *that* invocation. A binary from a
plain `go test -c` (no `-fuzz`) — exactly what every other gotest subcommand
compiles once and reuses across suites — has no such instrumentation baked
in; running it with `-test.fuzz` would silently degrade to undirected random
input with no crash minimization.

So `RunFuzzTargets` spawns one `go test -fuzz=<Func>` subprocess per
generated `Fuzz<Suite>_<Method>` target instead, each doing its own
build+instrument+run via `cmd/go`:

```
RunFuzzTargets(ctx, targets, cfg)
  │
  jobs := resolveFuzzJobs(cfg.Jobs)        // default max(1, GOMAXPROCS/2)
  budget := splitBudget(cfg.Total, len(targets))  // even split, 10s floor
  │
  for each target (bounded by a jobs-sized semaphore):
    go test <overlay-flag> -run=^$ -fuzz=^<quoted Func>$ [-fuzztime=<budget>] <buildFlags> <pkg>
      │
      stdout/stderr → lineWriter → os.Stdout/os.Stderr, each line
      prefixed "[<Func>] ", flushed live (not buffered to end-of-run)
      │
      testdata/fuzz/<Func>/ snapshotted before and after the run;
      new entries are reported as "[<Func>] new crasher: <path>"
  │
  returns per-target outcomes (exit code, canceled?, skipped?, new crashers);
  the session exit code is the worst *effective* code: a target stopped by
  the deadline or an interrupt without a finding maps to 0, a new crasher
  file maps to 1 even if the shutdown killed the process mid-crash
```

This is slower than the binary-reuse path everywhere else in gotest — every
target pays its own compile cost — but it is the only way to get real,
coverage-guided fuzzing. A target whose semaphore slot doesn't open before
the context is cancelled (all targets share the pipeline's one global
`--timeout`) is skipped with `[<Func>] skipped: session ended before this
target started` rather than silently dropped; `--for` gives every
target its own explicit budget so a run with more targets than `--jobs`
doesn't starve later waves. Time exhaustion never fails the session by
itself: findings do.

### Streaming Execution (Compile-Execute Overlap)

`RunPipeline` with `Streaming: true` is the primary execution path. It overlaps
compilation with test execution:

```
time ─────────────────────────────────────────────────────────────────▶

CompilePackagesStream:
  [compile pkg1] [compile pkg2] [compile pkg3] [compile pkg4]
       │              │              │              │
       ▼              ▼              │              │
  [run suites    [run suites     │              │
   from pkg1]    from pkg2]      ▼              │
                            [run suites         ▼
                             from pkg3]    [run suites
                                            from pkg4]

Each CompileResult immediately produces SuiteTargets that enter
the execution semaphore. No "compile all, then run all" barrier.
```

### Per-Suite Fixture Readiness

Each suite has a set of shared fixture keys it needs (including transitive
dependencies). The runner tracks which fixtures have emitted their state
and dispatches suites as soon as their specific dependencies are all ready:

```
Suite goroutine:
  │
  ├─ needsFixture? ──yes──▶ waitForFixtureKeys(suite.SharedFixtureKeys)
  │                              │
  │                              ├─ Blocks until all keys in the suite's
  │                              │   dependency set have emitted state
  │                              ├─ Writes per-suite state file (only
  │                              │   the entries this suite needs)
  │                              ├─ Returns env with GOTEST_SHARED_STATE_FILE
  │                              └─ Error? → streamCancel(), return
  │
  └─ needsFixture? ──no───▶ use baseEnv directly (no blocking)
```

Suites that only need `PostgresSharedFixture` start as soon as Postgres
is ready — they don't wait for `SchemaSharedFixture` or other unrelated
fixtures. This reduces wall-clock time when fixtures have different
startup durations.

---

## 5. Timeout Architecture

```
┌────────────────────── Timeout Hierarchy ──────────────────────────────┐
│                                                                        │
│  --timeout (CLI flag)                                                  │
│  └─ Global pipeline deadline (covers compile + setup + test + teardown)│
│     Default: 15m (0 to disable)                                        │
│                                                                        │
│  --setup-timeout (CLI flag)                                            │
│  └─ Total budget for shared fixture subprocess to emit JSON state      │
│     Default: 2m (0 to disable)                                         │
│                                                                        │
│  SharedFixtureConfig.Timeout (per shared fixture)                      │
│  └─ Deadline on each BeforeAll/AfterAll call in the setup subprocess   │
│     Default: 2m (DefaultFixtureConfig)                                 │
│     Container preset: 5m + 1 retry + 5s delay (ContainerFixtureConfig) │
│                                                                        │
│  FixtureConfig.Timeout (per package fixture)                           │
│  └─ Deadline on BeforeAll/AfterAll during fixture setup                │
│     Default: 2m                                                        │
│                                                                        │
│  SuiteConfig.SetupTimeout (per suite)                                  │
│  └─ Deadline on suite-level BeforeAll/AfterAll                         │
│     Default: 30s                                                       │
│                                                                        │
│  SuiteConfig.Timeout (per test method)                                 │
│  └─ Deadline on each t.Run subtest body                                │
│     Default: 30s                                                       │
│                                                                        │
│  Teardown Budget (per suite subprocess)                                │
│  └─ Written to BudgetFile after fixture setup completes                │
│     = max(fixture tree path timeout) + max(suite setup timeout) + 30s  │
│     Used by RunSingleSuite on context cancellation before SIGKILL      │
│                                                                        │
│  GracefulShutdownDelay (hardcoded: 5m30s)                              │
│  └─ Fallback when no budget file exists                                │
│     Must cover longest possible fixture teardown                       │
│                                                                        │
│  BuildShutdownDelay (hardcoded: 10s)                                   │
│  └─ WaitDelay for go build / go test -c compile commands               │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

### Timeout Enforcement Points

```
                     ┌─────────────────────────┐
                     │   CLI invocation         │
                     │   signal.NotifyContext   │
                     │   (SIGINT, SIGTERM)      │
                     └───────────┬─────────────┘
                                 │ ctx
                    ┌────────────┼────────────┐
                    │            │            │
                    ▼            ▼            ▼
            ┌──────────┐  ┌──────────┐  ┌──────────┐
            │ compile  │  │ shared   │  │ suite    │
            │ cmd      │  │ fixture  │  │ cmd      │
            │          │  │ cmd      │  │          │
            │ Cancel:  │  │ Cancel:  │  │ Cancel:  │
            │ SIGTERM  │  │ SIGTERM  │  │ SIGTERM  │
            │ →pgroup  │  │ →pgroup  │  │ →pgroup  │
            │          │  │          │  │          │
            │ WaitDly: │  │ WaitDly: │  │ WaitDly: │
            │ 10s      │  │ 0 (mgd)  │  │ 0 (mgd)  │
            └──────────┘  └──────────┘  └──────────┘
                                             │
                                 On ctx.Done():
                                 ┌───────────┘
                                 │
                                 ▼
                          read budget file
                          (or use 5m30s default)
                                 │
                                 ▼
                          time.After(budget)
                                 │
                          still running?
                                 │
                         yes ────┼──── no
                          │             │
                   ForceKillProcess   normal
                   Group(SIGKILL)     exit
```

### Process Group Isolation

All subprocesses are managed by `ManagedProcess`, which sets `Setpgid: true`
internally and handles the full lifecycle: start, signal (SIGTERM to the
process group via negative PID), grace period, and escalation to SIGKILL.
Three grace strategies control post-signal behavior:

- `GraceFixed` — wait a fixed duration (e.g., 10s for compile commands).
- `GraceBudget` — read the teardown budget from a sidecar file at runtime.
- `GraceKill` — skip SIGTERM and send SIGKILL immediately.

This ensures that when a suite subprocess is terminated, all its child
processes (e.g., processes spawned by tests) are also killed. SIGTERM goes
to the entire group, not just the leader process.

---

## 6. Output Modes

```
┌─ RunBatchText (default) ────────────────────────────────────────────┐
│                                                                      │
│  PackageBatcher collects results per package.                        │
│  When ALL suites for a package complete → flush in deterministic     │
│  order (by registration index, not completion time).                 │
│                                                                      │
│  Trailing PASS/FAIL line stripped from each suite, replaced with     │
│  a single package summary:  ok  pkg/path  1.234s                    │
│                                                                      │
├─ RunStreamJSON (-json flag) ─────────────────────────────────────────┤
│                                                                      │
│  Each suite wrapped with `go tool test2json -p <pkg> -t <binary>`    │
│  JSON events streamed to stdout as each suite completes.             │
│  No batching; order depends on completion time.                      │
│                                                                      │
├─ RunCaptureJSON (internal) ───────────────────────────────────────────┤
│                                                                      │
│  Same as StreamJSON but captured into bytes.Buffer.                   │
│  Nothing written to stdout. Used by watch, spec, and summary modes.  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 7. Suite Target Construction

```
BuildSuiteTargets(compiled, suitesByPkg, dirsByPkg, runFlags, userRunFilter)
  │
  for each package:
    for each suite struct name (e.g., "FooTestSuite"):
      │
      ├─ testFuncName = "Test" + "FooTestSuite" = "TestFooTestSuite"
      │
      ├─ If userRunFilter set:
      │   ├─ Split filter on first "/" → topLevel / subtest
      │   ├─ Match topLevel regex against testFuncName
      │   └─ Skip suite if no match
      │
      └─ SuiteTarget {
           Package:    "github.com/example/auth"
           Dir:        "/path/to/auth"
           BinaryPath: "/tmp/gotest/bin/auth_a1b2c3d4.test"
           SuiteName:  "TestFooTestSuite"
           RunFilter:  "" or user's -run value
           RunFlags:   ["-test.v", "-test.timeout=30s", ...]
           CoverProfile: "/tmp/gotest/cover/0.out" (if -coverprofile)
           BudgetFile:   "/tmp/gotest/bin/auth_a1b2c3d4.test.budget"
         }
```

Each target becomes one `exec.Command`:
```
go tool test2json -p <pkg> -t <binary> -test.run=^TestFooTestSuite$ [flags]
```

Or without test2json:
```
<binary> -test.run=^TestFooTestSuite$ [flags]
```

### Fuzz seed replay: run-filter alternation

`BuildSuiteTargets` (the plain, non-bench path) also takes a
`fuzzFuncsByPkg map[string]map[string][]string` parameter and attaches each
suite's generated `Fuzz<Suite>_<Method>` names to `SuiteTarget.FuzzFuncs`.
`buildSuiteCmd` branches on this in `-test.run` construction, in priority
order:

```
target.Bench            → "-test.run=^$" -test.bench=^Benchmark<Suite>$"
target.RunFilter != ""   → "-test.run=" + RunFilter            (user wins, untouched)
len(target.FuzzFuncs)>0  → "-test.run=^(?:<Suite>|<Fuzz1>|<Fuzz2>|...)$"  (widened)
default                  → "-test.run=^<Suite>$"
```

A user-supplied `-run` filter is never widened to also include the suite's
fuzz funcs — it becomes the entire `-test.run` value as-is, so a fuzz
target's seed corpus only replays under a user filter if the generated
`Fuzz<Suite>_<Method>` name happens to match that filter on its own merit
(controller ruling: user filter wins over replay widening). With no user
filter, the alternation runs the suite's `Test<Suite>` function and every
one of its fuzz funcs' seed corpora as ordinary subtests
(`Fuzz<Suite>_<Method>/seed#0`) in the same subprocess — this is what makes
`gotest ./...` — and `spec`/`watch`/`summary`, whose `PipelineConfig`
literals thread `FuzzFuncsByPkg` the same way — replay fuzz seeds for free,
at zero `-fuzz` cost.

---

## 8. Full Timeline (Happy Path)

```
t=0s    CLI starts
        ├─ Parse flags, load config
        ├─ LoadPackages(./...)
        ├─ GenerateOverlay → overlay.json
        ├─ signal.NotifyContext (SIGINT/SIGTERM → ctx cancel)
        │
t=0.5s  RunPipeline begins (Streaming: true)
        ├─ Start goroutine: CompilePackagesStream → compileCh
        ├─ Start goroutine: startSharedFixtures (if any)
        │
t=1s    pkg/auth compiles first → CompileResult on compileCh
        ├─ BuildSuiteTargets → [AuthTestSuite, TokenTestSuite]
        ├─ AuthTestSuite needs shared fixtures → blocks on resolveFixtureEnv()
        └─ TokenTestSuite doesn't → acquires sem slot, starts subprocess
           └─ ./auth_hash.test -test.run=^TestTokenTestSuite$ -test.v=test2json
        │
t=2s    pkg/cart compiles → 2 more suites start
        │
t=3s    Shared fixture setup completes → JSON state written
        ├─ resolveFixtureEnv() unblocks
        └─ AuthTestSuite now starts (had been waiting)
        │
t=4s    TokenTestSuite finishes (exit 0)
        ├─ batcher.Record("pkg/auth", 1, result) → not all done yet
        │
t=5s    AuthTestSuite finishes (exit 0)
        ├─ batcher.Record("pkg/auth", 0, result) → all done!
        └─ batcher.Flush("pkg/auth") → print both suites, package summary
        │
t=8s    All suites done, wg.Wait() returns
        ├─ setupProc.Teardown()
        │   ├─ TerminateProcessGroup(pid) → SIGTERM
        │   └─ Shared fixture subprocess runs AfterAll, exits
        └─ Return worst exit code
```

---

## Key Design Decisions

1. **AST over reflection**: Discovery is compile-time, not runtime. This
   enables static analysis, IDE integration (`gotest discover` JSON), and
   code generation without running test code.

2. **One subprocess per suite**: Each suite runs in complete isolation. A
   panicking suite cannot affect others. The OS enforces memory isolation.

3. **Overlay filesystem**: Generated code is injected via Go's `-overlay` flag.
   Source files are never modified. Overlays are written to a content-addressable
   cache (`~/.cache/gotest/overlays/<hash>/`) for reuse across runs. Cache entries
   auto-evict after 7 days. Use `--no-cache` to force fresh generation.

4. **Streaming compilation**: `CompilePackagesStream` sends results to a
   channel as each package finishes. Test execution begins before all packages
   are compiled, reducing total wall-clock time.

5. **Process groups via ManagedProcess**: Every subprocess is wrapped in a
   `ManagedProcess` that sets `Setpgid: true` internally. Signals target
   the group (negative PID), ensuring child processes spawned by tests are
   also cleaned up. Grace period strategy is configuration, not per-callsite
   reimplementation.

6. **Budget-based teardown**: Each suite subprocess computes its own teardown
   budget (max fixture timeout + max suite setup timeout + 30s headroom) and
   writes it to a sidecar file. The runner reads this before deciding when to
   escalate from SIGTERM to SIGKILL.

7. **No coverage interception needed**: Fixture setup no longer uses
   `TestMain` + `os.Exit(m.Run())`, so Go's coverage machinery works
   without interception. The test binary exits normally via `t.Cleanup`.

8. **Unified pipeline entry point**: `cmd/gotest` is a thin CLI shell that
   delegates to `internal/gotestrunner.RunPipeline`. The pipeline encapsulates
   the compile -> fixture-setup -> execute -> teardown -> output flow and
   supports both streaming (`Streaming: true`) and batch modes.
