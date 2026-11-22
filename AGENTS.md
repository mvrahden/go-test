# gotest — Agent Guide

gotest is a code-generation framework for Go test suites: suite structs with lifecycle methods, a generator that writes the test functions, a runner that gives each suite a process.
This file is for an agent working in this repository. The reference for writing gotest tests is the skill in [`skills/writing-gotest-tests`](skills/writing-gotest-tests): read `SKILL.md` before writing or changing a test, and look signatures up in [`reference/api.md`](skills/writing-gotest-tests/reference/api.md).
Every behavioral promise gotest makes is stated once in [`specs/`](specs/README.md), each scenario bound to a test.

## Rules

These override any default instincts from stdlib `testing` or other Go test frameworks.

**Never call `t.T().Helper()` when using gotest assertions.**
gotest automatically resolves assertion failures to the correct call site in your test code.
Calling `t.T().Helper()` is counterproductive — it removes useful location information from failure output.
The same holds for `b.B().Helper()` and `f.F().Helper()`.
Without it, failures show both where the assertion was called (inside the helper) and where the helper was called (in the test).
This applies to all gotest assertion functions (`gotest.Equal`, `gotest.True`, etc.).
If you pass `t.T()` to third-party code that uses Go's standard `t.Errorf`, Go's `t.Helper()` convention applies as normal.
This holds in a module whose path has no dot and under `-trimpath` too ([Failure locations](specs/assertions/spec.md#requirement-failure-locations)).

**Never call `t.T().Cleanup()` inside suite methods.**
Use `BeforeAll`/`AfterAll` for suite-level resources and `BeforeEach`/`AfterEach` for per-test resources.
The generated harness manages cleanup ordering (LIFO).
Calling `t.T().Cleanup()` inside suite methods breaks the predictable lifecycle.
Nor `b.B().Cleanup()`/`f.F().Cleanup()` — benchmarks get `BeforeEach`/`AfterEach` per method, fuzz targets `BeforeAll`/`AfterAll`.

**Never call `t.T().Fatalf()` in reusable test helpers.**
Use gotest assertions instead.
Inside `Eventually`/`Consistently` the callback gets a `*gotest.R`, which has no `T()` method, so a helper that needs `t.T()` cannot be used there at all.
Under a benchmark every hook's `t.T()` is nil, under a fuzz target `BeforeAll`'s and `AfterAll`'s (its `BeforeEach`/`AfterEach` run per input with a real `*testing.T`); the generator refuses a hook whose own body calls it there unchecked, but cannot see a helper the hook calls ([Hooks that call t.T() where it is nil](specs/cli/spec.md#requirement-hooks-that-call-tt-where-it-is-nil)).
Helpers built on gotest assertions work in every position.

**Never use `t.T().Skip()` for environment gating.**
Use `SuiteGuard() string` instead.
SuiteGuard runs before the suite's fixtures, config resolution, `BeforeAll`, and every test method — the suite's own setup never executes.
A shared fixture the suite binds is the exception: the runner starts it anyway, and its setup failure fails the suite before the guard is evaluated ([A guard does not hold back a shared fixture](specs/fixtures/spec.md#requirement-a-guard-does-not-hold-back-a-shared-fixture)).
A guard reads the environment, not fixture fields: they are still nil when it runs.
`t.T().Skip()` inside `BeforeAll` runs even later, after suite setup has started.

**`Nil`/`NotNil` are for non-comparable nilables only.**
They carry a runtime type guard: slices, maps, funcs, channels, pointers, interfaces, and unsafe pointers are accepted; any other type fails with a type-guard error (the `assertion-type-guard` lint rule catches misuse statically).
Use `Zero`/`NotZero` for comparable nilables (pointers, interfaces, channels).
For slices and maps, prefer `Empty`/`NotEmpty` — a nil slice and an empty slice are equivalent for most test intent; reach for `Nil`/`NotNil` only when the nil vs empty distinction matters.

**`HasPrefix`/`HasSuffix` assertions do not exist.**
Use `Regexp(t, "^prefix", str)` or `Regexp(t, "suffix$", str)`.

**Prefer specific assertions over generic boolean checks.**
The linter enforces assertion simplification.
`True(t, a == b)` → `Equal(t, a, b)`. `True(t, err != nil)` → `Error(t, err)`. `True(t, len(x) == 0)` → `Empty(t, x)`. `Equal(t, true, v)` → `True(t, v)`. `Equal(t, nil, v)` → `Zero(t, v)`. `Len(t, x, 0)` → `Empty(t, x)`.
The more specific assertion produces better failure messages.

**Always pair `BeforeAll` with `AfterAll` in suites.**
If `BeforeAll` acquires resources, add an `AfterAll` to release them.
The linter flags unpaired `BeforeAll` as a likely resource leak.

## Working in This Repository

- **The gate.** `make test` builds, vets and runs every suite of the module and of `examples/` through gotest's own runner, with the race detector and the coverage floor. `make lint` runs gotest's linter. `make checks` runs gofmt, govulncheck, golangci-lint and the runtime compatibility checks. `make drill` applies each mutant under `tests/drill/mutants` and requires the suites to catch it. Run one package with `go run ./cmd/gotest ./path/`.
- **This repository tests itself with gotest.** Tests are suites; a plain `func Test*` is a lint finding unless its file says why. The code every other test's reporting rests on uses raw checks instead of gotest assertions; `ARCHITECTURE.md`, "Testing gotest", says which code that is and why.
- **A behavior change starts in `specs/`.** State the requirement and its scenarios in `specs/<capability>/spec.md`, bind each scenario to a `When`/`It` in `tests/conformance`, and regenerate the catalog with `make catalog`. [`specs/README.md`](specs/README.md) has the format; `tests/conformance/README.md` has the binding rules.
- **The reference lists are written by hand and checked.** Keep their groups and wording; `tests/conventions/reference_suite_test.go` fails when the skill's `api.md` misses an assertion or lists a signature the source does not declare, when the README's assertion tour or the website lacks an assertion or a config field, or when a `GOTEST_*` variable the code reads is not tabled. The website's flag table is checked with the spec's in `cmd/gotest`.
- **Guides link to `specs/` and never quote it.** `README.md`, this file, `ARCHITECTURE.md`, `docs/` and the skill explain and link a requirement; a suite fails on a restated clause and on a link that names no heading.
- **Never commit generated files.** `gotest_psuite_test.go` and `gotest_pxsuite_test.go` are written by `gotest generate` and removed by `gotest clean ./...`.
- **What a test binary links imports the standard library and this module only.** `pkg/gotest` and `pkg/gotestruntime` are linked into every user's test binary; `pkg/lint` is the analyzer and needs `golang.org/x/tools`.
- **Run the CLI from the tree**, `go run ./cmd/gotest <args>`, never a `gotest` binary found on `PATH`.
