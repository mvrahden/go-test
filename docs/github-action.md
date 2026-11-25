# The GitHub Action

Use the official action for CI pipelines with failure summaries, inline PR annotations, and coverage reporting:

```yaml
- uses: mvrahden/go-test@v1
  with:
    packages: ./...
    race: true
    coverage: true
    min-coverage: 80
```

By default (`version: gomod`), the action builds the `gotest` your `go.mod` selects once per job and runs that binary — no version drift between CI and local development. Without a declared tool it resolves the CLI's dependencies in a copy of `go.mod`, so the checkout's `go.sum` stays as committed, and it notes the `go get -tool` line that declares it. Set `version: latest` or a specific tag to install a standalone binary instead.

The action emits `::error` annotations that appear inline on PR diffs and writes a markdown summary to the GitHub step summary panel; the summary also notes the stdlib tests beside your suites that gotest did not run.

With `fuzz: true`, a fuzz step runs after the tests (`gotest fuzz` over the same packages, for `fuzz-for` or the CLI's default minute): the session's per-target table lands in the step summary, a new crasher fails the step and its corpus file is left in the checkout, listed in the `fuzz-crashers` output for an `upload-artifact` step or a `gotest fuzz promote` step in the same job (promote takes only the crashers the latest session in that job's gotest cache wrote). Seed replay needs no such step, since every ordinary run already replays seeds. The step also carries Go's fuzz cache between runs: it restores the latest corpus of the branch or its base and saves this run's under its own key, so a short budget compounds into a deeper search over time, main's corpus feeds every pull request, and no pull request pollutes main. `fuzz-cache: false` runs from the seeds alone. (The build cache `actions/setup-go` keeps is not enough for this: it is keyed on go.sum and never re-saved on a hit, so a corpus would only survive a dependency bump.)

With `bench: true`, a benchmark step runs after the tests (`gotest bench --spec --json`): the step summary gets the benchmark count, a per-package results table (ns/op, B/op, allocs/op), the delta table when a baseline was compared, and the gate verdict when one was set; the versioned JSON report lands in a temp file exposed as the `bench-report` output, and a breached gate fails the step with the offending keys in `bench-breached-keys`.

## Reference

### Inputs

The tables below are the canonical action surface — a drift guard test keeps them in sync with `action.yml`.

| Input | Description |
|---|---|
| `packages` | Package patterns to test (default `./...`) |
| `race` | Enable the race detector (default `false`) |
| `coverage` | Enable coverage profiling and reporting (default `false`) |
| `min-coverage` | Minimum coverage percentage (0-100, fails if below) |
| `flags` | Additional gotest flags (`--double-dash` style; also forwarded to the bench step) |
| `go-test-flags` | Additional go test flags (`-single-dash` style) |
| `badge` | Render a coverage badge and, on the default branch, publish it as `coverage.svg` to `badge-branch` (needs `contents: write`; turns coverage on; default `false`) |
| `badge-branch` | Branch the coverage badge is published to; created on first use (default `ci/badges`) |
| `bench` | Run benchmarks after tests via `gotest bench --spec --json` (default `false`) |
| `bench-baseline` | Baseline JSON file to compare benchmarks against (`--against`) |
| `bench-gate` | Fail if any benchmark regresses by more than this percent (`--gate`); needs `-count=4` or more in `flags`, or the step exits 2 |
| `bench-save` | Save the run as a JSON baseline at this path (`--save`); an explicit empty string saves to `bench.baseline` from `.gotest.yml`; default `false` saves nothing |
| `fuzz` | Run a budgeted fuzz session after tests via `gotest fuzz --for`; a new crasher fails the step (default `false`) |
| `fuzz-for` | Approximate wall-clock budget for the whole fuzz session (default: the CLI's own, one minute) |
| `fuzz-cache` | Carry Go's fuzz cache between runs so each session resumes where the last stopped; `false` for a from-seeds run (default `true`) |
| `events-file` | Also save the run's go test `-json` event stream to this path (`--events-file`); prefer one under `runner.temp`; needs gotest v1.36.0 or newer, and the step fails naming it with an older CLI; default empty saves nothing |
| `audit` | `auto` (default) appends `gotest audit`'s comparison with the pull request's base to the step summary on `pull_request` events; `true` also on other events, against `github.event.before`; `false` never ([Audit and attestation](#audit-and-attestation)) |
| `audit-fail-on` | Finding classes the audit fails the step on (`--fail-on`), comma-separated; default empty reports and never fails |
| `attest` | Attest the run's result with the in-toto test-result predicate through `actions/attest`; the job must grant `id-token: write` and `attestations: write` (default `false`) |
| `version` | `gomod` (default) runs the CLI from go.mod; a tag (e.g. `v1.0.0`, `latest`) installs globally |

### Outputs

| Output | Description |
|---|---|
| `exit-code` | The gotest exit code: 0, 1 (tests failed or coverage below the minimum), 2 (the suites did not run to completion), 130 (interrupted) |
| `coverage` | Coverage percentage (empty if coverage not enabled) |
| `badge` | Path of the rendered coverage badge SVG (empty unless badge is enabled and the tests ran) |
| `events-file` | Absolute path of the saved event stream (empty unless `events-file` is set and the file was written) |
| `audit-report` | Path of the audit's Markdown document (empty when the audit did not run) |
| `attestation-url` | URL of the run's attestation (empty unless `attest` is enabled and it was made) |
| `bench-report` | Path to the `gotest bench --json` report file (empty if bench not enabled) |
| `bench-breached-keys` | Comma-joined benchmark keys that breached the gate (empty if none or no gate) |
| `fuzz-crashers` | Comma-joined workspace-relative paths of the corpus files the fuzz session added (empty if none or fuzz not enabled) |

## Audit and attestation

On a `pull_request` event the action runs [`gotest audit`](../specs/audit/spec.md) after the tests and appends its Markdown document to the step summary: the behaviors the pull request adds, removes, rewords, moves and changes against `github.event.pull_request.base.sha`, the weakenings, and, from the run's own event stream and its coverage profile when `coverage` made one, the evidence for each changed behavior and the changed lines no test executed. The action keeps that stream in `runner.temp` when `events-file` is not set. `actions/checkout` fetches one commit, so the action fetches the base commit alone (`git fetch --depth=1 origin <sha>`); where that fails, the summary says the audit was skipped and why. `audit: true` also audits other events against `github.event.before`, the commit a push started from, and says the audit was skipped on an event that names none, such as a push that created its branch. The audit only reports: the step fails only when `audit-fail-on` names classes the document lists, and then also when the audit cannot judge them.

```yaml
- uses: mvrahden/go-test@v1
  with:
    coverage: true
    audit-fail-on: removed,focus,excluded
```

With `attest: true` the action writes the run's result as the in-toto [test-result predicate](https://github.com/in-toto/attestation/blob/main/spec/predicates/test-result.md) (`gotest summary --input=<stream> --format=intoto-test-result`, [A run's result as an in-toto predicate](../specs/cli/spec.md#requirement-a-runs-result-as-an-in-toto-predicate)) and attests it through [`actions/attest`](https://github.com/actions/attest). A test run attests the source it tested, so the subject is the checked-out commit: `actions/attest` takes only SHA-2 digests, not a git commit id, so the subject is named `git+<server>/<repository>@<commit>` and its digest is the sha256 of `git archive --format=tar <commit>`, which anyone holding the commit can recompute. A composite action cannot grant permissions; the calling job needs:

```yaml
permissions:
  contents: read
  id-token: write
  attestations: write
```

Without them, as on a pull request from a fork, the attestation fails softly: the job goes on and the summary says what was missing. Verify one with `gh attestation verify` against the archive, with `--predicate-type https://in-toto.io/attestation/test-result/v0.1`.

## Event Stream

`events-file` keeps the run's go test `-json` event stream, per-test timings of passing tests included, without a second run. Upload it for later analysis, `gotestsum --raw-command -- cat events.json` for JUnit, or `gotest summary --input` to render it again:

```yaml
- uses: mvrahden/go-test@v1
  id: test
  with:
    events-file: ${{ runner.temp }}/test-events.json
- uses: actions/upload-artifact@v7
  if: ${{ always() && steps.test.outputs.events-file != '' }}
  with:
    name: test-events-${{ matrix.os }}
    path: ${{ steps.test.outputs.events-file }}
    retention-days: 14
```

`always()` keeps the stream of a failed or timed-out job, the one most worth reading. GitHub masks secrets in job logs, not in artifacts: the stream holds every line a test printed, so a test that logs a token publishes it to everyone who can read the run's artifacts.

## Coverage Badge

The action can add a coverage badge to your README. `gotest` draws the badge as an SVG file and the workflow commits it to a branch of your own repository using the built-in `GITHUB_TOKEN`. Nothing outside GitHub is involved and there is no account or secret to set up.

```yaml
permissions:
  contents: write

steps:
  - uses: actions/checkout@v7
  - uses: mvrahden/go-test@v1
    with:
      packages: ./...
      min-coverage: 80
      badge: true
```

`badge: true` turns coverage reporting on by itself. The badge lands on the `ci/badges` branch (change it with `badge-branch`); embed it from there:

```markdown
![Coverage](https://raw.githubusercontent.com/<owner>/<repo>/ci/badges/coverage.svg)
```

Three rules keep the badge trustworthy:

- It is published from the default branch only. Pull requests and feature branches never change it.
- It is published only when the suites ran to completion. A build failure publishes nothing, while a failed test run or a missed `min-coverage` still publishes the coverage that run measured.
- It never affects test results. Several jobs publishing at once (a version matrix, for example) simply take turns, and a missing permission or a protected branch produces a warning, not a failed build.

Outside GitHub Actions, `gotest summary --badge=coverage.svg ./...` renders the same file for you to publish however you like.
