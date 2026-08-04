---
name: writing-gotest-tests
description: Use when writing, fixing, reviewing, or restructuring tests in a Go repository that uses the gotest framework (github.com/mvrahden/go-test) — `*TestSuite` structs, `gotest.T`, `BeforeEach`/`AfterAll` hooks, `*Fixture` types, or the `gotest` CLI — when migrating testify or stdlib tests to gotest, and when setting up or fixing CI for a gotest repository.
---

# Writing gotest tests

gotest inverts habits learned from stdlib/testify. Follow this file's rules;
consult `reference/` only when the task touches that area.

**Version check (do this FIRST):** run `go tool gotest version` (after
Bootstrap below); `dev (replace directive)` / `dev (source checkout)` mean
a source build — assume current behavior. Before Bootstrap, read go.mod
instead, remembering that a `replace` line overrides the `require`
version.

This skill targets **v1.26.0+**. On **v1.25.x** everything below still
applies EXCEPT these three, which fail there:

1. **The parallel recipe (rule 4)** — v1.25.x requires the `SuiteConfig`
   body to be a SINGLE return statement, so the compose form is a
   generation error. Return the literal `gotest.SuiteConfig{Parallel:
   true}` instead; on v1.25.x a partial literal DOES inherit the 30s
   defaults.
2. **Config literalism (rule 3)** — v1.25.x merges your marker over the
   defaults: `0` means "keep the default", `-1` disables, and
   `Parallel`/`FailFast` are one-way latches in that merge (once true,
   never reset to false).
3. **`Test*Async` (rule 1)** — recognized but never rendered on v1.25.x;
   write a synchronous test with `Eventually` instead.

Cosmetic-only on v1.25.x: no `[no suites]` note, `migrate` leaves no TODO
markers, `CI=false` still enables CI mode, no expanded value diffs.

## Bootstrap

The repo has the *library*; the CLI runs via Go's tool directive (requires
Go ≥ 1.24). One-time:

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest
go tool gotest version
```

Then every command is `go tool gotest <args>`. Never `go install` a global
binary — it goes stale against the pinned library version. (Bare
`go run github.com/mvrahden/go-test/cmd/gotest` fails on fresh consumers:
module pruning leaves the CLI's deps out of go.sum.)

## The Two Runners — a complete run is BOTH commands

`go test` ignores gotest suites (signature incompatibility, by design).
`gotest` runs suites and never runs stdlib tests — it prints
`[no suites]` plus a stderr note when it skips them. Running only one
command silently misses tests. Always finish with:

```sh
go tool gotest ./...
go test ./...
```

Add `-race` to both before calling anything done.

## The write loop

Write → lint → both runners → fix:

```sh
go tool gotest lint -fix ./...
go get -tool golang.org/x/tools/cmd/goimports
go tool goimports -l .
```

`lint -fix` applies suggested fixes as TEXTUAL edits — no formatting pass
runs. A fix that strands or misses an import leaves the file uncompilable:
install goimports once via the tool directive (as above — plain `go run`
of it fails on missing go.sum entries), then always run
`go tool goimports -w .` after `-fix` and re-run the loop.

The linter catches direct misuse (`t.T()` escapes even inside closures,
outer-`t` in poll callbacks, testify idioms, focus leftovers). Its
`stdlib-test` rule flags every stdlib `TestXxx(*testing.T)` — when a stdlib
test is intentional (assertion-layer tests, benchmarks-adjacent code), mark
its package clause with `//nolint:stdlib-test`, or the lint run fails. It
does NOT catch: plain `defer` cleanup, shared mutable state in parallel
suites, or structural problems — those are your job, below.

## Core rules

1. **Suites are structs, naming is the API.** `type XxxTestSuite struct{}`,
   exported, methods `func (s *X) TestBehavior(t *gotest.T)`. No `TestMain`,
   no registration — the CLI generates the harness invisibly (never commit
   `ƒƒ_*` files). `F_`/`X_` prefixes focus/exclude; `Test*Async(t, done)`
   declares async tests.
2. **Lifecycle hooks own resources.** Setup in `BeforeEach` (or `BeforeAll`
   for expensive read-only state), teardown in `AfterEach`/`AfterAll` as
   suite fields — NEVER `defer` or `t.T().Cleanup` in a test method. A plain
   `defer` lints clean and is still wrong: it skips teardown verification
   and blocks parallelization.
3. **Do not imitate existing tests blindly.** Observed failure: agents copy
   a repo's existing `SuiteConfig()` verbatim, propagating anti-patterns. A
   `SuiteConfig()` marker states *intent*: omit it entirely for defaults.
   The returned config is used literally — partial literals inherit
   NOTHING; a zero/omitted timeout means NO deadline, not "default".
4. **Ask "why is this suite NOT parallel?"** Observed failure: agents never
   parallelize unprompted, even when asked to improve tests. The recipe:

   ```go
   func (s *ShopTestSuite) SuiteConfig() gotest.SuiteConfig {
   	cfg := gotest.DefaultSuiteConfig()
   	cfg.Parallel = true
   	return cfg
   }

   type shopCtx struct{ inv *Inventory }

   func (s *ShopTestSuite) BeforeEach(t *gotest.T) *shopCtx {
   	return &shopCtx{inv: NewInventory()}
   }
   ```

   Every test method then takes `(t *gotest.T, ctx *shopCtx)` — the
   generator enforces this. Legitimate reasons to stay sequential: `Setenv`
   (panics in parallel tests), shared live resources without per-test
   keys/schemas (`-race` is process-local and cannot see datastore
   contention — it is necessary, not sufficient).
5. **Poll, never sleep.** `gotest.Eventually(t, waitFor, tick, func(poll
   *gotest.R) { ... })` — assert a stable fixed point, not a transient
   state. The callback's `poll` handle has only `Errorf`/`FailNow`/
   `Failed`/`Message`; pass `poll` (not `t`) to assertions inside it.
6. **Never call `t.T().Helper()`** — call sites resolve automatically; the
   linter flags it. Reach for `t.T()` only when nothing on `gotest.T`
   (`It`, `When`, `Context`, `TempDir`, `Setenv`, `Skipf`, `Errorf`,
   `FailNow`) covers the need.

## Restructuring existing suites (the blue phase)

Enter only at a green pause point when: setup is duplicated across tests, a
third similar test is being added, a touched suite already smells, or you
were asked to clean up. Follow `reference/refactoring.md` for the ladder and
smells list. Non-negotiable safety invariants (tests protect nothing —
observed failure: agents restructure with no case accounting):

1. Capture the executed case LIST before and after, into separate files,
   and diff them:

   ```sh
   go tool gotest -json ./... | grep -o '"Package":"[^"]*","Test":"[^"]*"' | sort -u > cases-before.txt
   test -s cases-before.txt
   ```

   Capture the Package+Test PAIR — bare `Test` names collapse identically
   named suites across packages, hiding whole-package deletions.

   After the refactor, capture `cases-after.txt` the same way and run
   `diff cases-before.txt cases-after.txt`. Enumerate every rename/merge
   BEFORE editing; every diff line must map to that list. Coverage may
   only grow.
2. Both runners + `-race` green before AND after.
3. Never delete or weaken an assertion without saying so in your report.
4. Never touch production code during a test refactor — a test that resists
   restructuring is a design finding to report.
5. Consider rule 4 (parallelization) part of every improvement pass.

## Minimal complete suite

```go
package shop_test

import (
	"example.com/shop"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type CartTestSuite struct {
	cart *shop.Cart
}

func (s *CartTestSuite) BeforeEach(t *gotest.T) {
	s.cart = shop.NewCart()
}

func (s *CartTestSuite) TestTotalsItems(t *gotest.T) {
	s.cart.Add("apple", 2)
	gotest.Equal(t, 2, s.cart.Count("apple"))
}
```

## References

- `reference/assertions.md` — full assertion surface, `Nil`/`NotNil` type
  guards, snapshot testing
- `reference/config.md` — literal config semantics, presets, compose form
- `reference/fixtures.md` — fixture DAG, shared fixtures, hooks
- `reference/cli.md` — the full CLI surface and flags
- `reference/refactoring.md` — restructuring ladder, smells → moves
- `reference/ci.md` — CI workflow shape, the gotest action, linter coexistence
- `reference/migration.md` — testify/stdlib → gotest
