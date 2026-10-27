# Configuration — zero means default

> Describes gotest v1.30.0+. On v1.26–v1.29 a zero duration meant NO
> deadline (so compose onto a preset there); v1.25.x merged the marker
> over the defaults — see `versions.md`.

A `SuiteConfig()` marker method states the config; durations it leaves at
zero behave as if the marker were absent:

- No marker → `DefaultSuiteConfig()` (30s test timeout, 30s setup timeout).
- Marker present → booleans (`Parallel`, `Exclusive`, `FailFast`) are used
  as written. From v1.30 a duration left at zero (omitted or explicit) gets
  the default: `SuiteConfig{Parallel: true}` bounds the contexts at
  30s/30s.
- Every positive duration the marker returns is also a verdict — one you
  write and one a preset supplies alike: a test that outlives it is failed
  even when it ignores its context. A defaulted duration (no marker, or
  zero) bounds the context and is never enforced. So
  `return gotest.DefaultSuiteConfig()` and the compose form below enforce
  30s/30s, while `gotest.SuiteConfig{Parallel: true}` only bounds the
  contexts at 30s/30s.
- `gotest.NoDeadline` (any negative duration) disables the deadline. Write the
  constant, not a bare negative: from v1.30 `gotest lint` reports a literal
  negative (`config-no-deadline`) and `-fix` spells it `gotest.NoDeadline`.
- Upgrading from v1.26–v1.29: a suite that relied on `Timeout: 0` for no
  deadline now has its context canceled at the default — code that honors
  `t.Context()` fails there, code that ignores it is not failed. Write
  `gotest.NoDeadline` where no deadline was the intent.
- `SetupTimeout` bounds `BeforeAll`/`AfterAll` of benchmark and fuzz
  suites too from v1.32; before, their contexts had no deadline.

Marker bodies are parsed statically (the generator needs `Parallel` at
generation time), so only three forms are legal:

1. a literal return;
2. a gotest preset call — `DefaultSuiteConfig()` or
   `IntegrationSuiteConfig()` (2m/5m) — custom helpers are rejected because
   they would silently drop `Parallel`;
3. the compose form, which keeps a preset's durations as budgets:

```go
func (s *ShopTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}
```

`Parallel` must be assigned a boolean literal; a literal must use keyed
fields. Delete markers that restate defaults — a marker is a statement of
intent, not boilerplate (agents copying default-restating markers between
suites is an observed failure). A bare `return gotest.DefaultSuiteConfig()`
counts as default-restating, but deleting it also drops its 30s verdict
(the context stays bounded): keep it, or an explicit `Timeout`, only
where a test must be failed at its budget.

Fixtures follow the same rule via `FixtureConfig()` /
`SharedFixtureConfig()` markers: no marker or a zero `Timeout` →
`DefaultFixtureConfig()`'s 2m; `Retries`/`RetryDelay` are used as written;
`ContainerFixtureConfig()` (5m, 1 retry) suits container startups. The
runner waits for shared fixture setup as long as the fixtures declare
(v1.34+), and 30s until the setup process reports its first fixture's
budget (v1.34+); the run's `--setup-timeout` (`setup-timeout:` in `.gotest.yml`)
replaces that with a flat budget.

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
follow their window (`fixtures.md`): one both phases need stays up across
the tail, one only the bulk needed is torn down before it, and one only
the tail needs starts there. Exclusive is not a serialization tool for
shared mutable state; use non-parallel suites for that.

## Exclusive groups (v1.36+)

`SuiteConfig{ExclusiveGroup: "e2e"}` puts a suite in a named group: the
default group runs first, then each named group by name, and no two
groups ever run at once. A group's suites run in parallel among
themselves, its `Exclusive` suites alone after them. It fits E2E suites
that must not share the machine with unit suites but may share it with
each other. The value must be a string literal or a string constant
(a constant from a shared package, `testgroups.E2E`, avoids typos), named
with `A-Z`, `a-z`, `0-9`, `_`, `-`; anything else is refused at
generation. A shared fixture only the group reads starts at the group's
start and is released at its last use. `--group=e2e` runs the group
alone, `--skip-group=e2e` everything else: one CI job per side, no build
tags. `gotest summary` prints each group's suites and wall time. Group
only what needs isolation — a group gives up its overlap with the rest
of the run.
