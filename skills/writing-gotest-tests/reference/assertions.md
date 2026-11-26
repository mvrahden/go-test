# Assertions

All package-level, first argument `t` (a `*gotest.T`, subtest `t`, the
`poll *gotest.R` handle inside `Eventually`/`Consistently`, a `*gotest.B`
or `*gotest.F`, or a plain `*testing.T`). Failures always
report the user call site automatically — never call `t.T().Helper()`.
Full signatures are in `api.md`.

Assertions ARE the control flow: they halt the test on failure, so never
wrap them (or manual fails) in guards — `if err != nil { gotest.Fail(t,
…) }` and `if got != want { t.Fatal(…) }` are `fail-guard` lint errors;
write `gotest.NoError(t, err)` / `gotest.Equal(t, want, got)` directly.
An `Each` row's `t` is the exception (v1.42+): a failing assertion on it
fails the row and returns, and the row and the rows after it go on. So a
guard on a row's `t` that calls `FailNow` or returns, or that chains
conditions, is no assertion in disguise: `fail-guard` leaves it alone
([Failure guards on an Each row](https://github.com/mvrahden/go-test/blob/main/specs/lint/spec.md#requirement-failure-guards-on-an-each-row)),
as `t-escape` does an assertion given `row.T()`, which halts
([Escapes and redundant pairs on an Each row](https://github.com/mvrahden/go-test/blob/main/specs/lint/spec.md#requirement-escapes-and-redundant-pairs-on-an-each-row)).
`Equal`/`NotEqual` take the EXPECTED value first. The linter does not
check the order of an `Equal` you write; it only puts the constant first
when it rewrites a `True(t, a == b)` or a guard into one.

| Assertion | Notes |
|---|---|
| `Equal[V]` / `NotEqual[V]` | typed; values past one short line render field by field, with the changed lines marked in a diff under the path of each change (`at [1].Name:`); no addresses; different dynamic types are named |
| `Same[V]` / `NotSame[V]` | v1.30.2+; pointer identity on `*V`; `Equal` on pointers compares structure, and `assertion-simplify` rewrites `True(t, p == q)` to this |
| `NoError` / `Error` | error presence |
| `ErrorIs` / `ErrorAs[E]` / `ErrorContains` | prefer over `Error`+string poking; `ErrorContains` fails on nil error |
| `True` / `False` | last resort — the `assertion-simplify` lint rule rewrites `True(t, x != nil)` and friends |
| `Nil` / `NotNil` | for non-comparable nilables (slices/maps/funcs) — pointers are comparable, prefer `Zero`/`NotZero`; type-guard error on non-nilable types |
| `Empty` / `NotEmpty` / `Len` | prefer `Empty` over `Nil` for slices/maps unless nil-vs-empty matters |
| `Zero[V]` / `NotZero[V]` | comparable zero values |
| `Contains` / `NotContains` / `Subset[V]` / `ElementsMatch[V]` | containers; v1.34+: a needle of a type the container cannot hold fails both, naming the type to convert it to when that keeps its value — a string takes only string needles (`string('x')`, `string(b)`) |
| `Greater[V]` / `GreaterOrEqual[V]` / `Less[V]` / `LessOrEqual[V]` / `InDelta[V]` | ordered / numeric |
| `Regexp[P]` / `JSONEq` | string shapes |
| `Panics` | panic expectation |
| `TimeWithin` / `TimeIsNow` | time comparison |
| `Eventually` / `Consistently` | `(t, waitFor, tick, func(poll *R))` — pass `poll` to inner assertions; outer `t` inside the callback is a lint error (`poll-scope`) |
| `Fail` | unconditional failure (Errorf + FailNow) — never as an if-guard body; state the assertion instead (`fail-guard`) |
| `MatchSnapshot` | `(t, value, name...)` — `value` is a string (a named string type too), `[]byte`, `encoding.TextMarshaler`, `fmt.Stringer`, `json.Marshaler`, `error` or `io.Reader`; a nil value or nil pointer fails as such, and any other type fails with a list of the common ones (string, `[]byte`, `encoding.TextMarshaler`, `fmt.Stringer`, `json.Marshaler`, `error`); golden snapshots in `testdata/__snapshots__/<TestName>.snap` beside the test file (`<TestName>_ext.snap` for an external `_test` package); the trailing strings name several snapshots in one test, they are not a message. Update via `go tool gotest ./... --update-snapshots` — which writes in CI too; CI only refuses to create a missing baseline |
| `Must[T]` | panic-on-error value extraction, for setup code |
| `Each[E]` | table driver: `for t, tc := range gotest.Each(t, entries)` — subtest names come from a `Desc` or `Name` string field; a failing row goes on and so do the rows after it (v1.42+) |

The poll handle `*gotest.R` intentionally satisfies testify's `TestingT`
contract (`Errorf`+`FailNow`) — its surface is `Errorf`, `FailNow`,
`Failed`, `Message`, nothing else; `poll.T()` does not compile.
