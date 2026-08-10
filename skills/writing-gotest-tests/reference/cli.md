# CLI surface

Everything runs as `go tool gotest …` (see SKILL.md Bootstrap). Bare
invocation runs tests — there is NO `test` subcommand:

- `go tool gotest ./...` — run suites (never stdlib tests; those need
  `go test`). Passthrough flags work: `-v`, `-race`, `-count`, `-run`,
  `-json` (streams `go test -json` events incl. every subtest name).
- `go tool gotest spec ./...` — run + render the behavioral spec view.
  `--format terminal|md|json` (terminal is the default), `--output <file>`; `--input <file|->` re-renders a
  captured `go test -json` stream WITHOUT running. `--input` exits
  non-zero when the stream contains failures (same rule as
  `summary --input`), so replaying a saved stream in CI needs no pipefail
  gymnastics — the render step itself is the verdict.
- `go tool gotest watch ./...` — rerun on change; supports `--spec`.
- `go tool gotest lint ./...` — the linter; `-fix` applies suggested
  fixes (textual only — follow with goimports, see SKILL.md; fixes can
  compose, so re-run until clean). Integrity rules suppress per line only
  (`//nolint:<rule>`); others also via `.gotest.yml` `lint.skip` /
  `-skip-<rule>`.
- `go tool gotest discover ./...` — static suite metadata as JSON (methods
  and direct suite→fixture edges; it cannot see `Each` rows — they are
  runtime values).
- `go tool gotest scaffold ./pkg/path.TypeName` — generate a suite; an
  interface target generates a generic contract suite. Own-module targets
  only (it writes into the target package's directory).
- `go tool gotest migrate ./...` — testify/suite conversion (see
  `migration.md` for its scope).
- `go tool gotest refactor toggle-focus <file> <Suite[.Method]>` — toggles
  the `F_` focus prefix only (`X_` is a manual edit).
- Also: `prepare`, `generate`, `clean`, `summary`, `version`, `help <cmd>`.

Flags shared by run modes: `--ci` (or env `GOTEST_CI`/`CI`; values `0` and
`false` are falsy) — enables the focus guard (committed `F_` prefixes fail
the run) AND makes snapshots read-only, so `MatchSnapshot` cannot write
baselines in CI-detected environments. `--update-snapshots` rewrites
`MatchSnapshot` baselines (outside CI). `--spec` renders the spec view
instead of default output.

Machine-readable capture, verified end-to-end:

```sh
go tool gotest -json ./... > events.json
grep -o '"Package":"[^"]*","Test":"[^"]*"' events.json | sort -u > cases.txt
grep -q 'TestProbeTargetTestSuite/TestAddsNumbers' cases.txt
grep -q 'two_plus_two' cases.txt
go tool gotest spec --input events.json > /dev/null
```
