# vscode-gotest — VS Code Extension Design Spec

## Overview

A VS Code extension providing full Testing API integration for the go-test framework. The extension understands go-test's native conventions (suite structs, method-based test cases, BDD constructs, lifecycle hooks, focus/exclude prefixes, fixtures) without requiring any source modifications or alignment with other frameworks.

## Goals

- First-class Test Explorer and CodeLens support for go-test suites
- Debug integration using standard Go tooling (dlv)
- Zero impact on library DX — no entry-point functions, no testify mimicry
- Single source of truth for convention detection (Go AST via `gotestast`)
- Peaceful coexistence with the official Go extension

## Non-Goals

- Replacing or wrapping the official Go extension
- Supporting standard `func TestXxx` tests (the Go extension handles those)
- Custom debugger implementation

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  VS Code Extension (TypeScript)                     │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────┐  │
│  │TestController │  │CodeLensProvider│ │Diagnostics│ │
│  │  + Resolver   │  │              │  │          │  │
│  │  + Runner     │  │              │  │          │  │
│  └──────┬───────┘  └──────┬───────┘  └────┬─────┘  │
│         │                  │               │        │
│         └──────────┬───────┴───────────────┘        │
│                    │                                 │
│            ┌───────▼────────┐                        │
│            │  Discovery Cache│                        │
│            │  (in-memory)    │                        │
│            └───────┬────────┘                        │
└────────────────────┼────────────────────────────────┘
                     │ spawns
          ┌──────────▼──────────┐
          │   gotest CLI (Go)   │
          │                     │
          │  discover  overlay  │
          │  test      spec     │
          └─────────────────────┘
```

### Principles

- The extension is a thin UI layer; the Go CLI is the authority on conventions
- Discovery is read-only and frequent; overlay generation is on-demand
- Each component (TestController, CodeLens, Diagnostics) reads from a shared discovery cache
- The extension never parses Go source itself — it delegates to `gotest discover`

### Repository Location

Monorepo — lives inside the go-test repository (e.g., `vscode-gotest/` directory), developed in its own worktree.

### Coexistence with Go Extension

Full independence. The Go extension handles standard `func TestXxx(t *testing.T)` via gopls. This extension handles go-test suites via `gotest discover`. No overlapping test items exist because go-test suites have no `func TestXxx` in source.

---

## CLI Interface (New Subcommands)

### `gotest discover`

Outputs suite/method/fixture metadata as JSON to stdout.

```
gotest discover ./...
gotest discover ./pkg/specific
```

Output schema:

```json
{
  "packages": [{
    "importPath": "github.com/mvrahden/go-test/examples/simple_suite",
    "dir": "/absolute/path/to/simple_suite",
    "suites": [{
      "name": "SimpleTestSuite",
      "parallel": false,
      "focused": false,
      "excluded": false,
      "file": "ptest_test.go",
      "line": 9,
      "col": 6,
      "lifecycle": ["BeforeAll", "AfterAll", "BeforeEach", "AfterEach"],
      "fixtures": ["SetupFixture"],
      "methods": [{
        "name": "TestSucceeds",
        "parallel": false,
        "focused": false,
        "excluded": false,
        "file": "ptest_test.go",
        "line": 35,
        "col": 1
      }, {
        "name": "TestFails",
        "parallel": false,
        "focused": false,
        "excluded": false,
        "file": "ptest_test.go",
        "line": 42,
        "col": 1
      }]
    }]
  }]
}
```

Field semantics:
- `parallel`: true for `*TestSuiteParallel` suffixed suites, or `TestParallel*` prefixed methods
- `focused`: true when name has `F_` prefix
- `excluded`: true when name has `X_` prefix
- `lifecycle`: which lifecycle methods are implemented (subset of BeforeAll/AfterAll/BeforeEach/AfterEach)
- `fixtures`: names of embedded fixture types

### `gotest overlay`

Generates the overlay filesystem for a package and outputs the overlay file path.

```
gotest overlay ./pkg/specific
```

Output schema:

```json
{
  "overlayFile": "/tmp/gotest-overlay-abc123/overlay.json",
  "package": "github.com/mvrahden/go-test/examples/simple_suite"
}
```

The overlay file is in the format expected by `go test -overlay` / `go build -overlay`.

---

## v1 Features

### 1. Test Discovery & Tree Structure

#### When discovery runs

| Trigger | Scope | Behavior |
|---------|-------|----------|
| Workspace open | `./...` | Full discovery, populate entire tree |
| `_test.go` file saved | Changed package only | Re-discover that package, update tree |
| `_test.go` file created/deleted | Affected package | Re-discover that package |
| User clicks refresh | `./...` | Full re-discovery |

The extension watches `**/*_test.go` via VS Code's `FileSystemWatcher`. On change, it re-runs `gotest discover ./affected/pkg` (scoped, not full workspace) for responsiveness.

#### Test Explorer tree hierarchy

```
Workspace
└── Package (github.com/mvrahden/go-test/examples/simple_suite)
    └── SimpleTestSuite                          [suite]
        ├── TestSucceeds                         [method]
        │   └── (after run) When values match    [subtest, dynamic]
        │       └── It passes                    [subtest, dynamic]
        ├── TestFails                            [method]
        └── TestParallelConcurrency              [method, parallel]
```

#### Tree item metadata

- **Suite node**: package path, suite name, parallel flag, focused/excluded state
- **Method node**: suite reference, method name, parallel flag, focused/excluded state, source location (file + line)
- **Subtest node** (dynamic): full `-test.run` regex path for re-running

#### Visual indicators

- Focused (`F_`) items: tagged for filtering in Test Explorer
- Excluded (`X_`) items: rendered with skipped state (grayed out)
- Parallel items: tagged for filtering

#### Staleness handling

Discovery cache is invalidated per-package on file change. If `gotest discover` fails (e.g., syntax error in test file), the extension retains the last-known-good tree for that package and shows a diagnostic on the problematic file.

---

### 2. Test Execution & Result Mapping

#### Run profiles

| Profile | Kind | Execution |
|---------|------|-----------|
| Run | `Run` | `gotest test ./pkg -json -run <filter>` |
| Debug | `Debug` | `gotest overlay ./pkg` → `dlv test` with overlay flag |

#### Filter construction

| Selection | `-run` flag |
|-----------|------------|
| Entire suite | `^TestSimpleTestSuite$` |
| Single method | `^TestSimpleTestSuite$/^TestSucceeds$` |
| Multiple methods (same suite) | `^TestSimpleTestSuite$/^(TestSucceeds\|TestFails)$` |
| BDD subtest (dynamic) | `^TestSimpleTestSuite$/^TestSucceeds$/^When_values_match$/^It_passes$` |
| Entire package | No `-run` flag |

The generated code wraps each method in `t.Run("MethodName", ...)`, so standard `-run` regex filtering works through the full subtest path.

#### Parsing `go test -json` output

The extension reads stdout line-by-line. Each line is a JSON event:

```json
{"Time":"...","Action":"run","Package":"...","Test":"TestSimpleTestSuite/TestSucceeds"}
{"Time":"...","Action":"output","Package":"...","Test":"TestSimpleTestSuite/TestSucceeds","Output":"..."}
{"Time":"...","Action":"pass","Package":"...","Test":"TestSimpleTestSuite/TestSucceeds","Elapsed":0.001}
```

Mapping `Test` field → tree items:
- `TestSimpleTestSuite` → suite node
- `TestSimpleTestSuite/TestSucceeds` → method node
- `TestSimpleTestSuite/TestSucceeds/When_values_match/It_passes` → dynamic subtest node

#### Result states

| JSON `Action` | TestItem state |
|---------------|---------------|
| `run` | Started (spinner) |
| `pass` | Passed |
| `fail` | Failed |
| `skip` | Skipped |
| `output` containing file:line | Attached as TestMessage with source location |

#### Failure messages

When `Action` is `output` and the content contains a `file:line` pattern (e.g., `ptest_test.go:37: assertion failed`), the extension attaches it as a `TestMessage` with clickable source location in the Test Explorer.

---

### 3. Debug Integration

#### Flow

1. User clicks "Debug Test" on a test item
2. Extension calls `gotest overlay ./pkg`
3. Receives overlay file path
4. Extension launches dlv via VS Code's DAP with:
   ```json
   {
     "mode": "test",
     "program": "./path/to/pkg",
     "buildFlags": "-overlay=/tmp/gotest-overlay-xxx/overlay.json",
     "args": ["-test.run", "^TestSimpleTestSuite$/^TestSucceeds$"]
   }
   ```
5. Delve compiles test binary with overlay, launches debugger
6. Breakpoints in original `_test.go` source files hit normally

#### Why breakpoints work

The overlay adds generated wrapper files — it does not replace the user's source files. The user's suite methods are compiled from their original file paths. DWARF debug info points to the real files. Breakpoints in user-authored code work as expected.

#### Launch mechanism

Uses `vscode.debug.startDebugging()` with a dynamically constructed launch configuration. No `launch.json` entry required.

#### Cleanup

Overlay temp directory is cleaned up when:
- The debug session ends (`onDidTerminateDebugSession`)
- The extension deactivates

#### Known limitation

Breakpoints inside generated lifecycle wrapper code (the sequencing logic around `BeforeEach`/`AfterEach`) are not navigable since that code lives in the overlay temp directory. Breakpoints inside the user's own lifecycle method bodies work fine.

---

### 4. CodeLens

#### Placement

| Location | CodeLens text |
|----------|---------------|
| Suite type declaration | `▶ Run Suite` \| `Debug Suite` |
| Test method signature | `▶ Run` \| `Debug` |
| Lifecycle methods | None (not independently runnable) |
| Fixture types | None (not directly runnable) |

#### Source of truth

CodeLens positions come from `line`/`col` fields in `gotest discover` output. When the discovery cache is stale (file changed, discovery hasn't re-run), CodeLens may be briefly misaligned — self-corrects on next discovery cycle triggered by file save.

#### Commands

| CodeLens | Action |
|----------|--------|
| ▶ Run Suite | Execute all methods in suite via TestController |
| Debug Suite | Debug all methods in suite |
| ▶ Run | Execute single method |
| Debug | Debug single method |

---

### 5. Focus/Exclude Management

#### Code actions

Available via light bulb menu and context menu on suite types and test methods:

| Current state | Available actions |
|---------------|-------------------|
| `TestSucceeds` | "Focus this test" → `F_TestSucceeds` |
| `TestSucceeds` | "Exclude this test" → `X_TestSucceeds` |
| `F_TestSucceeds` | "Unfocus this test" → `TestSucceeds` |
| `X_TestSucceeds` | "Include this test" → `TestSucceeds` |
| `SimpleTestSuite` | "Focus this suite" → `F_SimpleTestSuite` |
| `F_SimpleTestSuite` | "Unfocus this suite" → `SimpleTestSuite` |

#### Implementation

**Methods:** Single text edit — rename the method identifier.

**Suites:** Multi-edit WorkspaceEdit — rename the struct type declaration and every method receiver that references it. The extension constructs the edit set from discovery cache (which lists all methods with their file positions).

---

### 6. BDD Subtest Discovery (Runtime)

#### Dynamic tree population

Before a test run, the tree shows only statically-discovered nodes (suites + methods). After a run, the extension creates child `TestItem` nodes from JSON output:

```
TestEqual                                    [static, from discover]
├── When values are deeply equal             [dynamic, from run]
│   ├── It passes for ints                   [dynamic]
│   └── It passes for structs                [dynamic]
└── When values differ                       [dynamic]
    └── It fails with diff                   [dynamic]
```

Display names use human-readable descriptions (spaces restored from underscores). The full `/`-separated test path is stored as metadata for re-running.

#### Lifecycle

- **Created:** When a `-json` event references a path deeper than suite/method
- **Retained:** Across runs within the same session (tree doesn't flicker)
- **Cleared:** When the parent method is re-run (fresh results replace stale subtree)
- **Lost:** On workspace reload (re-discovered on next run)

#### Re-running a specific subtest

Clicking "Run" on a dynamic node constructs the full `-run` regex through the path hierarchy:
```
-run ^TestSimpleTestSuite$/^TestEqual$/^When_values_are_deeply_equal$/^It_passes_for_ints$
```

---

### 7. Diagnostics

#### Focus prefix warnings

Any `F_`-prefixed identifier gets a warning diagnostic:
```
⚠ Focused test — will cause CI failure (gotest --ci)
```

- Severity: Warning
- Source: `gotest`
- Location: Identifier position from discovery cache
- Quick fix: "Remove focus" code action

#### Status bar indicator

When any `F_` prefix exists across the workspace:
```
⚠ gotest: 2 focused tests
```

Clicking reveals a quick-pick list of all focused items with navigation.

#### Error states

| Scenario | Behavior |
|----------|----------|
| `gotest discover` fails (syntax error) | Retain last-known tree; error diagnostic on file |
| `gotest` CLI not found | Activation error with install instructions |
| `gotest test` fails (compilation error) | Show output in test output channel |
| `gotest overlay` fails | Show error, fall back to Run profile |

---

## v2 Features (Deferred)

### 8. Spec View Panel

A dedicated VS Code panel rendering BDD-formatted test output — the behavioral specification view that `gotest spec` produces on the CLI.

#### Design

A WebView panel activated via command palette ("Go Test: Show Spec View") or a button in the Test Explorer toolbar. Renders the hierarchical behavioral specification after a test run.

#### Content structure

```
SimpleTestSuite
  ✓ TestSucceeds
    When values match
      ✓ It passes for ints (0.001s)
      ✓ It passes for structs (0.002s)
  ✗ TestFails
    When values differ
      ✗ It reports diff (0.001s)
          Expected: 42
          Actual:   43
```

#### Data source

Two options (to be decided during implementation):
- **A)** Run `gotest spec ./pkg` after a test run and render its output
- **B)** Parse the same `-json` events used for Test Explorer and render a BDD view in the panel

Option B avoids running tests twice and shares the result-mapping infrastructure.

#### Interaction

- Clicking a spec line navigates to the source location
- Collapsible tree nodes for suites and `When` groups
- Filter by pass/fail state
- Auto-updates after each test run

#### Persistence

The spec view content persists within a session. On workspace reload, it shows "Run tests to generate spec view."

---

### 9. Watch Mode Integration

Integration with `gotest watch` for continuous test execution on file save.

#### Design

A status bar item and command palette entry to toggle watch mode per package or workspace-wide.

#### Activation

- Command palette: "Go Test: Start Watch" → prompts for package scope
- Status bar: Shows `👁 gotest watch` when active, click to stop
- Context menu on a package in Test Explorer: "Watch this package"

#### Execution

The extension spawns `gotest watch ./pkg -json` as a long-running background process. It reads the JSON event stream continuously and updates Test Explorer results in real-time as files change and tests re-run.

#### Multiple watchers

Support watching multiple packages simultaneously. Each watcher is an independent process. The status bar shows the count of active watchers.

#### Lifecycle

- Watchers are killed on extension deactivation
- Watchers are killed when the workspace folder is removed
- A watcher for a specific package is restarted if `gotest watch` crashes
- User can stop all watchers via command palette

#### Relation to VS Code's auto-save

Watch mode is independent of VS Code's auto-save setting. `gotest watch` uses its own filesystem watcher (200ms debounce). The extension doesn't need to trigger re-runs on save — the CLI handles that.

---

### 10. Scaffold Command Integration

Integration with `gotest scaffold` for generating test suite skeletons from Go types.

#### Design

Available via command palette and code actions.

#### Command palette

"Go Test: Scaffold Suite" → prompts for:
1. Package path (defaults to current file's package)
2. Type name (offers autocomplete from exported types in the package)

Runs `gotest scaffold ./pkg.TypeName` and opens the generated file.

#### Code action

On a Go struct type declaration (non-test file), offer a code action:
```
💡 Scaffold test suite for MyService
```

This runs `gotest scaffold ./pkg.MyService` and opens the result.

#### Post-scaffold

After scaffolding, trigger a discovery refresh for the affected package so the new suite immediately appears in the Test Explorer.

---

### 11. Coverage Gutter Rendering

Display test coverage information inline in the editor gutter.

#### Design

After running tests with coverage (`gotest test -cover -coverprofile=...`), parse the coverage profile and render line-by-line coverage in the editor gutter.

#### Run profile

Add a third `TestRunProfile` with kind `Coverage`:
- Executes `gotest test ./pkg -json -coverprofile=/tmp/cover.out -run <filter>`
- Parses the coverage profile after test completion
- Renders coverage data via VS Code's `FileCoverage` API

#### Visual rendering

- Green gutter: line is covered
- Red gutter: line is not covered
- No gutter: line is not coverable (declarations, comments, blank lines)

#### Scope

Coverage is shown for the package under test, not for the test file itself. This matches `go test -cover` behavior.

#### Integration with Test Explorer

The Test Explorer shows coverage percentage next to package and suite nodes when coverage data is available.

---

### 12. Fixture Visualization

A panel or hover showing the fixture dependency graph and lifecycle execution order.

#### Design

A tree view or hover popover that shows:
- Which fixtures a suite embeds
- The lifecycle execution order (fixture.BeforeAll → suite.BeforeAll → fixture.BeforeEach → suite.BeforeEach → test → suite.AfterEach → fixture.AfterEach → ...)
- Shared fixture reuse across suites

#### Data source

`gotest discover` already outputs fixture information per suite. An extended version could output:
```json
{
  "fixtures": [{
    "name": "DatabaseFixture",
    "file": "fixtures_test.go",
    "line": 12,
    "lifecycle": ["BeforeAll", "AfterAll", "BeforeEach", "AfterEach"],
    "usedBy": ["UserTestSuite", "OrderTestSuite"]
  }]
}
```

#### Hover

When hovering over a fixture type name in a suite struct, show a tooltip:
```
DatabaseFixture
  Lifecycle: BeforeAll → BeforeEach → [test] → AfterEach → AfterAll
  Used by: UserTestSuite, OrderTestSuite
```

#### Tree view

A dedicated view in the Testing sidebar showing the fixture graph:
```
Fixtures
├── DatabaseFixture
│   ├── UserTestSuite
│   └── OrderTestSuite
└── CacheFixture
    └── PerformanceTestSuiteParallel
```

---

## Extension Configuration

### Settings

```json
{
  "gotest.cliPath": "gotest",
  "gotest.discoverOnSave": true,
  "gotest.showCodeLens": true,
  "gotest.showFocusWarnings": true,
  "gotest.testFlags": [],
  "gotest.buildFlags": []
}
```

| Setting | Default | Description |
|---------|---------|-------------|
| `gotest.cliPath` | `"gotest"` | Path to gotest binary (if not in PATH) |
| `gotest.discoverOnSave` | `true` | Re-run discovery on file save |
| `gotest.showCodeLens` | `true` | Show Run/Debug CodeLens above suites and methods |
| `gotest.showFocusWarnings` | `true` | Show diagnostics for F_ prefixes |
| `gotest.testFlags` | `[]` | Additional flags passed to `gotest test` |
| `gotest.buildFlags` | `[]` | Additional flags passed to `go build` / `dlv` |

---

## Extension Activation

The extension activates when:
- A workspace contains `go.mod` AND any `_test.go` file importing `github.com/mvrahden/go-test/pkg/gotest`
- Lazy activation: don't activate for every Go project, only for projects using go-test

Detection: on workspace open, check if any `_test.go` file contains the go-test import. This can be a fast grep-based check before running full discovery.

---

## Technology Stack

- **Language:** TypeScript
- **Bundler:** esbuild (standard for VS Code extensions)
- **VS Code API:** TestController, CodeLensProvider, DiagnosticCollection, FileSystemWatcher, DebugAPI
- **Testing:** VS Code extension testing framework (@vscode/test-electron)
- **Package manager:** npm or pnpm

---

## Phasing Summary

| Phase | Features | Milestone |
|-------|----------|-----------|
| v1.0 | Discovery, Test Explorer tree, CodeLens, Run, Result mapping | Usable daily driver |
| v1.1 | Debug integration, BDD subtests (dynamic), Focus/Exclude toggle | Feature-complete for core workflow |
| v1.2 | Diagnostics (focus warnings, error handling) | Production-ready |
| v2.0 | Spec view panel, Watch mode | Enhanced DX |
| v2.1 | Scaffold integration, Coverage gutters | Full toolchain integration |
| v2.2 | Fixture visualization | Advanced introspection |
