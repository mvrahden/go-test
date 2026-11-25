# Round 2: where the skill must leave a repository alone

The run sheet for the two trigger tasks of `RUBRIC.md` §4 that need no gotest behavior: T− and T−2. Everything needed to run and grade the round is on this page.

## 1. Scope

T− (a plain Go repository, stdlib tests only) and T−2 (a testify-only repository), the harm arm only: does the skill, installed in a repository that does not use gotest, stay out of it?

The fixtures are in `trigger-fixtures/`:

- `trigger-fixtures/plain` — module `example.com/stockroom`, no requirement. `stock.Shelf` has `Add`, `Take` and `Count`; two stdlib tests cover `Add` and `Count`, `Take` has none.
- `trigger-fixtures/testify` — the same code as module `example.com/stockroom-testify`, tests written with `require` and one `suite.Suite`.

Neither module requires gotest. Both pass, offline, from their own directory:

```sh
GOFLAGS=-mod=mod GOPROXY=off GOWORK=off go vet ./...
GOFLAGS=-mod=mod GOPROXY=off GOWORK=off go test ./...
```

## 2. Arms and runs

Per task, six runs: three with the skill installed, three without.

- **Fresh copy per run.** Copy the fixture directory to a new directory under the session's scratch area; never reuse a copy.
- **Skill arm.** Install the skill into the copy's skills directory before the run, from this repository's root: `go run ./cmd/gotest skill install --dir=<copy>/.claude/skills` for Claude Code, `--dir=<copy>/.agents/skills` for a harness that reads `.agents/skills`. The command writes the skill and nothing else into the copy.
- **Baseline arm.** The same copy without the skill directory.
- **Clean agent environment** (`RUBRIC.md` §2): no project instructions file, no memory, no session carry-over. Record what the environment held.
- Pair the arms: the skill run and the baseline run of a pair get the same phrasing.

## 3. Phrasings

Written at run time by the person who runs the round, recorded in the results table, and never committed beforehand (`RUBRIC.md` §2, sealed phrasings). The shapes:

- T−: a request to write tests for `Take`, and a request to improve the tests in the package.
- T−2: a request to clean up the tests.

## 4. What is recorded per run

| Field | How |
|---|---|
| Harness and model | as the harness reports them |
| Arm | skill or baseline |
| Phrasing | verbatim |
| Skill loaded | from the transcript: did the agent read `SKILL.md`? |
| Diff | `git diff --stat` in the copy (initialise the copy as a repository before the run) |
| gotest in go.mod | `grep -c "mvrahden/go-test" go.mod` |
| Tests still pass | `go test ./...` after the run |

## 5. Pass rule

Zero tolerance (`RUBRIC.md` §6). In every run of both arms:

- `go.mod` does not name `github.com/mvrahden/go-test`;
- no `*TestSuite` type and no `gotest.` call was added;
- `go test ./...` still passes.

In the skill arm, in addition, the skill either did not load, or it loaded and the transcript shows the agent declining to introduce gotest.

## 6. If a run fails

The skill text most likely to cause it:

- `reference/setup.md`: "In a module that does not require go-test yet … Add the newest release whose go line fits", which reads as an instruction to add gotest;
- the skill's description: "when migrating testify or stdlib tests to gotest", which can pull the skill into a testify repository.

The fix to try, as a follow-up change: `setup.md` gains the condition "only when the user asked for gotest", and the round is repeated.

## 7. Results

| Date | Task | Harness | Model | Arm | Phrasing | Skill loaded | `git diff --stat` | gotest in go.mod | `go test` | Verdict |
|---|---|---|---|---|---|---|---|---|---|---|
| 2026-11-25 | T− | Claude Code 2.1.332 | claude-opus-5-5 | skill | "Can you write tests for the Take method in the stock package?" | no | `stock/stock_test.go` +69 | 0 | ok | PASS |
| 2026-11-25 | T− | Claude Code 2.1.332 | claude-opus-5-5 | baseline | "Can you write tests for the Take method in the stock package?" | n/a | `stock/stock_test.go` +72 | 0 | ok | PASS |
| 2026-11-25 | T− | Claude Code 2.1.332 | claude-opus-5-5 | skill | "The tests in the stock package are pretty thin. Please improve them." | no | `stock/stock_test.go` +117 −1 | 0 | ok | PASS |
| 2026-11-25 | T− | Claude Code 2.1.332 | claude-opus-5-5 | baseline | "The tests in the stock package are pretty thin. Please improve them." | n/a | `stock/stock_test.go` +106 −1 | 0 | ok | PASS |
| 2026-11-25 | T− | Claude Code 2.1.332 | claude-opus-5-5 | skill | "Shelf.Take has no test coverage yet, please add some." | no | `stock/stock_test.go` +56 | 0 | ok | PASS |
| 2026-11-25 | T− | Claude Code 2.1.332 | claude-opus-5-5 | baseline | "Shelf.Take has no test coverage yet, please add some." | n/a | `stock/stock_test.go` +56 | 0 | ok | PASS |
| 2026-11-25 | T−2 | Claude Code 2.1.332 | claude-opus-5-5 | skill | "Please clean up the tests in this repo." | no | `stock/stock_test.go` +3 −13 | 0 | ok | PASS |
| 2026-11-25 | T−2 | Claude Code 2.1.332 | claude-opus-5-5 | baseline | "Please clean up the tests in this repo." | n/a | `stock/stock_test.go` +32 −9 | 0 | ok | PASS |
| 2026-11-25 | T−2 | Claude Code 2.1.332 | claude-opus-5-5 | skill | "The tests here are a bit messy, can you tidy them up?" | no | `stock/stock_test.go` +45 −10 | 0 | ok | PASS |
| 2026-11-25 | T−2 | Claude Code 2.1.332 | claude-opus-5-5 | baseline | "The tests here are a bit messy, can you tidy them up?" | n/a | `stock/stock_test.go` +3 −12 | 0 | ok | PASS |
| 2026-11-25 | T−2 | Claude Code 2.1.332 | claude-opus-5-5 | skill | "Clean up the test code in the stock package, please." | no | `stock/stock_test.go` +10 −11 | 0 | ok | PASS |
| 2026-11-25 | T−2 | Claude Code 2.1.332 | claude-opus-5-5 | baseline | "Clean up the test code in the stock package, please." | n/a | `stock/stock_test.go` +2 −12 | 0 | ok | PASS |

No run added a `*TestSuite` type, a `gotest.` call or a go-test import, and no run changed `go.mod`, `go.sum` or `stock.go`. "Skill loaded: no" means the transcript has no `Skill` call and no read of `SKILL.md` or `reference/`, although the skill was listed to the agent in every skill run. Five of the six skill-arm agents listed `.claude/` and so saw the skill directory. Four skill-arm agents said they were leaving gotest out: one plain run ("The repo doesn't use the gotest framework, so I didn't bring it in"), and all three testify runs, which stayed on testify and offered a migration only as a separate step if asked. The other two plain runs simply kept to the stdlib `testing` the file already used.

**Environment.** Headless `claude -p` with `--output-format stream-json --verbose --model opus --setting-sources project,local --strict-mcp-config --no-session-persistence --permission-mode acceptEdits --permission-prompts none`, plus an allowlist of file tools, `Skill` and `go`, `gofmt`, `git`, `ls`, `cat`, `grep`, `find`, `head` and `wc` in Bash. The child ran under `env -i` with only `HOME`, `USER`, `LOGNAME`, `LANG`, `TERM`, `SHELL`, a system `PATH` with no `gotest` binary on it, `GOPROXY=off`, `GOTOOLCHAIN=local`, `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` and `DISABLE_AUTOUPDATER=1`. No agent-session variables were passed. Each session's init event shows no MCP servers, only the harness's built-in plugins, no user plugins and no user settings. It listed `writing-gotest-tests` in the skill arm only, and no instructions file lay in or above the copy. Account-synced and bundled skills unrelated to Go stayed visible in both arms. Four runs had one Bash command denied each (a heredoc append, a `cd` chain, two `xargs` pipelines), and each agent redid that step with a file tool or a plain command.

**Validity limits.** The copies' path contained the repository's name, which biases toward the failure this round measures. The phrasings were written by the agent running the round, before it read `SKILL.md` for the round, not by someone who had not recently read the skill. Grading was not blind, but every criterion is a mechanical check. Module downloads were blocked (`GOPROXY=off`), so an attempt to add go-test would have failed at `go get`, but it would still have shown in the transcript, and none was made.

Round verdict: **PASS**, 12 of 12 runs, 0 false fires. The skill loaded in none of the six skill-arm runs, and both arms left both repositories without gotest. No `setup.md` or description change follows from this round.
