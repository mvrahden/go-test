# vscode-gotest v2.1: Scaffold Integration & Coverage Gutters

## Overview

Two features completing the toolchain integration: scaffold test suite skeletons from within the editor, and render coverage gutters via a native Coverage run profile.

## Goals

- Scaffold test suites from struct types and from file-scoped package functions without leaving VS Code
- Render line-level coverage highlights using VS Code's built-in coverage API
- No new CLI subcommands — scaffold already exists, coverage uses standard `go test -coverprofile`

## Non-Goals

- Scaffold for unexported types or functions
- Branch coverage (Go only supports statement coverage)
- Coverage merging across multiple runs

---

## Feature 1: Scaffold Integration

### CLI Change

`gotest scaffold` gains a new target format: `./pkg/file.go`. When the target ends in `.go`, the CLI:

1. Locates the file within the package
2. Introspects all exported package-level functions (no receiver) in that file
3. Generates a test suite skeleton with one `Test<FuncName>` method per function
4. Suite name derived from filename: `pricing.go` → `PricingTestSuite`
5. Writes the file as `<snake_case_filename>_suite_test.go` in the same directory

Existing `./pkg.TypeName` behavior is unchanged.

#### Example

Given `pkg/pricing/calc.go`:
```go
package pricing

func CalculateDiscount(amount float64, tier string) float64 { ... }
func ApplyTax(amount float64, region string) float64 { ... }
func internalHelper() { ... }  // unexported — skipped
```

`gotest scaffold ./pkg/pricing/calc.go` generates `pkg/pricing/calc_suite_test.go`:
```go
package pricing

import "github.com/mvrahden/go-test/pkg/gotest"

type CalcTestSuite struct {
    gotest.TestSuite
}

func (s *CalcTestSuite) TestCalculateDiscount(t *gotest.T) {
    // TODO: implement test
}

func (s *CalcTestSuite) TestApplyTax(t *gotest.T) {
    // TODO: implement test
}
```

### Extension Components

#### ScaffoldCodeActionProvider

Registered for `{ language: "go", pattern: "**/*.go" }` (excludes `_test.go` files in the provider logic).

Two Code Actions, context-dependent:

| Cursor position | Code Action | CLI invocation |
|----------------|-------------|----------------|
| On `type Foo struct` line | "Generate test suite for Foo" | `gotest scaffold ./pkg.Foo` |
| Any line in a non-test `.go` file | "Generate test suite for this file" | `gotest scaffold ./pkg/file.go` |

Both actions appear simultaneously when the cursor is on a struct declaration line.

Detection:
- Struct: regex `/^\s*type\s+([A-Z]\w*)\s+struct/` on the cursor line
- File: always available in non-test `.go` files

Implementation:
- `CodeActionKind.RefactorExtract` (appears in the refactor menu)
- Spawns `gotest scaffold <target>` with cwd set to workspace root
- Parses stdout for the generated file path (`Generated: <path>`)
- Opens the generated file in the editor
- Triggers discovery refresh

#### Command Palette

"Go Test: Scaffold Suite" command (`gotest.scaffold`):
- Prompts for target string via `showInputBox`
- Placeholder: `./pkg/path.TypeName or ./pkg/path/file.go`
- Runs `gotest scaffold <target>`
- Opens generated file, triggers rediscovery

### Post-Scaffold Behavior

1. Extension spawns `gotest scaffold <target>` in workspace root
2. CLI writes the file, prints `Generated: <relative-path>` to stdout
3. Extension parses the path from stdout
4. Opens the generated file via `vscode.window.showTextDocument`
5. Calls `discoveryService.discover(workspaceDir)` to refresh the Test Explorer tree

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Type not found | Show CLI error in notification |
| File already exists | CLI overwrites (existing behavior) — extension shows the updated file |
| No exported functions in file | CLI generates an empty suite (valid but minimal) |
| CLI not found | Show error notification with install instructions |

---

## Feature 2: Coverage Gutters

### Architecture

```
User selects "Run with Coverage" profile
    → CoverageRunner.run(request, token)
        → gotest overlay ./pkg → overlay.json
        → go test -overlay=... -coverprofile=<tmp> -json ./pkg
        → Parse JSON events → update TestItems (same as Run)
        → Parse coverprofile → FileCoverage[] → run.addCoverage()
        → VS Code renders green/red gutters
```

### Run Profile

A third profile registered on the TestController:

```typescript
controller.createRunProfile(
  "Coverage",
  vscode.TestRunProfileKind.Coverage,
  (request, token) => coverageRunner.run(request, token),
  false, // not default
);
```

This adds "Run with Coverage" to the Test Explorer dropdown automatically.

### CoverageRunner

New class in `vscode-gotest/src/coverage.ts`:

```typescript
export class CoverageRunner {
  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onJsonOutput: (json: string) => void,
  ) {}

  async run(request: vscode.TestRunRequest, token: vscode.CancellationToken): Promise<void>;
}
```

**`run()` flow:**

1. Collect test items from request (same as TestRunner)
2. Group by package
3. For each package:
   a. Generate overlay: spawn `gotest overlay ./pkg`, parse JSON for overlay path
   b. Create temp file for coverprofile
   c. Spawn `go test -overlay=<overlay.json> -coverprofile=<tmpfile> -json <pkgDir>` with optional `-run` filter
   d. Parse JSON stdout for test results (reuse `parseTestEvents` + apply to TestRun)
   e. Parse coverprofile file into `vscode.FileCoverage[]`
   f. Call `run.addCoverage(fileCoverage)` for each file
   g. Clean up temp coverprofile and overlay dir
4. End the TestRun

### Coverprofile Parsing

Go's coverprofile format:
```
mode: set
github.com/mvrahden/go-test/examples/simple_suite/ptest.go:10.33,12.2 1 1
github.com/mvrahden/go-test/examples/simple_suite/ptest.go:14.29,16.2 1 0
```

Each data line: `file:startLine.startCol,endLine.endCol numStatements count`

Parser function:
```typescript
export function parseCoverProfile(
  content: string,
  pkgDir: string,
): vscode.FileCoverage[] 
```

- Skip the `mode:` header line
- Group entries by file path
- For each file, create `vscode.FileCoverage` with `StatementCoverage` entries
- Convert import paths to absolute file paths using `pkgDir`
- `count > 0` → covered, `count === 0` → uncovered

### Overlay Integration

Coverage runs need the overlay because go-test suites require code generation. The CoverageRunner:

1. Spawns `gotest overlay <pkgDir>` to get the overlay JSON path
2. Passes `-overlay=<path>` to `go test`
3. Cleans up the overlay temp dir after the test run

This is the same pattern as `DebugLauncher` — spawn overlay, use it, clean up.

### Lifecycle

- Coverage results are attached to the TestRun and managed by VS Code
- VS Code shows/hides gutters based on user interaction (standard behavior)
- Temp files (coverprofile, overlay dir) cleaned up in `finally` block
- Cancellation kills the `go test` process and cleans up

### Configuration

No new settings needed. The Coverage profile uses existing `gotest.cliPath` and `gotest.testFlags` settings. Users can add flags like `-covermode=atomic` via `gotest.testFlags`.

---

## File Structure

### Go CLI
- Modify: `internal/scaffold/scaffold.go` — add file-scoped scaffolding (parse `.go` target, introspect file functions)
- Test: `internal/scaffold/scaffold_test.go` — test file-scoped mode

### VS Code Extension
- Create: `vscode-gotest/src/scaffold.ts` — ScaffoldCodeActionProvider + scaffold command
- Create: `vscode-gotest/src/coverage.ts` — CoverageRunner + coverprofile parser
- Modify: `vscode-gotest/src/testController.ts` — add Coverage run profile
- Modify: `vscode-gotest/src/extension.ts` — wire scaffold and coverage components
- Modify: `vscode-gotest/package.json` — add scaffold command

---

## Extension Configuration (additions to package.json)

### Commands

```json
{
  "command": "gotest.scaffold",
  "title": "Go Test: Scaffold Suite"
}
```

### Menus (Code Actions)

Code Actions are registered programmatically via `registerCodeActionsProvider`, not via package.json menus.

---

## Testing Strategy

### CLI
- `TestParseTarget_FileTarget`: verify `ParseTarget` handles `./pkg/file.go` format
- `TestIntrospectFile`: verify file-scoped introspection finds exported functions, skips unexported
- `TestGenerateScaffold_File`: verify generated output contains correct suite name and test methods

### Extension (compile-time)
- `parseCoverProfile()`: verify parsing of coverprofile format into FileCoverage entries
- Type-check all new code via `tsc --noEmit`
