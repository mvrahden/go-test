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

This skill describes **v1.32**. On an older release, read
`reference/versions.md` before writing anything: it lists what each older
release lacks or does differently, including rules below that fail there
(on v1.25.x, rules 1, 3 and 4). A rule tagged **v1.27+** (or later) needs
that release; skip it on older ones.

## Bootstrap

The repo has the *library*; the CLI runs via Go's tool directive (requires
Go ≥ 1.25; v1.30+ requires Go ≥ 1.26). One-time:

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest@$(go list -m -f '{{.Version}}' github.com/mvrahden/go-test)
go tool gotest version
```

Keep the `@version`: without it `go get -tool` upgrades the library pin,
which setup must never do (`@latest` only in a project with no pin yet). In
a vendored module (`vendor/modules.txt`) the go.mod edit breaks every build
until `go mod vendor` reruns, and gotest cannot run vendored at all (the
overlay's `pkg/gotestruntime` import is never vendored) — report, do not
work around.

Then every command is `go tool gotest <args>`; if Go reports the short name
ambiguous, use `go tool github.com/mvrahden/go-test/cmd/gotest`. Never a
global `go install` binary: it drifts from the pin and from the module's Go,
and releases after v1.28.1 refuse to run on either drift (`FAIL:` naming the
version or the Go it was built with, exit 2). That refusal is never a fault
in the tests; switch to `go tool gotest` instead of bumping the pin. (Bare
`go run github.com/mvrahden/go-test/cmd/gotest` fails on fresh consumers:
module pruning leaves the CLI's deps out of go.sum.)

In a `go.work` workspace the tool declared in any `use` module works from
the root and inside each module, at the highest pin. `./...` is rejected at
the root, so name each module (`go tool gotest ./svc/... ./lib/...`) or run
inside one. The `go.work` go line must be at least the modules' go lines.

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
outer-`t` in poll callbacks, testify idioms, focus leftovers, and
`fail-guard`: any `if cond { gotest.Fail(t, …) }` or `if err != nil {
t.Fatal(err) }` guard — assertions halt on failure, so state them
directly: `gotest.NoError(t, err)`, never a guarded fail). Fixes can
compose — a rewritten guard may itself be simplifiable — so re-run
`lint -fix` until it reports nothing. When two findings share a construct
the linter reports only the stronger one (integrity over style); fix what
it says before expecting style suggestions there.

Suppression follows rule tiers: integrity rules (`poll-scope`,
`suite-lifecycle`, `focus`, …) accept only per-line `//nolint:<rule>`;
style and migration rules can also be skipped project-wide via
`.gotest.yml` `lint.skip` or `-skip-<rule>` flags. The `stdlib-test` rule
flags every stdlib `TestXxx(*testing.T)` — when a stdlib test is
intentional (assertion-layer tests, benchmarks-adjacent code), mark its
package clause with `//nolint:stdlib-test`, or the lint run fails. The
linter does NOT catch: plain `defer` cleanup, shared mutable state in
parallel suites, or structural problems — those are your job, below.

## Core rules

1. **Suites are structs, naming is the API.** `type XxxTestSuite struct{}`,
   exported, methods `func (s *X) TestBehavior(t *gotest.T)`. No `TestMain`,
   no registration — the CLI generates the harness invisibly (never commit
   the `gotest_psuite_test.go`/`gotest_pxsuite_test.go` files `gotest
   generate` writes; `gotest clean` removes them). `F_`/`X_` prefixes
   focus/exclude; `Test*Async(t, done)` declares async tests.
2. **Lifecycle hooks own resources.** Setup in `BeforeEach` (or `BeforeAll`
   for expensive read-only state), teardown in `AfterEach`/`AfterAll` as
   suite fields — NEVER `defer` or `t.T().Cleanup` in a test method. A plain
   `defer` lints clean and is still wrong: it skips teardown verification
   and blocks parallelization.
3. **Do not imitate existing tests blindly.** Observed failure: agents copy
   a repo's existing `SuiteConfig()` verbatim, propagating anti-patterns. A
   `SuiteConfig()` marker states *intent*: omit it entirely for defaults.
   A duration the marker leaves at zero gets the default, as if the marker
   were absent; `gotest.NoDeadline` disables one. On v1.26–v1.29 a
   zero/omitted duration meant NO deadline instead — compose onto a preset
   there.
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
7. **Ask "why is this wall-clock-asserting suite NOT Exclusive?"
   (v1.27+)** — the counterpart to rule 4. A suite whose assertions or
   timeout budgets measure elapsed time (latency bounds, timing budgets,
   contended ports/containers) cannot share a saturated machine: mark it
   `SuiteConfig{Exclusive: true}` (statically parsed like `Parallel` —
   boolean literal only; see `reference/config.md`) and it dispatches
   strictly alone after all other suites finish. A budget verdict taken
   under load is not a verdict you can act on.
8. **Write the condition, not the connective (v1.29+).** `t.When("email
   is valid")`, never `t.When("when email is valid")`; `t.It("creates the
   user")`, never `t.It("it creates the user")`. The spec renders every
   `When` label as `when <condition>` on every surface (terminal, JSON,
   discovery, the editor's tree and Spec View) and the ✓/✗ glyph plays
   the role of "it", so the word in the source is said twice. A `When`
   that opens with its own connective (`with …`, `given …`, `after …`,
   `if …`, …) is rendered as written. The `behavior-wording` rule flags
   the redundant word and `lint -fix` drops it. Subtest *names* never
   carry the connective — `-run` filters and snapshot keys are unaffected.

9. **Fuzz targets are suite methods, and they assert a property
   (v1.29+).** `func (s *XTestSuite) FuzzParse(f *gotest.F)`: `f.Add`
   typed seeds first, then `f.Fuzz(func(t *gotest.T, in …) { … })` whose
   body asserts something the input must satisfy (round-trip, idempotence,
   no panic is not enough — `fuzz-no-oracle`). Never a top-level
   `func FuzzX(*testing.F)`: gotest ignores it on every version. Struct
   and named-type arguments are fine and fan out per field; the refused
   shapes and the crasher loop are in `reference/fuzzing.md`. Search with
   `go tool gotest fuzz --for=30s ./...`; a crasher becomes a seed through
   `gotest fuzz promote`, never a hand-committed corpus file for a struct
   target.

10. **Gate on the environment with `SuiteGuard`, never a skip.** A suite
    that needs what a machine may lack (a `DATABASE_URL`, Docker, a
    credential) declares `func (s *X) SuiteGuard() string`: empty runs the
    suite, anything else skips it with that reason. It runs before the
    suite's config and `BeforeAll`, so the suite's own setup never starts;
    `t.T().Skip()` in `BeforeAll` runs after setup has begun. Fixtures the
    suite binds still set up before the guard, so a guard cannot keep a
    fixture from starting.

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
- `reference/fuzzing.md` — fuzz targets, seeds, struct arguments, the crasher loop
- `reference/versions.md` — what older releases lack or do differently
