---
title: "gotest VS Code Extension: Suite-Aware Go Testing"
date: 2026-07-23
description: "The gotest VS Code extension makes Go test suites visible in your editor: discovery, run/debug at any level, spec view, coverage gutters, and watch mode."
tags: ["Tooling"]
keywords: ["go test vscode extension", "vscode go test explorer", "gotest vscode"]
cta_text: "Install the gotest VS Code extension."
cta_command: "code --install-extension mvrahden.gotest"
---

You write gotest suites, open VS Code's Testing sidebar — and it's empty. No test tree, no CodeLens Run buttons above your suite methods, nothing to debug. Standard Go tooling has nothing to show you, because as far as gopls and the Go Test Explorer are concerned, your suites aren't tests at all.

The [gotest VS Code extension](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) exists to close this gap. Because it's purpose-built for suite-based testing rather than a generic test runner, it can surface things standard tooling can't: the suite hierarchy, the behavioral spec view, and coverage that survives across runs.

Installation is one command — `code --install-extension mvrahden.gotest` — or grab it from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) or [Open VSX](https://open-vsx.org/extension/mvrahden/gotest). The rest of this post is the feature tour.

## Why your editor can't see gotest suites

To understand why the extension matters, you need to understand what the editor sees without it.

gotest generates standard `go test` code from your suite definitions and injects it via Go's overlay filesystem. The generated files never touch your module — they live in a temporary directory, invisible to your source tree and to `git status`. This is a deliberate architectural choice: your module stays clean, the generated code can't go stale, and the overlay handles invalidation automatically.

But it comes with a trade-off. Standard Go tooling — gopls, the Go Test Explorer, `go test` itself — can only see what's in your module. Since the generated test functions aren't there, your editor doesn't know your suites are tests. It sees structs with methods. Not test suites. Not test cases. Just ordinary Go code.

Here's a gotest suite:

```go {title="cart_test.go"}
type CartTestSuite struct {
    cart *Cart
}

func (s *CartTestSuite) BeforeEach(t *gotest.T) {
    s.cart = NewCart()
}

func (s *CartTestSuite) TestAddItem(t *gotest.T) {
    t.When("the cart is empty", func(t *gotest.T) {
        s.cart.Add("widget", 1)
        t.It("adds the item", func(t *gotest.T) {
            gotest.Equal(t, 1, s.cart.Len())
        })
    })
}
```

To gopls, this is a struct called `CartTestSuite` with two methods. It's not a test. There is no `func TestXxx(t *testing.T)` in the file or anywhere in the module. The standard Go Test Explorer shows nothing. CodeLens doesn't offer a "Run Test" button. The test sidebar is empty.

When you run `gotest ./...` from the terminal, the CLI generates the wiring code into a temporary directory, passes it to `go test` via the `-overlay` flag, and everything works. But the editor never sees that overlay. It only sees your module — and your module has no test functions.

This is the gap the extension closes.

## Test discovery

The extension's first job is the most fundamental: making your suites visible as tests.

On activation, it scans your workspace for Go files, parses the AST, and identifies suite types (structs ending in `TestSuite`) and their test methods (methods starting with `Test`). It registers these with VS Code's Test Controller, and they appear in the Testing sidebar as a structured tree: **Package > Suite > Method**, with a fourth level — **Subtests** — populated after the first execution.

This is four levels of hierarchy. The standard Go Test Explorer, even in projects that use stdlib tests, shows two: Package > Function. And for testify/suite projects, it effectively shows one — the single `func TestRunSuite(t *testing.T)` entry point that wraps the entire suite. Individual methods are invisible.

Discovery re-runs automatically when `_test.go` files change. The tree stays in sync with your code without manual refresh.

## Run and debug at any level

Once suites are discovered, CodeLens buttons appear inline above every suite type and test method in your `_test.go` files: **Run** and **Debug**. Click to execute immediately.

Package-level and file-level actions appear on the `package` declaration line. When a file contains multiple suites, a "Run File" action runs just the suites in that file.

You can also run from the Testing sidebar: click a suite to run all its methods, click a method to run just that one. Multi-select is fully supported — pick three methods across two suites and run them in one action.

After the first execution, subtests appear as children under their method node — the `When` and `It` blocks that gotest maps to `t.Run` calls. These are navigable and individually re-runnable. Select a specific `When` block and run just that context with one click; the extension builds the correct `-run` regex to target it.

Debugging uses [Delve](https://github.com/go-delve/delve). The extension generates the overlay, prepares the test binary with debug flags, and launches a debug session with the right test filter. Set breakpoints in your suite methods and step through them like any other Go code.

## The Spec View

This is where the extension goes beyond what standard test tooling does for any framework.

After each test run, the **Spec View** panel renders your test results as a behavioral specification — a hierarchical tree showing suites, methods, and their `When`/`It` blocks with pass/fail/skip indicators:

{{< spec title="Spec View" >}}
Cart
  AddItem
    when the cart is empty
      <span class="t-pass">✓</span> adds the item
      <span class="t-pass">✓</span> sets quantity to 1
    when the item already exists
      <span class="t-pass">✓</span> increments the quantity
  RemoveItem
    when the item exists
      <span class="t-pass">✓</span> removes it from the cart
    when the item does not exist
      <span class="t-fail">✗</span> returns ErrNotFound
{{< /spec >}}

The Spec View is an interactive panel, not static text. It supports:

- **Go-to-source** — click any suite or method to navigate to its definition in the source file.
- **Filtering** — toggle passed, failed, or skipped tests. When you're debugging, filter to failures only and see just the behaviors that broke.
- **Search** — find specific behaviors by name across the entire spec tree.
- **Expand/collapse** — control the tree depth. Collapse everything to see suites only, expand everything to see every leaf behavior.
- **Clipboard export** — copy the full spec report as structured text.
- **Live updates** — auto-refreshes from test runs, coverage runs, and watch mode cycles.
- **Persistence** — the panel survives editor reload and restores its last state.

The Spec View is what connects the editor experience to the [tests-as-documentation]({{< ref "/blog/tests-as-documentation" >}}) philosophy. Your tests define what the system does. The Spec View renders that definition as a living document, always in sync with the code.

## Coverage

The extension integrates with VS Code's native coverage API. Run tests with the Coverage profile in Test Explorer, and statement-level coverage gutters appear inline in your source files — green for covered, red for uncovered, with execution counts per statement.

Three things make this more useful than running coverage from the terminal:

- **Persistence.** Coverage data survives editor restart and accumulates across packages. Run coverage for `pkg/user` now, `pkg/order` later — both show gutters simultaneously. Source file edits automatically invalidate stale coverage for the affected package.
- **Coverage on save.** An opt-in setting (`gotest.coverOnSave`) re-runs package coverage every time you save a `.go` file. The gutters update continuously as you write code.
- **Clipboard export.** Copy a tabular coverage summary to the clipboard for pasting into PR descriptions, documentation, or AI conversations.

## Watch mode

Start continuous testing with the **Start Watch** command. The extension spawns a `gotest watch` process that monitors file changes and re-runs affected tests. Results stream into Test Explorer and Spec View in real-time.

A status bar item shows the active watcher count. Click it to stop all watchers. If the watch process crashes, it auto-restarts with exponential backoff.

Combined with the Spec View, this creates a tight feedback loop: save a file, watch re-runs the affected tests, the Spec View updates with pass/fail status, coverage gutters refresh. You see the behavioral impact of every change without leaving the editor or typing a command.

## Structured exports for AI-assisted workflows

The extension's export capabilities — spec output, test results, and coverage summaries — produce structured data that feeds directly into AI-assisted development workflows.

Copy the Spec View output and paste it into an AI conversation. The behavioral specification tells the model what the system does at the level stakeholders care about: "UserService / Create / when email is valid / creates the user." That's more useful context than raw source code for understanding system behavior.

Copy test results with status filtering — failures only, for instance — and paste them alongside the relevant source. The AI sees exactly which behaviors broke and what the assertion messages said, without wading through passing tests.

Copy the coverage summary and paste it into a conversation about test coverage gaps. The tabular format shows exactly which functions are covered and which aren't.

These aren't AI-specific features. They're structured export features that happen to compose well with language models because the data is behavioral, not mechanical. The tests describe what the system promises. The exports make those descriptions portable.

## Scaffold generation

Reducing the friction of creating new suites matters, especially for teams adopting gotest incrementally. The extension offers scaffold generation through multiple entry points:

- **Code action on a type declaration** — place your cursor on a type and use the Quick Fix menu to generate a test suite for it.
- **Code action on a Go file** — generate suites for all types in the file.
- **Command palette** — manual target entry for full control.

The generated file opens automatically, and discovery refreshes to include the new suite immediately.

## The trade-off, revisited

gotest's code generation architecture is a deliberate choice. It enables compile-time validation of lifecycle hooks, overlay-based injection that keeps your module clean, process-level suite isolation, and zero reflection overhead. The [Code Generation vs Reflection]({{< ref "/blog/code-generation-not-reflection" >}}) post explains why these properties matter.

The cost is that standard Go tooling doesn't see your suites. This is a real cost. If you open a gotest project in VS Code without the extension, your suites are invisible to the Testing sidebar, CodeLens doesn't appear, and you're running everything from the terminal.

The extension makes that cost disappear — and then goes further. Suite-aware discovery, behavioral Spec View, persistent coverage, watch integration, structured exports, scaffold generation. These aren't features that standard Go test tooling provides for stdlib tests either. They're possible because the extension is purpose-built for a framework whose structure is rich enough to support them.

That's the architectural story: gotest's code generation creates a gap between your source code and the Go toolchain. The extension bridges that gap — and because it understands suite structure natively, it builds a richer bridge than a generic test runner ever could. If you prefer the same feedback loop in the terminal, [The Inner Loop]({{< ref "/blog/the-inner-loop" >}}) covers `gotest watch` and `--spec` from the command line; and [Go Tests as Living Documentation]({{< ref "/blog/tests-as-documentation" >}}) shows what to do with the spec output once you have it.
