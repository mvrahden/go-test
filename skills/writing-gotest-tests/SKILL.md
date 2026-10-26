---
name: writing-gotest-tests
description: Use when writing, fixing, reviewing, or restructuring tests in a Go repository that uses the gotest framework (github.com/mvrahden/go-test) — `*TestSuite` structs, `gotest.T`, `BeforeEach`/`AfterAll` hooks, `*Fixture` types, or the `gotest` CLI — when migrating testify or stdlib tests to gotest, and when setting up or fixing CI for a gotest repository.
---

# Writing gotest tests

gotest inverts habits learned from stdlib/testify. Follow this file's rules;
consult `reference/` only when the task touches that area — except
`reference/setup.md`, which every task follows before writing anything.

## Start here: version and upgrade

The repo has the *library*; the CLI runs via Go's tool directive, so every
command is `go tool gotest <args>` — never a global `go install` binary.
Before writing anything, check the pin and what the module can take:

```sh
go list -m -u -f '{{.Version}} {{with .Update}}{{.Version}}{{end}} {{with .Replace}}(replaced){{end}}' github.com/mvrahden/go-test
go list -m -f '{{.GoVersion}}'
go list -m -f '{{.Version}} {{.GoVersion}}' $(go list -m -versions github.com/mvrahden/go-test | tr ' ' '\n' | sed -n 's|^v|github.com/mvrahden/go-test@v|p')
```

The first line prints the pin (and a newer release, if any); the second
the module's go line; the third every release with its go line. Upgrade to
the newest release whose go line is not newer than the module's, when it
is newer than the pin —
`go get -tool github.com/mvrahden/go-test/cmd/gotest@<release>` — never to
`@latest` blindly: `go get` would silently raise the project's Go. Do not
upgrade when the pin is `(replaced)` (a source build: assume current
behavior), the module is vendored, instructions rule out dependency
changes, the task only reads code, or the check fails. Then follow
`reference/setup.md` (after-upgrade steps, keeping the pin, a module
without go-test yet, `go.work`, vendoring). A `FAIL:` refusal about a
drifted binary is never a fault in the tests: switch to `go tool gotest`;
never upgrade just to silence it.

This skill describes **v1.35**. On a release you could not upgrade from,
read `reference/versions.md` before writing anything: it lists what each
older release lacks or does differently, including rules below that fail
there (on v1.25.x, rules 2, 3 and 10). A rule tagged **v1.27+** (or later)
needs that release; skip it on older ones.

## The Two Runners — a complete run is BOTH commands

`go test` ignores gotest suites (signature incompatibility, by design).
`gotest` runs suites and never runs stdlib tests — it prints a stderr
`note: N stdlib test(s) … not run` when it skips them (and `[no suites]`
for a package holding nothing else). Running only one command silently
misses tests. Always finish with:

```sh
go tool gotest ./...
go test ./...
```

Add `-race` to both before calling anything done.

## The write loop

Write → lint → both runners → fix:

```sh
go tool gotest lint -fix ./...
go tool gotest lint ./...
```

`lint -fix` applies every suggested fix, formats the files it rewrites and
removes the imports its fixes left unused, so no gofmt or goimports pass
is needed after it. Findings without a fix (`t.T().Helper()`, a
`t.T().Log`, a stdlib test, …) need your edit: from v1.33 `-fix` lists them
and exits 3; below, it exits 0 and prints nothing, so only the plain
`lint` run shows them. Move on when plain `lint` exits 0.

The linter catches direct misuse (`t.T()` escapes even inside closures,
outer-`t` in poll callbacks, testify idioms, focus leftovers, and
`fail-guard`: any `if cond { gotest.Fail(t, …) }` or `if err != nil {
t.Fatal(err) }` guard — assertions halt on failure, so state them
directly: `gotest.NoError(t, err)`, never a guarded fail). Fixes can
compose — a rewritten guard may itself be simplifiable; from v1.33 one
`-fix` applies those too, below it re-run the pair until plain `lint` is
clean. When two findings share a construct
the linter reports only the stronger one (integrity over style); fix what
it says before expecting style suggestions there.

Suppression follows rule tiers: integrity rules (`poll-scope`,
`suite-lifecycle`, `focus`, …) can never be skipped project-wide; style
and migration rules can, via `.gotest.yml` `lint.skip` or `-skip-<rule>`
flags. Any rule yields to a `//nolint:<rule>` comment on the finding's
line or on a line of its own directly above it — and to one trailing the
`package` line itself (`package shop_test //nolint:stdlib-test`), which
covers the whole FILE (not the package), whatever the tier; on the line
above `package` it suppresses nothing. The `stdlib-test` rule flags every
stdlib `TestXxx(*testing.T)` (not `Benchmark`, `Fuzz` or `Example`
functions — those show only in the runner's note) — when a stdlib test is
intentional (assertion-layer tests, benchmarks-adjacent code), mark it
that way, or the lint run fails. The linter does NOT catch: plain `defer`
cleanup, shared mutable state in parallel suites, or structural problems —
those are your job, below.

## Core rules

1. **Suites are structs, naming is the API.** `type XxxTestSuite struct{}`,
   exported, methods `func (s *X) TestBehavior(t *gotest.T)`. No
   registration and no `TestMain` needed — the CLI generates the harness
   invisibly (never commit the `gotest_psuite_test.go`/
   `gotest_pxsuite_test.go` files `gotest generate` writes; `gotest clean ./...`
   removes them). A `TestMain` of your own still runs; keep resource setup
   out of it — that is what fixtures and `BeforeAll` are for. `F_`/`X_`
   prefixes focus/exclude. A generic suite runs only through one
   type alias per concrete type
   (`type ArticleIndexTestSuite = IndexContractTestSuite[Article]`);
   without an alias it silently never runs.
2. **Lifecycle hooks own resources.** Setup in `BeforeEach` (or `BeforeAll`
   for expensive read-only state), teardown in `AfterEach`/`AfterAll` —
   per-test state on suite fields in a sequential suite, in the returned
   context in a parallel one (rule 4). NEVER `t.T().Cleanup`, and no
   `defer` releasing a resource in a test method. Such a `defer` lints
   clean and is still wrong: it sits outside the teardown order the
   harness guarantees, and the setup cannot be shared or hoisted. Start
   goroutines with `gotest.Go(t, fn)`, never a bare `go`: a panic on a
   bare goroutine kills the process before any teardown runs.
   `wait := gotest.Go(…); defer wait()` is the one `defer` that belongs in
   a test, and only for work that finishes on its own — never wait in the
   test on a server or poller that `AfterEach` stops (it deadlocks).
   (v1.34+) A goroutine `BeforeAll` starts for `AfterAll` to stop goes on
   the hook's `t` or a `T` derived from it (`When`, `It`, `Each`,
   `NewTWithDeadline`; `gotest.NewT(t)` in a `*testing.T` hook), never on
   a subtest the hook starts with `t.Run`: its wait runs before `AfterAll`
   and deadlocks. Below v1.34 every such wait did.
   Every `BeforeAll` needs an `AfterAll` (`lifecycle-pair`, integrity
   tier). A suite's `AfterAll` runs even when its `BeforeAll` failed or
   panicked midway, so release only what was actually set up — nil-check
   each field.
3. **Do not imitate existing tests blindly.** Observed failure: agents copy
   a repo's existing `SuiteConfig()` verbatim, propagating anti-patterns. A
   `SuiteConfig()` marker states *intent*: omit it entirely for defaults.
   Every positive duration the marker returns — a preset's included — is a
   budget: a test that outlives it fails even when it ignores its context.
   A duration it leaves at zero gets the default and, like having no
   marker, only bounds the context; `gotest.NoDeadline` disables one. On
   v1.26–v1.29 a zero/omitted duration meant NO deadline instead — compose
   onto a preset there (`reference/config.md`).
4. **Ask "why is this suite NOT parallel?"** Observed failure: agents never
   parallelize unprompted, even when asked to improve tests. The recipe:

   ```go
   func (s *ShopTestSuite) SuiteConfig() gotest.SuiteConfig {
   	return gotest.SuiteConfig{Parallel: true}
   }

   type shopCtx struct{ inv *Inventory }

   func (s *ShopTestSuite) BeforeEach(t *gotest.T) *shopCtx {
   	return &shopCtx{inv: NewInventory()}
   }
   ```

   (v1.26–v1.29: that literal means no deadline at all — compose onto
   `gotest.DefaultSuiteConfig()` there, see `reference/config.md`.)
   Every test method then takes `(t *gotest.T, ctx *shopCtx)`, and an
   `AfterEach` takes `(t, ctx)` too — the generator enforces this, and
   rejects a `BeforeEach` that returns nothing in a parallel suite. A suite
   without fields (or with fixture pointers only) may be parallel with no
   `BeforeEach`. The generator cannot see methods writing suite fields —
   with no `BeforeEach`, or beside a returning one: that compiles, and
   races. A suite with `Benchmark*` or `Fuzz*` methods refuses a returning
   `BeforeEach` (and hooks taking `*testing.T`): move those methods to a
   sequential sibling suite. It also refuses a hook calling `t.T()` where
   that is nil — every hook under a benchmark, `BeforeAll`/`AfterAll` under
   a fuzz target — unless the hook checks it for nil; a helper the hook
   calls is not inspected, so keep `t.T()` out of those helpers.
   Legitimate reasons to stay sequential: `Setenv` (panics in parallel
   tests), shared live resources without per-test
   keys/schemas (`-race` is process-local and cannot see datastore
   contention — it is necessary, not sufficient).
5. **Poll, never sleep.** `gotest.Eventually(t, waitFor, tick, func(poll
   *gotest.R) { ... })` — assert a stable fixed point, not a transient
   state. The callback's `poll` handle has only `Errorf`/`FailNow`/
   `Failed`/`Message`; pass `poll` (not `t`) to assertions inside it.
6. **Never call `t.T().Helper()`** — call sites resolve automatically; the
   linter flags it. Reach for `t.T()` only when nothing on `gotest.T`
   (`It`, `When`, `Context`, `TempDir`, `Setenv`, `Skipf`, `Errorf`,
   `FailNow`) covers the need — never for `t.T().Context()`, which carries
   no suite `Timeout` (v1.33+: `t-escape` flags it, `-fix` writes
   `t.Context()`). There is no `Log`: context goes into an assertion's
   message arguments (`gotest.Equal(t, want, got, "order %s", id)`), and
   `t-escape` flags `t.T().Log`.
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
   `func FuzzX(*testing.F)`: gotest ignores it on every version (the one
   exception, a target that must also run under plain `go test -fuzz`, is
   in `reference/fuzzing.md`). Struct and named-type arguments are fine
   and fan out per field; the refused
   shapes and the crasher loop are in `reference/fuzzing.md`. Search with
   `go tool gotest fuzz --for=30s ./...`; a crasher becomes a seed through
   `gotest fuzz promote`, never a hand-committed corpus file for a struct
   target. Benchmarks are suite methods too:
   `func (s *XTestSuite) BenchmarkParse(b *gotest.B)` with the measured
   work inside `for b.Loop() { … }` (`reference/cli.md`).

10. **Gate on the environment with `SuiteGuard`, never a skip.** A suite
    that needs what a machine may lack (a `DATABASE_URL`, Docker, a
    credential) declares `func (s *X) SuiteGuard() string`: empty runs the
    suite, anything else skips it with that reason. It runs before the
    suite's package fixtures (v1.32+; see `reference/versions.md`), config
    and `BeforeAll`, so the suite's own setup never starts; `t.T().Skip()`
    in `BeforeAll` runs after setup has begun. Read the environment only:
    fixture fields are still nil in the guard. A shared fixture the suite
    binds is started by the runner regardless — no guard can stop it.

## When gotest itself is wrong

When gotest — or this skill — seems to misbehave, first check it really is
gotest's fault (most look-alikes are a stray binary or an old pin), then
draft a report and send it only on the user's yes: follow
`reference/reporting.md`. Never hide it behind a weakened or deleted test.

## Restructuring existing suites (the blue phase)

Enter only at a green pause point when: setup is duplicated across tests, a
third similar test is being added, a touched suite already smells, or you
were asked to clean up. Follow `reference/refactoring.md` for the ladder and
smells list. Non-negotiable safety invariants (tests protect nothing —
observed failure: agents restructure with no case accounting):

1. Capture the executed case LIST before and after, into separate files,
   and diff them:

   ```sh
   go tool gotest -json ./... > run-before.json
   go test -json ./... >> run-before.json
   grep -o '"Package":"[^"]*","Test":"[^"]*"' run-before.json | sort -u > cases-before.txt
   test -s cases-before.txt
   ```

   Both runs must exit 0 — a run that failed partway still leaves a
   non-empty list.

   Capture both runners (stdlib tests run only under `go test`) and the
   Package+Test PAIR — bare `Test` names collapse identically named suites
   across packages, hiding whole-package deletions.

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
- `reference/setup.md` — declaring, checking and upgrading the CLI; drift
  refusals, vendored modules, `go.work`
- `reference/config.md` — config semantics, budgets, presets, compose form
- `reference/fixtures.md` — fixture DAG, shared fixtures, hooks
- `reference/cli.md` — the full CLI surface and flags
- `reference/refactoring.md` — restructuring ladder, smells → moves
- `reference/ci.md` — CI workflow shape, the gotest action, linter coexistence
- `reference/migration.md` — testify/stdlib → gotest
- `reference/fuzzing.md` — fuzz targets, seeds, struct arguments, the crasher loop
- `reference/versions.md` — what older releases lack or do differently
- `reference/reporting.md` — telling a gotest bug from a look-alike, and reporting it
