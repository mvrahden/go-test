# Migrating to gotest

`go tool gotest migrate ./...` converts testify/**suite**-based code and
leaves `TODO(gotest-migrate)` markers where judgment is needed (embedded
suite calls get "unconverted assertion" markers). It does NOT convert bare
`require`/`assert` usage in plain `TestXxx(*testing.T)` functions — those
are manual:

1. External test package (`package foo_test`), one `FooTestSuite` struct
   per subject; each `TestXxx(t *testing.T)` becomes a
   `func (s *FooTestSuite) TestXxx(t *gotest.T)` method.
2. Map assertions: `require.NoError` → `gotest.NoError`;
   `require.Equal(t, want, got)` → `gotest.Equal(t, want, got)` (same
   expected-first order); collapse `require.Error` + `Contains(err.Error(),
   x)` into `gotest.ErrorContains(t, err, x)`; `require.Len` →
   `gotest.Len`; `require.True(t, x != nil)` → `gotest.NotNil(t, x)`.
3. Do NOT add a `SuiteConfig()` unless the suite needs non-default
   behavior (see `config.md`) — and do not copy one from a neighboring
   suite out of imitation.
4. `go mod tidy` to drop testify once no file imports it.
5. Verify with BOTH runners (`go tool gotest ./...` AND `go test ./...`) —
   during a partial migration the remaining stdlib tests only run under
   `go test`, and the suite side only under gotest.
