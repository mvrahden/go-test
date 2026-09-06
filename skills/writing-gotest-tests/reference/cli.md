# CLI surface

Everything runs as `go tool gotest …` (see SKILL.md Bootstrap). Bare
invocation runs tests — there is NO `test` subcommand:

- `go tool gotest ./...` — run suites (never stdlib tests; those need
  `go test`). Passthrough flags work: `-v`, `-race`, `-count`, `-run`,
  `-json` (streams `go test -json` events incl. every subtest name).
- `go tool gotest spec ./...` — run + render the behavioral spec view.
  `--format terminal|md|json` (terminal is the default), `--output <file>`; `--input <file|->` re-renders a
  captured `go test -json` stream WITHOUT running (v1.29+: it still reads
  the declared labels and `When` vocabulary from the source it can reach
  from the working directory, so a replay renders exactly like the run;
  packages it cannot load render from the names alone). `--input` exits
  non-zero when the stream contains failures (same rule as
  `summary --input`), so replaying a saved stream in CI needs no pipefail
  gymnastics — the render step itself is the verdict; `--render-only`
  lifts that rule for pure renderers (exit 0 on a failing stream, 2 on an
  unreadable one). `--static` renders the spec from source without
  running anything — nodes carry no verdict, and incompletely enumerable
  methods are reported on stderr.
- `go tool gotest watch ./...` — rerun on change; supports `--spec`.
- `go tool gotest lint ./...` — the linter; `-fix` applies suggested
  fixes (textual only — follow with goimports, see SKILL.md; fixes can
  compose, so re-run until clean). Integrity rules suppress per line only
  (`//nolint:<rule>`); others also via `.gotest.yml` `lint.skip` /
  `-skip-<rule>`.
- `go tool gotest discover ./...` — static suite metadata as JSON: suites,
  methods, direct suite→fixture edges, and (v1.27+) each method's declared
  `When`/`It` tree — `name` is the subtest segment `go test` will print,
  `display` the label as the spec renders it (v1.29+: spoken in its
  vocabulary, so `When("email is valid")` reads `when email is valid`),
  `kind` the call it came from. Literal `Each` tables appear as rows;
  anything runtime-valued (a `When` behind a condition, a non-literal
  description or table) is missing and `behaviorsComplete: false` says
  so — never present an incomplete list as the whole specification.
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
instead of default output. `--timeout <dur>` is the global pipeline
deadline (default 15m) and `--setup-timeout <dur>` the shared-fixture
setup budget (default 2m) — `0` disables either; `--min <pct>` gates
coverage, `--no-cache` forces fresh generation, `--debug` keeps overlays.

## Benchmarks — `gotest bench` (v1.27+)

`go tool gotest bench ./...` runs `Benchmark*` suite methods (signature
`func (s *X) BenchmarkParse(b *gotest.B)`, or stdlib `*testing.B`) through
the generated wrappers — always serially, ignoring `--parallel`, because
concurrent benchmarks time contention instead of code. `-test.benchmem`
is on by default. Flags:

- `--spec` — render the spec view; under GitHub Actions the markdown
  (with delta table) lands in the step summary automatically.
- `--save=<path>` — write this run as a JSON baseline. Bare `--save=`
  (empty value) falls back to `bench.baseline` from `.gotest.yml` and
  errors when neither names a path.
- `--against=<path>` — compare against a saved baseline and render the
  delta table (significant rows only unless `-v`; defaults to
  `bench.baseline`). Significance is Welch-tested, so run with `-count`
  high enough to give it samples.
- `--gate=<pct>` — exit 1 when the worst significant regression exceeds
  the threshold (needs `--against` or `bench.baseline`).
- `--json` — emit ONE versioned report document to stdout instead of
  human output: `schemaVersion` 1, the run's results in baseline shape,
  `deltas` when a comparison ran, and `gate` with `breachedKeys` (every
  significant delta above the threshold) when a gate was active. Consume
  this, never scrape text.
- Scoping: `-bench` matches the generated `Benchmark<Suite>` wrapper by
  its first slash segment; later segments select methods —
  `-bench='^BenchmarkFooTestSuite$/^BenchmarkParse$'` runs one method.
  `-benchtime=100x|2s` and `-count=<n>` pass through.

Baseline workflow: `bench --save=` on the trunk build; `bench
--against= --gate=10` (or `bench.gate` in `.gotest.yml`) on branches;
promote a new baseline by re-running `--save=` after accepting a change.
In CI, prefer the action's `bench`/`bench-baseline`/`bench-gate`/
`bench-save` inputs (see `ci.md`).

Machine-readable capture, verified end-to-end:

```sh
go tool gotest -json ./... > events.json
grep -o '"Package":"[^"]*","Test":"[^"]*"' events.json | sort -u > cases.txt
grep -q 'TestProbeTargetTestSuite/TestAddsNumbers' cases.txt
grep -q 'two_plus_two' cases.txt
go tool gotest spec --input events.json > /dev/null
```
