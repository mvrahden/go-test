# Config Marker Methods

## Problem

When test suites use fixtures that spin up infrastructure (testcontainers, database pools, HTTP servers), three things go wrong:

1. **Hangs with no feedback.** Go's global `-timeout` (default 10m) is the only deadline. When fixture `BeforeAll` stalls on an image pull, the developer stares at a terminal for 10 minutes before a panic kills the binary.

2. **Transient failures in CI.** A flaky container pull or network blip fails `BeforeAll`, and the entire test run fails. Developers re-trigger CI. There's no retry mechanism.

3. **Cascading waste.** When the first test fails against broken infrastructure, 50 more tests run and fail with the same root cause. There's no short-circuit.

## Design Principles

1. **Sensible defaults always applied.** Every fixture and suite gets framework defaults. The optional marker method overlays on top — only non-zero fields override.
2. **The naming IS the API.** A single well-named marker method (`FixtureConfig`, `SuiteConfig`) — no struct tags, no registration calls, no config files.
3. **Runtime-computable.** Config methods return structs, so values can depend on environment (`os.Getenv("CI")`) or other runtime state.
4. **Incremental adoption.** A developer can add `FixtureConfig()` to one fixture to override specific defaults. The marker method is never required.

## API Surface

### Config Types

```go
package gotest

import "time"

type FixtureConfig struct {
    Timeout    time.Duration // applied to BeforeAll/AfterAll via context.WithTimeout
    Retries    int           // additional BeforeAll attempts on failure
    RetryDelay time.Duration // pause between retry attempts
}

type SuiteConfig struct {
    Timeout      time.Duration // per-test-case deadline via t.Context()
    SetupTimeout time.Duration // BeforeAll/AfterAll deadline
    Retries      int           // per-test-case retry attempts
    FailFast     bool          // stop suite on first failure
}
```

### Value Semantics

| Value | Meaning |
|-------|---------|
| `> 0` | Use this duration |
| `0`   | Keep default (field not overridden) |
| `< 0` | Explicitly disabled (no timeout) |

This applies to `Timeout`, `SetupTimeout`, and `RetryDelay`. The `int` and `bool` fields use standard zero-means-default semantics. `FailFast` only overrides to `true` — a false overlay does not reset a true base.

### Overlay Functions

```go
func OverlayFixtureConfig(base *FixtureConfig, overlay FixtureConfig)
func OverlaySuiteConfig(base *SuiteConfig, overlay SuiteConfig)
```

Only non-zero overlay fields replace the base. This enables composable presets — start with a preset, override one field.

### Presets

| Preset | Timeout | Retries | RetryDelay | Use case |
|--------|---------|---------|------------|----------|
| `DefaultFixtureConfig()` | 2 min | 0 | 0 | Standard fixtures |
| `ContainerFixtureConfig()` | 5 min | 1 | 5s | Testcontainers, image pulls |
| `DefaultSuiteConfig()` | 30s (+ 30s setup) | 0 | — | Unit/integration tests |
| `IntegrationSuiteConfig()` | 2 min (+ 5 min setup) | 0 | — | Heavier integration tests |

### `NewTWithDeadline`

```go
func NewTWithDeadline(t *testing.T, timeout time.Duration) *T
```

Creates a `*gotest.T` with a context deadline. `t.Context()` returns the deadline-aware context. Existing `NewT` callers are unaffected — `Context()` falls back to `testing.T.Context()` when no custom context is set.

## Usage

### Fixture with container setup

```go
type InfraFixture struct {
    Pool *pgxpool.Pool
}

func (f *InfraFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error {
    // ctx carries a 5-minute deadline from FixtureConfig.
    // Testcontainers respects context cancellation, so a stalled pull
    // fails cleanly after 5 minutes instead of hanging for 10.
    pg, err := postgres.Run(ctx, "postgres:16")
    if err != nil {
        return err
    }
    f.Pool, err = pgxpool.New(ctx, pg.MustConnectionString(ctx))
    return err
}
```

### Suite with per-test timeout and fail-fast

```go
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

### Runtime-computed config

```go
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

The code generator recognizes these exact signatures:

```go
// On fixture types (*Fixture suffix):
func (f *MyFixture) FixtureConfig() gotest.FixtureConfig

// On suite types (*TestSuite / *TestSuiteParallel suffix):
func (s *MySuite) SuiteConfig() gotest.SuiteConfig
```

Requirements:
- Pointer receiver matching the fixture/suite type name
- No parameters
- Single return value of the exact config struct type
- Exported method name exactly `FixtureConfig` or `SuiteConfig`

Invalid signatures (wrong params, wrong return type) produce a compile-time error reported by the collector.

## Generated Code Behavior

### Fixture lifecycle

The generated harness always resolves config, then uses it to control BeforeAll/AfterAll execution:

1. **Config resolution:** `ƒcfg := gotest.DefaultFixtureConfig()` + optional `gotest.OverlayFixtureConfig(&ƒcfg, fixture.FixtureConfig())` when the marker is present.

2. **BeforeAll retry loop:** Attempts `1 + ƒcfg.Retries` times. Each attempt wraps `ctx` with `context.WithTimeout(t.Context(), ƒcfg.Timeout)` when timeout > 0. On failure, logs the attempt number and sleeps `ƒcfg.RetryDelay` before retrying. Fatal after all attempts exhausted.

3. **AfterAll cleanup:** Registered via `t.Cleanup`. Wraps `context.Background()` with timeout when > 0. Reports errors via `t.Errorf` (does not fatal — cleanup must complete).

### Suite test case execution

1. **Config resolution:** `ƒcfg := gotest.DefaultSuiteConfig()` + optional `gotest.OverlaySuiteConfig(&ƒcfg, s.SuiteIdentifier.SuiteConfig())` when the marker is present.

2. **Per-test timeout:** When `ƒcfg.Timeout > 0`, each test case receives a `*gotest.T` created via `NewTWithDeadline(it, ƒcfg.Timeout)`. The test's `t.Context()` carries the deadline.

3. **FailFast:** After each test case, checks `t.Failed()`. If true and `FailFast` is set, breaks the loop. Remaining tests don't execute. This matches Go's `-failfast` semantics scoped to one suite.

### Nested fixtures

Child fixtures use independent config variables (`ƒcfg_child`) with the same resolution and retry pattern. Parent and child configs are independent — no inheritance between fixture levels.

## Interaction with Existing Features

### Parallel suites
`FailFast` only applies to sequential execution within a single suite. Parallel test cases (WaitGroup-coordinated) complete or hit the global timeout. This matches Go's behavior where `t.Parallel()` subtests are not cancelled when a sibling fails.

### Focus/Exclude (`F_`/`X_` prefixes)
Config applies to the effective test set after focus/exclude filtering. Skipped suites get their unchanged skip stubs.

### Global `-timeout`
`FixtureConfig.Timeout` wraps `t.Context()` or `context.Background()`, which are bounded by Go's global timeout. The effective timeout is `min(FixtureConfig.Timeout, remaining global timeout)` — handled naturally by `context.WithTimeout` inheriting the parent's shorter deadline.

### SharedFixtures
SharedFixtures have `() error` signatures with no context parameter. `FixtureConfig` does not apply. A `SharedFixtureConfig` type can be added independently if needed.

## Out of Scope

- **Per-test-method timeout overrides.** Individual methods inherit the suite's `Timeout`. Per-method overrides can be added later via a `TestConfig` pattern.
- **Retry with backoff strategies.** `RetryDelay` is a fixed duration. Exponential backoff can be added to the struct later without breaking changes.
- **Cross-suite FailFast.** `FailFast` is per-suite. A "bail on first failure across all suites" flag would be a CLI flag (`--bail`), not a config method.
- **SlowThreshold.** Marking tests as "slow" in spec output. Independent — can be a future `SuiteConfig` field.

## Extensibility

Both config structs are plain Go structs. Adding new fields is backward-compatible — existing code that doesn't set the new field gets the zero value, which means "no change." No interfaces, no options pattern, no breaking changes.

Future candidates:
- `SuiteConfig.SlowThreshold time.Duration` — highlight slow tests in spec output
- `SuiteConfig.MaxParallel int` — limit concurrency within a parallel suite
- `FixtureConfig.CleanupTimeout time.Duration` — separate timeout for AfterAll
