# Configuration — literal semantics

> Applies to gotest v1.26.0+ only — see SKILL.md's version gate; v1.25.x
> and older have inverted zero/-1 semantics.

A `SuiteConfig()` marker method OWNS the config verbatim:

- No marker → `DefaultSuiteConfig()` (30s test timeout, 30s setup timeout).
- Marker present → the returned value is used AS-IS. Partial literals
  inherit nothing: `SuiteConfig{Parallel: true}` has NO timeouts.
- Zero or negative duration = no deadline (matches `go test -timeout 0`).

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

Fixtures use the same literal rule via `FixtureConfig()` /
`SharedFixtureConfig()` markers: no marker → `DefaultFixtureConfig()` (2m);
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
