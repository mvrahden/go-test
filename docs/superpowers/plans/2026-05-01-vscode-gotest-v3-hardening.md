# vscode-gotest v3: Bug Fixes, Deduplication & Hardening — Implementation Plan

**Goal:** Fix all known bugs in the VS Code extension, eliminate ~300 lines of code duplication across runner modules, and add incremental discovery, graceful degradation, and a TypeScript test suite. This plan takes the extension from "feature-complete prototype" to "releasable quality."

**Branches:** All work targets the `vscode-extension` branch. Each phase is a logical unit that can be reviewed independently.

**Constraints:**
- No new features beyond what's listed — this is a hardening pass.
- Every bug fix must be verifiable by describing expected before/after behavior.
- Shared infrastructure extraction (Phase 2) must not change any observable behavior.

---

## Dependency Graph

```
Phase 1: Critical Bug Fixes (independent of each other)
  Task 1  Watch event.Package ──┐
  Task 2  Hardcoded dev path    │
  Task 3  buildRunFilter multi  │  (all independent, can be done in parallel)
  Task 4  Stale discovery cache │
                                │
Phase 2: Runner Deduplication ──┘ (depends on Phase 1 being merged)
  Task 5  Extract shared runner infra
      ├── Task 6  Watch error locations (uses shared applyResults)
      ├── Task 7  Test output streaming (uses shared applyResults)
      └── Task 8  Watch spec view per-cycle refresh

Phase 3: Polish (depends on Phase 2)
  Task 9   Dead code removal
  Task 10  CTS leaks & dispose cleanup
  Task 11  Discovery debounce re-queue
  Task 12  Controller↔Runner init
  Task 13  Build config fixes (enableScripts, esbuild target)
  Task 14  Graceful degradation on missing CLI

Phase 4: Quality & Features (depends on Phase 3)
  Task 15  Incremental per-package discovery
  Task 16  Extension test suite
  Task 17  Multi-root workspace support (stretch)
```

---

## Phase 1: Critical Bug Fixes

### Task 1: Fix watch mode test item resolution

**Problem:** `watch.ts:232` passes `pkgScope` (e.g., `./...`) as the import path to `applyWatchEvents`. This flows to `resolveTestItem` at line 333 which builds IDs like `./.../${suiteName}` — these never match controller items (which use real import paths like `github.com/mvrahden/go-test/internal/foo`). With the default `./...` scope, watch mode silently drops all test results.

**Root cause:** `applyWatchEvents` at line 325 accepts `importPath: string` but receives the user-supplied scope pattern. Each JSON event already contains the correct import path in `event.Package`, but the code ignores it.

**Fix:**

- [ ] **Step 1:** In `watch.ts`, modify `applyWatchEvents` to use `event.Package` instead of the outer `importPath` parameter:

```typescript
// watch.ts — applyWatchEvents method
private applyWatchEvents(
    run: vscode.TestRun,
    jsonLines: string,
): void {
    const events = parseTestEvents(jsonLines);
    for (const event of events) {
      if (!event.Test) {
        continue;
      }

      const item = this.resolveTestItem(event.Test, event.Package);
```

Remove the `importPath` parameter from the method signature entirely. Update the call site at line 232:

```typescript
this.applyWatchEvents(run, jsonLines);
```

- [ ] **Step 2: Verify** — Start watch mode with default `./...` scope. Confirm test pass/fail icons appear on the correct test items in Test Explorer after each watch cycle.

---

### Task 2: Remove hardcoded development path

**Problem:** `cli.ts:20-25` falls back to `/home/ubuntu/projects/mvrahden/go-test/bin/gotest` before trying `go run`. This absolute path breaks for every non-developer user.

**Fix:**

- [ ] **Step 1:** Remove the TODO block in `cli.ts` (lines 20-25):

```typescript
// DELETE these lines:
// TODO: remove once discover/overlay are published in a tagged release
const localBin = "/home/ubuntu/projects/mvrahden/go-test/bin/gotest";
try {
  await access(localBin);
  return { bin: localBin, args: subcommandArgs };
} catch {
  // fall through to go run
}
```

Also remove the unused `access` import from `node:fs/promises`.

- [ ] **Step 2: Verify** — With no `gotest.cliPath` setting and no local binary, confirm the extension falls through to `go run <module>@<version>` and discovery/overlay/run all work.

---

### Task 3: Fix `buildRunFilter` for multi-suite selection

**Problem:** `runner.ts:239-244` — when a depth-1 item (suite) is encountered in the loop, the method returns immediately with a filter for that single suite. If a user multi-selects 2+ suites in Test Explorer, only the first suite's filter is generated.

Similarly, `runner.ts:264-280` returns immediately for the first depth-3+ item (subtest), ignoring additional selected subtests.

**Fix:**

- [ ] **Step 1:** Restructure `buildRunFilter` in `runner.ts` to accumulate across all items before returning. The method should:
  1. First pass: if any item is depth 0 (package), return `undefined`.
  2. Second pass: if any suite has fixtures, return `undefined`.
  3. Group all items by suite name.
  4. For each suite group: if the group contains the suite itself (depth 1), add `^TestSuiteName$`. If it contains methods (depth 2), combine them. If it contains subtests (depth 3+), combine them.
  5. Join all suite-level filters with `|`.

```typescript
private buildRunFilter(
    items: vscode.TestItem[],
    importPath: string,
): string | undefined {
    // If any item is the package itself, run everything
    if (items.some((item) => this.getItemDepth(item) === 0)) {
      return undefined;
    }

    // Group by suite
    const suiteFilters = new Map<string, { wholeSuite: boolean; methods: string[]; subtests: string[] }>();

    for (const item of items) {
      const depth = this.getItemDepth(item);

      if (depth === 1) {
        const suiteName = item.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteFilters.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteFilters.set(suiteName, group);
        }
        group.wholeSuite = true;
      }

      if (depth === 2) {
        const suiteName = item.parent!.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteFilters.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteFilters.set(suiteName, group);
        }
        group.methods.push(item.label);
      }

      if (depth >= 3) {
        let current = item;
        const subtestParts: string[] = [];
        while (this.getItemDepth(current) > 2) {
          subtestParts.unshift(current.label);
          current = current.parent!;
        }
        const methodName = current.label;
        const suiteName = current.parent!.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteFilters.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteFilters.set(suiteName, group);
        }
        group.subtests.push(`^Test${suiteName}$/^${methodName}$/^${subtestParts.join("/")}$`);
      }
    }

    // Build combined filter
    const filters: string[] = [];
    for (const [suiteName, group] of suiteFilters) {
      if (group.wholeSuite) {
        filters.push(`^Test${suiteName}$`);
      } else if (group.subtests.length > 0) {
        filters.push(...group.subtests);
      } else if (group.methods.length === 1) {
        filters.push(`^Test${suiteName}$/^${group.methods[0]}$`);
      } else if (group.methods.length > 1) {
        filters.push(`^Test${suiteName}$/^(${group.methods.join("|")})$`);
      }
    }

    return filters.length === 0 ? undefined : filters.length === 1 ? filters[0] : filters.join("|");
}
```

- [ ] **Step 2:** Apply a similar structural fix to the simpler `buildRunFilter` in `coverage.ts:323-340` so it also handles multi-item selection correctly.

- [ ] **Step 3: Verify** — In Test Explorer, multi-select two different suites (Ctrl+Click), click Run. Confirm both suites execute. Repeat with two methods from different suites.

---

### Task 4: Fix stale discovery cache

**Problem:** `discovery.ts:31-34` — `DiscoveryCache.update()` merges new packages into the cache but never removes entries for packages that no longer exist. If a test file or package is deleted/renamed, the old entry persists until extension restart. The test controller at `testController.ts:61-95` inherits these stale entries.

**Fix:**

- [ ] **Step 1:** Change `update()` to accept a scope parameter indicating what was discovered, and remove stale entries within that scope:

```typescript
update(packages: DiscoverPackage[], scannedImportPaths?: Set<string>): void {
    if (scannedImportPaths) {
      // Remove packages that were in the scan scope but not in the results
      for (const key of this.cache.keys()) {
        if (scannedImportPaths.has(key) && !packages.some((p) => p.importPath === key)) {
          this.cache.delete(key);
        }
      }
    }
    for (const pkg of packages) {
      this.cache.set(pkg.importPath, pkg);
    }
    this._onDidUpdate.fire();
}
```

- [ ] **Step 2:** Update `DiscoveryService.discover()` to compute the scanned set. For full `./...` discovery, pass all returned import paths as the scope. For single-package discovery, pass just that package:

```typescript
// In discover(), after parsing output:
const resultPaths = new Set(output.packages.map((p) => p.importPath));
// For ./... discovery, any previously-cached path not in results is stale
if (patterns?.includes("./...") || !patterns) {
    // Full scan — all cache keys are in scope
    const allCached = new Set(this.cache.packages.map((p) => p.importPath));
    this.cache.update(output.packages, allCached);
} else {
    this.cache.update(output.packages, resultPaths);
}
```

- [ ] **Step 3: Verify** — Discover tests. Delete a `_test.go` file. Save another test file to trigger rediscovery. Confirm the deleted file's suites disappear from Test Explorer without restarting.

---

## Phase 2: Runner Deduplication & Watch Quality

### Task 5: Extract shared runner infrastructure

**Problem:** The following methods are duplicated across `runner.ts`, `coverage.ts`, and `watch.ts`:

| Method | runner.ts | coverage.ts | watch.ts | Lines each |
|--------|-----------|-------------|----------|------------|
| `resolveTestItem` | L415 | L439 | L366 | ~30 |
| `collectItems` | L155 | L277 | — | ~12 |
| `groupByPackage` | L172 | L291 | — | ~15 |
| `getRootItem` | L192 | L305 | — | ~6 |
| `getItemDepth` | L200 | L313 | — | ~8 |
| `applyResults` | L346 | L380 | L322* | ~40 |

*watch.ts has a simplified `applyWatchEvents` that lacks output collection and error locations.

**Fix:**

- [ ] **Step 1:** Create `vscode-gotest/src/runnerUtils.ts` with the shared functions:

```typescript
import * as vscode from "vscode";
import type { GoTestController } from "./testController.js";
import { parseTestEvents, extractTestMessages, type TestEvent } from "./outputParser.js";

export function collectItems(
    controller: GoTestController,
    request: vscode.TestRunRequest,
): vscode.TestItem[] { ... }

export function groupByPackage(
    items: vscode.TestItem[],
): Map<string, vscode.TestItem[]> { ... }

export function getRootItem(item: vscode.TestItem): vscode.TestItem { ... }

export function getItemDepth(item: vscode.TestItem): number { ... }

export function resolveTestItem(
    controller: GoTestController,
    testPath: string,
    importPath: string,
): vscode.TestItem | undefined { ... }

export function applyResults(
    controller: GoTestController,
    run: vscode.TestRun,
    events: TestEvent[],
    importPath: string,
    pkgDir: string,
): void { ... }

export function spawnTestProcess(
    bin: string,
    args: string[],
    cwd: string,
    token: vscode.CancellationToken,
    outputChannel: vscode.OutputChannel,
    label: string,
): Promise<string> { ... }
```

- [ ] **Step 2:** Refactor `runner.ts` to import and delegate to the shared functions. Remove the now-redundant private methods. Keep `buildRunFilter` in runner.ts (it has fixture-aware logic specific to the run profile).

- [ ] **Step 3:** Refactor `coverage.ts` to import and delegate to the shared functions. Remove the now-redundant private methods. Keep `buildRunFilter` in coverage.ts (it has simpler logic appropriate for coverage runs).

- [ ] **Step 4:** Refactor `watch.ts` to import `resolveTestItem` from the shared module.

- [ ] **Step 5: Verify** — Run all three profiles (Run, Debug, Coverage) and watch mode. Confirm identical behavior to before the refactor. No observable changes.

---

### Task 6: Add error locations to watch mode

**Problem:** `watch.ts:322-356` `applyWatchEvents` reports failures as `new vscode.TestMessage("Test failed")` with no file:line information. The regular runner's `applyResults` extracts `file:line:message` patterns via `extractTestMessages()` and attaches `Location` objects. Watch mode skips this entirely.

**Depends on:** Task 5 (shared `applyResults`).

**Fix:**

- [ ] **Step 1:** Replace the custom `applyWatchEvents` in `watch.ts` with the shared `applyResults` from `runnerUtils.ts`. This requires:
  - The watch manager needs access to `DiscoveryCache` to resolve `event.Package` → `pkgDir`.
  - Add `cache: DiscoveryCache` to `WatchManager` constructor.

- [ ] **Step 2:** Update `applyWatchEvents` to use the shared implementation:

```typescript
private applyWatchEvents(
    run: vscode.TestRun,
    jsonLines: string,
): void {
    const events = parseTestEvents(jsonLines);

    // Group events by package for pkgDir resolution
    const byPackage = new Map<string, TestEvent[]>();
    for (const event of events) {
      const pkg = event.Package;
      let group = byPackage.get(pkg);
      if (!group) {
        group = [];
        byPackage.set(pkg, group);
      }
      group.push(event);
    }

    for (const [importPath, pkgEvents] of byPackage) {
      const pkgDir = this.cache.resolveImportPath(importPath);
      if (pkgDir) {
        applyResults(this.controller, run, pkgEvents, importPath, pkgDir);
      }
    }
}
```

- [ ] **Step 3:** Update `extension.ts` to pass `cache` to `WatchManager` constructor.

- [ ] **Step 4: Verify** — Start watch mode. Introduce a test failure. Confirm the failure marker in Test Explorer shows the exact file:line location, not just "Test failed".

---

### Task 7: Stream test output to Test Explorer output panel

**Problem:** Neither `runner.ts` nor `coverage.ts` call `run.appendOutput()`. The VS Code Test Explorer output panel stays empty during test runs — users can't see test stdout/stderr.

**Depends on:** Task 5 (shared `applyResults` is the right place to add this).

**Fix:**

- [ ] **Step 1:** In the shared `applyResults` in `runnerUtils.ts`, add `run.appendOutput()` calls for `output` events:

```typescript
for (const event of events) {
    if (event.Action === "output" && event.Output) {
        // Stream output to Test Explorer panel
        // VS Code expects \r\n line endings in appendOutput
        const line = event.Output.replace(/\n$/, "\r\n");
        const testItem = event.Test
            ? resolveTestItem(controller, event.Test, importPath)
            : undefined;
        run.appendOutput(line, undefined, testItem);
    }
    // ... existing pass/fail/skip handling
}
```

- [ ] **Step 2: Verify** — Run a test that produces stdout output. Open the Test Explorer output panel (click the test result). Confirm the test's stdout appears.

---

### Task 8: Refresh spec view per watch cycle

**Problem:** `watch.ts:218` — the `onCycleStart` callback clears `cycleJsonAccumulator` without first flushing the previous cycle's data to `onCycleComplete` (which triggers spec view refresh). The spec view only updates when the watch process exits (`watch.ts:250-251`).

**Fix:**

- [ ] **Step 1:** In `watch.ts`, modify the `onCycleStart` callback to flush before clearing:

```typescript
// onCycleStart
() => {
    const existingRun = this.activeRuns.get(pkgScope);
    if (existingRun) {
      existingRun.end();
    }

    // Flush previous cycle to spec view before clearing
    if (cycleJsonAccumulator) {
      this.onCycleComplete(cycleJsonAccumulator);
    }
    cycleJsonAccumulator = "";

    const run = this.controller.createTestRun(
      new vscode.TestRunRequest(),
      `Watch: ${pkgScope}`,
    );
    this.activeRuns.set(pkgScope, run);
},
```

- [ ] **Step 2: Verify** — Open spec view (`Go Test: Show Spec View`). Start watch mode. Save a test file to trigger a cycle. Confirm the spec view updates with BDD-formatted output after each cycle, not just when stopping the watcher.

---

## Phase 3: Polish & Correctness

### Task 9: Remove dead code

**Problem:** Four items are defined but never used.

**Fix:**

- [ ] **Step 1:** Remove `FocusedItem` interface from `diagnostics.ts:5-10`:

```typescript
// DELETE:
interface FocusedItem {
  label: string;
  description: string;
  file: string;
  line: number;
}
```

Also remove unused imports of `DiscoverMethod` and `DiscoverSuite` from `diagnostics.ts:3` if they become unreferenced.

- [ ] **Step 2:** Remove `lastHtml` field from `specView.ts:79` and its assignment at line 126:

```typescript
// DELETE field declaration:
private lastHtml = "";

// DELETE assignment:
this.lastHtml = content;
```

- [ ] **Step 3:** Remove `clearDynamicSubtests` method from `testController.ts:165-174`.

- [ ] **Step 4:** Remove empty `disposables` array from `focusExclude.ts:10` and its iteration in `dispose()`:

```typescript
// DELETE:
private disposables: vscode.Disposable[] = [];

// Simplify dispose() to empty body:
dispose(): void {}
```

- [ ] **Step 5: Verify** — `npm run compile` succeeds with no errors.

---

### Task 10: Fix CancellationTokenSource leaks and dispose cleanup

**Problem:**
- `extension.ts:104,119` — `new CancellationTokenSource()` created without store or dispose.
- `runner.ts` `dispose()` doesn't cancel `activeRun`.
- `coverage.ts` `dispose()` is empty, doesn't cancel `activeRun`.

**Fix:**

- [ ] **Step 1:** Fix command handlers in `extension.ts` to properly manage CTS lifecycle:

```typescript
const runTestCmd = vscode.commands.registerCommand(
    "gotest.runTest",
    async (testId: string) => {
      const item = controller.findItem(testId);
      if (!item) { return; }
      const cts = new vscode.CancellationTokenSource();
      try {
        const request = new vscode.TestRunRequest([item]);
        await runner.run(request, cts.token);
      } finally {
        cts.dispose();
      }
    },
);
```

Apply the same pattern to `gotest.debugTest`.

- [ ] **Step 2:** Add `activeRun` cancellation to `runner.ts` `dispose()`:

```typescript
dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
    this._onDidComplete.dispose();
}
```

- [ ] **Step 3:** Add `activeRun` cancellation to `coverage.ts` `dispose()`:

```typescript
dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
}
```

- [ ] **Step 4: Verify** — Extension activates, runs tests, deactivates cleanly. No warnings in Developer Tools console about leaked disposables.

---

### Task 11: Add trailing-edge re-queue to discovery debounce

**Problem:** `discovery.ts:50-52` — if `discover()` is called while a discovery is already running, the new request is silently dropped. This can miss file changes that occur during an active scan.

**Fix:**

- [ ] **Step 1:** Add a `pending` field to `DiscoveryService` and re-run after completion:

```typescript
export class DiscoveryService {
    private running = false;
    private pending: { workspaceDir: string; patterns?: string[] } | undefined;

    async discover(workspaceDir: string, patterns?: string[]): Promise<void> {
        if (this.running) {
            this.pending = { workspaceDir, patterns };
            return;
        }

        this.running = true;
        try {
            // ... existing discovery logic ...
        } finally {
            this.running = false;
            if (this.pending) {
                const next = this.pending;
                this.pending = undefined;
                this.discover(next.workspaceDir, next.patterns);
            }
        }
    }
}
```

- [ ] **Step 2: Verify** — Save two test files in rapid succession. Confirm both changes are reflected in the test tree without needing a manual refresh.

---

### Task 12: Clean up controller-runner initialization

**Problem:** `extension.ts:28-43` uses closure-over-`let` to work around the circular dependency between `GoTestController` and `TestRunner`/`CoverageRunner`. This works but is fragile and non-obvious.

**Fix:**

- [ ] **Step 1:** Introduce a `RunnerRegistry` object that the controller receives, and which gets populated after construction:

```typescript
interface RunnerRegistry {
    run: (request: vscode.TestRunRequest, token: vscode.CancellationToken) => Promise<void>;
    debug: (request: vscode.TestRunRequest, token: vscode.CancellationToken) => Promise<void>;
    coverage: (request: vscode.TestRunRequest, token: vscode.CancellationToken) => Promise<void>;
}
```

- [ ] **Step 2:** Update `GoTestController` constructor to accept a `RunnerRegistry` object with late-bound methods, or accept the handlers as writable properties set after construction. Choose whichever approach requires the least change.

The simplest approach is to keep the closure pattern but document the temporal coupling with a clear structural comment. If the closure approach is kept, at minimum make the `let` declarations adjacent to their assignment and add an assertion:

```typescript
let runner!: TestRunner;
let coverageRunner!: CoverageRunner;

const controller = new GoTestController(/* ... */);

runner = new TestRunner(controller, cache, outputChannel);
coverageRunner = new CoverageRunner(controller, cache, coverageStore, outputChannel, (jsonOutput) => {
    specView.refresh(jsonOutput);
});
```

Using the definite assignment assertion (`!`) makes the temporal contract explicit.

- [ ] **Step 3: Verify** — Extension activates. All three run profiles work.

---

### Task 13: Fix build configuration issues

**Problem:**
- `specView.ts:93` — `enableScripts: true` but no scripts in the webview HTML. Unnecessary attack surface.
- `esbuild.config.mjs:10` — `target: "node22"` but VS Code `^1.95.0` bundles Electron 32 with Node 20. Risk of incompatible code generation.

**Fix:**

- [ ] **Step 1:** Remove `enableScripts` from webview options in `specView.ts`:

```typescript
this.panel = vscode.window.createWebviewPanel(
    "gotestSpecView",
    "Go Test: Spec View",
    vscode.ViewColumn.Beside,
    {},  // no options needed
);
```

- [ ] **Step 2:** Change esbuild target in `esbuild.config.mjs` to match VS Code's Node version:

```javascript
target: "node20",
```

- [ ] **Step 3: Verify** — `npm run compile` succeeds. Spec view renders correctly after a test run.

---

### Task 14: Add graceful degradation when CLI is unavailable

**Problem:** When `go` is not installed or `go run` fails (network issue, module not found), discovery fails silently — the error is only written to the output channel (`discovery.ts:68`). Users see zero tests with no explanation.

**Fix:**

- [ ] **Step 1:** Add a one-time warning notification on first discovery failure in `DiscoveryService`:

```typescript
export class DiscoveryService {
    private hasShownError = false;

    async discover(workspaceDir: string, patterns?: string[]): Promise<void> {
        // ...
        } catch (err: unknown) {
            const message = err instanceof Error ? err.message : String(err);
            this.outputChannel.appendLine(`[discovery] error: ${message}`);

            if (!this.hasShownError) {
                this.hasShownError = true;
                vscode.window.showWarningMessage(
                    `Go Test Suites: discovery failed. Ensure 'go' is installed and the gotest module is accessible. Error: ${message}`,
                    "Open Output",
                ).then((choice) => {
                    if (choice === "Open Output") {
                        this.outputChannel.show();
                    }
                });
            }
        }
    }
}
```

- [ ] **Step 2: Verify** — Temporarily set `gotest.modulePath` to a nonexistent module. Open a Go workspace. Confirm a warning notification appears once with an "Open Output" action.

---

## Phase 4: Quality & Features

### Task 15: Incremental per-package discovery

**Problem:** The file watcher in `extension.ts:195-204` triggers full `./...` discovery on every `_test.go` change. For large codebases, this is slow. The infrastructure for per-package discovery already exists (`discoverPackage()` at `discovery.ts:76`, `resolveFileToPackage()` at `discovery.ts:25`) but isn't wired up.

**Fix:**

- [ ] **Step 1:** Update the file watcher callback in `extension.ts` to accept the URI and use per-package discovery when possible:

```typescript
const onFileChange = (uri: vscode.Uri) => {
    const discoverOnSave =
        vscode.workspace.getConfiguration("gotest").get<boolean>("discoverOnSave") ?? true;
    if (!discoverOnSave) {
        return;
    }

    const importPath = cache.resolveFileToPackage(uri.fsPath);
    if (importPath) {
        discoveryService.discoverPackage(workspaceDir, importPath);
    } else {
        // Unknown package (new file, moved file) — fall back to full scan
        discoveryService.discover(workspaceDir);
    }
};
```

- [ ] **Step 2:** Also apply to `onDidCreate` — new test files in unknown directories need full discovery to be found, which the fallback handles.

- [ ] **Step 3:** For `onDidDelete` — a deleted test file might leave an empty package. Full discovery is appropriate here to clean up (works with Task 4's stale cache fix):

```typescript
const onFileDelete = (_uri: vscode.Uri) => {
    if (discoverOnSave) {
        discoveryService.discover(workspaceDir);
    }
};

const watcherChangeDisposable = watcher.onDidChange(onFileChange);
const watcherCreateDisposable = watcher.onDidCreate(onFileChange);
const watcherDeleteDisposable = watcher.onDidDelete(onFileDelete);
```

- [ ] **Step 4: Verify** — In a multi-package workspace, modify a test file. Observe (via output channel) that only `gotest discover ./path/to/package` is invoked, not `gotest discover ./...`.

---

### Task 16: Extension test suite

**Problem:** Zero TypeScript tests. The extension has four pure functions that are highly testable and critical to correctness.

**Fix:**

- [ ] **Step 1:** Add `vitest` as a dev dependency:

```bash
cd vscode-gotest && npm install -D vitest
```

Add a test script to `package.json`:

```json
"test": "vitest run"
```

- [ ] **Step 2:** Create `vscode-gotest/src/outputParser.test.ts` — test `parseTestEvents` and `extractTestMessages`:

Test cases for `parseTestEvents`:
- Parses valid JSON lines into TestEvent array
- Skips non-JSON lines (build output, blank lines)
- Handles all action types: run, pass, fail, skip, output, pause, cont
- Handles events with and without Test field
- Handles events with and without Elapsed

Test cases for `extractTestMessages`:
- Extracts `file.go:42: error message` patterns
- Prepends pkgDir to relative paths
- Preserves absolute paths unchanged
- Returns empty array for output with no file:line patterns
- Handles multiple messages in one output block

- [ ] **Step 3:** Create `vscode-gotest/src/specView.test.ts` — test `ansiToHtml`:

Test cases:
- Converts bold (`\x1b[1m`) to `<span class="ansi-bold">`
- Converts colors (31=red, 32=green, 33=yellow)
- Handles reset code (`\x1b[0m`) closing all open spans
- HTML-escapes content before processing ANSI
- Closes unclosed spans at end of string
- Passes through text with no ANSI codes unchanged

- [ ] **Step 4:** Create `vscode-gotest/src/coverage.test.ts` — test `parseCoverProfile`:

Test cases:
- Parses standard Go coverprofile format
- Skips `mode:` header line
- Skips blank lines
- Converts 1-based Go positions to 0-based VS Code positions
- Maps import paths to directories via moduleToDir callback
- Returns empty array when moduleToDir returns undefined
- Handles count=0 (uncovered) as `false` in StatementCoverage

Note: `parseCoverProfile` uses `vscode.FileCoverage` and `vscode.Range` which are VS Code API types. The tests will need to mock the `vscode` module. Create a minimal mock:

```typescript
// vscode-gotest/src/__mocks__/vscode.ts
export class Range {
    constructor(public startLine: number, public startCol: number,
                public endLine: number, public endCol: number) {}
}
export class Position {
    constructor(public line: number, public character: number) {}
}
export class Uri {
    static file(path: string) { return { fsPath: path }; }
}
export class StatementCoverage {
    constructor(public count: number | boolean, public range: Range) {}
}
export class FileCoverage {
    static fromDetails(uri: any, details: any[]) {
        return { uri, details, statementCoverage: { covered: 0, total: 0 } };
    }
}
```

- [ ] **Step 5: Verify** — `cd vscode-gotest && npm test` passes all tests.

---

### Task 17: Multi-root workspace support (stretch)

**Problem:** 7 usages of `workspaceFolders[0]` across the codebase. Multi-module Go projects using VS Code multi-root workspaces are unsupported.

**Scope:** This is a stretch goal. It requires significant architectural changes and should only be attempted if the previous 16 tasks are complete.

**Fix (high-level — detailed steps deferred):**

- [ ] **Step 1:** Change `DiscoveryCache` to partition by workspace folder (key = `folderUri + importPath`).

- [ ] **Step 2:** Change `DiscoveryService` to accept a workspace folder parameter and run discovery per-folder.

- [ ] **Step 3:** In `extension.ts`, iterate `workspaceFolders` during activation. Register per-folder watchers.

- [ ] **Step 4:** In `runner.ts`, `coverage.ts`, `debug.ts`, `scaffold.ts` — resolve the workspace folder from the test item's URI via `vscode.workspace.getWorkspaceFolder(item.uri)` instead of hardcoding `[0]`.

- [ ] **Step 5:** In `cli.ts` `qualifyModulePath` — read `go.mod` from the relevant workspace folder, not just the first one.

- [ ] **Step 6: Verify** — Open a multi-root workspace with two Go modules. Confirm both modules' tests appear in Test Explorer and can be run independently.

---

## Summary

| Phase | Tasks | Estimated scope | Key outcome |
|-------|-------|-----------------|-------------|
| 1 | Tasks 1-4 | ~150 lines changed | All known bugs fixed. Watch mode, multi-select, and cache staleness resolved |
| 2 | Tasks 5-8 | ~400 lines changed (net reduction ~200) | Runner duplication eliminated. Watch mode reaches parity with regular runner |
| 3 | Tasks 9-14 | ~100 lines changed | Dead code removed, resource leaks fixed, degradation UX added |
| 4 | Tasks 15-17 | ~200 lines changed | Incremental discovery, test suite, multi-root (stretch) |
