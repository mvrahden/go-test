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
 │           │ <-chan CompileResult          │ *SharedFixtureProcess
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
  │           ├─ Parent fixture (at most one, builds tree)
  │           └─ SharedFixture references
  │
  ├─ Suite gets linked: fixture.ChildSuites ← append(suite)
  └─ Suite categorized: FixtureBound or Standalone
```

Constraint: at most **one root PackageFixture per package**.
Fixtures form a single-inheritance tree (parent via embedding).

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

For **fixture-bound suites** (have a fixture), a `TestMain` is generated:

```go
func TestMain(m *testing.M) { os.Exit(ƒƒ_GOTEST_main(m)) }

func ƒƒ_GOTEST_main(m *testing.M) (code int) {
    // 1. Read shared fixture state (if needed)
    // 2. Instantiate root fixture
    // 3. BeforeAll on root fixture (with retries, timeout)
    // 4. Instantiate child fixtures (concurrent)
    // 5. BeforeAll on each child fixture (concurrent, with retries)
    // 6. Compute teardown budget → write to budget file
    // 7. code = m.Run()   ← runs all TestXxx functions
    // 8. defer: AfterAll children (concurrent), then AfterAll root
}
```

The overlay filesystem (`-overlay=path/overlay.json`) injects generated files
without modifying source. Go's compiler reads virtual paths from the overlay.

---

## 3. Fixtures: Lifecycle Models

### PackageFixture Lifecycle

```
┌──────────────────── PER PACKAGE (via TestMain) ─────────────────────┐
│                                                                      │
│  ┌─ RootFixture.BeforeAll(ctx) ─────────────────────────────────┐   │
│  │  timeout: FixtureConfig.Timeout (default 2m)                 │   │
│  │  retries: FixtureConfig.Retries (default 0)                  │   │
│  │                                                               │   │
│  │  ┌─ ChildFixture.BeforeAll(ctx) ──────────────────────┐      │   │
│  │  │  (runs concurrently with sibling child fixtures)    │      │   │
│  │  └────────────────────────────────────────────────────┘      │   │
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
│  deferred: ChildFixture.AfterAll(ctx)  ← concurrent                 │
│  deferred: RootFixture.AfterAll(ctx)   ← after children complete    │
│  deferred: flush coverage counters                                   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### SharedFixture Lifecycle

SharedFixtures live in a **separate subprocess** that outlives all test
binaries. State is transferred via JSON serialization.

```
┌── SETUP SUBPROCESS ──────────────────────────────────────────────────┐
│                                                                       │
│  for each SharedFixture (concurrent):                                 │
│    sf.BeforeAll(ctx)     timeout: SharedFixtureConfig.Timeout (5m)    │
│                          retries: SharedFixtureConfig.Retries (1)     │
│                                                                       │
│  Serialize transfer fields → JSON → stdout                            │
│  Include _teardownBudget = maxTimeout + 30s                           │
│                                                                       │
│  ═══════════ block on SIGTERM/SIGINT ═══════════                      │
│                                                                       │
│  for each SharedFixture (reverse order):                              │
│    sf.AfterAll(ctx)      timeout: SharedFixtureConfig.Timeout         │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘

         │ stdout: JSON state       │ state.json file
         ▼                          ▼

┌── TEST PROCESS (per suite binary) ───────────────────────────────────┐
│                                                                       │
│  Read GOTEST_SHARED_STATE_FILE                                        │
│  json.Unmarshal → SharedFixture struct (transfer fields only)         │
│  sf.Hydrate(ctx)    ← reconstruct local fields (e.g., DB handles)    │
│  t.Cleanup → sf.Dehydrate(ctx)   ← release local resources           │
│                                                                       │
│  Suite runs with shared fixture pointer available on its struct       │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

**Transfer vs Local fields:**
- Transfer fields: exported, JSON-serializable, sent across processes
- Local fields: assigned inside `Hydrate()`, reconstructed in each test
  process (e.g., `*sql.DB` handles that can't serialize)

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
│  Mechanism:  it.Parallel() + sync.WaitGroup                         │
│  Concurrency governed by `-test.parallel` (default: GOMAXPROCS)     │
│                                                                     │
│  ┌─ Suite subprocess ──────────────────────────────────────────┐    │
│  │ BeforeAll()                                                  │    │
│  │ t.Run("TestA", func() { it.Parallel(); ctx := BeforeEach(); │    │
│  │                          TestA(t, ctx); AfterEach(t, ctx) }) │    │
│  │ t.Run("TestB", func() { it.Parallel(); ctx := BeforeEach(); │    │
│  │                          TestB(t, ctx); AfterEach(t, ctx) }) │    │
│  │ wg.Wait()                                                    │    │
│  │ AfterAll()                                                   │    │
│  └──────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

### Streaming Execution (Compile-Execute Overlap)

`executeTestsStreaming` is the primary execution path. It overlaps compilation
with test execution:

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

### Lazy Fixture Environment Resolution

Suites that need shared fixtures call `resolveFixtureEnv()` which blocks
(via `sync.Once`) until the shared fixture setup completes. Suites that
don't need shared fixtures skip this entirely and use `baseEnv`:

```
Suite goroutine:
  │
  ├─ needsFixture? ──yes──▶ resolveFixtureEnv()
  │                              │
  │                              ├─ sync.Once blocks until fixtureReady
  │                              ├─ Returns env with GOTEST_SHARED_STATE_FILE
  │                              └─ Error? → streamCancel(), return
  │
  └─ needsFixture? ──no───▶ use baseEnv directly (no blocking)
```

---

## 5. Timeout Architecture

```
┌────────────────────── Timeout Hierarchy ──────────────────────────────┐
│                                                                        │
│  --setup-timeout (CLI flag)                                            │
│  └─ Total budget for shared fixture subprocess to emit JSON state      │
│     Default: 0 (no external deadline; per-fixture timeouts govern)     │
│                                                                        │
│  SharedFixtureConfig.Timeout (per shared fixture)                      │
│  └─ Deadline on each BeforeAll/AfterAll call in the setup subprocess   │
│     Default: 2m (DefaultFixtureConfig)                                 │
│     Container preset: 5m + 1 retry + 5s delay (ContainerFixtureConfig) │
│                                                                        │
│  FixtureConfig.Timeout (per package fixture)                           │
│  └─ Deadline on BeforeAll/AfterAll in TestMain                         │
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
│  └─ Written to BudgetFile after fixture setup in TestMain              │
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

Every subprocess gets its own process group via `Setpgid: true`:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
cmd.Cancel = func() error {
    return syscall.Kill(-pid, syscall.SIGTERM)  // negative PID = group
}
```

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
├─ RunCaptureJSON (internal, for watch mode) ──────────────────────────┤
│                                                                      │
│  Same as StreamJSON but captured into bytes.Buffer.                   │
│  Nothing written to stdout. Used for programmatic consumption.       │
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

---

## 8. Full Timeline (Happy Path)

```
t=0s    CLI starts
        ├─ Parse flags, load config
        ├─ LoadPackages(./...)
        ├─ GenerateFromLoaded → overlay.json
        ├─ signal.NotifyContext (SIGINT/SIGTERM → ctx cancel)
        │
t=0.5s  executeTestsStreaming begins
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
   Source files are never modified. The overlay is a temp directory cleaned up
   on exit.

4. **Streaming compilation**: `CompilePackagesStream` sends results to a
   channel as each package finishes. Test execution begins before all packages
   are compiled, reducing total wall-clock time.

5. **Process groups**: Every subprocess gets `Setpgid: true`. Signals target
   the group (negative PID), ensuring child processes spawned by tests are
   also cleaned up.

6. **Budget-based teardown**: Each suite subprocess computes its own teardown
   budget (max fixture timeout + max suite setup timeout + 30s headroom) and
   writes it to a sidecar file. The runner reads this before deciding when to
   escalate from SIGTERM to SIGKILL.

7. **Coverage interception**: The fixture template intercepts Go's coverage
   `tearDown` function via `go:linkname` to ensure coverage counters survive
   past `m.Run()` and capture fixture teardown code.
