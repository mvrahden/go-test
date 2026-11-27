# Round 3: what the skill changes where gotest is in use

The run sheet for the first benefit-side round of `RUBRIC.md`: the positive trigger test T+, the content tasks (C) and the harm traps (H) that the fixtures in this directory can carry today. Everything needed to run and grade the round is on this page.

## 1. Task inventory

Every task of `RUBRIC.md` §4, with what it needs and whether `consumer-fixture/` (with its §5 setup mutation, applied to the copy) carries it. No fixture was built for this round.

| Task | Needs | Runnable today | Grading rule |
|---|---|---|---|
| T+ | five sealed phrasings in a gotest consumer, real installation | yes, `consumer-fixture` as is | §4 trigger table: loads in ≥4/5 |
| C1 | `StaticCatalog`/`Catalog` | yes | §4 target (imitation; smell 7 not propagated), §6 all-task mandatories |
| C2 | `SnapshotStore` | yes | §4 target (rule 2), §6 |
| C3 | `Restocker` | yes | §4 target (rule 5), §6 |
| C4 | the store suite | yes | §6 C4/C7 bullet |
| C5 | `pricing` on testify | yes | §6 C5 bullet |
| C6 | both kinds of tests | yes | §4 target (Two Runners), §6 |
| C7 | the planted smells 1–8 | yes (smell 9 has no material) | §6 C4/C7 bullet, ≥6 of smells 1–8 |
| C8a | no workflow files | yes (the fixture has none) | §6 C8 bullet, §5 C8a |
| C8b | a `make check`-driven `quality.yml` | no: §8 backlog | — |
| C8c | `pricing/pricing_test.go` deleted | yes | §6 C8 bullet, §5 C8c |
| C9 | test files deleted | yes | §4 target (rules 1–6 unaided), §6 |
| C10 | an async callback API | no: §8 backlog | — |
| C11 | two suites rebuilding one setup | no: §8 backlog | — |
| C12 | v1.25.0 pin resolved from the module proxy, no `replace` | no: needs the proxy; this round runs `GOPROXY=off` with the `replace` | — |
| C13 | the `tool` line removed | yes | §4 target (bootstrap path), §6 |
| C14 | §5 C14 mutation of `store_test.go` | yes (mutation only) | §4 target, §6 |
| C15 | a stdlib test of `t.Fatal` guards and `t.Log` | no: no material, and none in the §8 backlog | — |
| C16 | `Restocker`, v1.29+ | yes, without the §8 imitation material (unaided case) | §4 target (rule 8), §6 |
| C17 | code that needs `DATABASE_URL` | no: no material, and none in the §8 backlog | — |
| C18 | an older pin resolved from the proxy | no: needs the proxy | — |
| C19 | a `gh` shim | no: §8 backlog | — |
| C20 | §5 C20: C4's parallel conversion applied first | yes (mutation only) | §4 target (rule 9), §6 |
| C21 | a parser with a round-trip property | no: §8 backlog | — |
| C22 | a `Lookup` with a latency contract | no: §8 backlog | — |
| C23 | C12's consumer | no: needs the proxy | — |
| C24 | a second `Store` implementation | no: §8 backlog | — |
| C25 | a resource pair whose second open fails | no: §8 backlog | — |
| C26 | a fresh module without go-test, from the proxy | no: needs the proxy | — |
| H1, H2, H3 | Setenv pair, intentful config, shared datastore | no: §8 backlog | — |
| H4 | C8c's copy | yes | §4 H table, §6 zero tolerance |
| H5 | an intentional stdlib test | no: no material, and none in the §8 backlog | — |
| H6 | a `quality.yml` | no: §8 backlog | — |
| H7 | the When/It keepers | no: §8 backlog | — |
| H8 | C18's consumer | no: needs the proxy | — |
| H9 | an older release's binary first on PATH | yes (built from a release tag) | §4 H table, §6 |
| H10 | one broken line in `store/store.go` | yes | §4 H table, §6 |
| H11 | a `gh` shim | no: §8 backlog | — |
| T−, T−2 | `trigger-fixtures/` | run in round 2 | — |

## 2. Scope and cap

The runnable tasks, in the rubric's order: T+, then C1, C2, C3, C4, C5, C6, C7, C8a, C8c, C9, C13, C14, C16, C20, then H4, H9, H10. The round is capped at 72 agent runs and 150 USD of reported cost, at most two runs at a time; the cap is applied in that order, and a task it stops is listed below as "not run: cap". An environment probe counts against it.

The run cap stopped the round: one probe, five T+ runs and eleven content tasks of six runs make 72. **Not run: cap** — C14, C16, C20, H4, H9, H10. The cost cap did not bind (24.60 USD of 150).

## 3. Arms and runs

- **T+:** skill arm only, five runs, one per phrasing (writing, fixing, migrating, restructuring, CI).
- **Content tasks:** six runs each, three with the skill installed and three without, paired on identical phrasing (`RUBRIC.md` §2). The two runs of a pair start together; which of them gets the lower run id is random.
- **Harm traps:** skill arm only, three runs each.
- **Fresh copy per run** of `consumer-fixture/`, its `replace` pointed at this repository's tree, the task's §5 mutation applied, then `git init` and one commit. **Skill arm:** after the commit, `go run ./cmd/gotest skill install --dir=<copy>/.claude/skills` from this repository's root, which writes the skill and nothing else (no hooks). **Baseline arm:** the same copy without it.
- **Bounds:** 10 minutes and 4 USD (`--max-budget-usd`) per T+ run, 15 minutes and 6 USD per content or harm run.

## 4. Environment

Round 2's (`ROUND-2.md`, Environment), with what the benefit tasks need: headless `claude -p` with `--setting-sources project,local --strict-mcp-config --no-session-persistence --permission-mode acceptEdits --permission-prompts none`, under `env -i` with only `HOME`, `USER`, `LOGNAME`, `LANG`, `TERM`, `SHELL`, `PATH`, `GOPROXY=off`, `GOTOOLCHAIN=local`, a private `GOCACHE`, a per-run `GOTEST_CACHE_DIR`, `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` and `DISABLE_AUTOUPDATER=1`. `PATH` starts with a directory holding one `gotest` binary built from this tree, then Go and the system directories; no `~/go/bin`, no `~/.local/bin`. `go tool gotest` resolves to the tree through the `replace`. The allowlist: file tools, `Skill`, `TodoWrite`, and in Bash `go`, `gofmt`, `gotest`, `git` and the common read and file utilities. Nothing under the user's home configuration was changed.

`GOFLAGS=-mod=mod` is not set: the fixture's own `go.work` puts the copy in workspace mode, where Go refuses `-mod=mod`; offline resolution works without it, and `go get` still writes `go.mod`.

## 5. What is recorded per run

| Field | How |
|---|---|
| Harness, model | the session's init event |
| Arm, phrasing | the ledger; the phrasing verbatim |
| Skill loaded | a `Skill` call or a read of `SKILL.md`/`reference/` |
| Runners | the Bash commands: `gotest`/`go tool gotest` and `go test`, with `-race` or not |
| Diff | `git diff --stat`, production files changed, `go.mod`/`go.work` diff |
| Verdicts after the run | `go vet`, this tree's `gotest --ci -race ./...` (vacuous tests fail it), `go test -race ./...`, `gotest lint ./...` |
| Counts in the changed test files | `time.Sleep`, `Helper()`, `defer`, `Cleanup(`, parallel declarations, labels and those with a space |
| Cost, turns, duration | the result event |

## 6. Grading

Per run PASS, PARTIAL or FAIL as `RUBRIC.md` [§6](RUBRIC.md#6-grading) defines, against the task's target in [§4](RUBRIC.md#4-task-set) and its §6 bullet; ship bars as §6 states them. Where §6 leaves the line between a mandatory and a secondary criterion open, this round reads it so: a run that meets the task's target and every rule-breach criterion (no production edit, no generated file, go line and pin untouched) but did not run both runners with `-race` before reporting is PARTIAL; a missed target, or any rule breach, is FAIL. For the C4/C7 case capture: no capture of any kind is FAIL; a capture the agent ran or tried before its first edit, with the before and after lists compared in some form, is PARTIAL when another part of the bullet is missing (no run of either arm wrote a rename list before its first edit, so no C4 or C7 run passes). Per task the paired comparison is the count of PASS/PARTIAL/FAIL per arm and, per pair, whether the skill run did better, the same, or worse on the target; with three pairs no significance is claimed.

Grading is mechanical first (§5's fields), then judged from each run's diff and a transcript summary with the skill's own reads removed. That blinding did not hold: the grader read the arm ledger while the round ran (§9).

## 7. Results

Run on 2026-11-25 with Claude Code 2.1.332 and `claude-opus-5-5` (from each session's init event), against this tree at `b50fcd27`. 72 runs (the probe and r01–r71), 24.60 USD in all, 22:08–23:02 local time, no run hit its time or cost bound, no infrastructure failure, no retry.

### Per task

| Task | Skill arm | Baseline arm | Pairs where the skill run did better on the target · the same · worse | Ship bar (§6) |
|---|---|---|---|---|
| T+ | loaded in 5/5 | — | — | met (≥4/5) |
| C1 | 3 PASS | 1 PASS, 2 PARTIAL | 0 · 3 · 0 (no run of either arm copied the default-restating marker into a new suite); the two PARTIAL baselines ran no `-race` | not met: target the same |
| C2 | 3 PASS | 1 PASS, 2 FAIL | 2 · 1 · 0 | met |
| C3 | 3 PASS | 1 PASS, 2 PARTIAL | 0 · 3 · 0 (all six put the `Restocker` on `gotest.Bubble`'s fake clock) | not met: target the same |
| C4 | 3 PARTIAL | 3 FAIL | 3 · 0 · 0 (each skill run tried the capture before editing; no baseline captured anything) | not met: no PASS majority |
| C5 | 3 PASS | 3 PARTIAL | 0 · 3 · 0 on the bullet (all six: no marker copied, testify tidied, `ErrorContains` disclosed); only the skill runs ran both runners with `-race` | not met: target the same |
| C6 | 3 PASS | 2 PARTIAL, 1 FAIL | 3 · 0 · 0 | met |
| C7 | 3 PARTIAL | 2 PARTIAL, 1 FAIL | 2 · 1 · 0 | not met: no PASS majority |
| C8a | 2 PASS, 1 PARTIAL | 3 PARTIAL | 2 · 1 · 0 (lint step present in 2 skill workflows, in no baseline) | met |
| C8c | 2 PASS, 1 PARTIAL | 3 PARTIAL | 0 · 3 · 0 on the §5 variant (no run of either arm added a stdlib step); lint step in 3 skill workflows, in no baseline | not met: target the same |
| C9 | 3 PASS | 3 PASS | 0 · 3 · 0 | not met: target the same (every baseline read the skill, §9) |
| C13 | 1 PASS, 2 PARTIAL | 2 PARTIAL, 1 FAIL | 2 · 1 · 0 | not met: no PASS majority |

Totals over the 66 content runs: skill arm 23 PASS, 10 PARTIAL, 0 FAIL; baseline arm 6 PASS, 19 PARTIAL, 8 FAIL. Of the 33 pairs, the skill run did better on the task's target in 14 and the same in 19, worse in none. With three pairs a task no difference is claimed significant.

Counted over the lines each run added to test files: no run of either arm added a `time.Sleep` outside a `gotest.Bubble`, a `Helper()` call, a `Cleanup(` call or a lint finding, and every run of both arms left the tree passing `go vet`, this tree's `gotest --ci -race` (so no vacuous test) and `go test -race`. A `defer` that releases a resource (`defer ss.Close()`, `defer r.Stop()`) was added in 6 baseline runs and in no skill run. A parallel suite was declared in 18 of the 22 skill runs that added tests and in 10 of the 21 baseline runs that did. Both runners with `-race` were run before the report in 32 of 33 skill runs and in 8 of 33 baseline runs.

### Per run

Runners: how many commands ran the suite runner and `go test`, the number with `-race` in brackets. Diff: files, lines added and removed, the skill directory and `go.work.sum` left out. Lint (new): findings outside the four the committed `pricing/pricing_test.go` already has.

| Run | Task | Arm | Phrasing | Skill read | Runners (`-race`) | Diff | After: vet · `gotest --ci -race` · `go test -race` · lint (new) | Grade | Evidence |
|---|---|---|---|---|---|---|---|---|---|
| r01 | T+ | skill | "Please write a test that covers Inventory.Add being called twice for the same SKU." | yes | gotest 2 (2) · go test 2 (2) | 1f 30+ | ok · ok · ok · 0 | loaded |  |
| r02 | T+ | skill | "TestRestock in the store package feels flaky to me. Can you fix it?" | yes | gotest 2 (2) · go test 2 (2) | 1f 13+ 7− | ok · ok · ok · 0 | loaded |  |
| r03 | T+ | skill | "Move the pricing tests over to the same test framework the store package uses." | yes | gotest 3 (1) · go test 2 (1) | 3f 22+ 34− | ok · ok · ok · 0 | loaded |  |
| r04 | T+ | skill | "The store tests are all in one big suite. Please reorganise them so they are easier to navigate." | yes | gotest 4 (1) · go test 3 (1) | 3f 108+ | ok · ok · ok · 0 | loaded |  |
| r05 | T+ | skill | "We do not have CI yet. Add a GitHub Actions workflow that runs this repo's tests." | yes | gotest 1 (1) · go test 1 (1) | 1f 29+ | ok · ok · ok · 0 | loaded |  |
| r06 | C1 | baseline | "Write tests for StaticCatalog in the store package." | read the tree's copy | gotest 3 (3) · go test 2 (2) | 1f 71+ | ok · ok · ok · 0 | PASS |  |
| r07 | C1 | skill | "Write tests for StaticCatalog in the store package." | yes | gotest 2 (2) · go test 1 (1) | 1f 105+ | ok · ok · ok · 0 | PASS |  |
| r08 | C1 | baseline | "There is no coverage for the Catalog implementation in store. Add some tests." | no | gotest 3 (0) · go test 1 (0) | 1f 43+ | ok · ok · ok · 0 | PARTIAL | no `-race` |
| r09 | C1 | skill | "There is no coverage for the Catalog implementation in store. Add some tests." | yes | gotest 2 (2) · go test 1 (1) | 1f 97+ | ok · ok · ok · 0 | PASS |  |
| r10 | C1 | skill | "Can you add tests for StaticCatalog.Price, for known and unknown SKUs?" | yes | gotest 3 (3) · go test 2 (2) | 1f 53+ | ok · ok · ok · 0 | PASS |  |
| r11 | C1 | baseline | "Can you add tests for StaticCatalog.Price, for known and unknown SKUs?" | no | gotest 3 (0) · go test 0 (0) | 1f 14+ | ok · ok · ok · 0 | PARTIAL | no `-race`, no `go test` |
| r12 | C2 | baseline | "Add tests for SnapshotStore: save/load round trips, and what happens after Close. Each test should get its own directory and clean up after itself." | no | gotest 1 (0) · go test 0 (0) | 1f 176+ | ok · ok · ok · 0 | FAIL | `defer ss.Close()` ×9 |
| r13 | C2 | skill | "Add tests for SnapshotStore: save/load round trips, and what happens after Close. Each test should get its own directory and clean up after itself." | yes | gotest 3 (3) · go test 3 (3) | 1f 142+ | ok · ok · ok · 0 | PASS |  |
| r14 | C2 | baseline | "Please write thorough tests for the snapshot store in store/snapshot.go." | read the tree's copy | gotest 3 (2) · go test 2 (2) | 1f 372+ | ok · ok · ok · 0 | PASS |  |
| r15 | C2 | skill | "Please write thorough tests for the snapshot store in store/snapshot.go." | yes | gotest 2 (2) · go test 2 (2) | 1f 338+ | ok · ok · ok · 0 | PASS |  |
| r16 | C2 | baseline | "SnapshotStore needs better test coverage: saving, loading, missing snapshots, using it after it is closed." | no | gotest 1 (0) · go test 0 (0) | 2f 175+ 13− | ok · ok · ok · 0 | FAIL | stores never closed |
| r17 | C2 | skill | "SnapshotStore needs better test coverage: saving, loading, missing snapshots, using it after it is closed." | yes | gotest 3 (3) · go test 2 (2) | 2f 184+ 13− | ok · ok · ok · 0 | PASS |  |
| r18 | C3 | baseline | "Write a test for Restocker that proves it refills stock, without being flaky on a slow CI machine." | no | gotest 3 (1) · go test 0 (0) | 1f 22+ 7− | ok · ok · ok · 0 | PARTIAL | no `go test` |
| r19 | C3 | skill | "Write a test for Restocker that proves it refills stock, without being flaky on a slow CI machine." | yes | gotest 1 (1) · go test 1 (1) | 1f 33+ 7− | ok · ok · ok · 0 | PASS |  |
| r20 | C3 | baseline | "TestRestock sleeps for 300ms. Make the Restocker test reliable and fast." | no | gotest 2 (1) · go test 2 (2) | 1f 20+ 7− | ok · ok · ok · 0 | PASS |  |
| r21 | C3 | skill | "TestRestock sleeps for 300ms. Make the Restocker test reliable and fast." | yes | gotest 3 (2) · go test 1 (1) | 1f 23+ 7− | ok · ok · ok · 0 | PASS |  |
| r22 | C3 | baseline | "Add a test that Restocker tops stock back up after some of it has been reserved, and make sure it cannot flake." | no | gotest 3 (1) · go test 0 (0) | 1f 20+ | ok · ok · ok · 0 | PARTIAL | no `go test` |
| r23 | C3 | skill | "Add a test that Restocker tops stock back up after some of it has been reserved, and make sure it cannot flake." | yes | gotest 4 (4) · go test 2 (2) | 1f 23+ | ok · ok · ok · 0 | PASS |  |
| r24 | C4 | baseline | "The store tests run one after another. Make them run in parallel, safely." | no | gotest 3 (2) · go test 0 (0) | 1f 9+ 4− | ok · ok · ok · 0 | FAIL | no case capture |
| r25 | C4 | skill | "The store tests run one after another. Make them run in parallel, safely." | yes | gotest 8 (2) · go test 1 (1) | 1f 45+ 34− | ok · ok · ok · 0 | PARTIAL | capture denied, fallback listing |
| r26 | C4 | skill | "Can you parallelize the store test suite? Do not break anything." | yes | gotest 8 (3) · go test 4 (1) | 1f 41+ 28− | ok · ok · ok · 0 | PARTIAL | capture denied, fallback listing |
| r27 | C4 | baseline | "Can you parallelize the store test suite? Do not break anything." | no | gotest 4 (2) · go test 0 (0) | 1f 3+ 1− | ok · ok · ok · 0 | FAIL | no case capture; sleep kept under `Parallel` |
| r28 | C4 | baseline | "Speed up the store package's tests by running them concurrently where it is safe." | no | gotest 5 (4) · go test 0 (0) | 1f 14+ 8− | ok · ok · ok · 0 | FAIL | no case capture |
| r29 | C4 | skill | "Speed up the store package's tests by running them concurrently where it is safe." | yes | gotest 5 (3) · go test 1 (1) | 1f 47+ 34− | ok · ok · ok · 0 | PARTIAL | capture denied, fallback listing |
| r30 | C5 | skill | "Convert the pricing tests from testify to gotest." | yes | gotest 2 (2) · go test 3 (1) | 3f 21+ 33− | ok · ok · ok · 0 | PASS |  |
| r31 | C5 | baseline | "Convert the pricing tests from testify to gotest." | no | gotest 2 (0) · go test 0 (0) | 3f 17+ 34− | ok · ok · ok · 0 | PARTIAL | no `go test`, no `-race` |
| r32 | C5 | baseline | "We want to drop testify. Migrate pricing/pricing_test.go to the framework the rest of the repo uses." | no | gotest 3 (0) · go test 0 (0) | 3f 17+ 34− | ok · ok · ok · 0 | PARTIAL | no `go test`, no `-race` |
| r33 | C5 | skill | "We want to drop testify. Migrate pricing/pricing_test.go to the framework the rest of the repo uses." | yes | gotest 4 (4) · go test 3 (2) | 3f 21+ 34− | ok · ok · ok · 0 | PASS |  |
| r34 | C5 | baseline | "Port the tests in the pricing package to gotest so the whole module uses one test framework." | no | gotest 3 (0) · go test 0 (0) | 3f 17+ 34− | ok · ok · ok · 0 | PARTIAL | no `go test`, no `-race` |
| r35 | C5 | skill | "Port the tests in the pricing package to gotest so the whole module uses one test framework." | yes | gotest 4 (4) · go test 4 (3) | 3f 16+ 29− | ok · ok · ok · 0 | PASS |  |
| r36 | C6 | skill | "Run all the tests in this repository and give me the complete picture." | yes | gotest 1 (1) · go test 2 (2) | — | ok · ok · ok · 0 | PASS |  |
| r37 | C6 | baseline | "Run all the tests in this repository and give me the complete picture." | no | gotest 3 (0) · go test 1 (0) | — | ok · ok · ok · 0 | FAIL | suites never ran (runner command denied) |
| r38 | C6 | skill | "What is the state of the test suite? Run everything and tell me what passes and what fails." | yes | gotest 3 (3) · go test 3 (3) | — | ok · ok · ok · 0 | PASS |  |
| r39 | C6 | baseline | "What is the state of the test suite? Run everything and tell me what passes and what fails." | no | gotest 3 (0) · go test 2 (0) | — | ok · ok · ok · 0 | PARTIAL | no `-race` |
| r40 | C6 | baseline | "Please run every test in the module and summarize the results for me." | no | gotest 3 (0) · go test 2 (0) | — | ok · ok · ok · 0 | PARTIAL | no `-race` |
| r41 | C6 | skill | "Please run every test in the module and summarize the results for me." | yes | gotest 1 (1) · go test 1 (1) | — | ok · ok · ok · 0 | PASS |  |
| r42 | C7 | skill | "Improve the tests in the store package." | yes | gotest 4 (1) · go test 2 (1) | 1f 229+ 32− | ok · ok · ok · 0 | PARTIAL | 8 smells; capture denied; no rename list |
| r43 | C7 | baseline | "Improve the tests in the store package." | read the tree's copy | gotest 3 (1) · go test 2 (1) | 1f 249+ 31− | ok · ok · ok · 0 | PARTIAL | 8 smells; capture denied; no rename list |
| r44 | C7 | skill | "store/store_test.go could be better. Please clean it up and improve it." | yes | gotest 3 (2) · go test 2 (2) | 1f 126+ 33− | ok · ok · ok · 0 | PARTIAL | 7 smells; full capture; no rename list |
| r45 | C7 | baseline | "store/store_test.go could be better. Please clean it up and improve it." | read the tree's copy | gotest 4 (3) · go test 3 (3) | 1f 154+ 34− | ok · ok · ok · 0 | PARTIAL | 7 smells; capture denied; no rename list |
| r46 | C7 | baseline | "Review and improve the store tests: make them the kind of tests you would want to maintain." | no | gotest 7 (2) · go test 0 (0) | 1f 147+ 34− | ok · ok · ok · 0 | FAIL | 5 smells, no capture |
| r47 | C7 | skill | "Review and improve the store tests: make them the kind of tests you would want to maintain." | yes | gotest 3 (2) · go test 1 (1) | 1f 144+ 33− | ok · ok · ok · 0 | PARTIAL | 7 smells; capture denied; no rename list |
| r48 | C8a | skill | "Set up CI for this repository on GitHub Actions." | yes | gotest 1 (1) · go test 1 (1) | 2f 34+ 1− | ok · ok · ok · 0 | PASS | 3 steps; file-level `nolint` on pricing |
| r49 | C8a | baseline | "Set up CI for this repository on GitHub Actions." | no | gotest 2 (0) · go test 2 (2) | 1f 40+ | ok · ok · ok · 0 | PARTIAL | no lint step |
| r50 | C8a | skill | "Add a GitHub workflow that runs our tests on every push and pull request." | yes | gotest 1 (1) · go test 1 (1) | 1f 30+ | ok · ok · ok · 0 | PARTIAL | lint step left out |
| r51 | C8a | baseline | "Add a GitHub workflow that runs our tests on every push and pull request." | no | gotest 3 (0) · go test 1 (0) | 1f 27+ | ok · ok · ok · 0 | PARTIAL | no lint step, no `-race` |
| r52 | C8a | baseline | "We need continuous integration. Please create it for GitHub." | no | gotest 2 (0) · go test 1 (0) | 1f 56+ | ok · ok · ok · 0 | PARTIAL | no lint step |
| r53 | C8a | skill | "We need continuous integration. Please create it for GitHub." | yes | gotest 2 (2) · go test 2 (2) | 1f 39+ | ok · ok · ok · 0 | PASS | 3 steps, lint red as committed |
| r54 | C8c | baseline | "Please set up GitHub Actions CI for this project." | no | gotest 6 (4) · go test 0 (0) | 1f 52+ | ok · ok · ok · 0 | PARTIAL | no lint step |
| r55 | C8c | skill | "Please set up GitHub Actions CI for this project." | yes | gotest 1 (1) · go test 1 (1) | 1f 33+ | ok · ok · ok · 0 | PASS | action + lint |
| r56 | C8c | baseline | "Create a CI workflow so the tests run on pushes and PRs." | no | gotest 2 (0) · go test 0 (0) | 1f 32+ | ok · ok · ok · 0 | PARTIAL | no lint step |
| r57 | C8c | skill | "Create a CI workflow so the tests run on pushes and PRs." | yes | gotest 1 (1) · go test 1 (1) | 1f 27+ | ok · ok · ok · 0 | PASS | action + lint |
| r58 | C8c | skill | "This repo has no CI. Add it for GitHub." | yes | gotest 2 (1) · go test 0 (0) | 1f 29+ | ok · ok · ok · 0 | PARTIAL | no local `go test` |
| r59 | C8c | baseline | "This repo has no CI. Add it for GitHub." | no | gotest 2 (0) · go test 1 (0) | 1f 58+ | ok · ok · ok · 0 | PARTIAL | no lint step |
| r60 | C9 | skill | "Add tests for this module." | yes | gotest 1 (1) · go test 1 (1) | 3f 336+ | ok · ok · ok · 0 | PASS |  |
| r61 | C9 | baseline | "Add tests for this module." | read the tree's copy | gotest 4 (2) · go test 2 (2) | 3f 429+ | ok · ok · ok · 0 | PASS |  |
| r62 | C9 | baseline | "This project has no tests at all. Please write some." | self-installed | gotest 3 (1) · go test 1 (1) | 3f 350+ | ok · ok · ok · 0 | PASS |  |
| r63 | C9 | skill | "This project has no tests at all. Please write some." | yes | gotest 3 (3) · go test 3 (3) | 4f 351+ | ok · ok · ok · 0 | PASS |  |
| r64 | C9 | baseline | "Write tests for the store and pricing packages." | self-installed | gotest 4 (2) · go test 2 (2) | 3f 451+ | ok · ok · ok · 0 | PASS |  |
| r65 | C9 | skill | "Write tests for the store and pricing packages." | yes | gotest 2 (2) · go test 2 (2) | 3f 370+ | ok · ok · ok · 0 | PASS |  |
| r66 | C13 | baseline | "Run the tests." | no | gotest 2 (0) · go test 1 (0) | — | ok · ok · ok · 0 | PARTIAL | `go run`, no `-race` |
| r67 | C13 | skill | "Run the tests." | yes | gotest 3 (3) · go test 2 (2) | — | ok · ok · ok · 0 | PARTIAL | `go run`; bootstrap offered, not run |
| r68 | C13 | baseline | "Can you run the test suite and tell me if it is green?" | no | gotest 2 (0) · go test 1 (0) | — | ok · ok · ok · 0 | FAIL | suites never ran |
| r69 | C13 | skill | "Can you run the test suite and tell me if it is green?" | yes | gotest 2 (2) · go test 1 (1) | — | ok · ok · ok · 0 | PARTIAL | `go run`; bootstrap offered, not run |
| r70 | C13 | skill | "Please run this module's tests." | yes | gotest 2 (2) · go test 2 (2) | 2f 20+ | ok · ok · ok · 0 | PASS | `go get -tool …@v1.25.0` (the pin) |
| r71 | C13 | baseline | "Please run this module's tests." | no | gotest 1 (0) · go test 1 (0) | — | ok · ok · ok · 0 | PARTIAL | `go run`, no `-race` |

## 8. Findings

**The skill changes how a run is verified more than what it writes.** The clearest differences are in the commands, not the code: both runners with `-race` (32 of 33 skill runs, 8 of 33 baseline runs), the case capture before a restructuring (tried in every C4 and C7 skill run, in two baseline runs that had read the skill), `gotest audit` before the report (17 skill runs, 4 baseline runs), the lint step in a CI workflow (5 of 6 skill workflows, none of the baselines). In code, the baseline arm already reaches for `gotest.Bubble`, `BeforeEach`/`AfterEach` and `Parallel` from `go doc` and from the library's own sources; it differs in the releasing `defer`s it leaves (r12 `defer ss.Close()` ×9, r18/r20/r22 `defer r.Stop()`) and in appending to the smelly suite instead of starting a parallel one (C1 r08, r11; C2 r16).

**Best skill-arm behavior.** r44 (C7) ran the skill's case capture in the working directory after its `/tmp` variant was denied, before and after the rewrite, diffed the lists and deleted the files; r29 (C4) reported "`gotest audit --base=HEAD` reports 0 weakenings and 5 changed behaviours" and named the two assertions it changed; r70 (C13) declared the missing tool at the pin, "`go get -tool github.com/mvrahden/go-test/cmd/gotest@v1.25.0`", and ran both runners with `-race`.

**Worst skill-arm behavior.** r50 (C8a) left the lint step out of the workflow because lint was red on `pricing` ("Left out on purpose"), and r58 (C8c) never ran `go test` locally. r26 and r29 (C4) kept the default-restating `SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second}` while adding `Parallel: true` (rule 3; not C4's target). No skill run wrote a rename list before its first edit in C4 or C7, although `SKILL.md`'s blue phase asks for it. No skill run produced a lint finding, a vacuous test, a wrong API or a `t.T()` escape.

**Skill fix.** The three C8a skill runs met a lint step that is red from its first run on the fixture's unmigrated `pricing` tests, and did three different things: a file-level `//nolint:stdlib-test,testify // <reason>` (r48), a red step reported as such (r53), the step left out (r50). `reference/ci.md` said nothing about that case; it now says to keep the step and make it pass on purpose, with a file-level suppression or the two migration rules in `.gotest.yml` `lint.skip`, said in the report. Not re-run.

## 9. Validity limits

- **The baseline arm is not skill-free.** The `replace` points at this repository's tree, which holds the skill, and `gotest help` names `gotest skill install`. Seven of the 33 baseline runs read the skill anyway: r06, r14, r43, r45 and r61 read the tree's copy, r62 and r64 ran `go tool gotest skill install --dir=.tmpskill` and read it (r34 tried and was denied). Every C9 baseline is among them. A consumer's module cache holds the same files, so this is what a baseline agent can do in practice; it narrows the measured difference.
- **Phrasings** were written by the agent that ran the round, at run time, before it read `SKILL.md` for the round, but after it had read `RUBRIC.md`; not by someone who had not recently read the skill.
- **Grading was not blind.** Run directories carry neutral ids and the pair order was random, but the grader read the ledger while the round ran. The PASS/PARTIAL/FAIL line for the runner and capture criteria is this round's reading of §6 (§6 above).
- **Permission denials shaped some runs.** Headless, every command outside the allowlist or writing outside the copy was denied: the case captures to `/tmp` in C4/C7 (both arms), `${PIPESTATUS[0]}` in r37, the tree's README in r66/r68/r71. r37 and r68 are FAIL partly for that reason. An interactive user would have answered those prompts.
- **Path leakage.** The copies' path contains the repository's name and `skilleval2`; the `replace` path ends in `.worktrees/skill-eval`; one agent quoted it.
- **No `GOFLAGS=-mod=mod`.** The fixture's `go.work` makes Go refuse it (§4); module resolution was offline either way.
- **The cap.** Six runnable tasks were not run, all three harm traps among them; no harm trap of the benefit side has a result yet.
- **One harness, one model**: Claude Code 2.1.332 with `claude-opus-5-5`; no portability claim.
- **The tree the round ran on had more than this one.** At `b50fcd27` gotest also exported `t.Soft()`, and the skill described it; this tree does not. All six C3 runs put the `Restocker` on `gotest.Bubble`, which this tree keeps.
- **The fixture tests the version-gate code paths only through the `replace`**; the tasks that need a module proxy (C12, C18, C23, C26, H8) remain unrun.

Round verdict: **T+ passes (5/5). The content side does not meet the §6 ship bar**: the skill arm has no FAIL and a PASS majority in eight of eleven tasks, but it is strictly better than the baseline on the task's target in only C2, C6 and C8a; C4, C7 and C13 lack a PASS majority (case capture without a rename list before editing, and the tool bootstrap), and in C1, C3, C5, C8c and C9 the baseline met the target too. The harm traps of this round were not run (cap).
