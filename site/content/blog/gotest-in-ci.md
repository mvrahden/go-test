---
title: "Go Tests in GitHub Actions: The gotest CI Setup Guide"
date: 2026-07-22
description: "Run Go tests in GitHub Actions with the gotest action: failure-focused summaries, inline PR annotations, coverage thresholds, and the focus-prefix guard."
tags: ["CI"]
keywords: ["go test github actions", "go ci coverage threshold", "go test pr annotations", "gotest github action"]
cta_text: "Set up gotest in your CI pipeline today."
howto:
  name: "Set up gotest in GitHub Actions"
  steps:
    - name: "Add the action to your workflow"
      text: "Add the mvrahden/go-test@v1 step after checkout and setup-go — it handles installation, test execution, coverage reporting, and annotations."
    - name: "Configure inputs"
      text: "Set packages, race, coverage, and version inputs to control what the action runs and which gotest version it uses."
    - name: "Enable coverage thresholds"
      text: "Set min-coverage on the action, --min on the CLI, or min-coverage in .gotest.yml to fail the build when coverage drops below the threshold."
    - name: "Add the focus-prefix guard"
      text: "Let CI mode (auto-enabled via the CI environment variable, or explicitly with --ci) fail the build when a committed F_ prefix would silently skip tests."
---

Running gotest locally is straightforward. Making it work well in CI — with clear failure output, PR annotations, coverage enforcement, and safety guards — takes a bit of setup. This post covers the official GitHub Action, the `summary` command, coverage thresholds, and the CI mode that catches debugging artifacts before they reach your main branch.

If you haven't written a gotest suite yet, start with [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}) and come back — everything here builds on that workflow.

## The problem with `go test -v` in CI

The default `go test -v` output is verbose. For every test, it prints three lines: `=== RUN`, `=== CONT` (for parallel tests), and `--- PASS` or `--- FAIL`. In a 200-test suite, that is 600+ lines of output. When two tests fail, the failure messages are buried in the middle of 590 lines of passing noise.

Most teams work around this by piping output through `grep` or `gotestfmt`. gotest has a built-in answer: the `summary` command.

## `gotest summary`: failure-focused output

The `summary` subcommand filters test output to show only what matters. When all tests pass, it prints a single line:

{{< terminal title="all tests pass" >}}
147 tests passed (2.3s)
Coverage: 82.4%
{{< /terminal >}}

When tests fail, it shows only the failing tests with their assertion output — no `=== RUN`, no `=== CONT`, no `--- PASS` noise:

{{< terminal title="failures show assertion output only" >}}
3 of 147 tests failed

FAIL  pkg/foo TestValidateInput / empty string (12ms)
      foo_test.go:42: expected error, got nil

FAIL  pkg/bar TestProcessOrder / concurrent writes (1.2s)
      bar_test.go:88:
        expected: []string{"a", "b", "c"}
             got: []string{"a", "c", "b"}
{{< /terminal >}}

A developer reading this output sees exactly which tests failed, where, and why. No scrolling required.

## The GitHub Action

gotest includes a composite GitHub Action at `mvrahden/go-test@v1`. It wraps `gotest summary --github` and handles installation, test execution, coverage reporting, and annotations in one step.

Here is a minimal workflow:

```yaml {title=".github/workflows/test.yml"}
name: test

on:
  pull_request:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: mvrahden/go-test@v1
        with:
          packages: ./...
          race: true
          coverage: true
          min-coverage: 80
```

That is the entire CI setup. The action does the following:

1. **Installs gotest.** By default (`version: gomod`), it runs gotest through `go run`, resolving the version from your project's `go.mod`. This keeps the CI version in sync with local development — no version drift.
1. **Runs tests** with `gotest summary --github`. The `--github` flag is auto-detected in GitHub Actions via the `$GITHUB_ACTIONS` environment variable.
1. **Reports failures** as GitHub `::error` annotations. These appear inline on the PR diff, pointing at the exact file and line of each failing assertion.
1. **Writes a step summary** to the job summary panel with pass/fail counts and coverage.
1. **Enforces coverage.** If `min-coverage` is set and the coverage percentage falls below the threshold, the step fails.

### Action inputs

The action accepts these inputs:

| Name | Default | Description |
|---|---|---|
| `packages` | `./...` | Package patterns to test. |
| `race` | `false` | Enable the race detector. |
| `coverage` | `false` | Enable coverage profiling and reporting. |
| `min-coverage` | | Minimum coverage percentage (0–100). Fails the step if below. |
| `flags` | | Additional gotest flags (`--double-dash` style). |
| `go-test-flags` | | Additional `go test` flags (`-single-dash` style). |
| `version` | `gomod` | gotest version: `gomod` resolves from `go.mod`, or a version tag (e.g. `v1.0.0`, `latest`) to install globally. |

### Action outputs

The action exposes two outputs for downstream steps:

| Name | Description |
|---|---|
| `exit-code` | The test process exit code. |
| `coverage` | The coverage percentage (empty if coverage is not enabled). |

You can use these in subsequent steps, for example to post a coverage comment or gate a deployment:

```yaml {title=".github/workflows/test.yml (continued)"}
      - uses: mvrahden/go-test@v1
        id: test
        with:
          packages: ./...
          coverage: true

      - name: Coverage gate
        if: steps.test.outputs.coverage != ''
        run: echo "Coverage: ${{ steps.test.outputs.coverage }}%"
```

## Version resolution

The default `version: gomod` strategy runs gotest via `go run github.com/mvrahden/go-test/cmd/gotest`. This resolves the version pinned in your project's `go.mod`, which means CI runs the exact same version you use locally. No separate install step, no version drift.

For this to work, gotest needs to be in your `go.mod`. The cleanest way is a `tool` directive (Go 1.24+):

```go-mod {title="go.mod"}
module your-project

go 1.24

tool github.com/mvrahden/go-test/cmd/gotest
```

If your project does not depend on gotest as a library, set `version: latest` or a specific tag (e.g. `v1.2.0`) to install a standalone binary via `go install` instead.

## CI mode and the focus-prefix guard

gotest has a CI mode that activates two safety behaviors:

1. **Focus-prefix guard.** If any suite or method has an `F_` prefix (`F_TestSomething`, `F_MyTestSuite`), the build fails immediately. The `F_` prefix means "run only this" — useful during development, dangerous in CI. Without the guard, a committed `F_` prefix silently skips every other test in the package.
1. **Snapshot read-only mode.** Snapshot files cannot be updated in CI. If a snapshot does not match, the test fails instead of silently updating the expected value.

How focus prefixes fit into day-to-day development is covered in depth in [the inner loop post]({{< ref "/blog/the-inner-loop" >}}).

CI mode activates automatically when the `CI` environment variable is set, which GitHub Actions, GitLab CI, CircleCI, and most other CI systems do by default. You can also enable it explicitly with `--ci`:

```sh
gotest --ci ./...
```

To opt out when `CI` is set (rare, but occasionally useful for CI debugging), set `GOTEST_CI=0`.

> The focus-prefix guard is important enough to deserve emphasis: without it, a developer who forgets to remove `F_TestLogin` before pushing effectively disables every other test in that package. The suite passes because the one focused test passes. The 49 skipped tests are invisible in the output. CI mode turns this silent skip into a loud failure.

## Coverage thresholds

gotest calculates statement-weighted coverage from Go's built-in coverage profile. You can set a minimum threshold that fails the build if coverage drops below it:

```yaml {title="via the GitHub Action"}
- uses: mvrahden/go-test@v1
  with:
    coverage: true
    min-coverage: 80
```

```sh {title="via the CLI"}
gotest --min=80 ./...
```

```yaml {title="via .gotest.yml (checked into the repo)"}
min-coverage: 80
```

All three methods have the same effect. Using `.gotest.yml` has the advantage that the threshold is checked in both local development and CI without passing flags.

## Multi-platform testing

Use a matrix strategy to test across Go versions and operating systems:

```yaml {title=".github/workflows/test.yml"}
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go-version: ["1.24", "1.25", "1.26"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}

      - uses: mvrahden/go-test@v1
        with:
          packages: ./...
          race: true
```

The action works on all three platforms. Each matrix entry gets its own failure annotations and step summary.

## Non-GitHub CI systems

For GitLab CI, CircleCI, or any other system, run `gotest summary` directly. CI mode is auto-detected from the `CI` environment variable (which most CI systems set). The `--github` flag for annotations is auto-detected from `$GITHUB_ACTIONS`, so it stays inactive on other platforms.

```yaml {title=".gitlab-ci.yml"}
test:
  image: golang:1.24
  script:
    - go install github.com/mvrahden/go-test/cmd/gotest@latest
    - gotest summary ./... -race -coverprofile=coverage.out
    - go tool cover -html=coverage.out -o coverage.html
  artifacts:
    paths:
      - coverage.html
```

You can also pipe existing `go test -json` output into `gotest summary` without re-running tests:

```sh
go test -json ./... | gotest summary --input=-
```

This is useful if you have existing CI steps that run `go test` and you want to add summary output without changing the test execution.

## Combining stdlib and suite tests

Many projects have a mix of standard `func Test*` tests and gotest suites. The recommended CI pattern runs both:

```yaml {title=".github/workflows/test.yml"}
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
    with:
      go-version-file: go.mod

  # Standard tests (go test directly)
  - name: Test (stdlib)
    run: go test -coverprofile=coverage-stdlib.out ./... -race

  # Suite tests (gotest)
  - uses: mvrahden/go-test@v1
    with:
      packages: ./...
      race: true
      coverage: true
      min-coverage: 80
      go-test-flags: "-coverprofile=coverage-suites.out"
```

`go test` runs standard test functions. gotest runs suite tests. Both produce coverage profiles that you can merge if needed. The gotest action's `--ci` mode (auto-detected) catches focus prefixes and locks snapshots regardless of which step they appear in.

## Project configuration with `.gotest.yml`

Instead of passing flags in every CI step, you can commit a `.gotest.yml` to your project root. gotest reads it automatically in both local development and CI:

```yaml {title=".gotest.yml"}
min-coverage: 80
parallel: 12
tags: integration
lint:
  skip:
    - testify
```

CLI flags override `.gotest.yml`, so a developer can always run `gotest --min=0 ./...` locally to skip the coverage check during development. But the default, both locally and in CI, is whatever the project file says.

## A complete workflow

A complete CI setup for a project using gotest looks like this:

1. **Add gotest to `go.mod`** via a `tool` directive or `go get`.
1. **Create `.gotest.yml`** with your coverage threshold and any project-wide settings.
1. **Add the GitHub Action** to your workflow. One step, four inputs.
1. **Push.** CI mode activates automatically. Focus prefixes are caught, snapshots are read-only, failures get annotations, and coverage is enforced.

One developer sets this up. The entire team benefits. The coverage threshold prevents regression, the focus guard prevents accidental test skipping, and the failure summary makes every CI failure actionable without scrolling through logs. From here, [the inner loop post]({{< ref "/blog/the-inner-loop" >}}) covers the local workflow that CI mode guards, and [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}) is the place to send teammates who are new to gotest.
