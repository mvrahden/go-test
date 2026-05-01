# vscode-gotest v2.1: Scaffold Integration & Coverage Gutters — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add file-scoped scaffold generation to the Go CLI, expose scaffold via Code Actions + Command Palette in VS Code, and add a Coverage run profile that renders line-level coverage gutters.

**Architecture:** The CLI gains `IntrospectFile()` and a file-scoped template for `gotest scaffold ./pkg/file.go`. The extension adds `ScaffoldCodeActionProvider` (Code Actions on struct lines and any line in non-test `.go` files) plus a `gotest.scaffold` command. A new `CoverageRunner` spawns `go test -coverprofile` with overlay, parses the coverprofile, and calls `run.addCoverage()` to drive VS Code's built-in coverage gutters.

**Tech Stack:** Go (go/ast, go/types, x/tools/go/packages), TypeScript, VS Code Testing API (FileCoverage, StatementCoverage, TestRunProfileKind.Coverage)

---

## File Structure

### Go CLI
- Modify: `internal/scaffold/scaffold.go` — add `FileInfo`, `IntrospectFile()`, `GenerateFileScaffold()`, file-scoped template
- Modify: `internal/scaffold/scaffold_test.go` — add tests for file-scoped mode
- Create: `internal/scaffold/testdata/sampletype/funcs.go` — exported package functions for testing
- Create: `internal/scaffold/testdata/funcs_suite_test.go.golden` — golden file for file-scoped scaffold
- Modify: `cmd/gotest/scaffold.go` — detect `.go` suffix, branch to file-scoped mode
- Modify: `cmd/gotest/cli.go` — update usage text

### VS Code Extension
- Create: `vscode-gotest/src/scaffold.ts` — `ScaffoldCodeActionProvider` + scaffold command handler
- Create: `vscode-gotest/src/coverage.ts` — `CoverageRunner` + `parseCoverProfile()`
- Modify: `vscode-gotest/src/testController.ts` — add Coverage run profile
- Modify: `vscode-gotest/src/extension.ts` — wire scaffold provider, scaffold command, CoverageRunner
- Modify: `vscode-gotest/package.json` — add `gotest.scaffold` command

---

### Task 1: Add testdata for file-scoped scaffold

**Files:**
- Create: `internal/scaffold/testdata/sampletype/funcs.go`
- Create: `internal/scaffold/testdata/funcs_suite_test.go.golden`

- [ ] **Step 1: Create the sample functions file**

Create `internal/scaffold/testdata/sampletype/funcs.go` with exported package-level functions (no receiver) for testing file-scoped scaffold:

```go
package sampletype

func CalculateDiscount(amount float64, tier string) float64 { return 0 }

func ApplyTax(amount float64, region string) float64 { return 0 }

func internalHelper() {}
```

- [ ] **Step 2: Create the golden file**

Create `internal/scaffold/testdata/funcs_suite_test.go.golden` with the expected output. This is the file-scoped template: no `sut` field, no `BeforeEach`, suite name derived from filename (`funcs.go` → `FuncsTestSuite`), one `Test<FuncName>` method per exported function:

```go
package sampletype

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type FuncsTestSuite struct {
	gotest.TestSuite
}

func (s *FuncsTestSuite) TestApplyTax(t *gotest.T) {
	t.It("works", func(it *gotest.T) {
		// TODO: test ApplyTax(amount float64, region string) float64
	})
}

func (s *FuncsTestSuite) TestCalculateDiscount(t *gotest.T) {
	t.It("works", func(it *gotest.T) {
		// TODO: test CalculateDiscount(amount float64, tier string) float64
	})
}
```

- [ ] **Step 3: Verify the testdata compiles**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go build ./internal/scaffold/testdata/sampletype/`
Expected: Success (no output)

- [ ] **Step 4: Commit**

```bash
git add internal/scaffold/testdata/sampletype/funcs.go internal/scaffold/testdata/funcs_suite_test.go.golden
git commit -m "test(scaffold): add testdata for file-scoped scaffold"
```

---

### Task 2: Add `FileInfo` type, `IntrospectFile()`, and file-scoped template

**Files:**
- Modify: `internal/scaffold/scaffold.go`

- [ ] **Step 1: Write failing tests for `IntrospectFile`**

Add to `internal/scaffold/scaffold_test.go`:

```go
func TestIntrospectFile_Funcs(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "funcs.go")
	if err != nil {
		t.Fatalf("IntrospectFile failed: %v", err)
	}

	if info.SuiteName != "FuncsTestSuite" {
		t.Errorf("SuiteName: want %q, got %q", "FuncsTestSuite", info.SuiteName)
	}
	if info.PkgName != "sampletype" {
		t.Errorf("PkgName: want %q, got %q", "sampletype", info.PkgName)
	}
	if info.PkgDir == "" {
		t.Error("PkgDir should not be empty")
	}

	// Should have exactly 2 exported functions: ApplyTax, CalculateDiscount (sorted)
	if len(info.Funcs) != 2 {
		t.Fatalf("expected 2 funcs, got %d: %+v", len(info.Funcs), info.Funcs)
	}

	wantNames := []string{"ApplyTax", "CalculateDiscount"}
	for i, want := range wantNames {
		if info.Funcs[i].Name != want {
			t.Errorf("func[%d]: want %q, got %q", i, want, info.Funcs[i].Name)
		}
	}
}

func TestIntrospectFile_NoExported(t *testing.T) {
	// Create a scenario where file has no exported functions
	// The existing types.go has no package-level exported functions (only methods)
	info, err := IntrospectFile("./testdata/sampletype", "types.go")
	if err != nil {
		t.Fatalf("IntrospectFile failed: %v", err)
	}

	if len(info.Funcs) != 1 {
		t.Fatalf("expected 1 exported func (NewUserService), got %d: %+v", len(info.Funcs), info.Funcs)
	}
	if info.Funcs[0].Name != "NewUserService" {
		t.Errorf("func[0]: want %q, got %q", "NewUserService", info.Funcs[0].Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -run TestIntrospectFile -v`
Expected: FAIL — `IntrospectFile` undefined

- [ ] **Step 3: Add `FileInfo` type and `IntrospectFile()` to `scaffold.go`**

Add the `FileInfo` struct and `FuncInfo` struct after the existing `MethodInfo` struct (around line 29):

```go
// FuncInfo describes an exported package-level function.
type FuncInfo struct {
	Name      string
	Signature string
}

// FileInfo describes exported package-level functions in a single file.
type FileInfo struct {
	SuiteName string
	PkgName   string
	PkgDir    string
	Funcs     []FuncInfo
}
```

Add the `IntrospectFile` function after `IntrospectType` (around line 129):

```go
// IntrospectFile loads a package and extracts exported package-level functions
// (no receiver) from a specific file. The filename must be a base name (e.g. "calc.go").
func IntrospectFile(pkgPattern, filename string) (*FileInfo, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedFiles,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %q: %w", pkgPattern, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for pattern %q", pkgPattern)
	}

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("package %q has errors: %v", pkgPattern, pkg.Errors[0])
		}
	}

	pkg := pkgs[0]

	// Find the syntax file matching filename
	var targetFileIndex int = -1
	for i, f := range pkg.GoFiles {
		base := f[strings.LastIndex(f, "/")+1:]
		if base == filename {
			targetFileIndex = i
			break
		}
	}
	if targetFileIndex == -1 {
		return nil, fmt.Errorf("file %q not found in package %q", filename, pkgPattern)
	}

	// Walk the AST of the target file to find exported package-level function declarations
	astFile := pkg.Syntax[targetFileIndex]
	scope := pkg.Types.Scope()

	var funcs []FuncInfo
	for _, decl := range astFile.Decls {
		fnDecl, ok := decl.(*ast.FuncDecl)
		if !ok || fnDecl.Recv != nil {
			continue
		}
		name := fnDecl.Name.Name
		if !ast.IsExported(name) {
			continue
		}
		obj := scope.Lookup(name)
		if obj == nil {
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig := fn.Type().(*types.Signature)
		funcs = append(funcs, FuncInfo{
			Name:      name,
			Signature: formatSignature(sig),
		})
	}

	sort.Slice(funcs, func(i, j int) bool {
		return funcs[i].Name < funcs[j].Name
	})

	// Derive suite name from filename: "calc.go" → "CalcTestSuite"
	base := strings.TrimSuffix(filename, ".go")
	suiteName := toPascalCase(base) + "TestSuite"

	return &FileInfo{
		SuiteName: suiteName,
		PkgName:   pkg.Name,
		PkgDir:    determinePkgDir(pkg),
		Funcs:     funcs,
	}, nil
}

func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		result.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return result.String()
}
```

Add import `"go/ast"` to the imports block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -run TestIntrospectFile -v`
Expected: PASS

- [ ] **Step 5: Write failing test for `GenerateFileScaffold`**

Add to `internal/scaffold/scaffold_test.go`:

```go
func TestGenerateFileScaffold(t *testing.T) {
	info := &FileInfo{
		SuiteName: "CalcTestSuite",
		PkgName:   "pricing",
		Funcs: []FuncInfo{
			{Name: "ApplyTax", Signature: "(amount float64, region string) float64"},
			{Name: "CalculateDiscount", Signature: "(amount float64, tier string) float64"},
		},
	}

	out, err := GenerateFileScaffold(info)
	if err != nil {
		t.Fatalf("GenerateFileScaffold failed: %v", err)
	}

	src := string(out)

	if !strings.Contains(src, "package pricing") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(src, `"github.com/mvrahden/go-test/pkg/gotest"`) {
		t.Error("missing gotest import")
	}
	if !strings.Contains(src, "type CalcTestSuite struct") {
		t.Error("missing test suite struct")
	}
	if !strings.Contains(src, "gotest.TestSuite") {
		t.Error("missing embedded TestSuite")
	}
	if strings.Contains(src, "sut") {
		t.Error("file-scoped scaffold should NOT have sut field")
	}
	if strings.Contains(src, "BeforeEach") {
		t.Error("file-scoped scaffold should NOT have BeforeEach")
	}
	if !strings.Contains(src, "func (s *CalcTestSuite) TestApplyTax(t *gotest.T)") {
		t.Error("missing TestApplyTax method")
	}
	if !strings.Contains(src, "func (s *CalcTestSuite) TestCalculateDiscount(t *gotest.T)") {
		t.Error("missing TestCalculateDiscount method")
	}
	if !strings.Contains(src, "// TODO: test CalculateDiscount(amount float64, tier string) float64") {
		t.Error("missing TODO with signature")
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -run TestGenerateFileScaffold -v`
Expected: FAIL — `GenerateFileScaffold` undefined

- [ ] **Step 7: Add file-scoped template and `GenerateFileScaffold()` to `scaffold.go`**

Add the template after the existing `contractTemplate` (around line 332):

```go
var fileTemplate = template.Must(template.New("file").Parse(`package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.SuiteName}} struct {
	gotest.TestSuite
}
{{range .Funcs}}
func (s *{{$.SuiteName}}) Test{{.Name}}(t *gotest.T) {
	t.It("works", func(it *gotest.T) {
		// TODO: test {{.Name}}{{.Signature}}
	})
}
{{end}}`))

// GenerateFileScaffold generates a test suite scaffold for package-level functions.
func GenerateFileScaffold(info *FileInfo) ([]byte, error) {
	var buf strings.Builder
	if err := fileTemplate.Execute(&buf, info); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return nil, fmt.Errorf("go/format failed: %w", err)
	}

	return formatted, nil
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -run "TestGenerateFileScaffold|TestIntrospectFile" -v`
Expected: PASS

- [ ] **Step 9: Write and run the golden file integration test**

Add to `internal/scaffold/scaffold_test.go`:

```go
func TestScaffoldIntegration_File(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "funcs.go")
	if err != nil {
		t.Fatalf("IntrospectFile failed: %v", err)
	}

	out, err := GenerateFileScaffold(info)
	if err != nil {
		t.Fatalf("GenerateFileScaffold failed: %v", err)
	}

	goldenPath := filepath.Join("testdata", "funcs_suite_test.go.golden")

	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, out, 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Log("golden file updated")
		return
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found (run with UPDATE_GOLDEN=1 to create): %v", err)
	}

	if string(out) != string(golden) {
		t.Errorf("output does not match golden file.\n--- got ---\n%s\n--- want ---\n%s", string(out), string(golden))
	}
}
```

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && UPDATE_GOLDEN=1 go test ./internal/scaffold/ -run TestScaffoldIntegration_File -v`
Expected: PASS (golden file updated)

Then verify without UPDATE_GOLDEN:
Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -run TestScaffoldIntegration_File -v`
Expected: PASS

- [ ] **Step 10: Run all scaffold tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -v`
Expected: All tests PASS

- [ ] **Step 11: Commit**

```bash
git add internal/scaffold/scaffold.go internal/scaffold/scaffold_test.go internal/scaffold/testdata/funcs_suite_test.go.golden
git commit -m "feat(scaffold): add IntrospectFile and file-scoped template"
```

---

### Task 3: Wire file-scoped scaffold into the CLI

**Files:**
- Modify: `cmd/gotest/scaffold.go`
- Modify: `cmd/gotest/cli.go`

- [ ] **Step 1: Modify `runScaffold` to detect `.go` suffix and branch**

Replace the contents of `cmd/gotest/scaffold.go` with:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/scaffold"
)

func runScaffold(args []string) int {
	var target string
	for _, arg := range args {
		if !isFlag(arg) {
			target = arg
			break
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: gotest scaffold <./pkg/path.TypeName | ./pkg/path/file.go>")
		return 1
	}

	if strings.HasSuffix(target, ".go") {
		return runScaffoldFile(target)
	}
	return runScaffoldType(target)
}

func runScaffoldType(target string) int {
	pkgPattern, typeName, err := scaffold.ParseTarget(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	info, err := scaffold.IntrospectType(pkgPattern, typeName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	var out []byte
	if info.IsInterface {
		out, err = scaffold.GenerateContractScaffold(info)
	} else {
		out, err = scaffold.GenerateScaffold(info)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	filename := scaffold.ToSnakeCase(typeName) + "_suite_test.go"
	return writeScaffoldFile(info.PkgDir, filename, out)
}

func runScaffoldFile(target string) int {
	// Split "./pkg/path/file.go" into package pattern and filename
	dir := filepath.Dir(target)
	filename := filepath.Base(target)
	pkgPattern := "./" + filepath.ToSlash(dir)

	info, err := scaffold.IntrospectFile(pkgPattern, filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	out, err := scaffold.GenerateFileScaffold(info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	outFilename := scaffold.ToSnakeCase(strings.TrimSuffix(filename, ".go")) + "_suite_test.go"
	return writeScaffoldFile(info.PkgDir, outFilename, out)
}

func writeScaffoldFile(pkgDir, filename string, content []byte) int {
	outPath := filepath.Join(pkgDir, filename)

	if err := os.WriteFile(outPath, content, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: failed to write %s: %v\n", outPath, err)
		return 1
	}

	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		if rel, err := filepath.Rel(cwd, outPath); err == nil {
			fmt.Printf("Generated: %s\n", rel)
			return 0
		}
	}
	fmt.Printf("Generated: %s\n", outPath)
	return 0
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
```

- [ ] **Step 2: Update usage text in `cli.go`**

In `cmd/gotest/cli.go`, change the `scaffold` line in `printUsage()` from:

```
  scaffold    Generate test suite skeleton from a Go type
```

to:

```
  scaffold    Generate test suite skeleton from a type or file
```

- [ ] **Step 3: Build to verify compilation**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go build ./cmd/gotest/`
Expected: Success

- [ ] **Step 4: Run all tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./internal/scaffold/ -v && go test ./cmd/gotest/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/gotest/scaffold.go cmd/gotest/cli.go
git commit -m "feat(scaffold): wire file-scoped mode into CLI"
```

---

### Task 4: ScaffoldCodeActionProvider and scaffold command

**Files:**
- Create: `vscode-gotest/src/scaffold.ts`

- [ ] **Step 1: Create `scaffold.ts` with `ScaffoldCodeActionProvider`**

```typescript
import * as vscode from "vscode";
import { spawn } from "node:child_process";

export class ScaffoldCodeActionProvider implements vscode.CodeActionProvider, vscode.Disposable {
  static readonly providedCodeActionKinds = [vscode.CodeActionKind.RefactorExtract];

  private static readonly structPattern = /^\s*type\s+([A-Z]\w*)\s+struct/;

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range | vscode.Selection,
  ): vscode.CodeAction[] | undefined {
    if (document.fileName.endsWith("_test.go")) {
      return undefined;
    }

    const actions: vscode.CodeAction[] = [];
    const line = document.lineAt(range.start.line);
    const structMatch = ScaffoldCodeActionProvider.structPattern.exec(line.text);

    if (structMatch) {
      const typeName = structMatch[1];
      const action = new vscode.CodeAction(
        `Generate test suite for ${typeName}`,
        vscode.CodeActionKind.RefactorExtract,
      );
      action.command = {
        command: "gotest.scaffoldTarget",
        title: `Scaffold ${typeName}`,
        arguments: [this.buildTypeTarget(document, typeName)],
      };
      actions.push(action);
    }

    const fileAction = new vscode.CodeAction(
      "Generate test suite for this file",
      vscode.CodeActionKind.RefactorExtract,
    );
    fileAction.command = {
      command: "gotest.scaffoldTarget",
      title: "Scaffold file",
      arguments: [this.buildFileTarget(document)],
    };
    actions.push(fileAction);

    return actions;
  }

  dispose(): void {}

  private buildTypeTarget(document: vscode.TextDocument, typeName: string): string {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!workspaceFolder) {
      return "";
    }
    const relDir = document.uri.fsPath
      .slice(workspaceFolder.uri.fsPath.length + 1)
      .replace(/\/[^/]+$/, "");
    return `./${relDir}.${typeName}`;
  }

  private buildFileTarget(document: vscode.TextDocument): string {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!workspaceFolder) {
      return "";
    }
    const relPath = document.uri.fsPath.slice(workspaceFolder.uri.fsPath.length + 1);
    return `./${relPath}`;
  }
}

export async function runScaffoldCommand(
  outputChannel: vscode.OutputChannel,
  discoverCallback: () => void,
): Promise<void> {
  const target = await vscode.window.showInputBox({
    prompt: "Scaffold target",
    placeHolder: "./pkg/path.TypeName or ./pkg/path/file.go",
  });

  if (!target) {
    return;
  }

  await executeScaffold(target, outputChannel, discoverCallback);
}

export async function executeScaffold(
  target: string,
  outputChannel: vscode.OutputChannel,
  discoverCallback: () => void,
): Promise<void> {
  const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!workspaceDir) {
    return;
  }

  const cliPath =
    vscode.workspace.getConfiguration("gotest").get<string>("cliPath") ?? "gotest";

  outputChannel.appendLine(`[scaffold] ${cliPath} scaffold ${target}`);

  try {
    const stdout = await spawnScaffold(cliPath, target, workspaceDir);
    const match = /^Generated:\s*(.+)$/m.exec(stdout);
    if (match) {
      const generatedPath = match[1];
      const fullPath = generatedPath.startsWith("/")
        ? generatedPath
        : `${workspaceDir}/${generatedPath}`;
      const doc = await vscode.workspace.openTextDocument(fullPath);
      await vscode.window.showTextDocument(doc);
    }
    discoverCallback();
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(`gotest scaffold failed: ${message}`);
  }
}

function spawnScaffold(cliPath: string, target: string, cwd: string): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const child = spawn(cliPath, ["scaffold", target], { cwd });
    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
    });

    child.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
    });

    child.on("close", (code) => {
      if (code !== 0) {
        reject(new Error(stderr || `scaffold exited with code ${code}`));
      } else {
        resolve(stdout);
      }
    });

    child.on("error", (err: Error) => {
      reject(err);
    });
  });
}
```

- [ ] **Step 2: Verify TypeScript compilation**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npx tsc --noEmit`
Expected: No type errors (or only pre-existing ones)

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/scaffold.ts
git commit -m "feat(extension): add ScaffoldCodeActionProvider and scaffold command"
```

---

### Task 5: CoverageRunner and coverprofile parser

**Files:**
- Create: `vscode-gotest/src/coverage.ts`

- [ ] **Step 1: Create `coverage.ts` with `parseCoverProfile` and `CoverageRunner`**

```typescript
import * as vscode from "vscode";
import * as path from "node:path";
import { spawn } from "node:child_process";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { writeFile, rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { OverlayOutput } from "./types.js";
import {
  parseTestEvents,
  extractTestMessages,
  type TestEvent,
} from "./outputParser.js";

const execFileAsync = promisify(execFile);

export function parseCoverProfile(
  content: string,
  moduleToDir: (importPath: string) => string | undefined,
): vscode.FileCoverage[] {
  const lines = content.split("\n");
  const fileEntries = new Map<string, vscode.StatementCoverage[]>();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("mode:")) {
      continue;
    }

    // Format: file:startLine.startCol,endLine.endCol numStatements count
    const match = /^(.+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$/.exec(trimmed);
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
    statements.push(new vscode.StatementCoverage(count, range));
  }

  const result: vscode.FileCoverage[] = [];
  for (const [importFilePath, statements] of fileEntries) {
    // importFilePath is like "github.com/user/repo/pkg/file.go"
    // We need to resolve to absolute path
    const lastSlash = importFilePath.lastIndexOf("/");
    const fileName = importFilePath.slice(lastSlash + 1);
    const importDir = importFilePath.slice(0, lastSlash);
    const absDir = moduleToDir(importDir);
    if (!absDir) {
      continue;
    }
    const absPath = path.join(absDir, fileName);
    const uri = vscode.Uri.file(absPath);

    const covered = statements.filter((s) => s.executionCount > 0).length;
    const total = statements.length;

    const fileCoverage = new vscode.FileCoverage(
      uri,
      new vscode.TestCoverageCount(covered, total),
    );
    fileCoverage.detailedCoverage = statements;
    result.push(fileCoverage);
  }

  return result;
}

export class CoverageRunner implements vscode.Disposable {
  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onJsonOutput: (json: string) => void,
  ) {}

  async run(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
    const run = this.controller.createTestRun(request, "Go Test Coverage");

    try {
      const items = this.collectItems(request);
      if (items.length === 0) {
        run.end();
        return;
      }

      for (const item of items) {
        run.started(item);
      }

      const groups = this.groupByPackage(items);
      let allJsonOutput = "";

      for (const [importPath, groupItems] of groups) {
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

        const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
        if (!workspaceDir) {
          continue;
        }

        const cliPath =
          vscode.workspace.getConfiguration("gotest").get<string>("cliPath") ?? "gotest";
        const testFlags =
          vscode.workspace.getConfiguration("gotest").get<string[]>("testFlags") ?? [];

        let overlayDir: string | undefined;
        let coverFile: string | undefined;

        try {
          // Generate overlay
          this.outputChannel.appendLine(`[coverage] ${cliPath} overlay ${pkg.dir}`);
          const { stdout: overlayStdout } = await execFileAsync(
            cliPath,
            ["overlay", pkg.dir],
            { cwd: workspaceDir },
          );
          const overlay = JSON.parse(overlayStdout) as OverlayOutput;
          overlayDir = overlay.dir;

          // Create temp coverprofile
          const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
          coverFile = path.join(tmpDir, "cover.out");

          // Build filter
          const filter = this.buildRunFilter(groupItems, importPath);

          // Spawn go test with overlay and coverprofile
          const args = [
            "test",
            `-overlay=${overlay.overlayFile}`,
            `-coverprofile=${coverFile}`,
            "-json",
            pkg.dir,
          ];
          if (filter) {
            args.push("-run", filter);
          }
          args.push(...testFlags);

          this.outputChannel.appendLine(`[coverage] go ${args.join(" ")}`);
          const stdout = await this.spawnGoTest(args, workspaceDir, token);
          allJsonOutput += stdout;

          if (token.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          // Apply test results
          const events = parseTestEvents(stdout);
          this.applyResults(run, events, importPath, pkg.dir);

          // Parse coverprofile and add coverage
          try {
            const coverContent = await readFile(coverFile, "utf-8");
            const moduleToDir = (importDir: string) => {
              return this.cache.resolveImportPath(importDir);
            };
            const fileCoverages = parseCoverProfile(coverContent, moduleToDir);
            for (const fc of fileCoverages) {
              run.addCoverage(fc);
            }
          } catch {
            this.outputChannel.appendLine("[coverage] no coverprofile generated");
          }
        } finally {
          if (overlayDir) {
            rm(overlayDir, { recursive: true, force: true }).catch(() => {});
          }
          if (coverFile) {
            const coverDir = path.dirname(coverFile);
            rm(coverDir, { recursive: true, force: true }).catch(() => {});
          }
        }
      }

      if (allJsonOutput) {
        this.onJsonOutput(allJsonOutput);
      }
    } finally {
      run.end();
    }
  }

  dispose(): void {}

  private collectItems(request: vscode.TestRunRequest): vscode.TestItem[] {
    const items: vscode.TestItem[] = [];
    if (request.include && request.include.length > 0) {
      for (const item of request.include) {
        items.push(item);
      }
    } else {
      this.controller.testController.items.forEach((item) => {
        items.push(item);
      });
    }
    return items;
  }

  private groupByPackage(items: vscode.TestItem[]): Map<string, vscode.TestItem[]> {
    const groups = new Map<string, vscode.TestItem[]>();
    for (const item of items) {
      const root = this.getRootItem(item);
      let group = groups.get(root.id);
      if (!group) {
        group = [];
        groups.set(root.id, group);
      }
      group.push(item);
    }
    return groups;
  }

  private getRootItem(item: vscode.TestItem): vscode.TestItem {
    let current = item;
    while (current.parent) {
      current = current.parent;
    }
    return current;
  }

  private getItemDepth(item: vscode.TestItem): number {
    let depth = 0;
    let current = item;
    while (current.parent) {
      current = current.parent;
      depth++;
    }
    return depth;
  }

  private buildRunFilter(items: vscode.TestItem[], importPath: string): string | undefined {
    for (const item of items) {
      if (this.getItemDepth(item) === 0) {
        return undefined;
      }
    }

    const item = items[0];
    const depth = this.getItemDepth(item);
    if (depth === 1) {
      return `^Test${item.label}$`;
    }
    if (depth === 2) {
      const suite = item.parent!;
      return `^Test${suite.label}$/^${item.label}$`;
    }
    return undefined;
  }

  private spawnGoTest(
    args: string[],
    cwd: string,
    token: vscode.CancellationToken,
  ): Promise<string> {
    return new Promise<string>((resolve) => {
      const child = spawn("go", args, { cwd });
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

      child.on("close", () => {
        cancelListener.dispose();
        if (stderr) {
          this.outputChannel.appendLine(`[coverage] stderr: ${stderr}`);
        }
        resolve(stdout);
      });

      child.on("error", (err: Error) => {
        cancelListener.dispose();
        this.outputChannel.appendLine(`[coverage] error: ${err.message}`);
        resolve(stdout);
      });
    });
  }

  private applyResults(
    run: vscode.TestRun,
    events: TestEvent[],
    importPath: string,
    pkgDir: string,
  ): void {
    const outputMap = new Map<string, string>();

    for (const event of events) {
      if (event.Action === "output" && event.Test) {
        const existing = outputMap.get(event.Test) ?? "";
        outputMap.set(event.Test, existing + (event.Output ?? ""));
      }
    }

    for (const event of events) {
      if (!event.Test) {
        continue;
      }

      const item = this.resolveTestItem(event.Test, importPath);
      if (!item) {
        continue;
      }

      const duration =
        event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;

      switch (event.Action) {
        case "pass":
          run.passed(item, duration);
          break;
        case "fail": {
          const output = outputMap.get(event.Test) ?? "";
          const testMessages = extractTestMessages(output, pkgDir);
          const vscodeMessages = testMessages.map((msg) => {
            const message = new vscode.TestMessage(msg.message);
            message.location = new vscode.Location(
              vscode.Uri.file(msg.file),
              new vscode.Position(msg.line - 1, 0),
            );
            return message;
          });
          if (vscodeMessages.length === 0) {
            vscodeMessages.push(new vscode.TestMessage(output || "Test failed"));
          }
          run.failed(item, vscodeMessages, duration);
          break;
        }
        case "skip":
          run.skipped(item);
          break;
        case "run":
          run.started(item);
          break;
      }
    }
  }

  private resolveTestItem(
    testPath: string,
    importPath: string,
  ): vscode.TestItem | undefined {
    const segments = testPath.split("/");
    if (segments.length === 0) {
      return undefined;
    }

    const firstSegment = segments[0];
    const suiteName = firstSegment.startsWith("Test")
      ? firstSegment.slice(4)
      : firstSegment;

    const suiteId = `${importPath}/${suiteName}`;
    const suiteItem = this.controller.findItem(suiteId);
    if (!suiteItem) {
      return undefined;
    }

    if (segments.length === 1) {
      return suiteItem;
    }

    const methodName = segments[1];
    const methodId = `${suiteId}/${methodName}`;
    const methodItem = this.controller.findItem(methodId);
    if (!methodItem) {
      return undefined;
    }

    if (segments.length === 2) {
      return methodItem;
    }

    let parentItem = methodItem;
    for (let i = 2; i < segments.length; i++) {
      const subtestLabel = segments[i];
      const subtestPath = segments.slice(2, i + 1).join("/");
      parentItem = this.controller.createDynamicSubtest(
        parentItem,
        subtestPath,
        subtestLabel,
      );
    }

    return parentItem;
  }
}
```

- [ ] **Step 2: Add `resolveImportPath` method to `DiscoveryCache`**

The `parseCoverProfile` function needs to map Go import paths (like `github.com/user/repo/pkg`) to absolute directories. Add to `vscode-gotest/src/discovery.ts` in the `DiscoveryCache` class:

```typescript
resolveImportPath(importPath: string): string | undefined {
  const pkg = this.getPackage(importPath);
  return pkg?.dir;
}
```

Check the existing `DiscoveryCache` class first. If `getPackage` already returns the `dir`, this method is a thin wrapper. If the import path in the coverprofile doesn't match the discovery import path format, the method may need to iterate packages and match by suffix.

- [ ] **Step 3: Verify TypeScript compilation**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npx tsc --noEmit`
Expected: No type errors (or only pre-existing ones)

- [ ] **Step 4: Commit**

```bash
git add vscode-gotest/src/coverage.ts vscode-gotest/src/discovery.ts
git commit -m "feat(extension): add CoverageRunner and coverprofile parser"
```

---

### Task 6: Add Coverage run profile to GoTestController

**Files:**
- Modify: `vscode-gotest/src/testController.ts`

- [ ] **Step 1: Add Coverage run profile**

The `GoTestController` constructor currently accepts `runHandler` and `debugHandler`. Add a third parameter `coverageHandler` with the same signature. Then add:

```typescript
this.controller.createRunProfile(
  "Coverage",
  vscode.TestRunProfileKind.Coverage,
  (request, token) => coverageHandler(request, token),
  false,
);
```

Update the constructor signature:

```typescript
constructor(
  private readonly cache: DiscoveryCache,
  private readonly outputChannel: vscode.OutputChannel,
  runHandler: (
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ) => Promise<void>,
  debugHandler: (
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ) => Promise<void>,
  coverageHandler: (
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ) => Promise<void>,
) {
```

- [ ] **Step 2: Verify TypeScript compilation**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npx tsc --noEmit`
Expected: Will fail because `extension.ts` doesn't pass the coverage handler yet. That's expected — Task 7 will fix it.

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/testController.ts
git commit -m "feat(extension): add Coverage run profile to GoTestController"
```

---

### Task 7: Wire scaffold and coverage into extension.ts and package.json

**Files:**
- Modify: `vscode-gotest/src/extension.ts`
- Modify: `vscode-gotest/package.json`

- [ ] **Step 1: Add scaffold command to `package.json`**

Add to the `"commands"` array in `package.json`:

```json
{
  "command": "gotest.scaffold",
  "title": "Go Test: Scaffold Suite"
}
```

- [ ] **Step 2: Wire everything in `extension.ts`**

Add imports at the top:

```typescript
import { ScaffoldCodeActionProvider, runScaffoldCommand, executeScaffold } from "./scaffold.js";
import { CoverageRunner } from "./coverage.js";
```

Create the `CoverageRunner` after the `runner` is created. Update the `GoTestController` constructor call to pass the coverage handler. Register the scaffold Code Action provider and commands.

Update the controller creation to:

```typescript
let coverageRunner: CoverageRunner;

const controller = new GoTestController(
  cache,
  outputChannel,
  (request, token) => runner.run(request, token),
  (request, token) =>
    debugLauncher.debug(
      request,
      token,
      (items) => buildRunFilter(items),
      (item) => getPackageDir(item, cache),
    ),
  (request, token) => coverageRunner.run(request, token),
);

runner = new TestRunner(controller, cache, outputChannel);
coverageRunner = new CoverageRunner(controller, cache, outputChannel, (jsonOutput) => {
  specView.refresh(jsonOutput);
});
```

Register the scaffold Code Action provider:

```typescript
const scaffoldProvider = new ScaffoldCodeActionProvider();
const scaffoldCodeActionsDisposable = vscode.languages.registerCodeActionsProvider(
  { language: "go", pattern: "**/*.go" },
  scaffoldProvider,
  { providedCodeActionKinds: ScaffoldCodeActionProvider.providedCodeActionKinds },
);
```

Register the scaffold commands:

```typescript
const scaffoldCmd = vscode.commands.registerCommand(
  "gotest.scaffold",
  () => runScaffoldCommand(outputChannel, () => discoveryService.discover(workspaceDir)),
);

const scaffoldTargetCmd = vscode.commands.registerCommand(
  "gotest.scaffoldTarget",
  (target: string) => executeScaffold(target, outputChannel, () => discoveryService.discover(workspaceDir)),
);
```

Add all new disposables to `context.subscriptions`:

```typescript
context.subscriptions.push(
  // ... existing disposables ...
  coverageRunner,
  scaffoldProvider,
  scaffoldCodeActionsDisposable,
  scaffoldCmd,
  scaffoldTargetCmd,
);
```

- [ ] **Step 3: Verify TypeScript compilation**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Build the extension**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npm run compile`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add vscode-gotest/src/extension.ts vscode-gotest/package.json
git commit -m "feat(extension): wire scaffold and coverage into activation"
```

---

### Task 8: Verify full build and all tests pass

**Files:** None (verification only)

- [ ] **Step 1: Run all Go tests**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go test ./... -v`
Expected: All PASS

- [ ] **Step 2: Build Go CLI**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension && go build ./cmd/gotest/`
Expected: Success

- [ ] **Step 3: Build VS Code extension**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npm run compile`
Expected: Success

- [ ] **Step 4: TypeScript type check**

Run: `cd /home/ubuntu/projects/mvrahden/go-test-vscode-extension/vscode-gotest && npx tsc --noEmit`
Expected: No errors
