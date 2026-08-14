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
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - name: Stdlib tests
        run: go test ./... -race
      - name: Suites
        uses: mvrahden/go-test@main # pin a release tag in real projects
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
  (`-single-dash` style). The action adds a failure-focused summary,
  GitHub annotations, and coverage reporting on top of the plain CLI run.
- CI environments auto-arm `--ci` (any non-falsy `CI`/`GOTEST_CI` value):
  committed `F_` focus prefixes FAIL the run, and snapshots become
  read-only (`--update-snapshots` will not write). Opt out with
  `GOTEST_CI=0`.
- **Coexisting with golangci-lint:** there is NO golangci-lint plugin for
  gotest's rules — do not attempt to build or wire one. Run
  `go tool gotest lint` as its own step next to the project's existing
  linter; configure gotest rule skips in `.gotest.yml` (`lint:` /
  `skip: [rule-ids]`), never in `.golangci.yml`.
- Inside GitHub Actions the lint step auto-arms `--github`
  (`GITHUB_ACTIONS=true`): findings surface as inline PR annotations and a
  step-summary table on top of the plain output. No workflow changes
  needed; exit codes are unchanged.
- Gate formatting too (`gofmt -l`, goimports): `lint -fix` edits are
  textual and unformatted — see SKILL.md's write loop.
- **v1.27+:** `Exclusive` suites run as a serial tail after the parallel
  bulk, so CI wall-clock time grows by the SUM of exclusive suite
  durations — keep the exclusive set small and its suites short. Under
  `-race`/`-msan`/`-asan` the dispatch and compile concurrency defaults
  are additionally auto-halved (instrumented code costs ≥2× CPU per
  instruction stream); an explicit width always wins — pass it through
  the action's `flags` input (e.g. `flags: "--parallel 8"`) when a CI
  runner's shape is known and the halved default wastes it.

## gotest-exclusive projects: drop the stdlib step

If `go tool gotest ./...` prints NO "note: … stdlib test(s) … not run"
lines, the project runs exclusively on gotest suites — the `go test ./...`
step tests nothing and should be omitted: CI is the action + lint steps
only. This is safe on v1.26.0+ because both remaining steps prove the
tree: a package that fails to compile fails the run AND fails lint (which
refuses to lint what it cannot analyze), and the `stdlib-test` rule fails
the build if someone later adds a stdlib test, so nothing can go silently
unexecuted. On **v1.25.x it is NOT safe**: there a compile-broken package
can pass both the run (exit 0, streaming) and lint (analysis silently
skipped) — keep the `go test ./...` step on v1.25.x, it is the only
compile gate. Two corollaries:

- Do not ADD stdlib tests to a gotest-exclusive project to satisfy the
  three-step shape — the shape follows the tests, not the other way round.
- If stdlib tests later become intentional (marked
  `//nolint:stdlib-test`), reintroduce the `go test ./...` step in the
  same change.
