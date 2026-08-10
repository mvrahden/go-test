# Assertions

All package-level, first argument `t` (a `*gotest.T`, subtest `t`, or the
`poll *gotest.R` handle inside `Eventually`/`Consistently`). Failures always
report the user call site automatically — never call `t.T().Helper()`.

Assertions ARE the control flow: they halt the test on failure, so never
wrap them (or manual fails) in guards — `if err != nil { gotest.Fail(t,
…) }` and `if got != want { t.Fatal(…) }` are `fail-guard` lint errors;
write `gotest.NoError(t, err)` / `gotest.Equal(t, want, got)` directly.
`Equal`/`NotEqual` take the EXPECTED value first — the linter's autofix
moves constants into that slot.

| Assertion | Notes |
|---|---|
| `Equal[V]` / `NotEqual[V]` | typed; large values render expanded with a line diff |
| `NoError` / `Error` | error presence |
| `ErrorIs` / `ErrorAs[E]` / `ErrorContains` | prefer over `Error`+string poking; `ErrorContains` fails on nil error |
| `True` / `False` | last resort — the `assertion-simplify` lint rule rewrites `True(t, x != nil)` and friends |
| `Nil` / `NotNil` | for non-comparable nilables (slices/maps/funcs) — pointers are comparable, prefer `Zero`/`NotZero`; type-guard error on non-nilable types |
| `Empty` / `NotEmpty` / `Len` | prefer `Empty` over `Nil` for slices/maps unless nil-vs-empty matters |
| `Zero[V]` / `NotZero[V]` | comparable zero values |
| `Contains` / `NotContains` / `Subset[V]` / `ElementsMatch[V]` | containers |
| `Greater[V]` / `GreaterOrEqual[V]` / `Less[V]` / `LessOrEqual[V]` / `InDelta[V]` | ordered / numeric |
| `Regexp[P]` / `JSONEq` | string shapes |
| `Panics` | panic expectation |
| `TimeWithin` / `TimeIsNow` | time comparison |
| `Eventually` / `Consistently` | `(t, waitFor, tick, func(poll *R))` — pass `poll` to inner assertions; outer `t` inside the callback is a lint error (`poll-scope`) |
| `Fail` | unconditional failure (Errorf + FailNow) — never as an if-guard body; state the assertion instead (`fail-guard`) |
| `MatchSnapshot` | golden snapshots; update via `go tool gotest ./... --update-snapshots` (read-only under `--ci`/CI envs) |
| `Must[T]` | panic-on-error value extraction, for setup code |
| `Each[E]` | table driver: `for t, tc := range gotest.Each(t, entries)` — subtest names come from a `Desc` or `Name` string field |

The poll handle `*gotest.R` intentionally satisfies testify's `TestingT`
contract (`Errorf`+`FailNow`) — its surface is `Errorf`, `FailNow`,
`Failed`, `Message`, nothing else; `poll.T()` does not compile.
