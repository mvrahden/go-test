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

## Fuzzing — `gotest fuzz` (v1.27+)

Suite methods named `Fuzz*` (`func (s *X) FuzzParse(f *gotest.F)`) declare
fuzz targets; inside, `gotest.Fuzz/Fuzz2/Fuzz3(f, func(t *gotest.T, in …)
{ … })` binds the callback. Plain `gotest ./...` already replays every
`f.Add` seed and committed corpus entry as regular subtests — reach for
the subcommand only to SPEND TIME SEARCHING for new inputs.
`go test -fuzz` cannot see suite targets (generated wrappers need the
overlay), so the orchestrator runs each `Fuzz<Suite>_<Method>` wrapper as
its own `go test -fuzz` process:

- `go tool gotest fuzz ./...` — fuzz every discovered target. Flags:
  `--for=<dur>` splits an approximate wall-clock budget jobs-aware across
  targets (per-target share floors at 10s; the schedule prints up front);
  `--jobs=<n>` (default max(1, GOMAXPROCS/2)) bounds concurrent targets;
  `--target=<FuzzSuite_Method>` fuzzes exactly one wrapper (unmatched
  names error with the available list); `--no-harvest` disables
  table-test seed harvesting for the run. A `--for` that cannot fit the
  global `--timeout` (default 15m) is rejected up front.
- `go tool gotest fuzz triage ./...` — re-run each on-disk crasher under
  `testdata/fuzz/<Func>/`, print decoded input and cause; exit 1 while
  any still fails.
- `go tool gotest fuzz promote ./...` — splice each crasher into its
  method as a permanent typed `f.Add(...)` seed and delete the file —
  the durable form, especially for struct-typed targets whose corpus
  files are bound to the type's field order (the `fuzz-struct-corpus`
  lint rule flags them).

Any non-native argument (`gotest.Fuzz[T]`, and every position of `Fuzz2`
/`Fuzz3`) fuzzes through a generated fan — one engine argument per leaf
field. Write typed `f.Add` literals, never raw `[]byte` seeds
(`fuzz-raw-seed`; a wrong-typed seed is rejected at `gotest.Fuzz`), keep
targets deterministic (`fuzz-determinism`), give the callback a property
to assert (`fuzz-no-oracle`), and keep per-execution hooks IO-free
(`fuzz-hook-io`).

Machine-readable capture, verified end-to-end:

```sh
go tool gotest -json ./... > events.json
grep -o '"Package":"[^"]*","Test":"[^"]*"' events.json | sort -u > cases.txt
grep -q 'TestProbeTargetTestSuite/TestAddsNumbers' cases.txt
grep -q 'two_plus_two' cases.txt
go tool gotest spec --input events.json > /dev/null
```
