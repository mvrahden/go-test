# Migrating to gotest

`go tool gotest migrate ./...` converts testify/**suite**-based code
(directory or import-path patterns, read as `go test` reads them) and
leaves `TODO(gotest-migrate)` markers where judgment is needed: an
unconverted embedded-suite call, an `s.Run`, an assertion or `s.T()` in a
helper without `t`, a runner that does more than `suite.Run`, a suite
whose new name is taken. A suite embedding another suite is refused, with
every file it touches. From v1.33 it converts a suite wherever its
methods, runner and references are declared in the package, and exits 0
only when the result compiles; otherwise it exits 1 with a marker on each
line that needs work, on every run until none is left. `--dry-run` prints
the diff without writing (v1.30.2+). Grep for the markers before calling
the migration done, then run `go vet ./...` and the lint pair
(`lint -fix`, then `lint`). Below v1.33 a clean exit can still leave code
that does not compile or hooks that never run (methods in another file
than the suite, a removed import still in use), and a re-run exits 0 with
markers left. `migrate` maps assertions one-to-one, so `NotNil` on a
pointer or error comes out as `gotest.NotNil` for lint to rewrite, and
`s.Assert().X` becomes the fatal `gotest.X`: a soft check turns into a
hard stop. It does NOT convert bare
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
