# Go Test Suites

First-class VS Code integration for [go-test](https://github.com/mvrahden/go-test) — suite-based, BDD-style testing for Go.

Run, debug, and watch your test suites directly from the editor. See structured results in Test Explorer, get coverage gutters, and scaffold new suites without leaving VS Code.

## Why this extension?

Go's standard `testing` package gives you `func TestX(t *testing.T)` and nothing more. [go-test](https://github.com/mvrahden/go-test) adds suite-based organization with lifecycle hooks, fixtures, focus/exclude, and BDD-style output — all through naming conventions, zero runtime dependencies.

This extension makes that workflow native to VS Code:

- **Suite-aware Test Explorer** — Navigate tests as Package > Suite > Method > Subtest, not a flat list of functions.
- **One-click Run and Debug** — CodeLens buttons above every suite and method. Debug with Delve, breakpoints included.
- **Coverage gutters** — Native VS Code coverage integration with per-statement highlighting and persistent results across sessions.
- **Watch mode** — Continuous testing on file changes with streaming results and a status bar indicator.
- **Spec View** — BDD-formatted output rendered in a side panel after every test run.
- **Focus/Exclude management** — Toggle `F_`/`X_` prefixes via Quick Fix actions. Diagnostic warnings prevent focused tests from reaching CI.
- **Scaffold generation** — Generate test suite skeletons from types and files via code actions or the command palette.

## Getting started

### Prerequisites

- **Go** (1.21+)
- **VS Code** (1.101+)
- **[Delve](https://github.com/go-delve/delve)** (for debugging only)

The extension uses `go run` to invoke the gotest CLI automatically — no separate install needed. It resolves the version from your `go.mod`.

### Install

Search **"Go Test Suites"** in the VS Code Extensions panel, or install from the command line:

```
code --install-extension mvrahden.vscode-gotest
```

### First run

1. Open a Go project that uses [go-test](https://github.com/mvrahden/go-test) suites.
2. The extension discovers test suites automatically on activation.
3. Open the **Testing** sidebar to see your suites organized by package.
4. Click the **Run** or **Debug** button next to any suite or method.

## Features

### Test Explorer

Tests appear in a structured tree: **Package > Suite > Method > Subtest**. Run or debug at any level — a single method, an entire suite, or all tests in a package. Multi-select is fully supported.

### CodeLens

**Run** and **Debug** buttons appear inline above every suite struct and test method in `_test.go` files. Click to execute immediately.

### Coverage

Use the **Coverage** run profile in Test Explorer to run tests with `go test -coverprofile`. Results appear as native VS Code coverage gutters. Coverage data persists across sessions and accumulates across packages. Source file edits automatically invalidate stale coverage for the affected package.

Copy a tabular coverage summary to the clipboard via the **Go Test: Copy Coverage Summary** command.

### Watch mode

Start continuous testing with **Go Test: Start Watch**. The extension spawns a `gotest watch` process that re-runs tests on file changes. Results stream into Test Explorer in real-time. A status bar item shows active watcher count and lets you stop all watchers with a click.

### Spec View

After each test run, the **Spec View** panel renders BDD-formatted output with color-coded pass/fail/skip indicators. Open it with **Go Test: Show Spec View**.

### Focus and Exclude

Place your cursor on a suite or method definition and use the **Quick Fix** menu (Ctrl+. / Cmd+.) to:

- **Focus** a test (`F_` prefix) — only focused tests run
- **Exclude** a test (`X_` prefix) — test is skipped
- **Unfocus / Include** — remove the prefix

A status bar warning and inline diagnostics alert you when focused tests exist, preventing CI failures from `gotest --ci`.

### Scaffold

Generate test suite skeletons from existing code:

- **Code action on a type declaration** — "Generate test suite for TypeName"
- **Code action on any Go file** — "Generate test suite for this file"
- **Command palette** — "Go Test: Scaffold Suite" for manual target entry

The generated file opens automatically and discovery refreshes.

### Multi-root workspaces

Fully supported. Each workspace folder is discovered independently. Commands resolve the correct workspace folder from the active editor, and file watchers trigger per-folder discovery.

## Commands

| Command | Description |
|---------|-------------|
| Go Test: Run | Run a specific test by ID |
| Go Test: Debug | Debug a specific test by ID |
| Go Test: Refresh | Re-run test discovery for all workspace folders |
| Go Test: Show Focused Tests | List all focused tests and navigate to them |
| Go Test: Show Spec View | Open the BDD spec output panel |
| Go Test: Start Watch | Start continuous testing for a package scope |
| Go Test: Stop Watch | Stop all active watch processes |
| Go Test: Scaffold Suite | Generate a test suite from a target |
| Go Test: Copy Coverage Summary | Copy coverage table to clipboard |

## Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `gotest.modulePath` | `github.com/mvrahden/go-test/cmd/gotest` | Go module path for the gotest CLI |
| `gotest.cliPath` | `""` | Path to a pre-installed gotest binary (bypasses `go run`) |
| `gotest.discoverOnSave` | `true` | Re-discover tests when `_test.go` files change |
| `gotest.showCodeLens` | `true` | Show Run/Debug CodeLens above suites and methods |
| `gotest.showFocusWarnings` | `true` | Show diagnostics for `F_` prefixed (focused) tests |
| `gotest.testFlags` | `[]` | Additional flags passed to `go test` |
| `gotest.buildFlags` | `[]` | Additional build flags passed to Delve |
| `gotest.specView.autoRefresh` | `true` | Auto-refresh Spec View after test runs |
| `gotest.watch.autoRestart` | `true` | Auto-restart watch process on crash |
| `gotest.watch.scope` | `./...` | Default package scope for watch mode |

## Requirements

- VS Code 1.101 or later
- Go 1.21 or later
- A Go project using [go-test](https://github.com/mvrahden/go-test) suites
- [Delve](https://github.com/go-delve/delve) for debug support

## License

MIT
