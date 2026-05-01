# vscode-gotest v2.0: Spec View Panel & Watch Mode

## Overview

Two features extending the vscode-gotest extension to provide continuous feedback during development: a behavioral specification panel rendering test results in BDD format, and watch mode integration for automatic re-execution on file changes.

## Goals

- Render the `gotest spec` behavioral view inside VS Code without re-running tests
- Stream watch mode results into Test Explorer in real-time
- Minimal CLI changes (one new flag each for `spec` and `watch`)
- Reuse existing extension infrastructure (runner, output parser, result mapping)

## Non-Goals

- Interactive spec view (clickable navigation, filtering) — deferred to v2.1
- Coverage integration with watch mode
- Custom watch debounce configuration from VS Code settings

---

## CLI Changes

### `gotest spec --input=-`

New flag: `--input` accepts a file path or `-` for stdin. When present, the command reads `go test -json` output from the specified source instead of running tests itself. It skips overlay generation and test execution, proceeding directly to event parsing, tree building, and rendering.

```
gotest spec --input=- --format=terminal < results.json
gotest spec --input=results.json --format=md
```

Behavior:
- `--input=-`: read from stdin
- `--input=<path>`: read from file
- Without `--input`: existing behavior (generate overlay, run tests, render)
- Compatible with all existing flags (`--format`, `--output`)

### `gotest watch -json`

New flag: `-json` on the watch subcommand. When present, outputs `go test -json` events to stdout instead of ANSI terminal rendering. Between test cycles, emits a sentinel event to delimit runs:

```json
{"Action":"watch-start","Package":"./pkg/gotest"}
```

Behavior:
- Suppresses terminal clearing (`clearTerminal()`) and ANSI status messages
- Each cycle starts with a `watch-start` sentinel containing the package patterns being tested
- Followed by standard `go test -json` event lines
- Cycle ends naturally when all pass/fail events for the package are emitted
- Errors are emitted as `{"Action":"watch-error","Output":"..."}` instead of writing to stderr

The existing non-`-json` terminal mode is unchanged.

---

## Feature 1: Spec View Panel

### Architecture

```
TestRunner (captures JSON) → specJsonCache (Map<pkgPath, string>)
                                    ↓ (on test run complete)
                            SpecViewPanel (webview)
                                    ↓ (pipes cached JSON)
                            gotest spec --input=- (child process)
                                    ↓ (ANSI stdout)
                            ansiToHtml() → webview.postMessage()
```

### Components

#### `specJsonCache` (in-memory store)

A `Map<string, string>` keyed by run identifier (timestamp or request ID). Stores the raw JSON stdout from the most recent test run. Overwritten on each run. Only the latest result is kept.

#### `SpecViewPanel` (webview)

- Singleton panel (only one instance)
- Created via command or toolbar button
- Receives ANSI-rendered HTML after each test run
- Shows placeholder text ("Run tests to generate spec view") when no results exist

#### ANSI-to-HTML conversion

Lightweight inline converter (~50 lines). Supports the subset used by `gotestspec/terminal.go`:
- `\033[0m` — reset
- `\033[1m` — bold
- `\033[2m` — dim
- `\033[31m` — red
- `\033[32m` — green
- `\033[33m` — yellow

No external dependency needed. Output wrapped in `<pre>` with a dark theme matching VS Code's terminal colors.

### Data Flow

1. `TestRunner.run()` completes and resolves with JSON stdout
2. Extension stores JSON in `specJsonCache`
3. If `SpecViewPanel` is visible, triggers refresh
4. Refresh spawns `gotest spec --input=- --format=terminal`, writes cached JSON to stdin
5. Captures ANSI stdout, converts to HTML
6. Posts HTML to webview via `panel.webview.postMessage()`
7. Webview renders inside `<pre class="spec-output">`

### Activation

- Command palette: "Go Test: Show Spec View"
- Test Explorer toolbar button (beaker/document icon)
- `package.json` contribution: `gotest.showSpecView` command

### Panel Lifecycle

- Panel created on first activation, reused on subsequent calls
- Auto-refreshes after every test run while visible
- On panel dispose (user closes it): stops refreshing, releases webview
- On reopen: shows last cached result immediately, then live-updates

### Webview Content

```html
<!DOCTYPE html>
<html>
<head>
  <style>
    body { background: var(--vscode-editor-background); color: var(--vscode-editor-foreground); margin: 0; padding: 16px; }
    .spec-output { font-family: var(--vscode-editor-font-family); font-size: var(--vscode-editor-font-size); white-space: pre-wrap; }
    .ansi-bold { font-weight: bold; }
    .ansi-dim { opacity: 0.6; }
    .ansi-red { color: var(--vscode-testing-iconFailed); }
    .ansi-green { color: var(--vscode-testing-iconPassed); }
    .ansi-yellow { color: var(--vscode-testing-iconSkipped); }
  </style>
</head>
<body>
  <pre class="spec-output" id="output">Run tests to generate spec view.</pre>
  <script>
    const vscode = acquireVsCodeApi();
    window.addEventListener('message', event => {
      document.getElementById('output').innerHTML = event.data.html;
    });
  </script>
</body>
</html>
```

### Configuration

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `gotest.specView.autoRefresh` | boolean | true | Auto-refresh spec view after test runs |

---

## Feature 2: Watch Mode

### Architecture

```
User activates watch → WatchManager.start(pkgScope)
                              ↓
                    spawn gotest watch -json ./pkg
                              ↓ (stdout line-by-line)
                    WatchEventParser (line → TestEvent)
                              ↓
                    on watch-start: create new TestRun
                    on pass/fail/skip: update TestItem results
                    on cycle end: end TestRun, refresh spec view
```

### Components

#### `WatchManager`

Manages the lifecycle of active watch processes.

Interface:
```typescript
class WatchManager implements vscode.Disposable {
  start(pkgScope: string, cwd: string): void;
  stop(pkgScope: string): void;
  stopAll(): void;
  isWatching(pkgScope: string): boolean;
  get activeCount(): number;
  readonly onDidChange: vscode.Event<void>;
}
```

Internals:
- `Map<string, WatchProcess>` keyed by package scope
- Starting a duplicate scope kills the old process first
- Emits `onDidChange` when a watcher starts, stops, or crashes

#### `WatchProcess`

Wraps a single `gotest watch -json` child process.

Responsibilities:
- Spawn process, pipe stdout
- Parse lines into events
- Detect `watch-start` sentinels to delimit cycles
- Auto-restart once on unexpected exit (2s backoff)
- Emit events to the WatchManager for result application
- Kill on dispose

#### `WatchStatusBar`

A `vscode.StatusBarItem` showing watch state:
- Hidden when no watchers active
- Shows `"$(eye) gotest watch (N)"` when N watchers active
- Click command: `gotest.stopWatch` (stops all if multiple, or the single one)

### Event Parsing

The watch output is a continuous stream of JSON lines. Parsing strategy:

1. Buffer stdout by newline
2. For each line, `JSON.parse()` into a `TestEvent`
3. If `event.Action === "watch-start"`: signal new cycle
4. If `event.Action === "watch-error"`: log to output channel
5. Otherwise: standard test event — delegate to result application

Cycle boundary detection:
- New cycle starts on `watch-start` sentinel
- Previous cycle implicitly ends when `watch-start` arrives
- On process exit without `watch-start`: treat as cycle end

### Result Application

Reuse the existing `TestRunner.applyResults()` logic. On each watch cycle:

1. `watch-start` received → create a new `TestRun` via `controller.createTestRun()`
2. Accumulate events for the cycle
3. On next `watch-start` or process idle (500ms without output after last event): end current `TestRun`
4. Apply results to TestItems using the same `resolveTestItem()` logic from `runner.ts`

### Activation

| Trigger | Behavior |
|---------|----------|
| Command: "Go Test: Start Watch" | Quick-pick for package scope, defaults to `./...` |
| Command: "Go Test: Stop Watch" | If 1 watcher: stops it. If multiple: quick-pick to choose or "Stop All" |
| Test Explorer context menu on package node | "Watch this package" / "Stop watching" |
| Status bar click | Same as "Stop Watch" command |

### Lifecycle

- Watchers are added to `context.subscriptions` for automatic cleanup on deactivation
- Process crash: auto-restart once with 2s delay. If it crashes again within 10s, show error notification and don't restart
- Workspace folder removed: kill watchers whose cwd was inside that folder
- Extension deactivation: kill all watchers (SIGTERM, then SIGKILL after 2s)

### Interaction with Existing Features

| Feature | Behavior |
|---------|----------|
| Discovery (file watcher) | Independent — discovery refreshes tree structure, watch refreshes results |
| Manual test runs | Run normally alongside watch. Watch results and manual results both update TestItems |
| Spec View | Watch cycles trigger spec view refresh (same path as manual runs) |
| Debug | Debug runs are independent of watch mode |

### Configuration

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `gotest.watch.autoRestart` | boolean | true | Auto-restart watcher on crash |
| `gotest.watch.scope` | string | `./...` | Default package scope for watch |

---

## Extension Configuration (additions to package.json)

### Commands

```json
[
  { "command": "gotest.showSpecView", "title": "Go Test: Show Spec View" },
  { "command": "gotest.startWatch", "title": "Go Test: Start Watch" },
  { "command": "gotest.stopWatch", "title": "Go Test: Stop Watch" }
]
```

### Menus

```json
{
  "view/title": [
    { "command": "gotest.showSpecView", "when": "view == testing", "group": "navigation" }
  ],
  "testing/item/context": [
    { "command": "gotest.startWatch", "when": "testId =~ /^[^/]+$/ && !gotest.watching", "group": "gotest" }
  ]
}
```

### Settings

```json
{
  "gotest.specView.autoRefresh": { "type": "boolean", "default": true },
  "gotest.watch.autoRestart": { "type": "boolean", "default": true },
  "gotest.watch.scope": { "type": "string", "default": "./..." }
}
```

---

## File Structure (new/modified)

### Go CLI
- Modify: `cmd/gotest/spec.go` — add `--input` flag handling
- Modify: `cmd/gotest/watch.go` — add `-json` flag, emit sentinels

### VS Code Extension
- Create: `src/specView.ts` — SpecViewPanel webview + ANSI converter
- Create: `src/watch.ts` — WatchManager, WatchProcess, WatchStatusBar
- Modify: `src/runner.ts` — expose JSON cache after runs
- Modify: `src/extension.ts` — wire new commands and components
- Modify: `package.json` — new commands, menus, settings

---

## Testing Strategy

### CLI
- `TestRunSpec_InputStdin`: pipe known JSON to `gotest spec --input=-`, verify ANSI output matches expected
- `TestRunWatch_JsonFlag`: start watch with `-json`, write a file, verify JSON events appear on stdout with sentinel

### Extension (unit)
- `ansiToHtml()`: verify conversion of all supported escape codes
- `WatchEventParser`: verify cycle boundary detection from event stream
- `specJsonCache`: verify store/retrieve/overwrite behavior

---

## Phasing

Both features can be implemented independently. Recommended order:

1. **Spec View** first — smaller scope, validates the `--input` CLI pattern
2. **Watch Mode** second — builds on the spec view (watch cycles refresh it)
