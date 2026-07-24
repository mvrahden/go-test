---
title: "Go Test Watch Mode and Focused Tests: The Inner Loop"
date: 2026-07-15
description: "Go test watch mode, F_ focus prefixes, and spec output: how gotest compresses the change-test-read cycle from 30 seconds to under one."
tags: ["Workflow"]
keywords: ["go test watch mode", "gotest watch", "go focused tests", "fast go test feedback"]
cta_text: "Compress your inner loop with watch mode."
---

The "inner loop" is the cycle a developer repeats hundreds of times a day: change code, run tests, read results. The speed of this loop directly affects how you write code. A 30-second cycle means you batch changes and debug in your head. A 1-second cycle means you try things.

gotest has three features that compress the inner loop: watch mode for automatic re-runs, focus prefixes for narrowing scope, and spec output for readable results. Separately, each one removes a specific friction point. Together, they change how you develop.

The whole post in one command:

{{< terminal title="gotest watch ./..." >}}
$ gotest watch ./...
UserService
  Create
    when email is valid
      <span class="t-pass">✓</span> persists the user
{{< /terminal >}}

## The default Go inner loop

Before looking at what gotest adds, it is worth describing what the loop looks like with the standard toolchain. Most Go developers follow some variation of this:

1. Edit code in the editor.
1. Switch to the terminal.
1. Type `go test ./...` or `go test -run TestSomething ./pkg/...`.
1. Wait for compilation and execution.
1. Scan verbose output for failures.
1. Switch back to the editor.

This works. It is reliable and well-understood. But three things slow it down.

**Manual re-runs.** Every save requires a conscious decision to switch to the terminal and re-run. This sounds trivial, but it breaks flow. The context switch is not just physical (alt-tab, type command, press enter). It is cognitive: you interrupt whatever you were thinking about to operate the test runner.

**Verbose targeting.** The `-run` flag uses regex matching, which is powerful but verbose. If you want to run a single test method in a suite, you write something like `go test -run "TestUserService/TestCreate/when_email_is_valid" ./pkg/user/`. That is a lot to type, easy to get wrong, and fragile when test names change.

**Flat output.** `go test -v` prints one line per test, interleaved with log output. When you have 40 tests in a package and three of them fail, finding the failures means scanning every line. There is no hierarchy, no grouping, no visual distinction between passing and failing branches.

None of these are bugs. They are consequences of `go test` being a general-purpose tool. gotest adds three layers on top that address each one.

## Watch mode: automatic re-runs

The first friction point is the manual re-run. Watch mode eliminates it entirely.

```sh
gotest watch ./...
```

That is the entire setup. After this command, the terminal clears and runs your tests. Every time you save a `.go` file, it clears the terminal and re-runs. You never switch to the terminal to type a command again.

Under the hood, watch mode uses fsnotify to monitor your source tree for `.go` file changes. When a file changes, it waits 200 milliseconds for the save to settle (this debounce is configurable via `--debounce`), then converts the changed file's directory into a package pattern and re-runs only that package.

That last point matters. If you have 20 packages and change a file in `pkg/user`, only `pkg/user` re-runs. The other 19 packages are not recompiled or re-executed. In a large project, this is the difference between a 1-second feedback cycle and a 15-second one.

The workflow becomes: edit code, save, glance at the terminal. The results are already there. There is no command to type, no package path to remember, no compilation wait for unrelated code.

## Focus prefixes: narrowing scope

Watch mode re-runs the right package automatically. But within a package, you might have 30 test methods across several suites. During development, you rarely need all of them. You are working on one thing, and you want to see results for that one thing.

The `F_` prefix narrows the run to just what matters.

```go {title="user_test.go"}
// Only this test method runs in the suite
func (s *UserServiceTestSuite) F_TestCreate(t *gotest.T) {
    t.When("email is valid", func(w *gotest.T) {
        w.It("persists the user", func(it *gotest.T) {
            err := s.svc.Create("alice@example.com")
            gotest.NoError(it, err)
        })
    })
}
```

Rename `TestCreate` to `F_TestCreate` and save. Watch mode picks up the change, re-runs the package, and only `F_TestCreate` executes. Every other test method in the suite is skipped. No regex, no `-run` flag, no package path to type.

Focus works at multiple levels:

- **Focus a method:** Rename `TestCreate` to `F_TestCreate`. Only this method runs in its suite; all other methods are skipped.
- **Focus a suite:** Rename `UserServiceTestSuite` to `F_UserServiceTestSuite`. Only this suite runs in the package; all other suites are skipped.
- **Back to normal:** Remove the `F_` prefix and save. Everything runs again.

The inverse also exists. The `X_` prefix excludes:

```go {title="user_test.go"}
// This test is always skipped
func (s *UserServiceTestSuite) X_TestSlowIntegration(t *gotest.T) {
    // ...
}
```

The rules are simple:

- `F_` works on both suite types and test methods.
- When any `F_` exists in a package, only focused items run.
- `X_` always skips, regardless of whether focus is active.
- `X_` takes precedence over `F_`.

Compare this to the `-run` regex approach. Instead of switching to the terminal and typing `go test -run "TestUserService/TestCreate" ./pkg/user/`, you add two characters to the method name in the file you are already editing. The scope change lives in the code, right next to the test, visible to anyone reading the file.

## The combined workflow

Watch mode and focus prefixes are useful independently, but the real shift happens when you use them together. Here is what a typical development session looks like:

1. Start watch mode: `gotest watch ./...`
1. Add `F_` to the test you are working on.
1. Save. Watch mode re-runs. Only the focused test executes.
1. Change the production code. Save. Results appear in about a second.
1. Iterate: change, save, read. No commands to type.
1. When the test passes, remove the `F_` prefix. Save. All tests in the package run.
1. Fix anything that broke. Remove the watch when you are done.

This is a sub-second feedback loop for a single test, with zero manual commands after the initial `gotest watch`. The scope narrows and widens by editing the test file, not by retyping shell commands. The terminal becomes a passive display: you glance at it, you never type into it.

This changes how you work. With a 1-second loop, you write smaller increments. You run the test before you think you are done, not after. You catch typos and logic errors immediately, while the context is still in your head. The test becomes part of the editing flow, not a separate step you do afterward.

## The CI guard

The `F_` prefix is a development tool. It is not meant to be committed. If it reaches your main branch, it silently skips every other test in the package. This is the kind of mistake that passes code review because the tests are green, they just are not running.

gotest's CI mode catches this. When running with the `--ci` flag (or when the `CI` environment variable is set, which most CI providers do automatically), gotest checks for focus prefixes before running any tests. If it finds any, the build fails immediately with an explicit error:

{{< terminal title="CI output" >}}
FAIL: focus prefix detected — remove F_ before merging:
  type F_UserServiceTestSuite
  OrderServiceTestSuite.F_TestCreate
{{< /terminal >}}

This turns a silent skip into a loud failure. The error message lists every focused item so you know exactly what to fix. No test suite runs with incomplete coverage because someone forgot to remove a prefix.

The `gotest refactor toggle-focus` CLI command and VS Code Quick Fix actions make toggling prefixes even faster. Instead of manually renaming the method, you run a command or click a code action. This makes focus prefixes something you add and remove in seconds, not something you carefully type and hope you remember to undo.

## Spec output: readable results

Watch mode handles re-running. Focus prefixes handle scoping. The third piece is reading the results after they arrive.

Standard `go test -v` output looks like this:

{{< gotest-output title="go test -v output" >}}
=== RUN   TestUserService
=== RUN   TestUserService/TestCreate
=== RUN   TestUserService/TestCreate/when_email_is_valid
=== RUN   TestUserService/TestCreate/when_email_is_valid/persists_the_user
--- PASS: TestUserService/TestCreate/when_email_is_valid/persists_the_user (0.00s)
=== RUN   TestUserService/TestCreate/when_email_is_valid/returns_no_error
--- PASS: TestUserService/TestCreate/when_email_is_valid/returns_no_error (0.00s)
=== RUN   TestUserService/TestCreate/when_email_is_duplicate
=== RUN   TestUserService/TestCreate/when_email_is_duplicate/returns_ErrDuplicate
--- PASS: TestUserService/TestCreate/when_email_is_duplicate/returns_ErrDuplicate (0.00s)
{{< /gotest-output >}}

Every line repeats the full path. The hierarchy is encoded in the names, but the output is flat. With 40 tests, this becomes a wall of text where your eyes glaze over looking for the word "FAIL."

The `gotest spec` command renders the same information as a tree:

{{< spec title="gotest spec output" >}}
UserService
  Create
    when email is valid
      <span class="t-pass">✓</span> persists the user
      <span class="t-pass">✓</span> returns no error
    when email is duplicate
      <span class="t-pass">✓</span> returns ErrDuplicate
{{< /spec >}}

The structure is immediately visible. Suite names become headings, methods become groups, `When`/`It` calls become nested branches. Passing tests get a checkmark. Failing tests get a clear marker with the failure message indented beneath them. You see the shape of your test coverage at a glance.

Run spec separately to see the behavioral tree at any point:

```sh
gotest spec ./...
```

Spec output also supports alternative formats for different contexts:

- `--format md` renders Markdown, useful for pasting into PR descriptions or documentation.
- `--format json` renders structured JSON, useful for tooling that consumes test results programmatically.
- `--no-color` strips ANSI codes for piping to files or other commands.

## VS Code integration

The CLI workflow described above is complete on its own. You do not need an editor extension to use watch mode, focus prefixes, or spec output. But the [gotest VS Code extension](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) brings these features into the editor and makes the loop even tighter.

- **Test Explorer** shows the suite and method tree in the sidebar, mirroring the spec output structure.
- **CodeLens** adds Run and Debug buttons above each test method, so you can run a single test without leaving the file.
- **Quick Fix actions** toggle `F_` and `X_` prefixes with one click. No manual renaming needed.
- **Watch mode integration** streams results into the Test Explorer in real-time as you save.
- **Coverage gutters** show statement coverage inline, so you see which lines are covered as you write tests.

This is optional. Everything the extension does maps to a CLI command. The extension makes it faster for developers who prefer to stay in their editor.

## The loop, compressed

The inner loop matters because it is where developers spend most of their time. Not in architecture meetings, not in code review, but in the tight cycle of change-test-read that produces working code.

Watch mode eliminates manual re-runs. You save, results appear. Focus prefixes eliminate waiting for irrelevant tests. You add two characters, and only the test you care about runs. Spec output eliminates scanning for failures. The results render as a readable tree, not a wall of flat text. The CI guard ensures that focus prefixes stay a development tool and never reach production.

Separately, each feature removes one friction point. Together, they compress the inner loop from a 30-second manual process to a sub-second automatic one. The shift is not just in speed. It is in how you think. When feedback is instant, you write differently: smaller steps, more experiments, fewer assumptions carried in your head.

If you are new to gotest, [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}) walks through setting up your first test suite. For CI integration, [gotest in CI]({{< ref "/blog/gotest-in-ci" >}}) covers the `--ci` flag, caching, and parallel execution in detail. And the [VS Code extension](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) brings the entire workflow into your editor.
