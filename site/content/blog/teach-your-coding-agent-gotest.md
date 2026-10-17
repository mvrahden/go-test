---
title: "Teach Your Coding Agent gotest"
date: 2026-10-17
draft: true
description: "Coding agents write Go tests from stdlib and testify habits. The writing-gotest-tests skill teaches them gotest's rules, matched to the version your go.mod pins."
tags: ["Tooling"]
keywords: ["gotest agent skill", "claude code go tests", "copilot go test suites", "agent skills go"]
cta_text: "Install the skill that matches your gotest version."
cta_command: "go tool gotest skill install"
---

Ask a coding agent to add a test to a gotest suite and it will usually write a good stdlib test. That is the problem. It cleans up with `defer` inside a suite method, waits for a goroutine with `time.Sleep`, skips a database suite with `t.T().Skip()` from `BeforeAll`, and checks equality with `gotest.True(t, got == want)`. Then it runs `go test ./...`, sees green, and reports success, although `go test` never ran a single suite.

None of this is the agent being careless. Its priors come from millions of stdlib and testify tests, and gotest deliberately inverts several of them: resources belong to lifecycle hooks, polling replaces sleeping, environment gating happens in `SuiteGuard` before any setup runs, and suites run under `go tool gotest`, not `go test`. An agent has no way to know that unless something tells it.

## What the skill is

`writing-gotest-tests` is an [Agent Skill](https://agentskills.io): a folder with a `SKILL.md` and reference files, in the open format that Claude Code, Codex, GitHub Copilot, Cursor, Gemini CLI, Antigravity and most other coding agents load on demand. The agent reads the short description up front and pulls in the rest only when a task is about gotest tests, so it costs nothing in sessions that never touch them.

`SKILL.md` holds the rules that matter on every task. Most of them exist because an agent broke them in practice:

- **Lifecycle hooks own resources.** Setup goes in `BeforeEach` or `BeforeAll`, teardown in the matching `After*`; never `defer` or `t.T().Cleanup()` inside a suite method.
- **Poll, never sleep.** `gotest.Eventually` and `gotest.Consistently`, with only gotest assertions inside the callback.
- **Ask why a suite is not parallel**, and give parallel methods their own state through a returning `BeforeEach`.
- **Gate on the environment with `SuiteGuard`**, not a skip.
- **A complete run is both runners**: `go test` for stdlib tests, `go tool gotest` for suites.

The reference files go deeper where a task needs it: assertions, configuration, fixtures, fuzzing, CI, migrating from testify, refactoring, and reporting a bug in gotest itself.

## It knows which gotest you run

gotest ships breaking changes in minor releases, so "how gotest behaves" depends on the version your `go.mod` pins. The skill starts every task by reading that pin. It upgrades the pin only to a release whose Go line your module already allows (never `@latest`, which can silently raise your Go version), and `reference/versions.md` lists what each older release lacks or does differently. An agent working on a project pinned to an older release writes code that compiles there, instead of code for the newest API.

## Installing it

From v1.34.0 the CLI installs the skill itself:

```bash
go tool gotest skill install                         # .agents/skills: Copilot, Codex, Cursor, Gemini CLI, Antigravity, ...
go tool gotest skill install --dir=.claude/skills    # Claude Code
```

This writes the copy of the skill that shipped with the gotest version your `go.mod` pins, and replaces an older copy. Rerun it after upgrading gotest; the skill's own upgrade checklist tells the agent to do the same.

Commit the folder. Then every teammate's agent, and every agent running in CI, works from the same rules, and an upgrade shows up in review as a diff to the skill next to the diff to `go.mod`.

If you would rather install it once for all projects, the agents with a plugin system can do that:

| Agent | Install |
|---|---|
| Claude Code | `/plugin marketplace add mvrahden/go-test`, then `/plugin install gotest@go-test` |
| Codex | `codex plugin marketplace add mvrahden/go-test`, then `codex plugin add gotest@go-test` |
| GitHub Copilot CLI | `copilot plugin marketplace add mvrahden/go-test`, then `copilot plugin install gotest@go-test` |
| VS Code with Copilot Chat | included in the [gotest extension](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) |
| Any other agent | `npx skills add mvrahden/go-test` |

A plugin carries the newest skill rather than the one for your pin: plugins install from the repository's main branch, and the VS Code extension bundles the skill of its release. The skill's version notes keep that safe for older pins, but `skill install` is the exact match.

## What it does not replace

The skill makes an agent write the right shape of test far more often; it does not make review optional. The mechanical rules are also enforced where they can be: `go tool gotest lint` flags `t.T().Helper()` and `t.T().Cleanup()` in suite methods, a `BeforeAll` without an `AfterAll`, boolean checks that have a specific assertion, and more, and `lint -fix` rewrites the findings that have a safe fix. Run both, and let the linter catch what the agent still gets wrong.
