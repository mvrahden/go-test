# Config Marker Methods — Design Spec

## Problem

When test suites use fixtures that spin up infrastructure (testcontainers, database pools, HTTP servers), three things go wrong:

1. **Hangs with no feedback.** Go's global `-timeout` (default 10m) is the only deadline. When fixture `BeforeAll` stalls on an image pull, the developer stares at a terminal for 10 minutes before a panic kills the binary. In the VS Code extension, tests show a gray arrow (started, never resolved).

2. **Transient failures in CI.** A flaky container pull or network blip fails `BeforeAll`, and the entire test run fails. Developers re-trigger CI. There's no retry mechanism.

3. **Cascading waste.** When the first test fails against broken infrastructure, 50 more tests run and fail with the same root cause. There's no short-circuit.

Go's test runner has a single global `-timeout` knob. Jest solves this with per-component timeouts, retries, and `bail`. gotest needs an equivalent that fits Go's idioms.

## Design Principles

1. **Sensible defaults always applied.** Every fixture and suite gets framework defaults (`DefaultFixtureConfig()` / `DefaultSuiteConfig()`). The optional marker method overlays on top — only non-zero fields override.
2. **The naming IS the API.** A single well-named marker method (`FixtureConfig`, `SuiteConfig`) — no struct tags, no registration calls, no config files.
3. **Runtime-computable.** Config methods return structs, so values can depend on environment (`os.Getenv("CI")`) or other runtime state.
4. **Incremental adoption.** A developer can add `FixtureConfig()` to one fixture to override specific defaults. The marker method is never required.

## Value semantics for `time.Duration` fields

| Value | Meaning |
|-------|---------|
| `> 0` | Use this duration |
| `0`   | Keep default (field not overridden) |
| `< 0` | Explicitly disabled (no timeout) |

This applies to `Timeout`, `SetupTimeout`, and `RetryDelay`. The `int` and `bool` fields use standard zero-means-default semantics (zero retries = keep default retries; to explicitly set zero retries, the default is already zero).

## API Surface

### `pkg/gotest/config.go` — new file

```go
package gotest

import "time"

// FixtureConfig controls timeout, retry, and lifecycle behavior for package fixtures.
// The framework always applies DefaultFixtureConfig(). The optional FixtureConfig()
// marker method overrides these defaults.
type FixtureConfig struct {
    // Timeout applied to BeforeAll and AfterAll calls via context.WithTimeout.
    // The fixture's ctx parameter will carry this deadline.
    Timeout time.Duration

    // Retries for BeforeAll. On transient failure, BeforeAll is retried up to this
    // many additional times. AfterAll is NOT retried.
    Retries int

    // RetryDelay is the pause between retry attempts.
    RetryDelay time.Duration
}

// SuiteConfig controls timeout, retry, and execution behavior for test suites.
// The framework always applies DefaultSuiteConfig(). The optional SuiteConfig()
// marker method overrides these defaults.
type SuiteConfig struct {
    // Timeout applied to each test case. The test receives a context with this deadline
    // via t.Context(). Tests that pass ctx to I/O operations will fail cleanly on expiry.
    Timeout time.Duration

    // SetupTimeout applied to suite-level BeforeAll and AfterAll.
    SetupTimeout time.Duration

    // Retries for failed test methods. On failure, the individual test is re-run
    // up to this many additional times. A test that passes on retry is reported as passed.
    Retries int

    // FailFast stops executing remaining test cases in this suite after the first failure.
    FailFast bool
}
```

### Presets — convenience constructors

```go
// DefaultFixtureConfig returns config suitable for fixtures with moderate setup cost.
func DefaultFixtureConfig() FixtureConfig {
    return FixtureConfig{Timeout: 2 * time.Minute}
}

// ContainerFixtureConfig returns config suitable for fixtures that start containers.
// Includes one retry with a 5-second delay to handle transient pull failures.
func ContainerFixtureConfig() FixtureConfig {
    return FixtureConfig{Timeout: 5 * time.Minute, Retries: 1, RetryDelay: 5 * time.Second}
}

// DefaultSuiteConfig returns config with a 30-second per-test timeout and 30-second setup timeout.
func DefaultSuiteConfig() SuiteConfig {
    return SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second}
}

// IntegrationSuiteConfig returns config suited for integration test suites
// with heavier setup and longer-running tests.
func IntegrationSuiteConfig() SuiteConfig {
    return SuiteConfig{Timeout: 2 * time.Minute, SetupTimeout: 5 * time.Minute}
}
```

### Usage in user code

```go
// Fixture with container setup — 5-minute timeout, one retry
type InfraFixture struct {
    Pool *pgxpool.Pool
}

func (f *InfraFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error {
    // ctx now carries a 5-minute deadline from the generated harness.
    // testcontainers respects context cancellation, so a stalled pull
    // fails cleanly after 5 minutes instead of hanging for 10.
    pg, err := postgres.Run(ctx, "postgres:16")
    if err != nil {
        return err
    }
    f.Pool, err = pgxpool.New(ctx, pg.MustConnectionString(ctx))
    return err
}
```

```go
// Suite with per-test timeout and fail-fast
type BatchTestSuite struct {
    *APIFixture
}

func (s *BatchTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{
        Timeout:  30 * time.Second,
        FailFast: true,
    }
}
```

```go
// Runtime-computed config — longer timeout in CI
func (f *InfraFixture) FixtureConfig() gotest.FixtureConfig {
    cfg := gotest.ContainerFixtureConfig()
    if os.Getenv("CI") != "" {
        cfg.Timeout = 10 * time.Minute
        cfg.Retries = 2
    }
    return cfg
}
```

## Detection

### Marker method signatures

The code generator recognizes these exact signatures on fixture and suite types:

```go
// On fixture types (*Fixture suffix):
func (f *MyFixture) FixtureConfig() gotest.FixtureConfig

// On suite types (*TestSuite / *TestSuiteParallel suffix):
func (s *MySuite) SuiteConfig() gotest.SuiteConfig
```

Requirements (same conventions as existing lifecycle methods):
- Pointer receiver matching the fixture/suite type name
- No parameters
- Single return value of the config struct type
- Exported method name exactly `FixtureConfig` or `SuiteConfig`

### AST changes

**`internal/gotestast/fixture.go`** — Add `Config *types.Func` field to `FixtureSpec`:

```go
type FixtureSpec struct {
    // ... existing fields ...
    Config     *types.Func   // FixtureConfig() method, may be nil
}
```

Add detection in `DetermineFixtureHarness`:

```go
case "FixtureConfig":
    // Validate: no params, one result of type gotest.FixtureConfig
    if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
        return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () gotest.FixtureConfig", methodID)
    }
    resType := sig.Results().At(0).Type().String()
    if !strings.HasSuffix(resType, "/gotest.FixtureConfig") {
        return m.Pos(), fmt.Errorf("unsupported return type for %q: expected gotest.FixtureConfig, got %s", methodID, resType)
    }
    f.Config = m
```

Also add `FixtureConfig` to the name check at line 123 so it's not rejected as unknown.

**`internal/gotestast/spec.go`** — Add `Config *types.Func` field to `TestSuiteHarness`:

```go
type TestSuiteHarness struct {
    // ... existing fields ...
    Config *types.Func // SuiteConfig() method, may be nil
}
```

Add detection in `DetermineTestSuiteHarness`. `SuiteConfig` must be added to the `IS_TEST_SUITE_METHOD` regex, and handled in the switch before the test-case branch:

```go
case "SuiteConfig":
    // Validate: no params, one result of type gotest.SuiteConfig
    if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
        return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () gotest.SuiteConfig", methodID)
    }
    resType := sig.Results().At(0).Type().String()
    if !strings.HasSuffix(resType, "/gotest.SuiteConfig") {
        return m.Pos(), fmt.Errorf("unsupported return type for %q: expected gotest.SuiteConfig, got %s", methodID, resType)
    }
    s.th.Config = m
```

Add a rendering accessor to `TestSuiteSpec`:

```go
func (ts *TestSuiteSpec) Config() *types.Func { return ts.th.Config }
```

### Renderer changes

**`internal/gotestgen/renderer.go`** — Add `HasConfig` to `FixtureViewModel`:

```go
type FixtureViewModel struct {
    // ... existing fields ...
    HasConfig bool
}
```

Set during `buildFixtureViewModels`:
```go
vm := &FixtureViewModel{
    // ... existing ...
    HasConfig: f.Config != nil,
}
```

**Import management:** Always add `"time"` to the imports list in `renderFileHeader` (config is always applied).

## Generated Code

### Fixture generated code

The generated code always starts with defaults. When the user defines `FixtureConfig()`, non-zero fields are overlaid.

```go
func Test_InfraFixture(t *testing.T) {
    fixture := &InfraFixture{}
    ƒcfg := gotest.DefaultFixtureConfig()
    // With FixtureConfig() marker → overlay non-zero fields:
    gotest.OverlayFixtureConfig(&ƒcfg, fixture.FixtureConfig())
    // Without marker → just uses defaults as-is
    t.Cleanup(func() {
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        if err := fixture.AfterAll(ctx); err != nil {
            t.Errorf("InfraFixture.AfterAll failed: %v", err)
        }
    })
    var ƒerr error
    ƒattempts := 1 + ƒcfg.Retries
    for ƒi := range ƒattempts {
        ctx := t.Context()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        ƒerr = fixture.BeforeAll(ctx)
        if ƒerr == nil {
            break
        }
        if ƒi < ƒattempts-1 {
            t.Logf("InfraFixture.BeforeAll attempt %d/%d failed: %v", ƒi+1, ƒattempts, ƒerr)
            if ƒcfg.RetryDelay > 0 {
                time.Sleep(ƒcfg.RetryDelay)
            }
        }
    }
    if ƒerr != nil {
        t.Fatalf("InfraFixture.BeforeAll failed after %d attempt(s): %v", ƒattempts, ƒerr)
    }
    // ... subtests (unchanged) ...
}
```

### Suite generated code

The generated code always starts with defaults. When the user defines `SuiteConfig()`, non-zero fields are overlaid.

```go
t.Run("BatchTestSuite", func(t *testing.T) {
    s := &ƒƒ_GOTEST_BatchTestSuite{...}
    ƒcfg := gotest.DefaultSuiteConfig()
    // With SuiteConfig() marker → overlay non-zero fields:
    gotest.OverlaySuiteConfig(&ƒcfg, s.BatchTestSuite.SuiteConfig())
    // Without marker → just uses defaults as-is

    newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
        return func(tt *gotest.T) {
            t := tt.T()
            t.Run(desc, func(it *testing.T) {
                ttt := gotest.NewT(it)
                if ƒcfg.Timeout > 0 {
                    ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
                }
                defer s.AfterEach(ttt)
                s.BeforeEach(ttt)
                ƒƒ_GOTEST_exec(testFn, ttt)
            })
        }
    }

    testCases := []gotest.TestCase{...}

    tt := gotest.NewT(t)
    t.Cleanup(func() { s.AfterAll(tt) })
    s.BeforeAll(tt)
    for _, tc := range testCases {
        tc(tt)
        if ƒcfg.FailFast && t.Failed() {
            break
        }
    }
})
```

**FailFast** — After each test case, check `t.Failed()`. If true and `FailFast` is set, break the loop. Remaining tests are not executed (not skipped — they simply don't start). This matches Go's `-failfast` flag semantics but scoped to one suite.

**Retries** — When `ƒcfg.Retries > 0`, the `newTestCase` wrapper runs the test body up to `1 + Retries` times. On failure, it logs the attempt and re-runs. On eventual success, it clears the failure. Implementation deferred to the implementation plan (requires careful handling of `t.Run` subtest naming to avoid duplicate names).

### `gotest.NewTWithDeadline` — new constructor

```go
// pkg/gotest/t.go

func NewTWithDeadline(t *testing.T, timeout time.Duration) *T {
    ctx, cancel := context.WithTimeout(t.Context(), timeout)
    t.Cleanup(cancel)
    return &T{t: t, ctx: ctx}
}
```

This requires adding an optional `ctx` field to `T` and making `Context()` prefer it:

```go
type T struct {
    t         *testing.T
    ctx       context.Context // overrides t.Context() when non-nil
    collector *collectingT
}

func (t *T) Context() context.Context {
    if t.ctx != nil {
        return t.ctx
    }
    return t.t.Context()
}
```

This is the minimal API surface change. Existing `NewT` callers are unaffected (ctx stays nil, Context() delegates to testing.T as before).

## Template Changes

### `gotest.fixture.tpl`

The fixture template always emits config-aware code. It always starts with `gotest.DefaultFixtureConfig()`. When `HasConfig` is true, it overlays with the user's method.

```
  func Test_{{ $f.Identifier }}(t *testing.T) {
      fixture := &{{ $f.Identifier }}{}
+     ƒcfg := gotest.DefaultFixtureConfig()
+ {{- if $f.HasConfig }}
+     gotest.OverlayFixtureConfig(&ƒcfg, fixture.FixtureConfig())
+ {{- end }}
      t.Cleanup(func() {
+         ctx := context.Background()
+         if ƒcfg.Timeout > 0 { ... WithTimeout ... }
  {{- if $f.AfterAll }}
+         if err := fixture.AfterAll(ctx); err != nil { ... }
```

The BeforeAll section always uses the retry loop pattern with `ƒcfg`.

### `gotest.suites.tpl`

Suite template always emits config-aware code. It always starts with `gotest.DefaultSuiteConfig()`. When `HasConfig()` is true, it overlays with the user's method.

## Interaction with Existing Features

### Parallel suites
`FailFast` only applies to sequential execution within a single suite. Parallel test cases (WaitGroup-coordinated) are not interrupted — they complete or hit the global timeout. This matches Go's behavior where `t.Parallel()` subtests are not cancelled when a sibling fails.

### Focus/Exclude (`F_`/`X_` prefixes)
Config applies to the effective test set after focus/exclude filtering. Skipped suites don't get config — their stubs are unchanged.

### Global `-timeout`
FixtureConfig.Timeout wraps `t.Context()` or `context.Background()`, which are themselves bounded by Go's global `-timeout`. The effective timeout is `min(FixtureConfig.Timeout, remaining global timeout)`. This is handled naturally by `context.WithTimeout` — if the parent context has a shorter deadline, the child inherits it.

### SharedFixtures
SharedFixtures run outside the test process (subprocess managed by the CLI). They have `() error` signatures, no context parameter. `FixtureConfig` does not apply to shared fixtures. If shared fixture timeouts are needed in the future, a `SharedFixtureConfig` type can be added independently.

## Out of Scope

- **Per-test-method timeout overrides.** Individual test methods inherit the suite's `Timeout`. Per-method overrides (like Jest's third argument to `test()`) can be added later via a `TestConfig` pattern, but the 80% case is suite-level timeout.
- **Retry with backoff strategies.** `RetryDelay` is a fixed duration. Exponential backoff can be added to the struct later without breaking changes.
- **Cross-suite FailFast.** `FailFast` is per-suite. A "bail on first failure across all suites" flag would be a CLI flag (`--bail`), not a config method.
- **SlowThreshold.** Marking tests as "slow" in spec output. Useful but independent — can be a future `SuiteConfig` field.

## Extensibility

Both config structs are plain Go structs. Adding new fields is backward-compatible — existing code that doesn't set the new field gets the zero value, which means "no change." No interfaces, no options pattern, no breaking changes.

Future candidates:
- `SuiteConfig.SlowThreshold time.Duration` — highlight slow tests in spec output
- `SuiteConfig.MaxParallel int` — limit concurrency within a parallel suite
- `FixtureConfig.CleanupTimeout time.Duration` — separate timeout for AfterAll (distinct from setup Timeout)
