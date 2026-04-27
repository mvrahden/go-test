# Superspec Status Overview

Comparison of `docs/superspec.md` against the current state of the project.

Last updated: 2026-04-27

---

## Tier 1 — Ship It

### 1.1 Focus Guard: `--ci`

**Status: Complete**

- `--ci` flag implemented and wired in default run mode
- `RunFocusGuard` performs static analysis scan before generation
- `FocusViolation` struct reports suite name, method name, file location
- Fails with non-zero exit code when `F_` prefixes detected

### 1.2 Assertion Library

**Status: Complete**

- Zero external dependencies (no testify, no go-spew, no go-difflib)
- Generic type-safe functional API (`gotest.Equal[T]`, `gotest.Greater[T cmp.Ordered]`, etc.)
- Fluent API (`t.Assert(v).Equal(w)`, `t.Assert(v).IsTrue()`, etc.)
- Core implementation in `pkg/gotest/internal/assert/` (~300 lines, pure stdlib)
- All functions from superspec implemented:
  - Equality: `Equal`, `NotEqual`
  - Zero/Empty: `Zero`, `NotZero`, `Empty`, `NotEmpty`
  - Bool: `True`, `False`
  - Error: `NoError`, `Error`, `ErrorIs`, `ErrorAs`, `ErrorContains`
  - Collection: `Contains`, `NotContains`, `Len`, `ElementsMatch`, `Subset`
  - Comparison: `Greater`, `GreaterOrEqual`, `Less`, `LessOrEqual`
  - String/Regex: `Regexp`
  - Numeric: `InDelta`
  - Serialization: `JSONEq` (string, []byte, json.RawMessage, io.Reader, marshalable)
  - Time: `TimeWithin`, `TimeIsNow`
  - Panic: `Panics`
  - Async: `Eventually`, `Consistently` (boolean polling forms)
  - Unwrap: `Must[T]`
- Diff output for equality failures with `-`/`+` markers
- All functions accept `testingT` interface (works with both `*gotest.T` and `*testing.T`)
- Dogfooded: project's own tests use this library exclusively

### 1.3 CLI Distribution

**Status: Mostly complete**

| Aspect | Spec | Current | Gap |
|--------|------|---------|-----|
| Binary name | `gotest` | `gotest` | None |
| Install path | `go install github.com/mvrahden/go-test/cmd/gotest@latest` | Works | None |
| Release binaries | Cross-platform via goreleaser | GitHub Actions matrix (linux/darwin/windows × amd64/arm64) | Different tool, same outcome |
| Version injection | Tag + hash at build time | `about.Version` set via `-ldflags -X` in release workflow | None |
| `gotest version` output | Prints version, Go version, OS/arch | Prints `github.com/mvrahden/go-test (version)` | Missing: Go version, OS/arch |

### 1.4 README and Examples

**Status: Not assessed** (declared out of scope for current plan)

- `examples/` directory exists with stdlib, simple_suite, focus_exclude, parallel_suite, generic_suite
- README exists but may not match superspec's prescribed structure

---

## Tier 2 — Adopt It

### 2.1 Auto-Scaffolding: `gotest scaffold`

**Status: Complete**

- `gotest scaffold ./pkg/user.UserService` generates suite skeleton
- Struct scaffolding: generates `TestSuite` with `BeforeEach` and per-method `Test*` stubs
- Interface scaffolding: generates generic contract suite with factory pattern
- Uses `packages.Load` for type introspection
- Method signatures included as comments in generated stubs
- Return types inform stub structure (error-returning methods get happy + error case)

### 2.2 Migration Path: `gotest migrate`

**Status: Complete**

- `gotest migrate ./...` converts testify/suite tests
- Renames suite structs to `*TestSuite` convention
- Renames lifecycle hooks (`SetupSuite` → `BeforeAll`, etc.)
- Transforms assertion calls
- Removes `suite.Run` boilerplate
- Internal implementation in `internal/migrate/`

### 2.3 BDD Vocabulary: `t.When()`

**Status: Complete**

- `t.When(description, fn)` implemented on `*gotest.T`
- Maps to `t.T().Run()` under the hood (same as `t.It()`)
- Semantic distinction: `When` = context grouping, `It` = leaf expectation
- Spec renderer understands the hierarchy
- Used extensively in project's own test suites

---

## Tier 3 — Love It

### 3.1 Behavior Specification: `gotest spec`

**Status: Complete**

- `gotest spec ./...` runs tests and renders behavioral specification
- Internally runs `go test -json` and parses event stream
- Reconstructs suite→method→When/It hierarchy from `/`-separated test paths
- Strips Go naming conventions for display (`TestUserServiceTestSuite` → `UserService`)

| Feature | Spec | Current | Gap |
|---------|------|---------|-----|
| Terminal output (color tree) | Default | Default | None |
| Markdown output | `--format=md` | `--format=md` | None |
| File output | `--output=path` | `--output=path` | None |
| Append to normal output | `--spec` flag | `--spec` flag | None |
| No-color mode | Not in spec | `--no-color` | Addition beyond spec |
| Summary line | Suite/behavior/test counts | Suite/behavior/test counts | None |
| Focus/exclude rendering | `— FOCUSED` / `— SKIPPED` labels | Implemented | None |
| Failed test error output | Inline under failed leaf | Implemented | None |

### 3.2 Watch Mode

**Status: Complete**

- `gotest watch ./...` re-runs on file changes
- Uses `fsnotify` for filesystem watching
- Debouncing on rapid changes
- Terminal clear between runs
- `--spec` flag supported in watch mode
- Focus integration: `F_` prefix narrows re-runs

### 3.3 Data-Driven Testing: `t.Each()`

**Status: Complete**

- Callback API: `t.Each(entries, func(tt *gotest.T, tc Entry) { ... })`
- Iterator API: `for tt, tc := range gotest.Each(t, entries) { ... }` (Go 1.23+ `iter.Seq2`)
- `Desc` field → subtest name
- `Name` field → subtest name (fallback)
- No Desc/Name → index-based naming (`#0`, `#1`, ...)
- Empty slice → no subtests run

### 3.4 Async Assertions: `t.Eventually()` / `t.Consistently()`

**Status: Complete**

Two tiers as specified:

| Tier | API | Status |
|------|-----|--------|
| Functional (boolean) | `gotest.Eventually(t, func() bool, waitFor, tick)` | Complete |
| Functional (boolean) | `gotest.Consistently(t, func() bool, waitFor, tick)` | Complete |
| T method (rich) | `t.Eventually(waitFor, tick, func(poll *T))` | Complete |
| T method (rich) | `t.Consistently(waitFor, tick, func(poll *T))` | Complete |

- Collecting poll wrapper: intermediate failures captured, not propagated
- On timeout: reports last poll's assertion failures with location
- `Consistently` fails on first `false` poll and reports which poll failed

### 3.5 Snapshot Testing: `t.MatchSnapshot()`

**Status: Complete (minor interface deviation)**

| Aspect | Spec | Current | Gap |
|--------|------|---------|-----|
| Storage location | `testdata/__snapshots__/<TestName>.snap` | `testdata/__snapshots__/<TestName>.snap` | None |
| First run behavior | Create snapshot, pass | Create snapshot, pass | None |
| Mismatch behavior | Fail with diff | Fail with diff | None |
| Multi-snapshot | `t.MatchSnapshot(v, "name")` | `t.MatchSnapshot(v, "name")` | None |
| Update mechanism | `gotest ./... --update-snapshots` (CLI flag) | `GOTEST_UPDATE_SNAPSHOTS=1` (env var) | Interface deviation |

---

## Tier 4 — Depend On It

### 4.1 Semantic Test Coverage: `gotest coverage`

**Status: Not implemented**

- `coverage` is registered in `knownSubcommands` (arg parser recognizes it)
- No handler in CLI switch — falls through to default mode silently
- No implementation exists (no `coverage.go`, no `internal/coverage/` package)
- Spec describes: static analysis comparing production API surface vs suite test inventory
- Would require `packages.Load` on production code + cross-referencing suite methods

### 4.2 CI Integration

**Status: Partially complete**

| Aspect | Spec | Current | Gap |
|--------|------|---------|-----|
| GitHub Actions workflow | `test.yml` | Exists with matrix (1.24, 1.25, 1.26) | None |
| `--ci` in CI | Safety net for `F_` prefixes | Implemented | None |
| Spec in CI summary | Markdown or rendered output | `--no-color --output=spec.txt` in code fence | None |
| Release workflow | Cross-platform binaries | Exists (6 targets) | None |
| `setup-gotest` action | `uses: mvrahden/setup-gotest@v1` | Not created | Missing |
| Coverage step in CI | `gotest coverage ./... --min=80` | Not possible (coverage not implemented) | Blocked |
| Exit codes | Match `go test` (0/1/2) | Implemented | None |

### 4.3 Go Generate Integration

**Status: Not wired**

- `generate` is registered in `knownSubcommands` (arg parser recognizes it)
- No handler in CLI switch — falls through to default mode silently
- The generation pipeline exists internally (`gotestgen`) but isn't exposed as generate-only mode
- Spec: runs generation step only (no test execution, no cleanup)
- Use case: `//go:generate gotest generate ./...` for checked-in generated files

### 4.4 Linter: `gotest-lint`

**Status: Not implemented**

- No code exists for this
- Spec describes: standalone binary or golangci-lint plugin
- Would detect: lifecycle hook typos, value receivers, missing AfterAll, committed F_ prefixes, orphaned ƒƒ_ files
- Partially overlaps with `--ci` (focus detection)

---

## CLI Interface — Flag Migration

### Subcommands

| Subcommand | Registered | Handled | Status |
|------------|-----------|---------|--------|
| *(default)* | — | Yes | Complete |
| `watch` | Yes | Yes | Complete |
| `spec` | Yes | Yes | Complete |
| `scaffold` | Yes | Yes | Complete |
| `migrate` | Yes | Yes | Complete |
| `coverage` | Yes | **No** | Stub only |
| `generate` | Yes | **No** | Stub only |
| `clean` | **No** | No | Missing entirely |
| `version` | Yes | Yes | Complete |
| `help` | Yes | Yes | Complete |

### Flags

| Flag | Spec | Current | Gap |
|------|------|---------|-----|
| `--ci` | Fail on `F_` prefixes | Implemented | None |
| `--spec` | Append spec after output | Implemented | None |
| `--debug` | Keep generated files | `-ƒƒ.internal.debug` only | Not aliased to `--debug` |
| `--update-snapshots` | Regenerate snapshots | `GOTEST_UPDATE_SNAPSHOTS=1` env var | Interface deviation |
| `--format=<fmt>` | Output format for spec/coverage | Implemented for spec | None |
| `--output=<path>` | Write output to file | Implemented for spec | None |
| `--no-color` | Strip ANSI from terminal output | Implemented for spec | Addition beyond spec |
| `--min=<pct>` | Minimum coverage threshold | Not implemented | Blocked on coverage |

### Old `-ƒƒ.*` Flags

| Old Flag | New Flag | Migration Status |
|----------|----------|-----------------|
| `-ƒƒ.internal.debug` | `--debug` | Old form still primary; new form not recognized |
| `-ƒƒ.clean` | `gotest clean` | Old form may still work; subcommand not implemented |
| `-ƒƒ.ci` | `--ci` | `--ci` works; unclear if old form still recognized |
| `-ƒƒ.watch` | `gotest watch` | Subcommand works; old form status unclear |
| `-ƒƒ.generate` | `gotest generate` | Neither works (subcommand not wired) |
| `-ƒƒ.version` | `gotest version` | Subcommand works |
| `-ƒƒ.update-snapshots` | `--update-snapshots` | Uses env var instead; neither flag form exists |

Spec says: old forms should be recognized as deprecated aliases during transition, then removed.

---

## Advanced Patterns

### Nested Suites via Embedding

**Status: Complete**

- Fixture detection and embedding implemented in `gotestast`/`gotestgen`
- Parent/child lifecycle hook chaining in generated code
- Spec renderer understands parent-child nesting
- Extensive test coverage in `internal/gotestgen/` (Collector, Renderer tests)

### Contract Testing via Generic Suites

**Status: Complete**

- Generic struct definitions with instantiated type aliases
- Each alias produces independent test suite
- Tested in `examples/generic_suite/`
- Constraint: only works in same-package tests (ptest), not pxtest

### Resource Lifecycle Guarantees

**Status: Complete**

- `AfterAll` registered via `t.Cleanup` before `BeforeAll` runs
- `AfterEach` is `defer`-ed (runs on `t.Fatal()`/`runtime.Goexit()`)
- `sync.WaitGroup` for parallel test coordination
- `wg.Done()` deferred to prevent deadlocks

---

## Design Principles Compliance

| Principle | Status | Notes |
|-----------|--------|-------|
| 1. Standard Go output | Compliant | All output is `go test` output; spec is post-processing |
| 2. Naming IS the API | Compliant | No config files, no struct tags, no annotations |
| 3. Zero runtime cost | Compliant | Generated code only; no reflection at test time |
| 4. Invisible until needed | Compliant | Suite structs are self-documenting; missing generated file = compile error |
| 5. Adopt incrementally | Compliant | Traditional `func Test*` coexists with suites |

---

## Non-Goals (Confirmed Not Built)

- Test dependency ordering — not implemented (correct)
- Mocking framework — not implemented (correct)
- Decorator/annotation syntax — not implemented (correct)
- Runtime suite registration — not implemented (correct)
- Cross-package suite inheritance — not implemented (correct)
- Replacing `go test` output — not implemented (correct)

---

## Architecture vs. Spec

### Phase 1 (Current)

```
cmd/gotest → gotestrunner → gotestgen → gotestast
                                         └── templates
pkg/gotest
  ├── T, Assert (fluent), It, When, Each, Eventually, Consistently, MatchSnapshot
  ├── Equal, NoError, ErrorIs, ... (functional assertions)
  └── internal/assert (~300 lines, pure stdlib, zero deps)
```

**Assessment:** Matches Phase 2 of the architecture evolution (scaffold + spec + most of the T methods). The project has reached Phase 2 maturity with Phase 3 watch mode already done.

### What's Missing for Phase 2 Completion

- `coverage` subcommand (static analysis, no test execution needed)
- `generate` subcommand (generation without execution)
- `clean` subcommand (filesystem walk + delete)

### What's Missing for Phase 3

- `gotest-lint` (go/analysis analyzers)
- Nested suite embedding detection is already done (ahead of spec's phase 3 timeline)

---

## Summary: Remaining Work

### Quick Wins (< 1 hour each)

1. Wire `--debug` as alias for `-ƒƒ.internal.debug`
2. Add `clean` subcommand (walk + regex delete)
3. Wire `generate` subcommand (call pipeline without test execution or cleanup)
4. Enrich `version` output with `runtime.Version()`, `GOOS`, `GOARCH`
5. Add `--update-snapshots` flag (sets env var before test execution)
6. Deprecation warnings for `-ƒƒ.*` flags on stderr

### Medium Effort (hours)

7. Stub `coverage` subcommand with "not yet implemented" message
8. Create `setup-gotest` GitHub Action for external consumers
9. Update `docs/spec.md` to reflect current state (it still references old `-ƒƒ.*` interface and incomplete assertion stubs)

### Large Effort (days)

10. Implement `gotest coverage` — semantic coverage analysis
11. Implement `gotest-lint` — go/analysis static checks
12. README rewrite per superspec §1.4 structure

### Spec Document Drift

`docs/spec.md` is now outdated in several areas:
- Still documents `-ƒƒ.*` flags as primary interface
- Lists `ContainsAll`/`ContainsAny` as stubs (they may have been removed/replaced)
- Doesn't mention `When`, `Each`, `Eventually`, `Consistently`, `MatchSnapshot`
- Doesn't mention `spec`, `scaffold`, `migrate` subcommands
- Doesn't mention functional assertion API

Consider either updating `spec.md` to match reality or deprecating it in favor of `superspec.md` as the canonical reference.
