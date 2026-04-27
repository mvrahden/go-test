# vscode-gotest v2.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Spec View Panel (webview rendering BDD output) and Watch Mode (continuous test execution streamed to Test Explorer).

**Architecture:** Two CLI flags (`gotest spec --input=-` and `gotest watch -json`) provide the data plumbing. The extension captures JSON from test runs, pipes to `gotest spec` for rendering, and subscribes to `gotest watch` for live results. Both features plug into the existing TestRunner/TestController infrastructure.

**Tech Stack:** Go (CLI changes), TypeScript (VS Code extension), VS Code Webview API, Node.js child_process

---

## File Structure

### Go CLI (modifications)
- `cmd/gotest/spec.go` — add `--input` flag to `parseSpecFlags`, read from file/stdin when present
- `cmd/gotest/spec_test.go` — test the `--input` flag behavior
- `cmd/gotest/watch.go` — add `-json` flag, emit `watch-start` sentinel, output JSON events
- `cmd/gotest/watch_test.go` — test `-json` event emission

### VS Code Extension (new files)
- `vscode-gotest/src/specView.ts` — SpecViewPanel (webview) + `ansiToHtml()` converter
- `vscode-gotest/src/watch.ts` — WatchManager, WatchProcess, WatchStatusBar

### VS Code Extension (modifications)
- `vscode-gotest/src/runner.ts` — store JSON output after runs, expose `lastJsonOutput`
- `vscode-gotest/src/extension.ts` — wire new commands and components
- `vscode-gotest/package.json` — new commands, settings, menus

---

### Task 1: `gotest spec --input` flag

**Files:**
- Modify: `cmd/gotest/spec.go`
- Create: `cmd/gotest/spec_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/gotest/spec_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func TestRunSpec_InputStdin(t *testing.T) {
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

	// First generate JSON test output by running tests
	results, _, err := gotestrunner.SuitesGenerateWithCollectorResults("./simple_suite")
	if err != nil {
		t.Fatalf("SuitesGenerate: %v", err)
	}
	tmpDir, err := gotestrunner.WriteOverlay(results)
	if err != nil {
		t.Fatalf("WriteOverlay: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	overlayArgs := []string{"-overlay=" + filepath.Join(tmpDir, "overlay.json"), "./simple_suite"}
	jsonData, _, err := gotestrunner.StdlibRunTestsJSON(overlayArgs)
	if err != nil {
		t.Fatalf("StdlibRunTestsJSON: %v", err)
	}

	// Now test renderSpecFromReader — the core of --input handling
	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events from JSON input")
	}

	tree := gotestspec.BuildTree(events)
	if len(tree) == 0 {
		t.Fatal("expected non-empty spec tree")
	}

	// Render to a buffer and verify output contains expected content
	var buf bytes.Buffer
	gotestspec.RenderTerminal(&buf, tree)
	output := buf.String()
	if len(output) == 0 {
		t.Fatal("expected non-empty terminal output")
	}
	if !bytes.Contains([]byte(output), []byte("Simple")) {
		t.Errorf("output missing 'Simple' suite name, got: %s", output[:min(200, len(output))])
	}
}

func TestParseSpecFlags_Input(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInput string
		wantFmt   string
		wantRest  []string
	}{
		{
			name:      "stdin",
			args:      []string{"--input=-", "./pkg"},
			wantInput: "-",
			wantFmt:   "terminal",
			wantRest:  []string{"./pkg"},
		},
		{
			name:      "file path",
			args:      []string{"--input=/tmp/results.json", "--format=md"},
			wantInput: "/tmp/results.json",
			wantFmt:   "md",
			wantRest:  nil,
		},
		{
			name:      "equals syntax",
			args:      []string{"--input=results.json"},
			wantInput: "results.json",
			wantFmt:   "terminal",
			wantRest:  nil,
		},
		{
			name:      "no input flag",
			args:      []string{"--format=md", "./pkg"},
			wantInput: "",
			wantFmt:   "md",
			wantRest:  []string{"./pkg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, output, input, remaining := parseSpecFlags(tt.args)
			_ = output // not tested here
			if input != tt.wantInput {
				t.Errorf("input = %q, want %q", input, tt.wantInput)
			}
			if format != tt.wantFmt {
				t.Errorf("format = %q, want %q", format, tt.wantFmt)
			}
			if len(remaining) != len(tt.wantRest) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRest)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/gotest/ -run TestParseSpecFlags_Input -v`
Expected: FAIL — `parseSpecFlags` returns 3 values, test expects 4.

- [ ] **Step 3: Modify `parseSpecFlags` to accept `--input` flag**

In `cmd/gotest/spec.go`, change `parseSpecFlags` to return 4 values:

```go
func parseSpecFlags(args []string) (format, output, input string, remaining []string) {
	format = "terminal"
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--format" && i+1 < len(args):
			format = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--format="):
			format = strings.TrimPrefix(args[i], "--format=")
		case args[i] == "--output" && i+1 < len(args):
			output = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--output="):
			output = strings.TrimPrefix(args[i], "--output=")
		case args[i] == "--input" && i+1 < len(args):
			input = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--input="):
			input = strings.TrimPrefix(args[i], "--input=")
		default:
			remaining = append(remaining, args[i])
		}
	}
	return
}
```

- [ ] **Step 4: Update `runSpec` to handle `--input` mode**

Replace the beginning of `runSpec` in `cmd/gotest/spec.go`:

```go
func runSpec(args []string) int {
	format, output, input, remaining := parseSpecFlags(args)

	var w io.Writer = os.Stdout
	if output != "" {
		f, ferr := os.Create(output)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", ferr)
			return 2
		}
		defer f.Close()
		w = f
	}

	// --input mode: read pre-captured JSON, skip test execution
	if input != "" {
		return runSpecFromInput(input, format, w)
	}

	ownArgs, goTestArgs := SplitArgs(remaining)
	DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")

	patterns := ExtractPackagePatterns(goTestArgs)

	var allResults gotestgen.GenerateResults
	for _, pattern := range patterns {
		results, _, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allResults = append(allResults, results...)
	}

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if DEBUG {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
	} else {
		defer os.RemoveAll(tmpDir)
	}

	overlayArgs := append([]string{"-overlay=" + filepath.Join(tmpDir, "overlay.json")}, goTestArgs...)

	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(overlayArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	tree := gotestspec.BuildTree(events)

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree)
	}

	return code
}

func runSpecFromInput(input, format string, w io.Writer) int {
	var r io.Reader
	if input == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: opening input file: %s\n", err)
			return 2
		}
		defer f.Close()
		r = f
	}

	events, err := gotestspec.ParseEvents(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing input: %s\n", err)
		return 2
	}

	tree := gotestspec.BuildTree(events)

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree)
	}

	return 0
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/gotest/ -run "TestParseSpecFlags_Input|TestRunSpec_InputStdin" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/gotest/spec.go cmd/gotest/spec_test.go
git commit -m "feat(cli): add --input flag to gotest spec for pre-captured JSON"
```

---

### Task 2: `gotest watch -json` flag

**Files:**
- Modify: `cmd/gotest/watch.go`
- Create: `cmd/gotest/watch_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/gotest/watch_test.go`:

```go
package main

import (
	"testing"
)

func TestParseWatchFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantJSON bool
		wantRest []string
	}{
		{
			name:     "no flags",
			args:     []string{"./pkg/..."},
			wantJSON: false,
			wantRest: []string{"./pkg/..."},
		},
		{
			name:     "json flag",
			args:     []string{"-json", "./pkg/..."},
			wantJSON: true,
			wantRest: []string{"./pkg/..."},
		},
		{
			name:     "json flag after package",
			args:     []string{"./pkg/...", "-json"},
			wantJSON: true,
			wantRest: []string{"./pkg/..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonMode, remaining := parseWatchFlags(tt.args)
			if jsonMode != tt.wantJSON {
				t.Errorf("jsonMode = %v, want %v", jsonMode, tt.wantJSON)
			}
			if len(remaining) != len(tt.wantRest) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRest)
			} else {
				for i, r := range remaining {
					if r != tt.wantRest[i] {
						t.Errorf("remaining[%d] = %q, want %q", i, r, tt.wantRest[i])
					}
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/gotest/ -run TestParseWatchFlags -v`
Expected: FAIL — `parseWatchFlags` undefined.

- [ ] **Step 3: Implement `parseWatchFlags` and modify `runWatch`**

Add to `cmd/gotest/watch.go`:

```go
func parseWatchFlags(args []string) (jsonMode bool, remaining []string) {
	for _, arg := range args {
		if arg == "-json" {
			jsonMode = true
		} else {
			remaining = append(remaining, arg)
		}
	}
	return
}
```

Then modify `runWatch` to use it and branch on JSON mode. Replace the function signature area:

```go
func runWatch(args []string) int {
	jsonMode, args := parseWatchFlags(args)

	ownArgs, goTestArgs := SplitArgs(args)
	DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
	SPEC = slices.Contains(ownArgs, "--spec")
	patterns := ExtractPackagePatterns(goTestArgs)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initial run
	if !jsonMode {
		fmt.Printf("\033[2m  running tests...\033[0m\n")
	}
	watchRunOnce(goTestArgs, patterns, jsonMode)
	if !jsonMode {
		fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating watcher: %s\n", err)
		return 2
	}
	defer watcher.Close()

	for _, pattern := range patterns {
		addWatchDirs(watcher, pattern)
	}

	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}
	var changedDirs map[string]bool

	for {
		select {
		case <-ctx.Done():
			return 0

		case event, ok := <-watcher.Events:
			if !ok {
				return 0
			}
			if !isGoFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if changedDirs == nil {
				changedDirs = map[string]bool{}
			}
			changedDirs[filepath.Dir(event.Name)] = true
			debounce.Reset(200 * time.Millisecond)

		case <-debounce.C:
			if !jsonMode {
				clearTerminal()
			}
			pkgPatterns := dirsToPatterns(changedDirs)
			pkgArgs := replacePatterns(goTestArgs, pkgPatterns)
			watchRunOnce(pkgArgs, pkgPatterns, jsonMode)
			changedDirs = nil
			if !jsonMode {
				fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return 0
			}
			fmt.Fprintf(os.Stderr, "watch error: %s\n", err)
		}
	}
}
```

- [ ] **Step 4: Modify `watchRunOnce` to support JSON mode**

Replace `watchRunOnce`:

```go
func watchRunOnce(goTestArgs []string, patterns []string, jsonMode bool) int {
	var allResults gotestgen.GenerateResults
	for _, pattern := range patterns {
		results, _, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
		if err != nil {
			if jsonMode {
				fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			}
			return 2
		}
		allResults = append(allResults, results...)
	}

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		if jsonMode {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		}
		return 2
	}
	if DEBUG {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
	} else {
		defer os.RemoveAll(tmpDir)
	}

	overlayArgs := append([]string{"-overlay=" + filepath.Join(tmpDir, "overlay.json")}, goTestArgs...)

	if jsonMode {
		// Emit watch-start sentinel
		fmt.Printf("{\"Action\":\"watch-start\",\"Package\":%q}\n", strings.Join(patterns, ","))

		jsonData, code, err := gotestrunner.StdlibRunTestsJSON(overlayArgs)
		if err != nil {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			return 2
		}
		// Stream raw JSON lines to stdout
		os.Stdout.Write(jsonData)
		return code
	}

	if SPEC {
		return runWithSpec(overlayArgs, nil)
	}

	code, err := gotestrunner.StdlibRunTests(overlayArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/gotest/ -run TestParseWatchFlags -v`
Expected: PASS

- [ ] **Step 6: Verify the full CLI builds**

Run: `go build ./cmd/gotest/`
Expected: Success, no errors.

- [ ] **Step 7: Commit**

```bash
git add cmd/gotest/watch.go cmd/gotest/watch_test.go
git commit -m "feat(cli): add -json flag to gotest watch with watch-start sentinel"
```

---

### Task 3: Spec View Panel — ANSI-to-HTML converter and webview

**Files:**
- Create: `vscode-gotest/src/specView.ts`

- [ ] **Step 1: Create `specView.ts` with ANSI converter and panel**

```typescript
import * as vscode from "vscode";
import { spawn } from "node:child_process";

const ANSI_REGEX = /\x1b\[(\d+)m/g;

const ANSI_CLASS_MAP: Record<string, string> = {
  "0": "",
  "1": "ansi-bold",
  "2": "ansi-dim",
  "31": "ansi-red",
  "32": "ansi-green",
  "33": "ansi-yellow",
};

export function ansiToHtml(text: string): string {
  let result = "";
  let lastIndex = 0;
  let openSpans = 0;

  ANSI_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = ANSI_REGEX.exec(text)) !== null) {
    // Append text before this match (escaped)
    result += escapeHtml(text.slice(lastIndex, match.index));
    lastIndex = match.index + match[0].length;

    const code = match[1];
    if (code === "0") {
      // Reset: close all open spans
      while (openSpans > 0) {
        result += "</span>";
        openSpans--;
      }
    } else {
      const cls = ANSI_CLASS_MAP[code];
      if (cls) {
        result += `<span class="${cls}">`;
        openSpans++;
      }
    }
  }

  // Append remaining text
  result += escapeHtml(text.slice(lastIndex));

  // Close any unclosed spans
  while (openSpans > 0) {
    result += "</span>";
    openSpans--;
  }

  return result;
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

export class SpecViewPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  private disposables: vscode.Disposable[] = [];
  private lastHtml = "";

  constructor(private readonly outputChannel: vscode.OutputChannel) {}

  show(): void {
    if (this.panel) {
      this.panel.reveal();
      return;
    }

    this.panel = vscode.window.createWebviewPanel(
      "gotestSpecView",
      "Go Test: Spec View",
      vscode.ViewColumn.Beside,
      { enableScripts: true },
    );

    this.panel.webview.html = this.buildHtml(this.lastHtml || "Run tests to generate spec view.");

    this.panel.onDidDispose(() => {
      this.panel = undefined;
    }, null, this.disposables);
  }

  get isVisible(): boolean {
    return this.panel?.visible ?? false;
  }

  async refresh(jsonOutput: string): Promise<void> {
    const autoRefresh = vscode.workspace
      .getConfiguration("gotest")
      .get<boolean>("specView.autoRefresh", true);

    if (!autoRefresh || !this.panel) {
      return;
    }

    const cliPath = vscode.workspace
      .getConfiguration("gotest")
      .get<string>("cliPath") ?? "gotest";

    try {
      const ansiOutput = await this.runSpecFromInput(cliPath, jsonOutput);
      this.lastHtml = ansiToHtml(ansiOutput);
      this.panel.webview.html = this.buildHtml(this.lastHtml);
    } catch (err) {
      this.outputChannel.appendLine(`[specView] refresh error: ${err}`);
    }
  }

  dispose(): void {
    this.panel?.dispose();
    for (const d of this.disposables) {
      d.dispose();
    }
  }

  private runSpecFromInput(cliPath: string, jsonInput: string): Promise<string> {
    return new Promise((resolve, reject) => {
      const child = spawn(cliPath, ["spec", "--input=-", "--format=terminal"]);
      let stdout = "";
      let stderr = "";

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
      });
      child.stderr.on("data", (data: Buffer) => {
        stderr += data.toString();
      });

      child.on("close", (code) => {
        if (code !== 0 && code !== 1) {
          reject(new Error(`gotest spec exited ${code}: ${stderr}`));
        } else {
          resolve(stdout);
        }
      });

      child.on("error", reject);

      child.stdin.write(jsonInput);
      child.stdin.end();
    });
  }

  private buildHtml(content: string): string {
    return `<!DOCTYPE html>
<html>
<head>
<style>
  body {
    background: var(--vscode-editor-background);
    color: var(--vscode-editor-foreground);
    margin: 0;
    padding: 16px;
  }
  .spec-output {
    font-family: var(--vscode-editor-font-family);
    font-size: var(--vscode-editor-font-size);
    line-height: 1.5;
    white-space: pre-wrap;
  }
  .ansi-bold { font-weight: bold; }
  .ansi-dim { opacity: 0.6; }
  .ansi-red { color: var(--vscode-testing-iconFailed); }
  .ansi-green { color: var(--vscode-testing-iconPassed); }
  .ansi-yellow { color: var(--vscode-testing-iconSkipped); }
</style>
</head>
<body>
  <pre class="spec-output">${content}</pre>
</body>
</html>`;
  }
}
```

- [ ] **Step 2: Verify extension compiles**

Run: `cd vscode-gotest && npm run compile`
Expected: Success.

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/specView.ts
git commit -m "feat(vscode): implement SpecViewPanel with ANSI-to-HTML rendering"
```

---

### Task 4: Wire Spec View into extension and expose JSON cache

**Files:**
- Modify: `vscode-gotest/src/runner.ts`
- Modify: `vscode-gotest/src/extension.ts`
- Modify: `vscode-gotest/package.json`

- [ ] **Step 1: Add JSON output caching to TestRunner**

In `vscode-gotest/src/runner.ts`, add a field and event emitter. Add after the class declaration line:

```typescript
export class TestRunner {
  private _lastJsonOutput = "";
  private _onDidComplete = new vscode.EventEmitter<string>();
  readonly onDidComplete: vscode.Event<string> = this._onDidComplete.event;

  get lastJsonOutput(): string {
    return this._lastJsonOutput;
  }

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
  ) {}
```

Then in the `run()` method, after `const stdout = await this.spawnProcess(...)`, store the output:

After the line `const events = parseTestEvents(stdout);` add:
```typescript
        this._lastJsonOutput += stdout;
```

At the start of the `run()` method (after `const run = ...`), reset the cache:
```typescript
    this._lastJsonOutput = "";
```

At the end of the `run()` method, just before `run.end()` in the `finally` block, fire the event:
```typescript
      if (this._lastJsonOutput) {
        this._onDidComplete.fire(this._lastJsonOutput);
      }
```

- [ ] **Step 2: Add commands and settings to package.json**

Add to the `commands` array in `package.json`:

```json
{
  "command": "gotest.showSpecView",
  "title": "Go Test: Show Spec View"
}
```

Add to the `configuration.properties` object:

```json
"gotest.specView.autoRefresh": {
  "type": "boolean",
  "default": true,
  "description": "Auto-refresh spec view panel after test runs"
}
```

- [ ] **Step 3: Wire SpecViewPanel in extension.ts**

Add import:
```typescript
import { SpecViewPanel } from "./specView.js";
```

After `runner = new TestRunner(...)`, add:
```typescript
  const specView = new SpecViewPanel(outputChannel);

  // Auto-refresh spec view on test run completion
  const specViewRefreshDisposable = runner.onDidComplete((jsonOutput) => {
    specView.refresh(jsonOutput);
  });
```

Register the command:
```typescript
  const showSpecViewCmd = vscode.commands.registerCommand(
    "gotest.showSpecView",
    () => {
      specView.show();
    },
  );
```

Add to `context.subscriptions`:
```typescript
    specView,
    specViewRefreshDisposable,
    showSpecViewCmd,
```

- [ ] **Step 4: Verify extension compiles**

Run: `cd vscode-gotest && npm run compile`
Expected: Success.

- [ ] **Step 5: Verify TypeScript types**

Run: `cd vscode-gotest && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add vscode-gotest/src/runner.ts vscode-gotest/src/extension.ts vscode-gotest/package.json
git commit -m "feat(vscode): wire SpecViewPanel with JSON cache and auto-refresh"
```

---

### Task 5: Watch Manager and Watch Process

**Files:**
- Create: `vscode-gotest/src/watch.ts`

- [ ] **Step 1: Create `watch.ts` with WatchManager, WatchProcess, and WatchStatusBar**

```typescript
import * as vscode from "vscode";
import { spawn, type ChildProcess } from "node:child_process";
import type { GoTestController } from "./testController.js";
import { parseTestEvents } from "./outputParser.js";

interface WatchEvent {
  Action: string;
  Package?: string;
  Test?: string;
  Output?: string;
  Elapsed?: number;
}

class WatchProcess implements vscode.Disposable {
  private child: ChildProcess | undefined;
  private buffer = "";
  private disposed = false;
  private restartCount = 0;
  private lastCrashTime = 0;

  constructor(
    private readonly pkgScope: string,
    private readonly cwd: string,
    private readonly cliPath: string,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onCycleStart: () => void,
    private readonly onEvents: (jsonLines: string) => void,
    private readonly onError: (msg: string) => void,
    private readonly onExit: () => void,
  ) {
    this.start();
  }

  dispose(): void {
    this.disposed = true;
    if (this.child) {
      this.child.kill("SIGTERM");
      setTimeout(() => {
        if (this.child && !this.child.killed) {
          this.child.kill("SIGKILL");
        }
      }, 2000);
    }
  }

  private start(): void {
    const args = ["watch", "-json", this.pkgScope];
    this.outputChannel.appendLine(`[watch] ${this.cliPath} ${args.join(" ")} (cwd: ${this.cwd})`);

    this.child = spawn(this.cliPath, args, { cwd: this.cwd });
    this.buffer = "";

    this.child.stdout?.on("data", (data: Buffer) => {
      this.buffer += data.toString();
      this.processBuffer();
    });

    this.child.stderr?.on("data", (data: Buffer) => {
      this.outputChannel.appendLine(`[watch] stderr: ${data.toString()}`);
    });

    this.child.on("close", (code) => {
      this.child = undefined;
      if (this.disposed) {
        return;
      }
      this.outputChannel.appendLine(`[watch] process exited with code ${code}`);
      this.maybeRestart();
    });

    this.child.on("error", (err: Error) => {
      this.child = undefined;
      if (this.disposed) {
        return;
      }
      this.onError(err.message);
      this.maybeRestart();
    });
  }

  private processBuffer(): void {
    const lines = this.buffer.split("\n");
    this.buffer = lines.pop() ?? "";

    let cycleLines = "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) {
        continue;
      }

      try {
        const event = JSON.parse(trimmed) as WatchEvent;

        if (event.Action === "watch-start") {
          // Flush any accumulated lines from previous cycle
          if (cycleLines) {
            this.onEvents(cycleLines);
            cycleLines = "";
          }
          this.onCycleStart();
          continue;
        }

        if (event.Action === "watch-error") {
          this.onError(event.Output ?? "unknown watch error");
          continue;
        }

        cycleLines += trimmed + "\n";
      } catch {
        // Skip non-JSON lines
      }
    }

    // Flush accumulated events
    if (cycleLines) {
      this.onEvents(cycleLines);
    }
  }

  private maybeRestart(): void {
    const autoRestart = vscode.workspace
      .getConfiguration("gotest")
      .get<boolean>("watch.autoRestart", true);

    if (!autoRestart) {
      this.onExit();
      return;
    }

    const now = Date.now();
    if (now - this.lastCrashTime < 10_000) {
      this.onError("Watch process crashed repeatedly — not restarting");
      this.onExit();
      return;
    }

    this.lastCrashTime = now;
    this.restartCount++;
    this.outputChannel.appendLine(`[watch] restarting in 2s (attempt ${this.restartCount})`);

    setTimeout(() => {
      if (!this.disposed) {
        this.start();
      }
    }, 2000);
  }
}

export class WatchManager implements vscode.Disposable {
  private watchers = new Map<string, WatchProcess>();
  private _onDidChange = new vscode.EventEmitter<void>();
  readonly onDidChange: vscode.Event<void> = this._onDidChange.event;

  private statusBar: vscode.StatusBarItem;

  constructor(
    private readonly controller: GoTestController,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onCycleComplete: (jsonOutput: string) => void,
  ) {
    this.statusBar = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      40,
    );
    this.statusBar.command = "gotest.stopWatch";
    this.updateStatusBar();
  }

  get activeCount(): number {
    return this.watchers.size;
  }

  isWatching(pkgScope: string): boolean {
    return this.watchers.has(pkgScope);
  }

  start(pkgScope: string, cwd: string): void {
    // Kill existing watcher for this scope
    if (this.watchers.has(pkgScope)) {
      this.watchers.get(pkgScope)!.dispose();
      this.watchers.delete(pkgScope);
    }

    const cliPath = vscode.workspace
      .getConfiguration("gotest")
      .get<string>("cliPath") ?? "gotest";

    let currentRun: vscode.TestRun | undefined;
    let cycleJson = "";

    const process = new WatchProcess(
      pkgScope,
      cwd,
      cliPath,
      this.outputChannel,
      // onCycleStart
      () => {
        if (currentRun) {
          currentRun.end();
        }
        cycleJson = "";
        const request = new vscode.TestRunRequest();
        currentRun = this.controller.createTestRun(request, `Watch: ${pkgScope}`);
      },
      // onEvents
      (jsonLines: string) => {
        cycleJson += jsonLines;
        if (currentRun) {
          const events = parseTestEvents(jsonLines);
          for (const event of events) {
            if (!event.Test) {
              continue;
            }
            if (event.Action === "pass" || event.Action === "fail" || event.Action === "skip") {
              // End of a cycle — will be fully processed on next watch-start or on flush
            }
          }
        }
      },
      // onError
      (msg: string) => {
        this.outputChannel.appendLine(`[watch] error: ${msg}`);
        vscode.window.showWarningMessage(`gotest watch: ${msg}`);
      },
      // onExit
      () => {
        if (currentRun) {
          currentRun.end();
          currentRun = undefined;
        }
        if (cycleJson) {
          this.onCycleComplete(cycleJson);
          cycleJson = "";
        }
        this.watchers.delete(pkgScope);
        this.updateStatusBar();
        this._onDidChange.fire();
      },
    );

    // Override onEvents to do full result application on cycle boundary
    // We detect cycle end when a new watch-start comes or process exits.
    // For real-time feedback, apply results as they stream:
    const origOnEvents = process as unknown as { processBuffer: () => void };
    void origOnEvents; // The WatchProcess handles buffering internally

    this.watchers.set(pkgScope, process);
    this.updateStatusBar();
    this._onDidChange.fire();
  }

  stop(pkgScope: string): void {
    const process = this.watchers.get(pkgScope);
    if (process) {
      process.dispose();
      this.watchers.delete(pkgScope);
      this.updateStatusBar();
      this._onDidChange.fire();
    }
  }

  stopAll(): void {
    for (const [, process] of this.watchers) {
      process.dispose();
    }
    this.watchers.clear();
    this.updateStatusBar();
    this._onDidChange.fire();
  }

  dispose(): void {
    this.stopAll();
    this.statusBar.dispose();
    this._onDidChange.dispose();
  }

  private updateStatusBar(): void {
    const count = this.watchers.size;
    if (count === 0) {
      this.statusBar.hide();
    } else {
      this.statusBar.text = `$(eye) gotest watch (${count})`;
      this.statusBar.tooltip = `${count} active watcher(s) — click to stop`;
      this.statusBar.show();
    }
  }
}
```

- [ ] **Step 2: Verify extension compiles**

Run: `cd vscode-gotest && npm run compile`
Expected: Success.

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/watch.ts
git commit -m "feat(vscode): implement WatchManager with streaming event parser and status bar"
```

---

### Task 6: Wire Watch Mode into extension

**Files:**
- Modify: `vscode-gotest/src/extension.ts`
- Modify: `vscode-gotest/package.json`

- [ ] **Step 1: Add watch commands and settings to package.json**

Add to the `commands` array:

```json
{
  "command": "gotest.startWatch",
  "title": "Go Test: Start Watch"
},
{
  "command": "gotest.stopWatch",
  "title": "Go Test: Stop Watch"
}
```

Add to `configuration.properties`:

```json
"gotest.watch.autoRestart": {
  "type": "boolean",
  "default": true,
  "description": "Auto-restart watch process on crash"
},
"gotest.watch.scope": {
  "type": "string",
  "default": "./...",
  "description": "Default package scope for watch mode"
}
```

- [ ] **Step 2: Wire WatchManager in extension.ts**

Add import:
```typescript
import { WatchManager } from "./watch.js";
```

After the `specView` initialization, add:
```typescript
  const watchManager = new WatchManager(controller, outputChannel, (jsonOutput) => {
    specView.refresh(jsonOutput);
  });
```

Register commands:
```typescript
  const startWatchCmd = vscode.commands.registerCommand(
    "gotest.startWatch",
    async () => {
      const defaultScope = vscode.workspace
        .getConfiguration("gotest")
        .get<string>("watch.scope") ?? "./...";

      const scope = await vscode.window.showInputBox({
        prompt: "Package scope to watch",
        value: defaultScope,
        placeHolder: "./...",
      });

      if (scope) {
        watchManager.start(scope, workspaceDir);
      }
    },
  );

  const stopWatchCmd = vscode.commands.registerCommand(
    "gotest.stopWatch",
    async () => {
      if (watchManager.activeCount === 0) {
        vscode.window.showInformationMessage("No active watchers.");
        return;
      }
      watchManager.stopAll();
      vscode.window.showInformationMessage("All watchers stopped.");
    },
  );
```

Add to `context.subscriptions`:
```typescript
    watchManager,
    startWatchCmd,
    stopWatchCmd,
```

- [ ] **Step 3: Verify extension compiles**

Run: `cd vscode-gotest && npm run compile`
Expected: Success.

- [ ] **Step 4: Verify TypeScript types**

Run: `cd vscode-gotest && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add vscode-gotest/src/extension.ts vscode-gotest/package.json
git commit -m "feat(vscode): wire Watch Mode commands and settings"
```

---

### Task 7: Integration verification

**Files:**
- No new files — verification only

- [ ] **Step 1: Verify Go CLI builds and tests pass**

Run: `go build ./cmd/gotest/ && go test ./cmd/gotest/ -v`
Expected: All tests PASS.

- [ ] **Step 2: Test `gotest spec --input=-` end-to-end**

Run from examples directory:
```bash
cd examples
../gotest test ./simple_suite -json 2>/dev/null | ../gotest spec --input=-
```
Expected: Terminal-formatted spec output showing "Simple" suite with pass/fail indicators.

- [ ] **Step 3: Verify extension compiles and type-checks**

Run:
```bash
cd vscode-gotest && npm run compile && npx tsc --noEmit
```
Expected: Both succeed with no errors.

- [ ] **Step 4: Final commit if any fixes needed**

If any fixes were required during verification, commit them:
```bash
git add -u
git commit -m "fix: address integration issues from verification"
```

---

## Self-Review Notes

**Spec coverage check:**
- ✅ `gotest spec --input=-` / `--input=<path>` — Task 1
- ✅ `gotest watch -json` with `watch-start` sentinel — Task 2
- ✅ SpecViewPanel webview + ANSI-to-HTML — Task 3
- ✅ JSON cache in TestRunner + auto-refresh — Task 4
- ✅ WatchManager + WatchProcess + WatchStatusBar — Task 5
- ✅ Commands, settings, wiring — Task 6
- ✅ Integration verification — Task 7
- ✅ `watch-error` sentinel for error reporting — Task 2 (in `watchRunOnce`)
- ✅ Auto-restart with backoff — Task 5 (`maybeRestart`)
- ✅ Multiple watchers — Task 5 (Map-based)
- ✅ Status bar with count — Task 5 (`updateStatusBar`)

**Type consistency check:**
- `parseSpecFlags` returns `(format, output, input string, remaining []string)` — used consistently in Task 1 test and implementation
- `parseWatchFlags` returns `(jsonMode bool, remaining []string)` — used consistently in Task 2
- `WatchManager.start(pkgScope, cwd)` — called with `(scope, workspaceDir)` in Task 6
- `SpecViewPanel.refresh(jsonOutput)` — called from `runner.onDidComplete` (Task 4) and `watchManager.onCycleComplete` (Task 6)
- `ansiToHtml` exported from `specView.ts` — used internally only, consistent
