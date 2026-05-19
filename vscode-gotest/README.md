# Go - Test Suites

<p align="center">
  <img src="https://raw.githubusercontent.com/mvrahden/go-test/main/static/gopher.png" alt="gotest gopher" width="360" />
</p>

Specification-driven test suites for AI-assisted Go development.

[go-test](https://github.com/mvrahden/go-test) turns your tests into behavioral specifications — BDD-style suites that read as documentation, with structured output that AI agents consume natively.
This extension brings that workflow into VS Code: run, debug, watch, and verify your specifications without leaving the editor.

## Why this extension?

AI writes code fast. The bottleneck is verification — you can't review AI-generated code line by line at the rate it's produced.

[go-test](https://github.com/mvrahden/go-test) closes that gap. Test suites become formal behavioral specifications. Every CLI command produces structured JSON. The Spec View renders your specification as an interactive tree. Coverage shows implementation progress. You define what the system should do — AI implements, tests verify.

- **Spec View** — Your quality dashboard. BDD-formatted specification tree with pass/fail/skip indicators, go-to-source navigation, and clipboard export. Paste specs into AI conversations as context.
- **Suite-aware Test Explorer** — Navigate tests as Package > Suite > Method > Subtest, not a flat list of functions.
- **Coverage gutters** — Track implementation progress with native VS Code coverage integration, per-statement highlighting, and persistent results across sessions.
- **One-click Run and Debug** — CodeLens buttons above every suite and method. Debug with Delve, breakpoints included.
- **Watch mode** — Continuous verification on file changes with streaming results and a status bar indicator.
- **Focus/Exclude management** — Toggle `F_`/`X_` prefixes via Quick Fix actions. Diagnostic warnings prevent focused tests from reaching CI.
- **Scaffold generation** — Generate test suite skeletons from types and files via code actions or the command palette.

## Getting started

### Prerequisites

- **Go** (1.24+)
- **VS Code** (1.101+)
- **[Delve](https://github.com/go-delve/delve)** (for debugging only)

The extension invokes the gotest CLI automatically — no separate install needed.
It resolves the version from your `go.mod` and uses `go run` to execute it.

### Install

Search **"gotest"** in the Extensions panel, or install from the command line:

```
code --install-extension mvrahden.gotest
```

Also available on [Open VSX](https://open-vsx.org/extension/mvrahden/gotest) for VS Code forks.

### First run

1. Open a Go project that uses [go-test](https://github.com/mvrahden/go-test) suites.
2. The extension discovers test suites automatically on activation.
3. Open the **Testing** sidebar to see your suites organized by package.
4. Click the **Run** or **Debug** button next to any suite or method.

## Features

### Test Explorer

Tests appear in a structured tree: **Package > Suite > Method > Subtest**.
Run or debug at any level — a single method, an entire suite, or all tests in a package.
Multi-select is fully supported.
Test results persist across sessions, so you see pass/fail state immediately after reopening the editor.

### CodeLens

**Run** and **Debug** buttons appear inline above every suite and test method in `_test.go` files.
Click to execute immediately.

Package-level and file-level actions appear on the `package` declaration line:

- **Run Package** — run all suites in the package
- **Run File** — run all suites defined in the current file (shown when the file contains multiple suites)

### Coverage

Use the **Coverage** run profile in Test Explorer to run tests with `go test -coverprofile`.
Results appear as native VS Code coverage gutters.
Coverage data persists across sessions and accumulates across packages.
Source file edits automatically invalidate stale coverage for the affected package.

Copy a tabular coverage summary to the clipboard via the **Go Test: Copy Coverage Summary** command.

### Watch mode

Start continuous testing with **Go Test: Start Watch**.
The extension spawns a `gotest watch` process that re-runs tests on file changes.
Results stream into Test Explorer in real-time.
A status bar item shows active watcher count and lets you stop all watchers with a click.

### Spec View

After each test run, the **Spec View** panel renders BDD-formatted output with color-coded pass/fail/skip indicators.
Open it with **Go Test: Show Spec View**.

- **Go-to-source** — click any suite or method to navigate to its definition
- **Toolbar** — expand/collapse all, filter by pass/fail/skip status, search behaviors by name
- **Copy / Clear** — copy the full spec report to clipboard or clear results
- **Persistence** — the panel survives editor reload and restores its last state
- **Live updates** — auto-refreshes from test runs, coverage runs, and watch mode

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

Fully supported.
Each workspace folder is discovered independently.
Commands resolve the correct workspace folder from the active editor, and file watchers trigger per-folder discovery.
Per-project settings like `cliPath`, `testFlags`, and `buildTags` can be configured per folder via `.vscode/settings.json`.
Projects using `go.work` are also supported.

## Commands

| Command | Description |
|---------|-------------|
| Go Test: Run | Run a specific test by ID |
| Go Test: Run File | Run all suites in the current file |
| Go Test: Debug | Debug a specific test by ID |
| Go Test: Refresh | Re-run test discovery for all workspace folders |
| Go Test: Show Focused Tests | List all focused tests and navigate to them |
| Go Test: Show Spec View | Open the BDD spec output panel |
| Go Test: Start Watch | Start continuous testing for a package scope |
| Go Test: Stop Watch | Stop all active watch processes |
| Go Test: Scaffold Suite | Generate a test suite from a target |
| Go Test: Scaffold Target | Generate a test suite for a specific target |
| Go Test: Copy Coverage Summary | Copy coverage table to clipboard |
| Go Test: Copy Test Results | Copy test results to clipboard (also available as context menu on test items) |

## Settings

### Per-project settings (resource scope)

These can be set in `.vscode/settings.json` per workspace folder:

| Setting | Default | Description |
|---------|---------|-------------|
| `gotest.cliPath` | `""` | Path to a gotest binary (overrides all other resolution) |
| `gotest.modulePath` | `github.com/mvrahden/go-test/cmd/gotest` | Go module path for the gotest CLI |
| `gotest.buildTags` | `""` | Comma-separated Go build tags (e.g. `integration,e2e`) |
| `gotest.testFlags` | `[]` | Additional flags passed to `go test` |
| `gotest.buildFlags` | `[]` | Additional build flags passed to Delve |
| `gotest.discoverOnSave` | `true` | Re-discover tests when `_test.go` files change |
| `gotest.coverOnRun` | `true` | Collect coverage data alongside normal test runs |
| `gotest.coverOnSave` | `false` | Re-run package coverage when a `.go` file is saved |
| `gotest.coverTestOnlyPackages` | `false` | Enable cross-package coverage instrumentation for test-only packages |
| `gotest.debug.prepareTimeout` | `60` | Seconds to wait for debug preparation before timing out |
| `gotest.watch.scope` | `./...` | Default package scope for watch mode |

### Global settings (window scope)

| Setting | Default | Description |
|---------|---------|-------------|
| `gotest.showCodeLens` | `true` | Show Run/Debug CodeLens above suites and methods |
| `gotest.showFocusWarnings` | `true` | Show diagnostics for `F_` prefixed (focused) tests |
| `gotest.specView.autoRefresh` | `true` | Auto-refresh Spec View after test runs |
| `gotest.watch.autoRestart` | `true` | Auto-restart watch process on crash |

### Example `.vscode/settings.json`

```jsonc
{
  // Use a local gotest binary instead of go run
  "gotest.cliPath": "./bin/gotest",

  // Pass build tags to discovery and test runs
  "gotest.buildTags": "integration,e2e",

  // Extra flags for go test (e.g. timeout, verbose)
  "gotest.testFlags": ["-timeout=120s", "-v"],

  // Extra build flags for Delve debug sessions
  "gotest.buildFlags": ["-gcflags=all=-N -l"]
}
```

### Gotest CLI resolution

The extension resolves the gotest CLI in this order:

1. **`gotest.cliPath`** — Explicit path to a binary. Highest priority; version-validated against the minimum required version.
2. **Workspace is gotest module** — If the workspace's `go.mod` declares the gotest module itself (development or `go.work` overlap), uses `go run ./cmd/gotest`.
3. **`go.mod` + replace directive** — If `go.mod` has a `replace` directive for the gotest module, uses `go run modulePath` (no version, respects replace resolution).
4. **`go.mod` pinned version** — If `go.mod` references the gotest module, uses `go run modulePath@version` with the pinned version.
5. **`go run @latest`** — Fallback when none of the above apply.

### Go binary resolution

The extension resolves the Go toolchain per workspace folder:

1. **`go.mod` go directive** — If `go.mod` declares `go 1.26.2`, the extension looks for `~/sdk/go1.26.2/bin/go` or `go1.26.2` on PATH.
2. **`GOROOT`** — `$GOROOT/bin/go` if set.
3. **Login shell** — Runs `bash -lc 'command -v go'` to find Go on the user's full PATH.
4. **Common paths** — `/usr/local/go/bin/go`, `~/go/bin/go`, `/usr/bin/go`, `~/sdk/go*/bin/go`.

## Requirements

- VS Code 1.101 or later
- Go 1.24 or later
- A Go project using [go-test](https://github.com/mvrahden/go-test) suites
- [Delve](https://github.com/go-delve/delve) for debug support

## License

MIT
