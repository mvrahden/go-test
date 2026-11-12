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

Round verdict: _not run yet_.
