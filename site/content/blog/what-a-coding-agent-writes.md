---
title: "What a Coding Agent Writes When You Ask for a Go Test"
date: 2026-09-24
draft: true
description: "Same package, same prompt, thirty runs: how a coding agent's Go tests change with no guidance, with a one-line AGENTS.md, and with the gotest skill installed."
tags: ["Research", "AI Engineering"]
keywords: ["coding agent go tests", "claude code go testing", "ai generated tests go", "agent skills go", "cursor go tests quality"]
cta_text: "Install the skill and ask your agent for the next test."
cta_command: "mkdir -p .claude/skills && curl -sL https://github.com/mvrahden/go-test/archive/refs/heads/main.tar.gz | tar -xz -C .claude/skills --strip-components=2 go-test-main/skills/writing-gotest-tests"
---

<!--
DRAFT. Every number marked [[…]] is a placeholder for the experiment described in
research/agent-skill-experiment/ (branch research/subtest-naming). Run the protocol,
paste results.csv into the tables, then choose the interpretation paragraphs.
Do not publish with a placeholder left in.
-->

Ask a coding agent for a Go test and it writes one. It compiles, it passes, and it looks like the tests it learned from. That last part is the problem. The [July census]({{< ref "/blog/how-go-actually-tests" >}}) measured what those tests look like across the 1,000 most-starred Go repositories: 68% of repos sleep in tests, 64% hand-annotate helpers with `t.Helper()`, 11.2% of test functions run in parallel. An agent trained on that corpus reproduces that corpus. Ask it for a test of anything asynchronous and the odds of a `time.Sleep` are the odds in the data.

We wanted to know how much of that an instruction can change, and which instruction. So we ran an experiment.

## The setup

One package, one prompt, three arms, ten runs each, every run in a fresh session with no memory of the others.

**The package.** A rate limiter with a background refill goroutine and a file-backed audit log. It is [in the repository](https://github.com/mvrahden/go-test/tree/research/subtest-naming/research/agent-skill-experiment/target), about eighty lines, and it is built to tempt. Timing invites a sleep. The goroutine invites a data race. The log file invites `defer os.Remove`. The three behaviors the prompt asks for cannot be tested well without confronting all three.

**The prompt**, identical in every run:

> Write tests for package `ratelimit`. Cover the refill timing, the burst limit, and the audit log. Run them and make sure they pass.

**The arms.**

- **Bare.** No instructions in the repository beyond the package.
- **AGENTS.md.** One sentence: "Tests in this repository use github.com/mvrahden/go-test suites." Nothing about how.
- **Skill.** The [writing-gotest-tests skill](https://github.com/mvrahden/go-test#coding-agents) installed in `.claude/skills/`, and nothing else.

**The scoring.** Every run's output is scored the same way the census scores a repository, by parsing, plus three checks that need execution:

| Metric | How it is measured |
|---|---|
| compiles | `go vet` on the package |
| passes | both runners green with `-race` (`go test` and `gotest`) |
| sleeps | `time.Sleep` call sites in test code |
| helper annotations | `t.Helper()` call sites |
| ad hoc teardown | `defer` and `t.Cleanup` in test functions |
| parallel | test functions or suites declared parallel |
| prose names | subtest or behavior labels containing a space |
| lint findings | `gotest lint` findings, for the arms that produce suites |

The scoring script, the prompt, and the per-run outputs are in the same directory as the target, so the whole thing reruns with one command against whichever agent you use.

## What came out

[[Table: one row per arm, columns = the eight metrics above, values = mean over ten runs with min–max in parentheses. Add a row for the census baseline where a metric exists there (sleeps per repo, helper share, parallel share).]]

[[Interpretation, chosen once the numbers exist. The three readings we expect to choose among:
- The bare arm reproduces the census: sleeps in most runs, helpers annotated, nothing parallel, teardown by defer. That is the training data speaking.
- The AGENTS.md arm changes the framework and little else: suites appear, but the sleeps and the defers come along, because one sentence says what to use and nothing about how. This is the most important row if it holds: naming the tool is not the same as teaching it.
- The skill arm removes the sleeps (Eventually), the helpers (automatic attribution), and the defers (BeforeEach/AfterEach), and parallelizes when the recipe allows. The interesting failures are the ones the skill did not prevent; list them honestly.]]

[[One paragraph on the runs that did not pass, per arm, and why. Agents that wrote a flaky sleep and got a green run by luck count as passing; note how many.]]

## What the skill actually says

The skill is a Markdown file the agent reads before it starts. It does not contain examples to copy. It contains the rules that invert the habits above, with the reason for each, because an agent that knows why is harder to argue out of it mid-task. The four that did the most work in this experiment:

- **Lifecycle hooks own resources.** Setup in `BeforeEach`, teardown in `AfterEach`, never `defer` or `t.Cleanup` in a test method. A `defer` lints clean and still skips teardown verification and blocks parallelization.
- **Poll, never sleep.** `Eventually` with a deadline and a tick, asserting a stable fixed point rather than a transient state.
- **Ask why the suite is not parallel.** The recipe is five lines, and the skill names the two legitimate reasons to stay sequential so the agent does not invent a third.
- **Write the condition, not the connective.** `When("the bucket is empty")`, so the spec renders `when the bucket is empty` and the behavior reads as a sentence.

Each rule is tagged with the gotest version it needs, and the skill tells the agent to check the version first, because advice for v1.29 applied to a v1.26 pin fails in ways the agent cannot see.

## Why this is a spec-driven development post

The [first post in this series]({{< ref "/blog/spec-driven-development-go" >}}) argued that the spec should be the test, because a test is the one spec a run can verify. That argument has a dependency it did not state: the tests have to be good enough to be a spec. A test that sleeps for a second and asserts a transient state is a spec of nothing. A test named `TestRefill2` is a spec nobody can read.

[[Sentence with the prose-names number: in the skill arm, [[P]]% of behaviors read as sentences; in the bare arm, [[Q]]% of subtests did.]] The agent that writes the spec is the agent you gave the rules to. The experiment above is what those rules are worth, in numbers you can reproduce.

## Reproducing it

```sh
git clone -b research/subtest-naming https://github.com/mvrahden/go-test
cd go-test/research/agent-skill-experiment
./run.sh bare 10 && ./run.sh agentsmd 10 && ./run.sh skill 10
./score.sh runs/ > results.csv
```

`run.sh` prepares a fresh copy of the target for each run and prints the prompt to paste into your agent; the session itself is yours to drive, because the point is to measure the agent you use. `score.sh` parses every run's test files with the census scanner, runs both runners with `-race`, and writes one CSV row per run. Send us yours.
