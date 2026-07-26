# test-census

A syntax-level census of how the most popular Go repositories write their
tests. Powers the blog post ["How Go Actually Tests: Data From 1,000
Repositories"](https://mvrahden.github.io/go-test/blog/how-go-actually-tests/).

The scanner parses every non-generated `_test.go` file with `go/parser` — no
compilation, no execution — and emits one CSV row per repository.

## Contents

| Path | What it is |
|---|---|
| `main.go` | the scanner (stdlib-only, single file) |
| `charts.py` | generates the blog post's SVG charts from the census CSV |
| `corpus/top1000.csv` | top-1,000 GitHub Go repos by stars (fetched 2026-07-24, `archived:false fork:false`) |
| `corpus/top1000-curated.csv` | same, with hard-fork lineage groups resolved (`lineage`, `keep` columns) |
| `corpus/census-full.csv` | full scan results for the 993 kept repos, star-joined, sorted by stars |
| `sample/` | the 100-repo pilot that preceded the full run |

## Reproducing the run

```sh
go build -o test-census . && export PATH=$PWD:$PATH
# pinned reproduction of the published dataset (fetches each repo at its recorded SHA):
./run.sh corpus/census-full.csv results.csv 50
# fresh run against current HEADs (e.g. the annual rerun):
./run.sh corpus/top1000-curated.csv results.csv 50
```

`run.sh` processes repos in windows (default 50): clone the window, scan it
with `-append`, delete it. Peak disk usage is one window (a few GB), not the
~48 GB of the whole corpus. Corpus refresh itself needs an authenticated
`gh` (see `corpus/top1000.csv` for the frozen 2026-07-24 result).

Each output row records the repo's HEAD SHA, so results are attributable to
exact commits. `corpus/census-full.csv` columns are documented below.

## Metric definitions (and their honesty labels)

**AST-exact** — `test_funcs` (`func TestXxx(*testing.T)`), `fuzz_funcs`,
`bench_funcs`, `ginkgo_specs` (`It`/`Specify`/`Entry` in ginkgo-importing
files), `testmain_pkgs`, `subtest_calls` / `subtest_max_depth`,
`parallel_tests` (Parallel at the test's top level) vs `parallel_subtests`
(inside `t.Run` closures), `tests_with_subtests` (test funcs containing at
least one `t.Run` call site), `tests_table_and_subtest` (both table-driven
and containing a `t.Run`), `sleep_calls`, `cleanup_calls`, `helper_calls`,
`skip_calls`, `short_guards`, framework imports.

**Heuristic (documented, errs toward undercounting)** —
`table_tests` (range over a slice/map composite literal, body drives a
t-var), `sleep_known_ms` (compile-time-constant durations only, resolved
against consts pooled per package), golden signals (`golden_files` = `*.golden` count,
`testdata_refs`, `update_flag`), `container_pkgs` / `testmain_tc_pkgs`
(BFS over repo-local imports from packages importing
testcontainers/dockertest), `container_images` / `compose_images`
(`ContainerRequest{Image:}`, dockertest `Run`, compose `image:` lines outside
fixture dirs; filtered to plausible image refs).

**CI (GitHub Actions only, line-attributed)** — `ci_workflows` (file count),
`ci_test_direct` (a workflow line invokes go test/gotestsum directly),
`ci_test_indirect` (tests behind make/task/just/mage/bazel/script targets —
flags invisible), `ci_race` / `ci_cover` / `ci_count1` / `ci_shuffle` (flag
on a direct test line, its continuation, or GOFLAGS; cover also counts
codecov/coveralls uploads), `ci_retry` (gotestsum --rerun-fails or a retry
action). Other CI systems are not read.

**Exclusions** — `vendor/`, `third_party/`, code under `testdata/`, files
with Go's `Code generated … DO NOT EDIT` header, hidden and `_`-prefixed
dirs.

## Known limitations

- Stars select for infrastructure and libraries; repo-level percentages
  weight kubernetes and a 5k-star library equally.
- Sleep totals are floors: computed durations and helper indirection are
  not counted.
- The lineage rule (active member wins, stars tie-break) uses last-push as
  the activity signal; sustained-commit-rate would be stricter (the
  v2ray/v2fly pair is the known judgment call).
- Multi-module repos resolve local imports against the root `go.mod` only.
