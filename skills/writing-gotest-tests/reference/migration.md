# Migrating to gotest

`go tool gotest migrate ./...` converts testify/**suite** code (directory
or import-path patterns) and, from v1.33, type-checks the result in every
build its `//go:build` tags need. Every line that does not compile, and
every construct it cannot convert, gets a `TODO(gotest-migrate)` marker.
Exit 0 means every migrated package compiles and every hook and test
testify ran is one gotest calls; exit 1 means markers are left (on every
run until none is); exit 2 means a pattern matched nothing, a package
did not compile before (it is left untouched) or the files could not all
be written (nothing is). Generated test files are skipped (named on
stderr), and only the host OS's builds are checked: run it on each
platform whose test files differ. The suite's own `s.Equal`,
`s.Run` (as `t.It`), direct hook calls and most assertions are converted;
promoted mixin hooks and method values are marked. Before v1.37 a suite
built on a base suite was marked; from v1.37 one built on a base suite of
the same test files migrates with it, the base becoming a helper the
child calls, and a base in another package exits 2
([base suites](https://github.com/mvrahden/go-test/blob/main/specs/migrate/spec.md#requirement-base-suites)).
The module must require go-test first
(`go get -tool`, see `setup.md`). `--dry-run` prints the diff without
writing (v1.30.2+). Exit 0 does not run the tests: run both runners
afterwards, and the lint pair (`lint -fix`, then `lint`). Below v1.33 a
clean exit can still leave code that does not compile or hooks that never
run, and a re-run exits 0 with markers left — grep for the markers and
`go vet ./...` there. `migrate` maps assertions one-to-one by type, so
every soft form — `s.Assert().X`, the suite's own `s.X` inherited from the
embedded `*assert.Assertions`, and `assert.X(s.T(), …)` — becomes the fatal
`gotest.X`, exactly as `s.Require().X` and `require.X(s.T(), …)` do: a soft
check turns into a hard stop. From v1.37, a translation that would change
a verdict, or what runs when, is left under a marker with its reason and
the gotest way instead
([kept verdicts](https://github.com/mvrahden/go-test/blob/main/specs/migrate/spec.md#requirement-kept-verdicts)):
read each marker, it names what to write. v1.37+ never writes
`go.mod`/`go.sum`
([module files](https://github.com/mvrahden/go-test/blob/main/specs/migrate/spec.md#requirement-module-files)).
It does NOT convert bare
`require`/`assert` usage in plain `TestXxx(*testing.T)` functions — those
are manual:

1. External test package (`package foo_test`), one `FooTestSuite` struct
   per subject; each `TestXxx(t *testing.T)` becomes a
   `func (s *FooTestSuite) TestXxx(t *gotest.T)` method. Where a test's
   `t.Run` cases become `When`/`It` blocks, word them per SKILL.md rule 8:
   a bare condition for `When`, a bare behavior for `It` — the spec
   supplies "when" and the glyph plays "it".
2. Map assertions: `require.NoError` → `gotest.NoError`;
   `require.Equal(t, want, got)` → `gotest.Equal(t, want, got)` (same
   expected-first order); collapse `require.Error` + `Contains(err.Error(),
   x)` into `gotest.ErrorContains(t, err, x)`; `require.Len` →
   `gotest.Len`; `require.True(t, x != nil)` / `require.NotNil(t, x)` by
   the type of `x`: `gotest.Error` for an error, `gotest.NotZero` for a
   pointer, interface or channel, `gotest.NotNil` for a slice, map or
   func.
3. Do NOT add a `SuiteConfig()` unless the suite needs non-default
   behavior (see `config.md`) — and do not copy one from a neighboring
   suite out of imitation.
4. `go mod tidy` to drop testify once no file imports it.
5. Verify with BOTH runners (`go tool gotest ./...` AND `go test ./...`) —
   during a partial migration the remaining stdlib tests only run under
   `go test`, and the suite side only under gotest.
