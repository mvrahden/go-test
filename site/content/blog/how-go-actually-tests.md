---
title: "How Go Actually Tests: Data From 1,000 Repositories"
date: 2026-07-24
description: "We parsed 720,118 test functions across the 1,000 most-starred Go repos. What the data says about parallelism, sleeps, golden files, and containers."
tags: ["Research"]
keywords: ["go testing statistics", "state of go testing", "go test parallel adoption", "testify vs ginkgo usage", "go testing survey"]
toc: true
cta_text: "See what structured suites change about these numbers."
faq:
  - q: "How many Go tests run in parallel?"
    a: "Across the 1,000 most-starred Go repositories, 11.2% of test functions call t.Parallel() themselves. 46% of repos use it somewhere — adoption is broad, but shallow."
  - q: "What testing framework do most Go projects use?"
    a: "56% of the top Go repos import testify's assert or require packages. Only 10% use testify/suite and 8% use ginkgo. The stdlib's httptest appears in 60% — more than any third-party framework."
  - q: "How much do Go tests sleep?"
    a: "We counted 27,453 time.Sleep calls in test code across 917 repos with tests — at least 251 hours of fixed waiting per full run of the corpus, counting only compile-time-constant durations."
---

The tests of the 1,000 most-starred Go repositories spend at least **251 hours asleep**. Not blocked on I/O, not waiting for a condition. Sleeping, in `time.Sleep` calls with constant durations, written into test code on purpose. That is ten and a half days of deliberate waiting per full run of the corpus, and it is a floor: we could only count durations that are compile-time constants.

We measured this because we couldn't find anyone who had. There is no shortage of opinions about how Go tests should be written. There is very little data about how they are written. So we parsed them.

## What we measured

We took the 1,000 most-starred Go repositories on GitHub (excluding archived repos and forks, with [hard-fork lineages deduplicated](#methodology-and-limitations)), pinned each to a commit, and parsed every non-generated `_test.go` file with `go/parser`. No compilation, no execution. 76 repos have no test files at all, which leaves **917 repos and 720,118 test functions**, plus 69,619 ginkgo specs counted separately.

The scanner, the corpus with commit SHAs, and the full per-repo dataset are [on GitHub](https://github.com/mvrahden/go-test/tree/research/test-census), so every number below is reproducible.

A note on how to read this post: we build a test framework, and this census exists because we wanted to know whether the problems gotest targets are real at ecosystem scale. The numbers are reported straight. The interpretation is ours, and we say so where it matters.

## Most Go tests never run in parallel

`t.Parallel()` is the stdlib's one-line speedup, and 46% of repos call it somewhere. But look at tests instead of repos and the picture inverts: **11.2% of test functions are parallel**. Even in the top 100 repos by stars, where 67% have adopted it somewhere, only 9.1% of tests actually use it.

![Grouped bar chart: share of repos using t.Parallel() anywhere (46% full corpus, 67% top 100, 41% long tail) versus share of tests that actually run in parallel (11.2%, 9.1%, 9.1%).](../../census/parallel.svg)

The gap between adoption and use has a simple explanation: `t.Parallel()` is only safe for tests that share nothing, and most tests share a database, a global, or a fixture. The flag is trivial. Making tests independent is the actual work, and most teams stop before it. [Why Your Go Tests Are Slow]({{< ref "/blog/go-testing-at-scale" >}}) makes the structural argument; this is its baseline. It is also why gotest runs each suite as its own process and makes per-test state explicit — a returning `BeforeEach` gives every test its own copy, so parallelism stops being a data-race gamble.

## The sleep problem is universal

68% of repos have `time.Sleep` in their test code — 27,453 calls in total. Where the duration was a constant we could resolve, the corpus sleeps 251 hours per full test run, roughly 24 minutes of fixed waiting per affected repo. Sleeps with computed durations, retry loops, and helper indirection are not counted, so the real figure is higher.

A sleep is a guess about how long the async thing takes. Guess short and the test flakes. Guess long and the suite crawls. Polling assertions like gotest's `Eventually` and `Consistently` replace the guess with a deadline and a check interval — that argument, and the migration path, is in [Testing Async Code in Go Without time.Sleep]({{< ref "/blog/testing-async-go" >}}).

## What the ecosystem imports

![Horizontal bar chart of framework adoption as share of 917 repos: httptest 60%, testify assert/require 56%, go-cmp 21%, testify/mock 14%, testify/suite 10%, gomock 9%, gomega 8%, ginkgo 8%, testing/quick 5%, testcontainers-go 3%.](../../census/frameworks.svg)

Three things stand out.

**`httptest` beats every third-party package.** The stdlib's HTTP test server appears in 60% of repos. Where the standard library actually solves a problem, Go developers use it and skip the dependency. That is worth remembering when reading the rest of this post: the gaps we measure below are places where the stdlib offers no equivalent to reach for.

**testify spread as an assertion library, not a suite framework.** 56% of repos import `assert`/`require`; only 10% use `testify/suite`. It would be easy to read that as "Go doesn't want test structure." The rest of the data argues otherwise — 42% of repos wire up `TestMain`, 36.7% of test names encode scenarios in underscores, and helper conventions are everywhere. The demand for structure is visible in every workaround column of our dataset. What 10% prices is the cost of the available implementation: testify/suite brings reflection, a runtime dependency, and sequential method execution. Teams looked at that trade and hand-rolled structure instead. (If you are in the 10%, [the migration guide]({{< ref "/blog/testify-migration-guide" >}}) covers moving a suite codebase to generated, parallel-safe suites.)

**BDD frameworks are a committed minority.** ginkgo appears in 8% of repos, but those repos contain 69,619 specs. BDD in Go is polarized, not fringe. Our position on why — and what behavioral structure looks like without a DSL — is in [BDD in Go]({{< ref "/blog/what-bdd-means-in-go" >}}).

Snapshot libraries register at 2%. That number is interesting only next to the following one.

## Golden files: everyone does it, nobody standardizes it

43% of repos reference `testdata/` from their test code — expected-output files compared against actual output. Only 7% have an update workflow (an `-update` flag that regenerates the files), and only 6% use the `.golden` naming convention. The corpus is full of `testdata/*.json`, `*.txtar`, and `*.txt` compared by hand-rolled helpers.

Golden testing is a 43% practice with a 7% workflow. Every team reinvents the read-compare-update loop, and most stop before the "update" part. [Snapshot Testing in Go]({{< ref "/blog/snapshot-testing-in-go" >}}) covers what a built-in version looks like — `MatchSnapshot` with parallel-safe writes and a read-only mode for CI.

## What Go tests against: Postgres

7% of repos start real containers in their tests, via testcontainers-go, dockertest, or compose files wired into test orchestration. What do they run?

![Horizontal bar chart of container images used in tests, by repo count: postgres 82, redis 46, mysql 45, prometheus 36, grafana 28, nginx 25, minio 25, jaeger 24, mongo 21, clickhouse 20.](../../census/containers.svg)

Postgres appears in 82 repos, nearly twice its closest rival. If you are building test infrastructure for Go, this ranking is a priority list.

The structural finding matters more than the ranking: **26 repos have the multi-package `TestMain` container pattern** — several packages, each with a `TestMain` that starts its own container, directly or through a helper package. coder/coder has 61 such packages; telegraf has 477 test packages that reach container code. `go test` runs each package as a separate OS process, so these packages cannot share a container through Go code. Each one pays startup independently, or the team gives up and orchestrates containers outside the test run. gotest's `SharedFixture` exists for exactly this boundary: one setup process starts the container, and its state is serialized and rehydrated into each package's process. The full mechanics are in [Sharing Test Fixtures Across Go Packages]({{< ref "/blog/shared-fixtures" >}}).

## The workaround gradient

We measured four markers across the corpus and compared the top 100 repos by stars with the bottom 400:

![Grouped bar chart comparing top 100 repos vs the long tail: TestMain 61% vs 38%, native fuzz tests 31% vs 15%, t.Cleanup 74% vs 45%, t.Helper 81% vs 56%.](../../census/maturity.svg)

Our first reading of this chart was "popular repos invest more in testing." We think that reading is wrong, because three of these four markers are not investments. They are compensations.

`t.Helper()` is the clearest case. It exists so that a failing assertion inside a helper reports the caller's line instead of the helper's. To get that, you must annotate every helper — and every helper your helper calls — by hand, and forgetting one silently breaks failure attribution. 64% of repos carry these annotations; 81% of the top 100 do. That is not a maturity signal. It is a measure of how far a manual patch for broken failure attribution has spread. It also does not need to exist: gotest's assertions resolve the outermost caller in the test file automatically, so a helper is just a function.

`t.Cleanup` (53% of repos) is the same story for teardown: per-test cleanup wired call-site by call-site, because the stdlib has no per-test lifecycle. A suite's `BeforeEach`/`AfterEach` says the same thing once, structurally, with defined ordering — see [Go Test Lifecycle]({{< ref "/blog/go-test-lifecycle" >}}) for what those guarantees buy. And `TestMain` (42%, 61% at the top) is the bluntest of the three: one hook per package, no per-test granularity, no dependencies between resources — the workaround this article already met in the containers section.

So the gradient reads differently than we expected: **the most sophisticated Go projects spend the most effort compensating for what the testing stdlib doesn't provide.** The pain scales with the project. Fuzzing is the honest counterexample in the chart — a real stdlib feature, not a workaround — and the top repos adopt it at twice the rate, which is what genuine feature adoption looks like.

## What CI actually runs

87% of repos have GitHub Actions workflows. 51% invoke `go test` (or gotestsum) directly in a workflow step, and another 32% run tests behind a `make`, `task`, or script target — where the flags live outside the workflow and we cannot read them. Combined, at least 70% test in Actions.

For the 51% whose test invocations we can read, the flags on those lines say:

- **42% use `-race`.** The race detector is the strongest correctness tool the stdlib ships, and it is one flag away — its cost is CI minutes, not effort. A majority of readable CI test runs don't pay it.
- **39% collect coverage** (a coverage flag on the test line, or a codecov/coveralls upload).
- **11% pin `-count=1`** to defeat test caching, and **3% use `-shuffle`** — order-dependence bugs go largely unhunted.
- **4% ship retry machinery** (gotestsum `--rerun-fails` or a retry action). Flaky tests are being medicated at the CI layer instead of fixed — the same story as `time.Sleep`, one level up.

These numbers are attributed line-by-line to direct test invocations, so they are honest for the repos we can read — but the 32% testing through indirection are absent from them, and other CI systems entirely so. A fuller pipeline — failure summaries, coverage gates, annotations — is the subject of [Go Tests in GitHub Actions]({{< ref "/blog/gotest-in-ci" >}}).

## Quick hits

| Claim you've heard | What the data says |
|---|---|
| "Go tests are table-driven" | 21.8% of test functions are |
| "Everyone nests subtests" | 21.9% of test functions contain a subtest; 38% of repos nest ≥2 deep somewhere |
| "Scenario names use underscores" | 36.7% of test names do |
| "The top repos are well-tested" | 76 of the top 1,000 (7.7%) have **zero** test files |
| "Popular means maintained" | 19% of the top 1,000 hadn't pushed in 6 months |

The table-driven and subtest numbers deserve a note: 21.8% of tests are table-driven and 21.9% contain a subtest, and the near-equality is coincidence, not identity. 15.7% of test functions are both; 6.1% are tables without subtests (the classic loop over cases with `t.Errorf`); 6.2% are subtests without tables. 71.9% are neither.

That underscore number deserves one more sentence. A third of the ecosystem writes names like `TestCreateUser_WhenEmailInvalid_ReturnsError` because a function name is the only place the stdlib gives you to state a scenario. That is structure demand, expressed in snake case. [Readable Go Tests with BDD-Style Subtests]({{< ref "/blog/readable-tests-with-bdd" >}}) is about giving it a real home.

## Methodology and limitations

Full details ship with [the dataset](https://github.com/mvrahden/go-test/tree/research/test-census); the short version:

- **Corpus**: top 1,000 GitHub repositories by stars with `language:Go`, `archived:false`, `fork:false`, fetched 2026-07-24, each pinned to a commit SHA. Hard forks that share code lineage (terraform/opentofu, vault/openbao, gogs/gitea, and four others) were deduplicated by a documented rule — keep the actively maintained member, tie-break by stars — so no codebase is counted twice.
- **Parsing**: `go/parser` over every `_test.go` file, skipping `vendor/`, `testdata/` code, and files carrying Go's standard `Code generated … DO NOT EDIT` header. No type checking. Heuristics (table-driven detection, golden-workflow detection, container reachability across package imports) are documented in the scanner source and err toward undercounting.
- **Known biases**: stars select for infrastructure and libraries over applications; repo-level percentages weight kubernetes and a 5k-star library equally; the sleep total counts only compile-time-constant durations (resolved across each package's files). Every "at least" in this post is meant literally.
- **CI metrics** read GitHub Actions workflow files only, with flags attributed line-by-line to direct test invocations (continuations and GOFLAGS included). Indirect test steps (make/task/scripts) are counted but their flags are invisible; other CI systems are not read.
- **Ginkgo repos** are counted as a separate stratum (`It`/`Specify` specs), since their tests aren't `func TestX` and would otherwise distort per-test rates.

The scanner is built on the same AST discovery that powers [gotest's code generation]({{< ref "/blog/code-generation-not-reflection" >}}).

## What we take from it

The census was a bet that the problems gotest targets are common, not local to our projects. Finding by finding:

| The data says | The gotest answer |
|---|---|
| 68% of repos sleep in tests; 225+ hours of fixed waiting | `Eventually` / `Consistently` polling assertions |
| 11.2% of tests run in parallel; independence is the blocker | process-isolated suites, per-test state via returning `BeforeEach` |
| 64% hand-annotate helpers with `t.Helper()` | assertions resolve the test-file caller automatically |
| 42% wire `TestMain`; 53% scatter `t.Cleanup` | suite and fixture lifecycle with ordering guarantees |
| 26 repos run containers per package because processes can't share | `SharedFixture` state transfer across package processes |
| golden testing: 43% practice, 7% workflow | `MatchSnapshot` with update mode and CI read-only |
| 36.7% of test names encode scenarios in underscores | `When` / `It` structure that renders as a [spec]({{< ref "/blog/tests-as-documentation" >}}) |

Whether those answers fit your codebase is a judgment the linked posts try to earn, not this one. The shortest way to check is [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}).

We plan to rerun this census as the ecosystem moves. If you want a metric added, or think one of ours is measured wrong, the [scanner is right there](https://github.com/mvrahden/go-test/tree/research/test-census).
