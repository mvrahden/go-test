# CI integration

**Integrate into the existing flow — never add an isolated workflow.**
Before writing anything, inspect `.github/workflows/` and the repo's build
entrypoints (Makefile, Taskfile, scripts): if a test/quality workflow
already exists, add the gotest steps THERE, in that flow's idiom (if the
workflow drives everything through `make check`, extend the make target,
not the workflow). Create a new workflow file only when the repo has none.

A complete CI run for a project with BOTH kinds of tests needs THREE steps
— the Two Runners plus lint. A workflow running only `go test ./...`
silently drops every suite test from CI, and nobody notices because the
run stays green:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - name: Stdlib tests
        run: go test ./... -race
      - name: Suites
        uses: mvrahden/go-test@v1
        with:
          packages: "./..."
          race: "true"
      - name: Lint
        run: go tool gotest lint ./...
```

- The action's `version` input defaults to `gomod`: the CLI version is
  resolved from the consumer's go.mod — the same skew-free property as the
  tool directive. Other inputs: `packages`, `race`, `coverage`,
  `min-coverage`, `flags` (`--double-dash` style), `go-test-flags`
  (`-single-dash` style), `badge` (+ `badge-branch`, default `ci/badges`).
  The action adds a failure-focused summary, GitHub annotations, and
  coverage reporting on top of the plain CLI run.
- **Coverage badge:** `badge: true` makes gotest render `coverage.svg` and
  the action commit it to the `badge-branch` of the consumer's own repo —
  from the default branch only, never from a PR — so the job needs
  `permissions: contents: write`. Embed it as
  `https://raw.githubusercontent.com/<owner>/<repo>/ci/badges/coverage.svg`.
  The badge needs no shields.io, gist or token setup — do not add any.
- **v1.29+ bench inputs:** `bench: true` runs `gotest bench --spec --json`
  after the tests. The `flags` input is forwarded to it, which is where a
  `-benchtime=1x` smoke run goes — and `gotest bench` rejects
  `--parallel`, so a width in `flags` breaks this step (exit 2).
  `bench-baseline` compares (`--against`),
  `bench-gate` fails on regressions above the percentage, and `bench-save`
  writes a baseline: a path, or an explicit empty string for
  `bench.baseline` from `.gotest.yml`; the `false` default saves nothing.
  Outputs: `bench-report` (path to the `--json` report file) and
  `bench-breached-keys` (comma-joined gate offenders). README.md's input
  and output tables are canonical and drift-guarded.
- **v1.29+ fuzz inputs:** `fuzz: true` runs `gotest fuzz` over the same
  packages after the tests, for `fuzz-for` (default: the CLI's own
  minute). The session's table lands in the step summary. A new crasher
  fails the step; its corpus file stays in the checkout and is listed in
  the `fuzz-crashers` output, for an upload-artifact step or
  `gotest fuzz promote`. Seed replay needs no input, since every ordinary
  run already replays seeds. The step carries Go's fuzz cache between
  runs by default, so sessions compound; `fuzz-cache: false` opts out.
  setup-go's build cache alone would not carry it: that cache is never
  re-saved on a key hit.
- CI environments auto-arm `--ci` (any non-falsy `CI`/`GOTEST_CI` value):
  committed `F_` focus prefixes FAIL the run, and a missing snapshot
  baseline fails instead of being created. `--update-snapshots` still
  rewrites baselines there, so never pass it in CI. Opt out with
  `GOTEST_CI=0`.
- **Coexisting with golangci-lint:** there is NO golangci-lint plugin for
  gotest's rules — do not attempt to build or wire one. Run
  `go tool gotest lint` as its own step next to the project's existing
  linter; configure gotest rule skips in `.gotest.yml` (`lint:` /
  `skip: [rule-ids]`), never in `.golangci.yml`.
- Inside GitHub Actions the lint step auto-arms `--github`
  (`GITHUB_ACTIONS=true`): findings surface as inline PR annotations
  (`::error` lines) and a step-summary table INSTEAD of the plain output.
  `-json` runs ignore it (and `-fix` runs below v1.33). No workflow changes needed; exit
  codes are unchanged.
- **v1.27+:** `Exclusive` suites run as a serial tail after the parallel
  bulk, so CI wall-clock time grows by the SUM of exclusive suite
  durations — keep the exclusive set small and its suites short. Under
  `-race`/`-msan`/`-asan` the dispatch and compile concurrency defaults
  are additionally auto-halved (instrumented code costs ≥2× CPU per
  instruction stream); an explicit width always wins — set
  `parallel:` in `.gotest.yml` when a CI runner's shape is known and the
  halved default wastes it (the action's `flags` input also reaches
  `gotest bench`, which rejects `--parallel`).

## gotest-exclusive projects: drop the stdlib step

If `go tool gotest ./...` prints NO "note: … stdlib test(s) … not run"
lines, the project runs exclusively on gotest suites — the `go test ./...`
step tests nothing and should be omitted: CI is the action + lint steps
only. On v1.26.0+ it is safe because both remaining steps prove the
tree: a package that fails to compile fails the run AND fails lint (which
refuses to lint what it cannot analyze), and the `stdlib-test` rule fails
the build if someone later adds a stdlib `TestXxx`. It does not flag a
stdlib `Benchmark`, `Fuzz` or `Example` function: one of those added
later shows only in that runner note, so the change that adds one must
bring back the `go test ./...` step. From v1.29.1 the census adds a third proof: a green `summary`,
`spec` or `-json` run exits 2 when a declared method produced no verdict,
so a harness that drops a test cannot pass either. gotest's own repository
runs this way. On **v1.25.x it is NOT safe**: there a compile-broken package
can pass both the run (exit 0, streaming) and lint (analysis silently
skipped) — keep the `go test ./...` step on v1.25.x, it is the only
compile gate. Two corollaries:

- Do not ADD stdlib tests to a gotest-exclusive project to satisfy the
  three-step shape — the shape follows the tests, not the other way round.
- If stdlib tests later become intentional (marked
  `//nolint:stdlib-test`), reintroduce the `go test ./...` step in the
  same change.
