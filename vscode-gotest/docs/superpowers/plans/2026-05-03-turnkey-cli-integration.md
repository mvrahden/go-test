# Turn-Key CLI Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the extension to delegate test execution, coverage, and debug preparation to the `gotest` CLI as single turn-key commands, eliminating the 3-subprocess orchestration.

**Architecture:** The CLI default mode already handles overlay + shared fixtures + go test internally. The extension will call the CLI directly for run/coverage, and a new `prepare` subcommand for debug. The `overlay` and `shared-setup` subcommands are dropped as they had no other consumer.

**Tech Stack:** Go (CLI), TypeScript/VSCode Extension API (extension), Vitest (extension tests)

---

## File Structure

### CLI (Go)
| File | Responsibility |
|------|---------------|
| `internal/gotestrunner/stdlib.go` | Context-aware `StdlibRunTests` |
| `internal/gotestrunner/json.go` | Context-aware `StdlibRunTestsJSON` |
| `cmd/gotest/exec.go` | Default-mode `Run()`, passes ctx to runner |
| `cmd/gotest/watch.go` | Watch mode with shared fixture support |
| `cmd/gotest/prepare.go` | New `prepare` subcommand |
| `cmd/gotest/cli.go` | Dispatch, help text |
| `cmd/gotest/args.go` | Subcommand registry |
| `cmd/gotest/overlay_test.go` → `generate_test.go` | Tests for `generateOverlay` internals |

### Extension (TypeScript)
| File | Responsibility |
|------|---------------|
| `src/runnerUtils.ts` | `spawnTestProcess` with structured result |
| `src/types.ts` | `PrepareOutput` (replaces `OverlayOutput`, `SharedFixtureInfo`) |
| `src/runner.ts` | Simplified single-CLI-spawn test runner |
| `src/coverage.ts` | Simplified single-CLI-spawn coverage runner |
| `src/debug.ts` | `gotest prepare` based debug launcher |
| `src/sharedFixtures.ts` | **Deleted** |

---

### Task 1: Context-aware `StdlibRunTests` and `StdlibRunTestsJSON`

**Files:**
- Modify: `internal/gotestrunner/stdlib.go`
- Modify: `internal/gotestrunner/json.go`
- Modify: `cmd/gotest/exec.go:69-131`
- Modify: `cmd/gotest/watch.go:110-165`

- [ ] **Step 1: Add context parameter to `StdlibRunTests`**

Replace the entire file `internal/gotestrunner/stdlib.go`:

```go
package gotestrunner

import (
	"context"
	"os"
	"os/exec"
)

func StdlibRunTests(ctx context.Context, args []string, extraEnv ...map[string]string) (int, error) {
	cmd := exec.CommandContext(ctx, "go", append([]string{"test"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if len(extraEnv) > 0 && len(extraEnv[0]) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv[0] {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	err := cmd.Run()
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return 2, err
	}
	return 0, nil
}
```

- [ ] **Step 2: Add context parameter to `StdlibRunTestsJSON`**

Replace the entire file `internal/gotestrunner/json.go`:

```go
package gotestrunner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

func StdlibRunTestsJSON(ctx context.Context, args []string, extraEnv ...map[string]string) ([]byte, int, error) {
	jsonArgs := make([]string, 0, len(args)+2)
	jsonArgs = append(jsonArgs, "test", "-json")
	jsonArgs = append(jsonArgs, args...)

	cmd := exec.CommandContext(ctx, "go", jsonArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if len(extraEnv) > 0 && len(extraEnv[0]) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv[0] {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	err := cmd.Run()
	if cmd.ProcessState != nil {
		return stdout.Bytes(), cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return nil, 2, err
	}
	return stdout.Bytes(), 0, nil
}
```

- [ ] **Step 3: Update `exec.go:Run()` to pass context**

In `cmd/gotest/exec.go`, the `Run` function already creates a context at line 85-86. Update the two call sites:

In the `if SPEC` branch (around line 122), `runWithSpec` already receives the args — update `runWithSpec` to accept `ctx`:

Change the `runWithSpec` signature and both calls inside `exec.go`:

```go
func Run(cfg ExecConfig) int {
	// ... existing code through line 121 ...

	if SPEC {
		return runWithSpec(ctx, goTestArgs, extraEnv)
	}

	code, err := gotestrunner.StdlibRunTests(ctx, goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}

func runWithSpec(ctx context.Context, goTestArgs []string, extraEnv map[string]string) int {
	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(ctx, goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// ... rest unchanged ...
```

- [ ] **Step 4: Update `watch.go:watchRunOnce()` to pass context**

`watchRunOnce` does not currently have a `ctx` parameter. Add one and pass it to `StdlibRunTests`/`StdlibRunTestsJSON`/`runWithSpec`. Also update the two callers in `runWatch` to pass `ctx`.

Change the signature of `watchRunOnce`:

```go
func watchRunOnce(ctx context.Context, goTestArgs []string, patterns []string, jsonMode bool) int {
```

Update the `StdlibRunTestsJSON` call (around line 146):

```go
		output, code, err := gotestrunner.StdlibRunTestsJSON(ctx, overlayArgs, extraEnv)
```

Update the `runWithSpec` call (around line 155):

```go
		return runWithSpec(ctx, overlayArgs, extraEnv)
```

Update the `StdlibRunTests` call (around line 159):

```go
	code, err := gotestrunner.StdlibRunTests(ctx, overlayArgs, extraEnv)
```

Update the two callers in `runWatch` (lines 46 and 95):

```go
	watchRunOnce(ctx, goTestArgs, patterns, jsonMode)
	// ...
			watchRunOnce(ctx, pkgArgs, pkgPatterns, jsonMode)
```

- [ ] **Step 5: Verify Go code compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go build ./cmd/gotest/...`
Expected: clean build, no errors.

- [ ] **Step 6: Run existing Go tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go test ./cmd/gotest/... -count=1`
Expected: all existing tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/gotestrunner/stdlib.go internal/gotestrunner/json.go cmd/gotest/exec.go cmd/gotest/watch.go
git commit -m "refactor(cli): add context parameter to StdlibRunTests and StdlibRunTestsJSON"
```

---

### Task 2: Fix `watchRunOnce()` shared fixture gap

**Files:**
- Modify: `cmd/gotest/watch.go:110-165`

- [ ] **Step 1: Wire shared fixtures into `watchRunOnce`**

After the `generateOverlay` call and before the test execution section, add shared fixture startup. The entire `watchRunOnce` function should become:

```go
func watchRunOnce(ctx context.Context, goTestArgs []string, patterns []string, jsonMode bool) int {
	if CI {
		violations, err := RunFocusGuard(patterns)
		if err != nil {
			if jsonMode {
				fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			}
			return 2
		}
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	overlay, cleanup, err := generateOverlay(patterns)
	if err != nil {
		if jsonMode {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		}
		return 2
	}
	defer cleanup()

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures)
		if err != nil {
			if jsonMode {
				fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			}
			return 2
		}
		defer setupProc.Teardown()
	}

	overlayArgs := append([]string{overlay.overlayFlag}, goTestArgs...)
	extraEnv := buildExtraEnv()
	if setupProc != nil {
		extraEnv["GOTEST_SHARED_STATE_FILE"] = setupProc.StateFile()
	}

	if jsonMode {
		fmt.Printf("{\"Action\":\"watch-start\",\"Package\":%q}\n", strings.Join(patterns, ","))
		output, code, err := gotestrunner.StdlibRunTestsJSON(ctx, overlayArgs, extraEnv)
		if err != nil {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			return 2
		}
		os.Stdout.Write(output)
		return code
	}

	if SPEC {
		return runWithSpec(ctx, overlayArgs, extraEnv)
	}

	code, err := gotestrunner.StdlibRunTests(ctx, overlayArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}
```

- [ ] **Step 2: Verify Go code compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go build ./cmd/gotest/...`
Expected: clean build.

- [ ] **Step 3: Run existing Go tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go test ./cmd/gotest/... -count=1`
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/gotest/watch.go
git commit -m "fix(cli): wire shared fixtures into watchRunOnce"
```

---

### Task 3: New `prepare` subcommand + drop `overlay`/`shared-setup`

**Files:**
- Create: `cmd/gotest/prepare.go`
- Delete: `cmd/gotest/overlay.go`
- Delete: `cmd/gotest/sharedsetup.go`
- Modify: `cmd/gotest/args.go`
- Modify: `cmd/gotest/cli.go`
- Rename: `cmd/gotest/overlay_test.go` → `cmd/gotest/generate_test.go`

- [ ] **Step 1: Create `cmd/gotest/prepare.go`**

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

type prepareOutput struct {
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
	StateFile   string `json:"stateFile,omitempty"`
}

func runPrepare(args []string) int {
	patterns := ExtractPackagePatterns(args)

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	overlay, cleanup, err := generateOverlay(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures)
		if err != nil {
			cleanup()
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
	}

	out := prepareOutput{
		OverlayFile: filepath.Join(overlay.tmpDir, "overlay.json"),
		Dir:         overlay.tmpDir,
	}
	if setupProc != nil {
		out.StateFile = setupProc.StateFile()
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		if setupProc != nil {
			setupProc.Teardown()
		}
		cleanup()
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	if setupProc != nil {
		setupProc.Teardown()
	}
	cleanup()
	return 0
}
```

- [ ] **Step 2: Delete `cmd/gotest/overlay.go`**

```bash
rm cmd/gotest/overlay.go
```

- [ ] **Step 3: Delete `cmd/gotest/sharedsetup.go`**

```bash
rm cmd/gotest/sharedsetup.go
```

- [ ] **Step 4: Update `cmd/gotest/args.go` — remove overlay/shared-setup, add prepare**

Replace the `knownSubcommands` map:

```go
var knownSubcommands = map[string]bool{
	"discover": true,
	"prepare":  true,
	"generate": true,
	"scaffold": true,
	"migrate":  true,
	"spec":     true,
	"watch":    true,
	"clean":    true,
	"version":  true,
	"help":     true,
}
```

- [ ] **Step 5: Update `cmd/gotest/cli.go` — remove overlay/shared-setup cases, add prepare**

In the `main()` switch statement, remove the `overlay` and `shared-setup` cases and add `prepare`:

```go
	switch subcmd {
	case "discover":
		os.Exit(runDiscover(remaining))
	case "prepare":
		os.Exit(runPrepare(remaining))
	case "scaffold":
		os.Exit(runScaffold(remaining))
	case "migrate":
		os.Exit(runMigrate(remaining))
	case "generate":
		os.Exit(runGenerate(remaining))
	case "clean":
		os.Exit(runClean(remaining))
	case "spec":
		os.Exit(runSpec(remaining))
	case "watch":
		os.Exit(runWatch(remaining))
	case "version":
		fmt.Println(about.LongInfo())
		return
	case "help":
		printUsage()
		return
	default:
```

- [ ] **Step 6: Rename and adjust `overlay_test.go` → `generate_test.go`**

```bash
mv cmd/gotest/overlay_test.go cmd/gotest/generate_test.go
```

Then edit `cmd/gotest/generate_test.go`. The first test uses `overlayOutput` struct (now deleted with `overlay.go`). Replace it with an inline struct. The second test doesn't reference `overlayOutput` so it only needs no changes.

Replace the first test function body to remove `overlayOutput` usage:

```go
func TestGenerateOverlay_ProducesValidOutput(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
		t.Skipf("examples directory not found: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absExamples, err := filepath.Abs(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(absExamples); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	results, err := gotestgen.Generate("./simple_suite")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one generate result")
	}

	tmpDir, err := gotestrunner.WriteOverlay(results)
	if err != nil {
		t.Fatalf("WriteOverlay: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	overlayFile := filepath.Join(tmpDir, "overlay.json")
	if _, err := os.Stat(overlayFile); err != nil {
		t.Fatalf("overlay.json not found: %v", err)
	}

	data, err := os.ReadFile(overlayFile)
	if err != nil {
		t.Fatalf("reading overlay.json: %v", err)
	}
	var overlayContent struct {
		Replace map[string]string `json:"Replace"`
	}
	if err := json.Unmarshal(data, &overlayContent); err != nil {
		t.Fatalf("overlay.json is not valid JSON: %v", err)
	}
	if len(overlayContent.Replace) == 0 {
		t.Fatal("overlay.json Replace map is empty")
	}
}
```

Also rename the second test function for consistency:

```go
func TestGenerateOverlay_NoSuitesReturnsEmpty(t *testing.T) {
```

- [ ] **Step 7: Verify Go code compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go build ./cmd/gotest/...`
Expected: clean build.

- [ ] **Step 8: Run all Go tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go test ./cmd/gotest/... -count=1`
Expected: all tests pass (the renamed tests run under new names).

- [ ] **Step 9: Commit**

```bash
git add cmd/gotest/prepare.go cmd/gotest/args.go cmd/gotest/cli.go cmd/gotest/generate_test.go
git rm cmd/gotest/overlay.go cmd/gotest/sharedsetup.go cmd/gotest/overlay_test.go
git commit -m "feat(cli): add prepare subcommand, drop overlay and shared-setup"
```

---

### Task 4: Improve `spawnTestProcess` return type

**Files:**
- Modify: `vscode-gotest/src/runnerUtils.ts:224-267`
- Modify: `vscode-gotest/src/runnerUtils.test.ts` (add tests)

- [ ] **Step 1: Add `SpawnResult` interface and update `spawnTestProcess`**

In `vscode-gotest/src/runnerUtils.ts`, add the interface and change the return type. The function currently resolves a `string`. Change it to resolve a `SpawnResult`.

Add after the existing imports (around line 10):

```typescript
export interface SpawnResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}
```

Replace the `spawnTestProcess` function (lines 224-267):

```typescript
export function spawnTestProcess(
  bin: string,
  args: string[],
  cwd: string,
  token: vscode.CancellationToken,
  outputChannel: vscode.OutputChannel,
  label: string,
  env?: Record<string, string>,
): Promise<SpawnResult> {
  return new Promise<SpawnResult>((resolve, reject) => {
    const child = spawn(bin, args, {
      cwd,
      env: env ? { ...process.env, ...env } : undefined,
    });
    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
    });

    child.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
    });

    const cancelListener = token.onCancellationRequested(() => {
      child.kill("SIGTERM");
    });

    child.on("close", (code) => {
      cancelListener.dispose();
      if (stderr) {
        outputChannel.appendLine(`[${label}] stderr: ${stderr}`);
      }
      resolve({ stdout, stderr, exitCode: code ?? 1 });
    });

    child.on("error", (err: Error) => {
      cancelListener.dispose();
      outputChannel.appendLine(`[${label}] error: ${err.message}`);
      reject(err);
    });
  });
}
```

- [ ] **Step 2: Update the export in `runnerUtils.ts`**

Make sure `SpawnResult` is in the imports list at the top of the file. It's defined in the same file, so it just needs to be exported (which the `export interface` already does). No additional changes needed.

- [ ] **Step 3: Run extension tests to confirm nothing breaks yet**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm test`
Expected: all existing tests pass (callers in runner.ts and coverage.ts are not changed yet, but since they only access `.stdout` via destructuring or as `const stdout = await spawnTestProcess(...)`, the TypeScript compiler will catch type mismatches when we modify callers in later tasks).

- [ ] **Step 4: Run format check**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm run format && npm run format:check`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add vscode-gotest/src/runnerUtils.ts
git commit -m "refactor(extension): return SpawnResult from spawnTestProcess"
```

---

### Task 5: Update `types.ts` — drop `OverlayOutput`/`SharedFixtureInfo`, add `PrepareOutput`

**Files:**
- Modify: `vscode-gotest/src/types.ts`

- [ ] **Step 1: Replace `OverlayOutput` and `SharedFixtureInfo` with `PrepareOutput`**

Replace the entire file `vscode-gotest/src/types.ts`:

```typescript
export interface DiscoverWarning {
  importPath: string;
  file?: string;
  line?: number;
  col?: number;
  message: string;
}

export interface DiscoverOutput {
  packages: DiscoverPackage[];
  warnings?: DiscoverWarning[];
}

export interface DiscoverPackage {
  importPath: string;
  dir: string;
  suites: DiscoverSuite[];
}

export interface DiscoverSuite {
  name: string;
  parallel: boolean;
  focused: boolean;
  excluded: boolean;
  file: string;
  line: number;
  col: number;
  lifecycle: string[];
  fixtures: string[];
  methods: DiscoverMethod[];
}

export interface DiscoverMethod {
  name: string;
  parallel: boolean;
  focused: boolean;
  excluded: boolean;
  file: string;
  line: number;
  col: number;
}

export interface PrepareOutput {
  overlayFile: string;
  dir: string;
  stateFile?: string;
}
```

- [ ] **Step 2: Commit**

```bash
git add vscode-gotest/src/types.ts
git commit -m "refactor(extension): drop OverlayOutput/SharedFixtureInfo, add PrepareOutput"
```

---

### Task 6: Simplify `runner.ts` — single CLI spawn

**Files:**
- Modify: `vscode-gotest/src/runner.ts`
- Delete: `vscode-gotest/src/sharedFixtures.ts`

- [ ] **Step 1: Rewrite `runner.ts`**

Replace the entire file:

```typescript
import * as vscode from "vscode";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents } from "./outputParser.js";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";
import {
  collectItems,
  groupByPackage,
  applyResults,
  spawnTestProcess,
  buildRunFilter,
} from "./runnerUtils.js";
import { runGoToolCoverFunc } from "./coverage.js";
import type { CoverageStore } from "./coverageStore.js";

export class TestRunner {
  private _lastJsonOutput = "";
  private readonly _onDidComplete = new vscode.EventEmitter<string>();
  readonly onDidComplete: vscode.Event<string> = this._onDidComplete.event;
  private activeRun: vscode.CancellationTokenSource | undefined;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly coverageStore?: CoverageStore,
  ) {}

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
    this._onDidComplete.dispose();
  }

  async run(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
    if (this.activeRun) {
      this.outputChannel.appendLine("[runner] cancelling previous run");
      this.activeRun.cancel();
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;
    const cancelSub = token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Run");
    this._lastJsonOutput = "";
    let anyCoverOnRun = false;

    try {
      const items = collectItems(this.controller, request);
      if (items.length === 0) {
        run.end();
        return;
      }

      for (const item of items) {
        run.started(item);
      }

      const groups = groupByPackage(items);

      for (const [importPath, groupItems] of groups) {
        if (effectiveToken.isCancellationRequested) {
          for (const item of groupItems) {
            run.skipped(item);
          }
          continue;
        }

        const pkg = this.cache.getPackage(importPath);
        if (!pkg) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(`Package not found: ${importPath}`),
            );
          }
          continue;
        }

        const workspaceDir = this.cache.getWorkspaceDir(importPath);
        if (!workspaceDir) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(
                `Workspace folder not found for: ${importPath}`,
              ),
            );
          }
          continue;
        }

        const filter = buildRunFilter(groupItems, importPath, this.cache);
        const config = scopedConfig(workspaceDir);
        const testFlags = config.get<string[]>("testFlags") ?? [];
        const coverOnRun =
          this.coverageStore !== undefined &&
          (config.get<boolean>("coverOnRun") ?? true);
        if (coverOnRun) anyCoverOnRun = true;

        let coverFile: string | undefined;
        try {
          if (coverOnRun) {
            const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
            coverFile = path.join(tmpDir, "cover.out");
          }

          const cliArgs: string[] = ["-json", "-count=1", importPath];
          if (coverFile) {
            cliArgs.push(`-coverprofile=${coverFile}`);
          }
          if (filter) {
            cliArgs.push("-run", filter);
          }
          cliArgs.push(...testFlags);

          const cmd = await buildCliCommand(
            cliArgs,
            workspaceDir,
            this.outputChannel,
          );
          this.outputChannel.appendLine(
            `[runner] ${formatCliCommand(cmd)}`,
          );

          const result = await spawnTestProcess(
            cmd.bin,
            cmd.args,
            workspaceDir,
            effectiveToken,
            this.outputChannel,
            "runner",
          );
          this._lastJsonOutput += result.stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          if (result.stdout) {
            const events = parseTestEvents(result.stdout);
            applyResults(this.controller, run, events, importPath, pkg.dir);
          } else if (result.exitCode !== 0) {
            const message =
              result.stderr.trim() || `gotest exited with code ${result.exitCode}`;
            for (const item of groupItems) {
              run.errored(item, new vscode.TestMessage(message));
            }
          }

          if (coverFile) {
            try {
              const coverContent = await readFile(coverFile, "utf-8");
              let funcOutput: string | undefined;
              try {
                funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
              } catch {
                this.outputChannel.appendLine(
                  "[runner] go tool cover -func failed",
                );
              }
              this.coverageStore!.update(importPath, coverContent, funcOutput);
            } catch {
              this.outputChannel.appendLine(
                "[runner] no coverprofile generated",
              );
            }
          }
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : String(err);
          this.outputChannel.appendLine(`[runner] error: ${message}`);
          for (const item of groupItems) {
            run.errored(item, new vscode.TestMessage(message));
          }
        } finally {
          if (coverFile) {
            rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
              () => {},
            );
          }
        }
      }

      if (anyCoverOnRun) {
        const allCoverages = this.coverageStore!.buildFileCoverages(this.cache);
        for (const fc of allCoverages) {
          run.addCoverage(fc);
        }
        await this.coverageStore!.save();
      }
    } finally {
      cancelSub.dispose();
      if (this.activeRun === cts) {
        this.activeRun = undefined;
      }
      cts.dispose();
      if (this._lastJsonOutput) {
        this._onDidComplete.fire(this._lastJsonOutput);
      }
      run.end();
    }
  }
}
```

Note: `runGoToolCoverFunc` currently takes `(goBin, coverFile, workspaceDir)` but after the refactor the extension no longer resolves `goBin` separately. We need to update the signature. See Step 2.

- [ ] **Step 2: Update `runGoToolCoverFunc` in `coverage.ts` to resolve its own go binary**

In `vscode-gotest/src/coverage.ts`, change the `runGoToolCoverFunc` function to accept `(coverFile, workspaceDir)` instead of `(goBin, coverFile, workspaceDir)`, and resolve the go binary internally:

```typescript
export async function runGoToolCoverFunc(
  coverFile: string,
  workspaceDir: string,
): Promise<string> {
  const goBin = await resolveGoBinary(undefined, workspaceDir);
  const { stdout } = await execFileAsync(
    goBin,
    ["tool", "cover", `-func=${coverFile}`],
    {
      cwd: workspaceDir,
      timeout: 10_000,
    },
  );
  return stdout;
}
```

Add the `resolveGoBinary` import at the top of `coverage.ts`:

```typescript
import {
  buildCliCommand,
  formatCliCommand,
  resolveGoBinary,
  scopedConfig,
} from "./cli.js";
```

- [ ] **Step 3: Delete `sharedFixtures.ts`**

```bash
rm vscode-gotest/src/sharedFixtures.ts
```

- [ ] **Step 4: Run format**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm run format`

- [ ] **Step 5: Verify TypeScript compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npx tsc --noEmit`
Expected: this will likely fail because `coverage.ts` still references old imports. That's expected — we fix it in Task 7.

- [ ] **Step 6: Commit**

```bash
git add vscode-gotest/src/runner.ts vscode-gotest/src/coverage.ts
git rm vscode-gotest/src/sharedFixtures.ts
git commit -m "refactor(extension): simplify runner.ts to single CLI spawn"
```

---

### Task 7: Simplify `coverage.ts` — single CLI spawn

**Files:**
- Modify: `vscode-gotest/src/coverage.ts`

- [ ] **Step 1: Rewrite `CoverageRunner` class**

Replace the entire file `vscode-gotest/src/coverage.ts`. The pure functions (`parseCoverProfile`, `buildFileCoverages`, `parseFuncCoverage`, `runGoToolCoverFunc`) stay unchanged. The `CoverageRunner` class and its imports are simplified:

```typescript
import * as vscode from "vscode";
import * as path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { CoverageStore } from "./coverageStore.js";
import { parseTestEvents } from "./outputParser.js";
import {
  buildCliCommand,
  formatCliCommand,
  resolveGoBinary,
  scopedConfig,
} from "./cli.js";
import {
  collectItems,
  groupByPackage,
  applyResults,
  spawnTestProcess,
  buildRunFilter,
} from "./runnerUtils.js";

const execFileAsync = promisify(execFile);

export interface ParsedFileCoverage {
  absPath: string;
  statements: vscode.StatementCoverage[];
}

export function parseCoverProfile(
  content: string,
  moduleToDir: (importPath: string) => string | undefined,
): ParsedFileCoverage[] {
  const lines = content.split("\n");
  const fileEntries = new Map<string, vscode.StatementCoverage[]>();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("mode:")) {
      continue;
    }

    const match = /^(.+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$/.exec(
      trimmed,
    );
    if (!match) {
      continue;
    }

    const filePath = match[1];
    const startLine = parseInt(match[2], 10) - 1;
    const startCol = parseInt(match[3], 10) - 1;
    const endLine = parseInt(match[4], 10) - 1;
    const endCol = parseInt(match[5], 10) - 1;
    const count = parseInt(match[7], 10);

    let statements = fileEntries.get(filePath);
    if (!statements) {
      statements = [];
      fileEntries.set(filePath, statements);
    }

    const range = new vscode.Range(
      new vscode.Position(startLine, startCol),
      new vscode.Position(endLine, endCol),
    );
    statements.push(
      new vscode.StatementCoverage(count > 0 ? count : false, range),
    );
  }

  const result: ParsedFileCoverage[] = [];
  for (const [importFilePath, statements] of fileEntries) {
    const lastSlash = importFilePath.lastIndexOf("/");
    const fileName = importFilePath.slice(lastSlash + 1);
    const importDir = importFilePath.slice(0, lastSlash);
    const absDir = moduleToDir(importDir);
    if (!absDir) {
      continue;
    }
    const absPath = path.join(absDir, fileName);
    result.push({ absPath, statements });
  }

  return result;
}

export function buildFileCoverages(
  parsed: ParsedFileCoverage[],
  declarations?: Map<string, vscode.DeclarationCoverage[]>,
): vscode.FileCoverage[] {
  return parsed.map((entry) => {
    const uri = vscode.Uri.file(entry.absPath);
    const decls = declarations?.get(entry.absPath);
    if (decls && decls.length > 0) {
      const details: (vscode.StatementCoverage | vscode.DeclarationCoverage)[] =
        [...entry.statements, ...decls];
      return vscode.FileCoverage.fromDetails(uri, details);
    }
    return vscode.FileCoverage.fromDetails(uri, entry.statements);
  });
}

export function parseFuncCoverage(
  content: string,
  moduleToDir: (importPath: string) => string | undefined,
): Map<string, vscode.DeclarationCoverage[]> {
  const result = new Map<string, vscode.DeclarationCoverage[]>();

  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("total:")) {
      continue;
    }

    const match = /^(.+):(\d+):\s+(\S+)\s+(\d+(?:\.\d+)?)%$/.exec(trimmed);
    if (!match) {
      continue;
    }

    const filePath = match[1];
    const lineNum = parseInt(match[2], 10) - 1;
    const funcName = match[3];
    const pct = parseFloat(match[4]);

    const lastSlash = filePath.lastIndexOf("/");
    const fileName = filePath.slice(lastSlash + 1);
    const importDir = filePath.slice(0, lastSlash);
    const absDir = moduleToDir(importDir);
    if (!absDir) {
      continue;
    }
    const absPath = path.join(absDir, fileName);

    let declarations = result.get(absPath);
    if (!declarations) {
      declarations = [];
      result.set(absPath, declarations);
    }

    const executed = pct > 0 ? pct / 100 : false;
    const position = new vscode.Position(lineNum, 0);
    declarations.push(
      new vscode.DeclarationCoverage(funcName, executed, position),
    );
  }

  return result;
}

export async function runGoToolCoverFunc(
  coverFile: string,
  workspaceDir: string,
): Promise<string> {
  const goBin = await resolveGoBinary(undefined, workspaceDir);
  const { stdout } = await execFileAsync(
    goBin,
    ["tool", "cover", `-func=${coverFile}`],
    {
      cwd: workspaceDir,
      timeout: 10_000,
    },
  );
  return stdout;
}

export class CoverageRunner implements vscode.Disposable {
  private activeRun: vscode.CancellationTokenSource | undefined;
  private activePackageRun: vscode.CancellationTokenSource | undefined;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly store: CoverageStore,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onJsonOutput: (json: string) => void,
  ) {}

  async run(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
    if (this.activeRun) {
      this.outputChannel.appendLine("[coverage] cancelling previous run");
      this.activeRun.cancel();
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;
    const cancelSub = token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Coverage");

    try {
      const items = collectItems(this.controller, request);
      if (items.length === 0) {
        run.end();
        return;
      }

      for (const item of items) {
        run.started(item);
      }

      const groups = groupByPackage(items);
      let allJsonOutput = "";

      for (const [importPath, groupItems] of groups) {
        if (effectiveToken.isCancellationRequested) {
          for (const item of groupItems) {
            run.skipped(item);
          }
          continue;
        }

        const pkg = this.cache.getPackage(importPath);
        if (!pkg) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(`Package not found: ${importPath}`),
            );
          }
          continue;
        }

        const workspaceDir = this.cache.getWorkspaceDir(importPath);
        if (!workspaceDir) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(
                `Workspace folder not found for: ${importPath}`,
              ),
            );
          }
          continue;
        }

        const config = scopedConfig(workspaceDir);
        const testFlags = config.get<string[]>("testFlags") ?? [];

        let coverFile: string | undefined;

        try {
          const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
          coverFile = path.join(tmpDir, "cover.out");

          const filter = buildRunFilter(groupItems, importPath, this.cache);

          const cliArgs: string[] = [
            "-json",
            "-count=1",
            `-coverprofile=${coverFile}`,
            importPath,
          ];
          if (filter) {
            cliArgs.push("-run", filter);
          }
          cliArgs.push(...testFlags);

          const cmd = await buildCliCommand(
            cliArgs,
            workspaceDir,
            this.outputChannel,
          );
          this.outputChannel.appendLine(
            `[coverage] ${formatCliCommand(cmd)}`,
          );

          const result = await spawnTestProcess(
            cmd.bin,
            cmd.args,
            workspaceDir,
            effectiveToken,
            this.outputChannel,
            "coverage",
          );
          allJsonOutput += result.stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          if (result.stdout) {
            const events = parseTestEvents(result.stdout);
            applyResults(this.controller, run, events, importPath, pkg.dir);
          } else if (result.exitCode !== 0) {
            const message =
              result.stderr.trim() ||
              `gotest exited with code ${result.exitCode}`;
            for (const item of groupItems) {
              run.errored(item, new vscode.TestMessage(message));
            }
          }

          try {
            const coverContent = await readFile(coverFile, "utf-8");
            let funcOutput: string | undefined;
            try {
              funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
            } catch {
              this.outputChannel.appendLine(
                "[coverage] go tool cover -func failed, skipping declaration coverage",
              );
            }
            this.store.update(importPath, coverContent, funcOutput);
          } catch {
            this.outputChannel.appendLine(
              "[coverage] no coverprofile generated",
            );
          }
        } finally {
          if (coverFile) {
            const coverDir = path.dirname(coverFile);
            rm(coverDir, { recursive: true, force: true }).catch(() => {});
          }
        }
      }

      const allCoverages = this.store.buildFileCoverages(this.cache);
      for (const fc of allCoverages) {
        run.addCoverage(fc);
      }
      await this.store.save();

      if (allJsonOutput) {
        this.onJsonOutput(allJsonOutput);
      }
    } finally {
      cancelSub.dispose();
      if (this.activeRun === cts) {
        this.activeRun = undefined;
      }
      cts.dispose();
      run.end();
    }
  }

  async copyCoverageSummary(): Promise<void> {
    const coverages = this.store.buildFileCoverages(this.cache);
    if (coverages.length === 0) {
      vscode.window.showInformationMessage(
        "No coverage data available. Run tests with coverage first.",
      );
      return;
    }

    const rows: { file: string; covered: number; total: number }[] = [];

    for (const fc of coverages) {
      let filePath = fc.uri.fsPath;
      const folder = vscode.workspace.getWorkspaceFolder(fc.uri);
      if (folder && filePath.startsWith(folder.uri.fsPath)) {
        filePath = filePath.slice(folder.uri.fsPath.length + 1);
      }
      rows.push({
        file: filePath,
        covered: fc.statementCoverage.covered,
        total: fc.statementCoverage.total,
      });
    }

    rows.sort((a, b) => a.file.localeCompare(b.file));

    const maxFileLen = Math.max(4, ...rows.map((r) => r.file.length));
    const header = `${"File".padEnd(maxFileLen)}  Stmts      Cover`;
    const separator = "-".repeat(header.length);

    const lines = [header, separator];
    let totalCovered = 0;
    let totalStmts = 0;

    for (const row of rows) {
      totalCovered += row.covered;
      totalStmts += row.total;
      const pct =
        row.total > 0
          ? ((row.covered / row.total) * 100).toFixed(1) + "%"
          : "N/A";
      const stmts = `${row.covered}/${row.total}`;
      lines.push(`${row.file.padEnd(maxFileLen)}  ${stmts.padEnd(9)}  ${pct}`);
    }

    lines.push(separator);
    const totalPct =
      totalStmts > 0
        ? ((totalCovered / totalStmts) * 100).toFixed(1) + "%"
        : "N/A";
    const totalStmtsStr = `${totalCovered}/${totalStmts}`;
    lines.push(
      `${"Total".padEnd(maxFileLen)}  ${totalStmtsStr.padEnd(9)}  ${totalPct}`,
    );

    const text = lines.join("\n");
    await vscode.env.clipboard.writeText(text);
    vscode.window.showInformationMessage(
      "Coverage summary copied to clipboard.",
    );
  }

  async runPackage(importPath: string): Promise<void> {
    this.activePackageRun?.cancel();
    const cts = new vscode.CancellationTokenSource();
    this.activePackageRun = cts;

    const pkg = this.cache.getPackage(importPath);
    if (!pkg) return;
    const workspaceDir = this.cache.getWorkspaceDir(importPath);
    if (!workspaceDir) return;

    const config = scopedConfig(workspaceDir);
    const testFlags = config.get<string[]>("testFlags") ?? [];
    let coverFile: string | undefined;

    try {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");

      const cliArgs: string[] = [
        "-count=1",
        `-coverprofile=${coverFile}`,
        importPath,
      ];
      cliArgs.push(...testFlags);

      const cmd = await buildCliCommand(
        cliArgs,
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.appendLine(
        `[coverage:save] ${formatCliCommand(cmd)}`,
      );

      await spawnTestProcess(
        cmd.bin,
        cmd.args,
        workspaceDir,
        cts.token,
        this.outputChannel,
        "coverage",
      );

      if (cts.token.isCancellationRequested) return;

      const coverContent = await readFile(coverFile, "utf-8");
      let funcOutput: string | undefined;
      try {
        funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
      } catch {
        this.outputChannel.appendLine(
          "[coverage:save] go tool cover -func failed",
        );
      }
      this.store.update(importPath, coverContent, funcOutput);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[coverage:save] failed: ${message}`);
      return;
    } finally {
      if (this.activePackageRun === cts) this.activePackageRun = undefined;
      cts.dispose();
      if (coverFile)
        rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
          () => {},
        );
    }

    const request = new vscode.TestRunRequest();
    const run = this.controller.createTestRun(request, "Cover on Save");
    const allCoverages = this.store.buildFileCoverages(this.cache);
    for (const fc of allCoverages) {
      run.addCoverage(fc);
    }
    run.end();
    await this.store.save();
  }

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
    this.activePackageRun?.cancel();
    this.activePackageRun = undefined;
  }
}
```

- [ ] **Step 2: Run format**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm run format`

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npx tsc --noEmit`
Expected: clean (all imports resolved, types match).

- [ ] **Step 4: Run extension tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm test`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add vscode-gotest/src/coverage.ts
git commit -m "refactor(extension): simplify coverage.ts to single CLI spawn"
```

---

### Task 8: Refactor `debug.ts` — use `gotest prepare`

**Files:**
- Modify: `vscode-gotest/src/debug.ts`

- [ ] **Step 1: Rewrite `debug.ts`**

Replace the entire file. Key changes:
- Instead of spawning `gotest overlay`, spawn `gotest prepare` as long-running process
- Read first JSON line to get `PrepareOutput`
- Add `env` to DebugConfiguration for shared fixture state file
- Track prepare child processes instead of overlay dirs
- Kill prepare processes on session end

```typescript
import * as vscode from "vscode";
import { spawn, type ChildProcess } from "node:child_process";
import type { PrepareOutput } from "./types.js";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";

export class DebugLauncher implements vscode.Disposable {
  private readonly prepareProcesses = new Map<string, ChildProcess>();
  private sessionListener: vscode.Disposable | undefined;

  constructor(private readonly outputChannel: vscode.OutputChannel) {}

  async debug(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
    buildRunFilter: (items: readonly vscode.TestItem[]) => string | undefined,
    getPackageDir: (item: vscode.TestItem) => string | undefined,
  ): Promise<void> {
    const items = request.include;
    if (!items || items.length === 0) {
      return;
    }

    const pkgDir = getPackageDir(items[0]);
    if (!pkgDir) {
      return;
    }

    const workspaceFolder = vscode.workspace.getWorkspaceFolder(
      vscode.Uri.file(pkgDir),
    );
    if (!workspaceFolder) {
      return;
    }

    let prepare: PrepareOutput;
    let child: ChildProcess;
    try {
      const cmd = await buildCliCommand(
        ["prepare", pkgDir],
        workspaceFolder.uri.fsPath,
        this.outputChannel,
      );
      this.outputChannel.appendLine(`[debug] ${formatCliCommand(cmd)}`);

      const result = await this.spawnPrepare(
        cmd.bin,
        cmd.args,
        workspaceFolder.uri.fsPath,
      );
      prepare = result.output;
      child = result.child;
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Unknown error running prepare";
      vscode.window.showErrorMessage(`gotest prepare failed: ${message}`);
      return;
    }

    const sessionName = `Go Test Suite Debug`;
    this.prepareProcesses.set(sessionName, child);

    const runFilter = buildRunFilter(items);

    const extraBuildFlags = scopedConfig(workspaceFolder.uri.fsPath).get<
      string[]
    >("buildFlags", []);

    const debugConfig: vscode.DebugConfiguration = {
      type: "go",
      name: sessionName,
      request: "launch",
      mode: "test",
      program: pkgDir,
      buildFlags:
        `-overlay=${prepare.overlayFile} ${extraBuildFlags.join(" ")}`.trim(),
      args: runFilter ? ["-test.run", runFilter] : [],
    };

    if (prepare.stateFile) {
      debugConfig.env = { GOTEST_SHARED_STATE_FILE: prepare.stateFile };
    }

    this.outputChannel.appendLine(
      `[debug] launching: ${JSON.stringify(debugConfig)}`,
    );

    const started = await vscode.debug.startDebugging(
      workspaceFolder,
      debugConfig,
    );

    if (!started) {
      this.killPrepareProcess(sessionName);
    }
  }

  registerCleanupOnSessionEnd(context: vscode.ExtensionContext): void {
    this.sessionListener = vscode.debug.onDidTerminateDebugSession(
      (session) => {
        this.killPrepareProcess(session.name);
      },
    );
    context.subscriptions.push(this.sessionListener);
  }

  dispose(): void {
    this.sessionListener?.dispose();
    for (const [name] of this.prepareProcesses) {
      this.killPrepareProcess(name);
    }
  }

  private spawnPrepare(
    bin: string,
    args: string[],
    cwd: string,
  ): Promise<{ output: PrepareOutput; child: ChildProcess }> {
    return new Promise((resolve, reject) => {
      const child = spawn(bin, args, { cwd });
      let stdout = "";
      let settled = false;

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
        if (!settled && stdout.includes("\n")) {
          settled = true;
          try {
            const output = JSON.parse(stdout.split("\n")[0]) as PrepareOutput;
            resolve({ output, child });
          } catch {
            child.kill("SIGTERM");
            reject(
              new Error(
                `Failed to parse prepare output: ${stdout.trim()}`,
              ),
            );
          }
        }
      });

      child.stderr.on("data", (data: Buffer) => {
        this.outputChannel.appendLine(
          `[debug:prepare] ${data.toString().trimEnd()}`,
        );
      });

      child.on("error", (err: Error) => {
        if (!settled) {
          settled = true;
          reject(err);
        }
      });

      child.on("close", (code) => {
        if (!settled) {
          settled = true;
          reject(new Error(`prepare exited with code ${code} before ready`));
        }
      });
    });
  }

  private killPrepareProcess(sessionName: string): void {
    const child = this.prepareProcesses.get(sessionName);
    if (child) {
      this.prepareProcesses.delete(sessionName);
      child.kill("SIGTERM");
    }
  }
}
```

- [ ] **Step 2: Run format**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm run format`

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 4: Run extension tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm test`
Expected: all tests pass.

- [ ] **Step 5: Run format check**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm run format:check`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add vscode-gotest/src/debug.ts
git commit -m "refactor(extension): use gotest prepare for debug sessions"
```

---

### Task 9: Final verification and cleanup

**Files:**
- Verify: all modified files

- [ ] **Step 1: Verify no remaining references to deleted files**

```bash
cd /home/ubuntu/projects/mvrahden/go-test
grep -rn "sharedFixtures\|SharedSetupProcess\|startSharedSetup\|OverlayOutput\|SharedFixtureInfo" vscode-gotest/src/ --include="*.ts" | grep -v node_modules | grep -v ".test."
```

Expected: no results (all references removed).

- [ ] **Step 2: Verify no remaining references to dropped subcommands in extension**

```bash
grep -rn '"overlay"' vscode-gotest/src/ --include="*.ts" | grep -v node_modules | grep -v ".test."
grep -rn '"shared-setup"' vscode-gotest/src/ --include="*.ts" | grep -v node_modules | grep -v ".test."
```

Expected: no results.

- [ ] **Step 3: Full Go build**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go build ./cmd/gotest/...`
Expected: clean.

- [ ] **Step 4: Full Go tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test && go test ./cmd/gotest/... -count=1`
Expected: all pass.

- [ ] **Step 5: Full TypeScript compile check**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 6: Full extension test suite**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm test`
Expected: all pass.

- [ ] **Step 7: Format check**

Run: `cd /home/ubuntu/projects/mvrahden/go-test/vscode-gotest && npm run format:check`
Expected: clean.

- [ ] **Step 8: Verify git status is clean**

```bash
git status
git diff --stat HEAD~8
```

Expected: all changes committed across 8 commits. The diff should show:
- Go: `prepare.go` created, `overlay.go`/`sharedsetup.go` deleted, `stdlib.go`/`json.go`/`exec.go`/`watch.go`/`cli.go`/`args.go`/`generate_test.go` modified
- TS: `runner.ts`/`coverage.ts`/`debug.ts`/`types.ts`/`runnerUtils.ts` modified, `sharedFixtures.ts` deleted
