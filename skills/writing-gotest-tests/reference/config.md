# Configuration — zero means default

> Describes gotest v1.30.0+. On v1.26–v1.29 a zero duration meant NO
> deadline (so compose onto a preset there); v1.25.x merged the marker
> over the defaults — see SKILL.md's version gate.

A `SuiteConfig()` marker method states the config; durations it leaves at
zero behave as if the marker were absent:

- No marker → `DefaultSuiteConfig()` (30s test timeout, 30s setup timeout).
- Marker present → booleans (`Parallel`, `Exclusive`, `FailFast`) are used
  as written. From v1.30 a duration left at zero (omitted or explicit) gets
  the default: `SuiteConfig{Parallel: true}` runs with 30s/30s (v1.26–v1.29
  it meant no deadline).
- `gotest.NoDeadline` (any negative duration) disables the deadline. Write the
  constant, not a bare negative: from v1.30 `gotest lint` reports a literal
  negative (`config-no-deadline`) and `-fix` spells it `gotest.NoDeadline`.
- Upgrading from v1.26–v1.29: a suite that relied on `Timeout: 0` for no
  deadline now gets the default one and fails when it exceeds it. Write
  `gotest.NoDeadline` where that was the intent.

Marker bodies are parsed statically (the generator needs `Parallel` at
generation time), so only three forms are legal:

1. a literal return;
2. a gotest preset call — `DefaultSuiteConfig()` or
   `IntegrationSuiteConfig()` (2m/5m) — custom helpers are rejected because
   they would silently drop `Parallel`;
3. the compose form (preferred for parallel suites):

```go
func (s *ShopTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}
```

`Parallel` must be assigned a boolean literal. Delete markers that restate
defaults — a marker is a statement of intent, not boilerplate (agents
copying default-restating markers between suites is an observed failure).

Fixtures follow the same rule via `FixtureConfig()` /
`SharedFixtureConfig()` markers: no marker or a zero `Timeout` →
`DefaultFixtureConfig()`'s 2m; `Retries`/`RetryDelay` are used as written;
`ContainerFixtureConfig()` (5m, 1 retry) suits container startups.

## Exclusive suites (v1.27+)

`SuiteConfig{Exclusive: true}` is parsed statically exactly like
`Parallel` — assign a boolean literal (the compose form works:
`cfg.Exclusive = true`). Exclusive suites are held back until every
non-exclusive suite has finished, then dispatched strictly alone, one at
a time, in deterministic (package, suite) order — batch and streaming
runs alike. Use it for suites whose *verdicts* measure wall-clock
behavior or that fight over machine-wide resources (timing budgets,
containers, ports, per-invocation child builds): a budget verdict taken
on a saturated machine is not a verdict you can act on. Shared fixtures
stay up across the exclusive tail — they are infrastructure, not
competing suites. Exclusive is not a serialization tool for shared
mutable state; use non-parallel suites for that.
