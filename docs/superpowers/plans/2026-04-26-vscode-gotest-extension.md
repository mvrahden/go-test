# vscode-gotest Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a VS Code extension that provides Test Explorer, CodeLens, and debug integration for go-test suites, powered by two new CLI subcommands (`discover` and `overlay`).

**Architecture:** The Go CLI gains two subcommands that reuse `gotestgen.Collect` and `gotestrunner.WriteOverlay`. The TypeScript extension is a thin UI layer that shells out to `gotest discover` for metadata and `gotest overlay` for debug prep, then wires results into VS Code's TestController, CodeLensProvider, and debug APIs.

**Tech Stack:** Go 1.24 (CLI), TypeScript + VS Code Extension API (extension), esbuild (bundler), `@vscode/test-electron` (testing)

---

## File Structure

### Go CLI (new subcommands)

```
cmd/gotest/
├── discover.go          — `gotest discover` subcommand entry point
├── discover_test.go     — integration test for discover output
├── overlay_cmd.go       — `gotest overlay` subcommand entry point (named to avoid collision with internal/gotestrunner/overlay.go)
└── overlay_cmd_test.go  — integration test for overlay output
```

Note: `cli.go` and `args.go` are modified to register the new subcommands.

### VS Code Extension

```
vscode-gotest/
├── package.json             — extension manifest, contributes, activation events
├── tsconfig.json            — TypeScript config
├── esbuild.config.mjs       — bundle config
├── .vscodeignore            — files to exclude from VSIX
├── src/
│   ├── extension.ts         — activate/deactivate, wire components
│   ├── discovery.ts         — spawn `gotest discover`, parse JSON, cache results
│   ├── types.ts             — DiscoverOutput, SuiteInfo, MethodInfo interfaces
│   ├── testController.ts    — TestController, resolver (cache → TestItems), runner
│   ├── codeLens.ts          — CodeLensProvider reading from discovery cache
│   ├── diagnostics.ts       — DiagnosticCollection for F_ warnings + status bar
│   ├── focusExclude.ts      — Code actions to toggle F_/X_ prefixes
│   ├── debug.ts             — gotest overlay + dlv DAP launch configuration
│   └── outputParser.ts      — Parse go test -json lines, map to TestItem results
└── test/
    ├── suite/
    │   └── index.ts         — test runner setup
    └── unit/
        ├── discovery.test.ts
        ├── outputParser.test.ts
        └── focusExclude.test.ts
```

---

## Task 1: `gotest discover` Subcommand

**Files:**
- Create: `cmd/gotest/discover.go`
- Create: `cmd/gotest/discover_test.go`
- Modify: `cmd/gotest/cli.go` (add case to switch)
- Modify: `cmd/gotest/args.go` (add to `knownSubcommands`)

- [ ] **Step 1: Register subcommand in CLI**

In `cmd/gotest/args.go`, add `"discover"` to `knownSubcommands`:

```go
var knownSubcommands = map[string]bool{
	"discover": true,
	"generate": true,
	"scaffold": true,
	"migrate":  true,
	"spec":     true,
	"watch":    true,
	"coverage": true,
	"version":  true,
	"help":     true,
}
```

In `cmd/gotest/cli.go`, add the case before `default`:

```go
case "discover":
	os.Exit(runDiscover(remaining))
```

Also update `printUsage()` to include `discover` in the help text:

```go
Subcommands:
  discover    Output test suite metadata as JSON (for IDE integration)
  spec        Render behavioral specification from test suites
  ...
```

- [ ] **Step 2: Implement `discover.go`**

Create `cmd/gotest/discover.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"golang.org/x/tools/go/packages"
)

type DiscoverOutput struct {
	Packages []DiscoverPackage `json:"packages"`
}

type DiscoverPackage struct {
	ImportPath string          `json:"importPath"`
	Dir        string          `json:"dir"`
	Suites     []DiscoverSuite `json:"suites"`
}

type DiscoverSuite struct {
	Name      string           `json:"name"`
	Parallel  bool             `json:"parallel"`
	Focused   bool             `json:"focused"`
	Excluded  bool             `json:"excluded"`
	File      string           `json:"file"`
	Line      int              `json:"line"`
	Col       int              `json:"col"`
	Lifecycle []string         `json:"lifecycle"`
	Fixtures  []string         `json:"fixtures"`
	Methods   []DiscoverMethod `json:"methods"`
}

type DiscoverMethod struct {
	Name     string `json:"name"`
	Parallel bool   `json:"parallel"`
	Focused  bool   `json:"focused"`
	Excluded bool   `json:"excluded"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
}

func runDiscover(args []string) int {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{"."}
	}

	output := DiscoverOutput{}

	for _, pattern := range patterns {
		pkgs, err := discoverPackages(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		output.Packages = append(output.Packages, pkgs...)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return 0
}

func discoverPackages(targetPkg string) ([]DiscoverPackage, error) {
	const packageEvalMode = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps

	totalFoundPkgs, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return nil, err
	}

	// Group by package path (same logic as gotestgen.loadPackages)
	type loadResult struct {
		pkgDir  string
		pkgPath string
		ptest   *packages.Package
		pxtest  *packages.Package
	}

	grouped := map[string]*loadResult{}
	for _, p := range totalFoundPkgs {
		if p.Module == nil {
			continue
		}
		if !strings.HasSuffix(p.ID, ".test]") {
			continue
		}
		isPxTest := strings.HasSuffix(p.Name, "_test")
		pkgPath := p.PkgPath
		if isPxTest {
			pkgPath = strings.TrimSuffix(pkgPath, "_test")
		}
		if _, ok := grouped[pkgPath]; !ok {
			grouped[pkgPath] = &loadResult{pkgPath: pkgPath, pkgDir: gotestgen.DeterminePkgDir(p)}
		}
		if !isPxTest {
			grouped[pkgPath].ptest = p
		} else {
			grouped[pkgPath].pxtest = p
		}
	}

	var result []DiscoverPackage
	c := gotestgen.ExportedCollector()

	for _, lr := range grouped {
		dp := DiscoverPackage{
			ImportPath: lr.pkgPath,
			Dir:        lr.pkgDir,
		}

		for _, pkg := range []*packages.Package{lr.ptest, lr.pxtest} {
			if pkg == nil {
				continue
			}
			collected := c.CollectSuiteSpecs(pkg)
			if len(collected.Errs) > 0 {
				return nil, collected.Errs[0].Err
			}
			for _, suite := range collected.Suites {
				ds := buildDiscoverSuite(suite, pkg.Fset)
				dp.Suites = append(dp.Suites, ds)
			}
		}

		if len(dp.Suites) > 0 {
			result = append(result, dp)
		}
	}

	return result, nil
}

func buildDiscoverSuite(suite *gotestast.TestSuiteSpec, fset *token.FileSet) DiscoverSuite {
	pos := fset.Position(suite.TypeSpecPos())
	ds := DiscoverSuite{
		Name:     suite.Identifier(),
		Parallel: strings.HasSuffix(suite.Identifier(), "TestSuiteParallel"),
		Focused:  strings.HasPrefix(suite.Identifier(), "F_"),
		Excluded: strings.HasPrefix(suite.Identifier(), "X_"),
		File:     filepath.Base(pos.Filename),
		Line:     pos.Line,
		Col:      pos.Column,
	}

	if suite.BeforeAll() != nil {
		ds.Lifecycle = append(ds.Lifecycle, "BeforeAll")
	}
	if suite.AfterAll() != nil {
		ds.Lifecycle = append(ds.Lifecycle, "AfterAll")
	}
	if suite.BeforeEach() != nil {
		ds.Lifecycle = append(ds.Lifecycle, "BeforeEach")
	}
	if suite.AfterEach() != nil {
		ds.Lifecycle = append(ds.Lifecycle, "AfterEach")
	}

	if suite.Fixture() != nil {
		ds.Fixtures = append(ds.Fixtures, suite.Fixture().Identifier())
	}

	for _, m := range suite.TestCases() {
		mPos := fset.Position(m.Pos())
		ds.Methods = append(ds.Methods, DiscoverMethod{
			Name:     m.Identifier(),
			Parallel: m.IsParallel(),
			Focused:  strings.HasPrefix(m.Identifier(), "F_"),
			Excluded: strings.HasPrefix(m.Identifier(), "X_"),
			File:     filepath.Base(mPos.Filename),
			Line:     mPos.Line,
			Col:      mPos.Column,
		})
	}

	return ds
}
```

- [ ] **Step 3: Export required symbols from internal packages**

The `discover` command needs access to the collector and position info. Add these to `internal/gotestgen/collector.go`:

```go
// ExportedCollector returns a collector for use by the discover subcommand.
func ExportedCollector() collector {
	return collector{}
}
```

Add to `internal/gotestast/spec.go` on `TestSuiteSpec`:

```go
// TypeSpecPos returns the position of the type spec name identifier.
func (ts *TestSuiteSpec) TypeSpecPos() token.Pos {
	return ts.ts.Name.Pos()
}
```

Add to `internal/gotestast/spec.go` on `TestSuiteMethod`:

```go
// Pos returns the position of the method name identifier.
func (m *TestSuiteMethod) Pos() token.Pos {
	return m.m.Pos()
}
```

Export `DeterminePkgDir` from `internal/gotestgen` (check if already exported — it's used in `generator.go`). If not exported, add:

```go
func DeterminePkgDir(pkg *packages.Package) string {
	// ... existing implementation
}
```

- [ ] **Step 4: Write integration test**

Create `cmd/gotest/discover_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DiscoverTestSuite struct{}

func (s *DiscoverTestSuite) TestDiscoverSimpleSuite(t *gotest.T) {
	t.When("running discover on examples/simple_suite", func(w *gotest.T) {
		w.It("outputs valid JSON with suite metadata", func(it *gotest.T) {
			cmd := exec.Command("go", "run", ".", "discover", "../../examples/simple_suite")
			cmd.Dir = filepath.Join(mustGetwd(), ".")
			out, err := cmd.Output()
			gotest.NoError(it, err)

			var result DiscoverOutput
			err = json.Unmarshal(out, &result)
			gotest.NoError(it, err)
			gotest.Len(it, result.Packages, 1)

			pkg := result.Packages[0]
			gotest.Contains(it, pkg.ImportPath, "simple_suite")
			gotest.NotEmpty(it, pkg.Suites)

			suite := pkg.Suites[0]
			gotest.Equal(it, "SimpleTestSuite", suite.Name)
			gotest.False(it, suite.Parallel)
			gotest.False(it, suite.Focused)
			gotest.Equal(it, "ptest_test.go", suite.File)
			gotest.Greater(it, suite.Line, 0)
			gotest.NotEmpty(it, suite.Methods)
		})
	})
}

func mustGetwd() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/gotest/ -run TestDiscoverTestSuite -v`

Expected: test suite generates and passes (may need adjustments based on exact export API)

- [ ] **Step 6: Commit**

```bash
git add cmd/gotest/discover.go cmd/gotest/discover_test.go cmd/gotest/cli.go cmd/gotest/args.go internal/gotestast/spec.go internal/gotestgen/collector.go
git commit -m "feat(cli): add gotest discover subcommand for IDE integration"
```

---

## Task 2: `gotest overlay` Subcommand

**Files:**
- Create: `cmd/gotest/overlay_cmd.go`
- Modify: `cmd/gotest/cli.go` (add case)
- Modify: `cmd/gotest/args.go` (add to knownSubcommands)

- [ ] **Step 1: Register subcommand**

In `cmd/gotest/args.go`, add `"overlay"` to `knownSubcommands`.

In `cmd/gotest/cli.go`, add the case:

```go
case "overlay":
	os.Exit(runOverlay(remaining))
```

Update `printUsage()`:

```go
  overlay     Generate test overlay and output path (for debug tooling)
```

- [ ] **Step 2: Implement `overlay_cmd.go`**

Create `cmd/gotest/overlay_cmd.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

type OverlayOutput struct {
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
}

func runOverlay(args []string) int {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{"."}
	}

	var allResults gotestgen.GenerateResults
	for _, pattern := range patterns {
		results, err := gotestrunner.SuitesGenerate(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allResults = append(allResults, results...)
	}

	if len(allResults) == 0 {
		fmt.Fprintf(os.Stderr, "FAIL: no test suites found\n")
		return 1
	}

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	output := OverlayOutput{
		OverlayFile: filepath.Join(tmpDir, "overlay.json"),
		Dir:         tmpDir,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		os.RemoveAll(tmpDir)
		return 2
	}
	return 0
}
```

- [ ] **Step 3: Write test**

Create `cmd/gotest/overlay_cmd_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type OverlayCmdTestSuite struct{}

func (s *OverlayCmdTestSuite) TestOverlayProducesValidOutput(t *gotest.T) {
	t.When("running overlay on examples/simple_suite", func(w *gotest.T) {
		w.It("outputs JSON with overlayFile path", func(it *gotest.T) {
			cmd := exec.Command("go", "run", ".", "overlay", "../../examples/simple_suite")
			cmd.Dir = filepath.Join(mustGetwd(), ".")
			out, err := cmd.Output()
			gotest.NoError(it, err)

			var result OverlayOutput
			err = json.Unmarshal(out, &result)
			gotest.NoError(it, err)
			gotest.NotEmpty(it, result.OverlayFile)
			gotest.NotEmpty(it, result.Dir)

			// Verify overlay file exists
			_, err = os.Stat(result.OverlayFile)
			gotest.NoError(it, err)

			// Cleanup
			os.RemoveAll(result.Dir)
		})
	})
}
```

- [ ] **Step 4: Run test**

Run: `go test ./cmd/gotest/ -run TestOverlayCmdTestSuite -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/gotest/overlay_cmd.go cmd/gotest/overlay_cmd_test.go cmd/gotest/cli.go cmd/gotest/args.go
git commit -m "feat(cli): add gotest overlay subcommand for debug tooling"
```

---

## Task 3: VS Code Extension Scaffold

**Files:**
- Create: `vscode-gotest/package.json`
- Create: `vscode-gotest/tsconfig.json`
- Create: `vscode-gotest/esbuild.config.mjs`
- Create: `vscode-gotest/.vscodeignore`
- Create: `vscode-gotest/src/extension.ts`
- Create: `vscode-gotest/src/types.ts`

- [ ] **Step 1: Create `package.json`**

```json
{
  "name": "vscode-gotest",
  "displayName": "Go Test Suites",
  "description": "Test Explorer, CodeLens, and debug integration for go-test suites",
  "version": "0.1.0",
  "publisher": "mvrahden",
  "license": "MIT",
  "engines": {
    "vscode": "^1.95.0"
  },
  "categories": ["Testing"],
  "activationEvents": [
    "workspaceContains:**/*_test.go"
  ],
  "main": "./dist/extension.js",
  "contributes": {
    "configuration": {
      "title": "Go Test Suites",
      "properties": {
        "gotest.cliPath": {
          "type": "string",
          "default": "gotest",
          "description": "Path to the gotest CLI binary"
        },
        "gotest.discoverOnSave": {
          "type": "boolean",
          "default": true,
          "description": "Re-run discovery when a _test.go file is saved"
        },
        "gotest.showCodeLens": {
          "type": "boolean",
          "default": true,
          "description": "Show Run/Debug CodeLens above suites and methods"
        },
        "gotest.showFocusWarnings": {
          "type": "boolean",
          "default": true,
          "description": "Show diagnostics for F_ prefixed tests"
        },
        "gotest.testFlags": {
          "type": "array",
          "items": { "type": "string" },
          "default": [],
          "description": "Additional flags passed to gotest test"
        },
        "gotest.buildFlags": {
          "type": "array",
          "items": { "type": "string" },
          "default": [],
          "description": "Additional build flags passed to dlv"
        }
      }
    }
  },
  "scripts": {
    "compile": "node esbuild.config.mjs",
    "watch": "node esbuild.config.mjs --watch",
    "lint": "eslint src",
    "test": "node ./dist/test/suite/index.js",
    "package": "vsce package"
  },
  "devDependencies": {
    "@types/node": "^22.0.0",
    "@types/vscode": "^1.95.0",
    "@vscode/test-electron": "^2.4.0",
    "esbuild": "^0.24.0",
    "typescript": "^5.7.0"
  }
}
```

- [ ] **Step 2: Create `tsconfig.json`**

```json
{
  "compilerOptions": {
    "module": "Node16",
    "target": "ES2022",
    "lib": ["ES2022"],
    "moduleResolution": "Node16",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
```

- [ ] **Step 3: Create `esbuild.config.mjs`**

```javascript
import * as esbuild from "esbuild";

const watch = process.argv.includes("--watch");

const config = {
  entryPoints: ["src/extension.ts"],
  bundle: true,
  outdir: "dist",
  external: ["vscode"],
  format: "cjs",
  platform: "node",
  target: "node22",
  sourcemap: true,
  minify: !watch,
};

if (watch) {
  const ctx = await esbuild.context(config);
  await ctx.watch();
  console.log("Watching...");
} else {
  await esbuild.build(config);
}
```

- [ ] **Step 4: Create `.vscodeignore`**

```
src/**
test/**
node_modules/**
tsconfig.json
esbuild.config.mjs
.eslintrc*
```

- [ ] **Step 5: Create `src/types.ts`**

```typescript
export interface DiscoverOutput {
  packages: DiscoverPackage[];
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

export interface OverlayOutput {
  overlayFile: string;
  dir: string;
}
```

- [ ] **Step 6: Create `src/extension.ts` (skeleton)**

```typescript
import * as vscode from "vscode";

export function activate(context: vscode.ExtensionContext): void {
  const outputChannel = vscode.window.createOutputChannel("Go Test Suites");
  outputChannel.appendLine("Go Test Suites extension activated");

  // Components will be wired here in subsequent tasks
}

export function deactivate(): void {}
```

- [ ] **Step 7: Install dependencies and verify build**

Run:
```bash
cd vscode-gotest && npm install && npm run compile
```

Expected: `dist/extension.js` is produced without errors.

- [ ] **Step 8: Commit**

```bash
git add vscode-gotest/
git commit -m "feat(vscode): scaffold extension project with types and build config"
```

---

## Task 4: Discovery Module

**Files:**
- Create: `vscode-gotest/src/discovery.ts`
- Create: `vscode-gotest/test/unit/discovery.test.ts`

- [ ] **Step 1: Implement discovery module**

Create `vscode-gotest/src/discovery.ts`:

```typescript
import * as vscode from "vscode";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { DiscoverOutput, DiscoverPackage } from "./types.js";

const execFileAsync = promisify(execFile);

export class DiscoveryCache {
  private cache = new Map<string, DiscoverPackage>();
  private readonly _onDidUpdate = new vscode.EventEmitter<void>();
  readonly onDidUpdate = this._onDidUpdate.event;

  get packages(): DiscoverPackage[] {
    return [...this.cache.values()];
  }

  getPackage(importPath: string): DiscoverPackage | undefined {
    return this.cache.get(importPath);
  }

  update(packages: DiscoverPackage[]): void {
    for (const pkg of packages) {
      this.cache.set(pkg.importPath, pkg);
    }
    this._onDidUpdate.fire();
  }

  clear(): void {
    this.cache.clear();
    this._onDidUpdate.fire();
  }

  dispose(): void {
    this._onDidUpdate.dispose();
  }
}

export class DiscoveryService {
  private running = false;

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
  ) {}

  async discover(workspaceDir: string, patterns: string[] = ["./..."]): Promise<void> {
    if (this.running) return;
    this.running = true;

    try {
      const cliPath = vscode.workspace
        .getConfiguration("gotest")
        .get<string>("cliPath", "gotest");

      const { stdout, stderr } = await execFileAsync(cliPath, ["discover", ...patterns], {
        cwd: workspaceDir,
        timeout: 30_000,
      });

      if (stderr) {
        this.outputChannel.appendLine(`[discover stderr] ${stderr}`);
      }

      const output: DiscoverOutput = JSON.parse(stdout);
      this.cache.update(output.packages);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[discover error] ${msg}`);
    } finally {
      this.running = false;
    }
  }

  async discoverPackage(workspaceDir: string, pkgPattern: string): Promise<void> {
    return this.discover(workspaceDir, [pkgPattern]);
  }
}
```

- [ ] **Step 2: Write unit test for cache**

Create `vscode-gotest/test/unit/discovery.test.ts`:

```typescript
import * as assert from "node:assert";
import { describe, it } from "node:test";
import type { DiscoverPackage } from "../../src/types.js";

// Test the cache logic independent of VS Code APIs
describe("DiscoveryCache (logic)", () => {
  it("stores and retrieves packages", () => {
    const packages: DiscoverPackage[] = [
      {
        importPath: "example.com/pkg",
        dir: "/tmp/pkg",
        suites: [
          {
            name: "FooTestSuite",
            parallel: false,
            focused: false,
            excluded: false,
            file: "foo_test.go",
            line: 5,
            col: 6,
            lifecycle: ["BeforeEach"],
            fixtures: [],
            methods: [
              {
                name: "TestBar",
                parallel: false,
                focused: false,
                excluded: false,
                file: "foo_test.go",
                line: 10,
                col: 1,
              },
            ],
          },
        ],
      },
    ];

    // Verify structure is correct
    assert.strictEqual(packages[0].importPath, "example.com/pkg");
    assert.strictEqual(packages[0].suites[0].name, "FooTestSuite");
    assert.strictEqual(packages[0].suites[0].methods[0].name, "TestBar");
  });
});
```

- [ ] **Step 3: Run unit test**

Run: `cd vscode-gotest && npx tsx --test test/unit/discovery.test.ts`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add vscode-gotest/src/discovery.ts vscode-gotest/test/unit/discovery.test.ts
git commit -m "feat(vscode): implement discovery service and cache"
```

---

## Task 5: TestController & Resolver

**Files:**
- Create: `vscode-gotest/src/testController.ts`

- [ ] **Step 1: Implement TestController**

Create `vscode-gotest/src/testController.ts`:

```typescript
import * as vscode from "vscode";
import type { DiscoverPackage, DiscoverSuite, DiscoverMethod } from "./types.js";
import type { DiscoveryCache } from "./discovery.js";

export class GoTestController {
  private readonly controller: vscode.TestController;
  private readonly runProfile: vscode.TestRunProfile;
  private readonly debugProfile: vscode.TestRunProfile;

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly runHandler: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
    private readonly debugHandler: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
  ) {
    this.controller = vscode.tests.createTestController("gotest", "Go Test Suites");

    this.runProfile = this.controller.createRunProfile(
      "Run",
      vscode.TestRunProfileKind.Run,
      (request, token) => this.runHandler(request, token),
      true,
    );

    this.debugProfile = this.controller.createRunProfile(
      "Debug",
      vscode.TestRunProfileKind.Debug,
      (request, token) => this.debugHandler(request, token),
      true,
    );

    this.cache.onDidUpdate(() => this.rebuild());
  }

  get testController(): vscode.TestController {
    return this.controller;
  }

  rebuild(): void {
    const existingIds = new Set<string>();

    for (const pkg of this.cache.packages) {
      const pkgItem = this.getOrCreateItem(
        this.controller.items,
        pkg.importPath,
        pkg.importPath,
        undefined,
      );
      existingIds.add(pkg.importPath);

      const suiteIds = new Set<string>();
      for (const suite of pkg.suites) {
        const suiteId = `${pkg.importPath}/${suite.name}`;
        const uri = vscode.Uri.file(`${pkg.dir}/${suite.file}`);
        const range = new vscode.Range(suite.line - 1, suite.col - 1, suite.line - 1, suite.col - 1);
        const suiteItem = this.getOrCreateItem(pkgItem.children, suiteId, suite.name, uri);
        suiteItem.range = range;
        suiteItem.tags = this.buildTags(suite);
        suiteIds.add(suiteId);

        const methodIds = new Set<string>();
        for (const method of suite.methods) {
          const methodId = `${suiteId}/${method.name}`;
          const methodUri = vscode.Uri.file(`${pkg.dir}/${method.file}`);
          const methodRange = new vscode.Range(
            method.line - 1,
            method.col - 1,
            method.line - 1,
            method.col - 1,
          );
          const methodItem = this.getOrCreateItem(
            suiteItem.children,
            methodId,
            method.name,
            methodUri,
          );
          methodItem.range = methodRange;
          methodItem.tags = this.buildTags(method);
          methodIds.add(methodId);
        }

        // Remove stale methods
        suiteItem.children.forEach((item) => {
          if (!methodIds.has(item.id) && !item.id.includes("/dynamic/")) {
            suiteItem.children.delete(item.id);
          }
        });
      }

      // Remove stale suites
      pkgItem.children.forEach((item) => {
        if (!suiteIds.has(item.id)) {
          pkgItem.children.delete(item.id);
        }
      });
    }

    // Remove stale packages
    this.controller.items.forEach((item) => {
      if (!existingIds.has(item.id)) {
        this.controller.items.delete(item.id);
      }
    });
  }

  createDynamicSubtest(
    parentItem: vscode.TestItem,
    subtestPath: string,
    label: string,
  ): vscode.TestItem {
    const id = `${parentItem.id}/dynamic/${subtestPath}`;
    const existing = parentItem.children.get(id);
    if (existing) return existing;

    const item = this.controller.createTestItem(id, label, parentItem.uri);
    parentItem.children.add(item);
    return item;
  }

  clearDynamicSubtests(parentItem: vscode.TestItem): void {
    parentItem.children.forEach((item) => {
      if (item.id.includes("/dynamic/")) {
        parentItem.children.delete(item.id);
      }
    });
  }

  createTestRun(request: vscode.TestRunRequest, name: string): vscode.TestRun {
    return this.controller.createTestRun(request, name);
  }

  findItem(id: string): vscode.TestItem | undefined {
    const parts = id.split("/");
    let items: vscode.TestItemCollection = this.controller.items;
    let item: vscode.TestItem | undefined;

    // Walk the tree by reconstructing incremental IDs
    for (let i = 0; i < parts.length; i++) {
      const partialId = parts.slice(0, i + 1).join("/");
      item = items.get(partialId);
      if (!item) return undefined;
      items = item.children;
    }
    return item;
  }

  dispose(): void {
    this.controller.dispose();
  }

  private getOrCreateItem(
    collection: vscode.TestItemCollection,
    id: string,
    label: string,
    uri: vscode.Uri | undefined,
  ): vscode.TestItem {
    const existing = collection.get(id);
    if (existing) {
      existing.label = label;
      return existing;
    }
    const item = this.controller.createTestItem(id, label, uri);
    collection.add(item);
    return item;
  }

  private buildTags(item: { focused: boolean; excluded: boolean; parallel: boolean }): vscode.TestTag[] {
    const tags: vscode.TestTag[] = [];
    if (item.focused) tags.push(new vscode.TestTag("focused"));
    if (item.excluded) tags.push(new vscode.TestTag("excluded"));
    if (item.parallel) tags.push(new vscode.TestTag("parallel"));
    return tags;
  }
}
```

- [ ] **Step 2: Verify build**

Run: `cd vscode-gotest && npm run compile`

Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/testController.ts
git commit -m "feat(vscode): implement TestController with resolver and tree builder"
```

---

## Task 6: Test Runner & Output Parser

**Files:**
- Create: `vscode-gotest/src/outputParser.ts`
- Create: `vscode-gotest/src/runner.ts`
- Create: `vscode-gotest/test/unit/outputParser.test.ts`

- [ ] **Step 1: Implement output parser**

Create `vscode-gotest/src/outputParser.ts`:

```typescript
import * as vscode from "vscode";

export interface TestEvent {
  Time: string;
  Action: "run" | "pass" | "fail" | "skip" | "output" | "pause" | "cont";
  Package: string;
  Test?: string;
  Output?: string;
  Elapsed?: number;
}

export interface TestResult {
  testPath: string;
  action: "pass" | "fail" | "skip";
  elapsed?: number;
  output: string[];
  messages: TestMessage[];
}

export interface TestMessage {
  file: string;
  line: number;
  message: string;
}

const FILE_LINE_REGEX = /^\s*(.+?):(\d+):\s*(.+)$/;

export function parseTestEvents(jsonLines: string): TestEvent[] {
  const events: TestEvent[] = [];
  for (const line of jsonLines.split("\n")) {
    if (!line.trim()) continue;
    try {
      events.push(JSON.parse(line));
    } catch {
      // Skip non-JSON lines (build output, etc.)
    }
  }
  return events;
}

export function extractTestMessages(output: string, pkgDir: string): TestMessage[] {
  const messages: TestMessage[] = [];
  for (const line of output.split("\n")) {
    const match = FILE_LINE_REGEX.exec(line);
    if (match) {
      messages.push({
        file: `${pkgDir}/${match[1]}`,
        line: parseInt(match[2], 10),
        message: match[3],
      });
    }
  }
  return messages;
}

export function buildTestRunFilter(
  items: readonly vscode.TestItem[],
): { pkg: string; runRegex: string } | null {
  if (items.length === 0) return null;

  // Extract package from the first item's ID
  // ID format: "importPath/SuiteName/MethodName"
  const firstId = items[0].id;
  const parts = firstId.split("/");

  // Find where the package path ends and suite begins
  // Package paths contain dots, suite names end in TestSuite
  let pkgPath = "";
  let suiteName = "";
  let methodNames: string[] = [];

  for (const item of items) {
    const id = item.id;
    // Walk up the tree to determine context
    if (!item.parent) {
      // This is a package-level item
      pkgPath = id;
    } else if (!item.parent.parent) {
      // Suite level
      pkgPath = item.parent.id;
      suiteName = item.label;
    } else {
      // Method level or deeper
      let current = item.parent;
      while (current?.parent?.parent) {
        current = current.parent;
      }
      if (current?.parent) {
        pkgPath = current.parent.id;
        suiteName = current.label;
      }
      methodNames.push(item.label);
    }
  }

  if (!pkgPath) return null;

  let runRegex = "";
  if (suiteName) {
    runRegex = `^Test${suiteName.replace(/^(F_|X_)/, "")}$`;
    if (methodNames.length > 0) {
      const methodPart =
        methodNames.length === 1
          ? `^${methodNames[0]}$`
          : `^(${methodNames.join("|")})$`;
      runRegex += `/${methodPart}`;
    }
  }

  return { pkg: `./${pkgPath.split("/").slice(-1)[0]}`, runRegex };
}
```

- [ ] **Step 2: Implement test runner**

Create `vscode-gotest/src/runner.ts`:

```typescript
import * as vscode from "vscode";
import { spawn } from "node:child_process";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents, extractTestMessages, type TestEvent } from "./outputParser.js";

export class TestRunner {
  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
  ) {}

  async run(request: vscode.TestRunRequest, token: vscode.CancellationToken): Promise<void> {
    const run = this.controller.createTestRun(request, "Go Test Run");
    const items = this.collectItems(request);

    for (const item of items) {
      run.started(item);
    }

    try {
      const { pkg, args, workspaceDir } = this.buildCommand(request);
      if (!workspaceDir) {
        run.end();
        return;
      }

      const cliPath = vscode.workspace
        .getConfiguration("gotest")
        .get<string>("cliPath", "gotest");
      const extraFlags = vscode.workspace
        .getConfiguration("gotest")
        .get<string[]>("testFlags", []);

      const fullArgs = [...args, "-json", ...extraFlags];
      this.outputChannel.appendLine(`[run] ${cliPath} ${fullArgs.join(" ")}`);

      const output = await this.spawnGotest(cliPath, fullArgs, workspaceDir, token);
      const events = parseTestEvents(output);
      this.applyResults(events, run, items, workspaceDir);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      for (const item of items) {
        run.errored(item, new vscode.TestMessage(msg));
      }
    } finally {
      run.end();
    }
  }

  private collectItems(request: vscode.TestRunRequest): vscode.TestItem[] {
    const items: vscode.TestItem[] = [];
    if (request.include) {
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

  private buildCommand(request: vscode.TestRunRequest): {
    pkg: string;
    args: string[];
    workspaceDir: string | undefined;
  } {
    const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!request.include || request.include.length === 0) {
      return { pkg: "./...", args: ["test", "./..."], workspaceDir };
    }

    // Determine package and run filter from selected items
    const item = request.include[0];
    const pkgPath = this.getPackagePath(item);
    const pkg = this.findPackageDir(pkgPath);
    const args = ["test", pkg || "./..."];

    const runFilter = this.buildRunFilter(request.include);
    if (runFilter) {
      args.push("-run", runFilter);
    }

    return { pkg: pkg || "./...", args, workspaceDir };
  }

  private buildRunFilter(items: readonly vscode.TestItem[]): string | undefined {
    if (items.length === 0) return undefined;

    const first = items[0];
    const depth = this.getDepth(first);

    if (depth === 0) return undefined; // Package level — run all

    if (depth === 1) {
      // Suite level
      const suiteName = first.label.replace(/^(F_|X_)/, "");
      return `^Test${suiteName}$`;
    }

    // Method level or deeper
    const suite = first.parent;
    if (!suite) return undefined;
    const suiteName = suite.label.replace(/^(F_|X_)/, "");

    if (items.length === 1 && first.id.includes("/dynamic/")) {
      // Dynamic subtest — build full path
      const dynamicPart = first.id.split("/dynamic/")[1];
      return `^Test${suiteName}$/${dynamicPart.split("/").map((s) => `^${s}$`).join("/")}`;
    }

    const methodNames = items.map((i) => i.label);
    const methodPart =
      methodNames.length === 1 ? `^${methodNames[0]}$` : `^(${methodNames.join("|")})$`;
    return `^Test${suiteName}$/${methodPart}`;
  }

  private getDepth(item: vscode.TestItem): number {
    let depth = 0;
    let current = item.parent;
    while (current) {
      depth++;
      current = current.parent;
    }
    return depth;
  }

  private getPackagePath(item: vscode.TestItem): string {
    let current: vscode.TestItem | undefined = item;
    while (current?.parent) {
      current = current.parent;
    }
    return current?.id || "";
  }

  private findPackageDir(importPath: string): string | undefined {
    const pkg = this.cache.getPackage(importPath);
    return pkg?.dir;
  }

  private applyResults(
    events: TestEvent[],
    run: vscode.TestRun,
    items: vscode.TestItem[],
    workspaceDir: string,
  ): void {
    const outputByTest = new Map<string, string[]>();

    for (const event of events) {
      if (!event.Test) continue;

      if (event.Action === "output" && event.Output) {
        const outputs = outputByTest.get(event.Test) || [];
        outputs.push(event.Output);
        outputByTest.set(event.Test, outputs);
      }

      if (event.Action === "pass" || event.Action === "fail" || event.Action === "skip") {
        const item = this.findItemByTestPath(event.Test, items);
        if (!item) continue;

        const testOutput = (outputByTest.get(event.Test) || []).join("");

        if (event.Action === "pass") {
          run.passed(item, event.Elapsed ? event.Elapsed * 1000 : undefined);
        } else if (event.Action === "fail") {
          const messages = extractTestMessages(testOutput, workspaceDir);
          const vscMessages = messages.map((m) => {
            const msg = new vscode.TestMessage(m.message);
            msg.location = new vscode.Location(
              vscode.Uri.file(m.file),
              new vscode.Position(m.line - 1, 0),
            );
            return msg;
          });
          if (vscMessages.length > 0) {
            run.failed(item, vscMessages, event.Elapsed ? event.Elapsed * 1000 : undefined);
          } else {
            run.failed(item, new vscode.TestMessage(testOutput), event.Elapsed ? event.Elapsed * 1000 : undefined);
          }
        } else if (event.Action === "skip") {
          run.skipped(item);
        }
      }
    }
  }

  private findItemByTestPath(
    testPath: string,
    items: vscode.TestItem[],
  ): vscode.TestItem | undefined {
    // testPath format: "TestSuiteName/MethodName/SubtestPath..."
    const parts = testPath.split("/");
    if (parts.length === 0) return undefined;

    // Find the suite item
    const suiteName = parts[0].replace(/^Test/, "");
    for (const item of items) {
      if (item.label === suiteName || item.label === `F_${suiteName}` || item.label === `X_${suiteName}`) {
        if (parts.length === 1) return item;

        // Find method
        const methodName = parts[1];
        let methodItem: vscode.TestItem | undefined;
        item.children.forEach((child) => {
          if (child.label === methodName) {
            methodItem = child;
          }
        });

        if (!methodItem) return item;
        if (parts.length === 2) return methodItem;

        // Dynamic subtest — create if not exists
        const subtestPath = parts.slice(2).join("/");
        const label = parts.slice(2).join(" / ").replace(/_/g, " ");
        return this.controller.createDynamicSubtest(methodItem, subtestPath, label);
      }

      // Also check children for method-level selections
      let found: vscode.TestItem | undefined;
      item.children.forEach((child) => {
        if (!found) {
          const result = this.findInChildren(child, testPath);
          if (result) found = result;
        }
      });
      if (found) return found;
    }

    return undefined;
  }

  private findInChildren(item: vscode.TestItem, testPath: string): vscode.TestItem | undefined {
    // Check if this item matches the test path
    if (item.id.endsWith(testPath.replace(/^Test/, ""))) return item;
    let found: vscode.TestItem | undefined;
    item.children.forEach((child) => {
      if (!found) {
        found = this.findInChildren(child, testPath);
      }
    });
    return found;
  }

  private spawnGotest(
    cliPath: string,
    args: string[],
    cwd: string,
    token: vscode.CancellationToken,
  ): Promise<string> {
    return new Promise((resolve, reject) => {
      const proc = spawn(cliPath, args, { cwd });
      let stdout = "";
      let stderr = "";

      proc.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
      });
      proc.stderr.on("data", (data: Buffer) => {
        stderr += data.toString();
      });

      proc.on("close", (code) => {
        if (stderr) {
          this.outputChannel.appendLine(`[stderr] ${stderr}`);
        }
        // go test exits non-zero on test failure — that's fine, we have JSON
        resolve(stdout);
      });

      proc.on("error", reject);

      token.onCancellationRequested(() => {
        proc.kill("SIGTERM");
      });
    });
  }
}
```

- [ ] **Step 3: Write output parser unit test**

Create `vscode-gotest/test/unit/outputParser.test.ts`:

```typescript
import * as assert from "node:assert";
import { describe, it } from "node:test";
import { parseTestEvents, extractTestMessages } from "../../src/outputParser.js";

describe("parseTestEvents", () => {
  it("parses valid JSON lines", () => {
    const input = [
      '{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/pkg","Test":"TestFoo/TestBar"}',
      '{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"example.com/pkg","Test":"TestFoo/TestBar","Elapsed":0.5}',
    ].join("\n");

    const events = parseTestEvents(input);
    assert.strictEqual(events.length, 2);
    assert.strictEqual(events[0].Action, "run");
    assert.strictEqual(events[0].Test, "TestFoo/TestBar");
    assert.strictEqual(events[1].Action, "pass");
    assert.strictEqual(events[1].Elapsed, 0.5);
  });

  it("skips non-JSON lines", () => {
    const input = "not json\n" + '{"Action":"pass","Package":"x","Test":"T"}' + "\n\n";
    const events = parseTestEvents(input);
    assert.strictEqual(events.length, 1);
  });
});

describe("extractTestMessages", () => {
  it("extracts file:line messages", () => {
    const output = "    foo_test.go:42: expected 1, got 2\n    bar_test.go:10: nil pointer\n";
    const messages = extractTestMessages(output, "/workspace/pkg");
    assert.strictEqual(messages.length, 2);
    assert.strictEqual(messages[0].file, "/workspace/pkg/foo_test.go");
    assert.strictEqual(messages[0].line, 42);
    assert.strictEqual(messages[0].message, "expected 1, got 2");
    assert.strictEqual(messages[1].file, "/workspace/pkg/bar_test.go");
    assert.strictEqual(messages[1].line, 10);
  });

  it("handles output with no file references", () => {
    const output = "some debug output\nno file refs here\n";
    const messages = extractTestMessages(output, "/workspace");
    assert.strictEqual(messages.length, 0);
  });
});
```

- [ ] **Step 4: Run unit test**

Run: `cd vscode-gotest && npx tsx --test test/unit/outputParser.test.ts`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add vscode-gotest/src/outputParser.ts vscode-gotest/src/runner.ts vscode-gotest/test/unit/outputParser.test.ts
git commit -m "feat(vscode): implement test runner and JSON output parser"
```

---

## Task 7: CodeLens Provider

**Files:**
- Create: `vscode-gotest/src/codeLens.ts`

- [ ] **Step 1: Implement CodeLens provider**

Create `vscode-gotest/src/codeLens.ts`:

```typescript
import * as vscode from "vscode";
import type { DiscoveryCache } from "./discovery.js";
import type { DiscoverSuite, DiscoverMethod } from "./types.js";

export class GoTestCodeLensProvider implements vscode.CodeLensProvider {
  private readonly _onDidChangeCodeLenses = new vscode.EventEmitter<void>();
  readonly onDidChangeCodeLenses = this._onDidChangeCodeLenses.event;

  constructor(private readonly cache: DiscoveryCache) {
    this.cache.onDidUpdate(() => this._onDidChangeCodeLenses.fire());
  }

  provideCodeLenses(
    document: vscode.TextDocument,
    _token: vscode.CancellationToken,
  ): vscode.CodeLens[] {
    const enabled = vscode.workspace
      .getConfiguration("gotest")
      .get<boolean>("showCodeLens", true);
    if (!enabled) return [];

    if (!document.fileName.endsWith("_test.go")) return [];

    const lenses: vscode.CodeLens[] = [];
    const fileName = document.fileName.split("/").pop() || "";

    for (const pkg of this.cache.packages) {
      if (!document.fileName.startsWith(pkg.dir)) continue;

      for (const suite of pkg.suites) {
        if (suite.file === fileName) {
          lenses.push(...this.suiteLenses(suite, pkg.importPath, pkg.dir));
        }

        for (const method of suite.methods) {
          if (method.file === fileName) {
            lenses.push(...this.methodLenses(method, suite, pkg.importPath, pkg.dir));
          }
        }
      }
    }

    return lenses;
  }

  private suiteLenses(
    suite: DiscoverSuite,
    importPath: string,
    pkgDir: string,
  ): vscode.CodeLens[] {
    const range = new vscode.Range(suite.line - 1, 0, suite.line - 1, 0);
    const suiteId = `${importPath}/${suite.name}`;

    return [
      new vscode.CodeLens(range, {
        title: "▶ Run Suite",
        command: "gotest.runTest",
        arguments: [suiteId],
      }),
      new vscode.CodeLens(range, {
        title: "Debug Suite",
        command: "gotest.debugTest",
        arguments: [suiteId],
      }),
    ];
  }

  private methodLenses(
    method: DiscoverMethod,
    suite: DiscoverSuite,
    importPath: string,
    pkgDir: string,
  ): vscode.CodeLens[] {
    const range = new vscode.Range(method.line - 1, 0, method.line - 1, 0);
    const methodId = `${importPath}/${suite.name}/${method.name}`;

    return [
      new vscode.CodeLens(range, {
        title: "▶ Run",
        command: "gotest.runTest",
        arguments: [methodId],
      }),
      new vscode.CodeLens(range, {
        title: "Debug",
        command: "gotest.debugTest",
        arguments: [methodId],
      }),
    ];
  }

  dispose(): void {
    this._onDidChangeCodeLenses.dispose();
  }
}
```

- [ ] **Step 2: Verify build**

Run: `cd vscode-gotest && npm run compile`

Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/codeLens.ts
git commit -m "feat(vscode): implement CodeLens provider for suites and methods"
```

---

## Task 8: Debug Integration

**Files:**
- Create: `vscode-gotest/src/debug.ts`

- [ ] **Step 1: Implement debug launcher**

Create `vscode-gotest/src/debug.ts`:

```typescript
import * as vscode from "vscode";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as path from "node:path";
import type { OverlayOutput } from "./types.js";

const execFileAsync = promisify(execFile);

export class DebugLauncher {
  private activeOverlayDirs: Set<string> = new Set();

  constructor(private readonly outputChannel: vscode.OutputChannel) {}

  async debug(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
    buildRunFilter: (items: readonly vscode.TestItem[]) => string | undefined,
    getPackageDir: (item: vscode.TestItem) => string | undefined,
  ): Promise<void> {
    const items = request.include;
    if (!items || items.length === 0) return;

    const pkgDir = getPackageDir(items[0]);
    if (!pkgDir) {
      vscode.window.showErrorMessage("Cannot determine package directory for debug");
      return;
    }

    const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!workspaceDir) return;

    // Generate overlay
    const cliPath = vscode.workspace
      .getConfiguration("gotest")
      .get<string>("cliPath", "gotest");

    let overlay: OverlayOutput;
    try {
      const { stdout } = await execFileAsync(cliPath, ["overlay", pkgDir], {
        cwd: workspaceDir,
        timeout: 30_000,
      });
      overlay = JSON.parse(stdout);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      vscode.window.showErrorMessage(`gotest overlay failed: ${msg}`);
      return;
    }

    this.activeOverlayDirs.add(overlay.dir);

    // Build run filter
    const runFilter = buildRunFilter(items);
    const testArgs: string[] = [];
    if (runFilter) {
      testArgs.push("-test.run", runFilter);
    }

    // Build flags
    const extraBuildFlags = vscode.workspace
      .getConfiguration("gotest")
      .get<string[]>("buildFlags", []);
    const buildFlags = [`-overlay=${overlay.overlayFile}`, ...extraBuildFlags].join(" ");

    // Launch debug session
    const debugConfig: vscode.DebugConfiguration = {
      type: "go",
      name: "Go Test Suite Debug",
      request: "launch",
      mode: "test",
      program: pkgDir,
      buildFlags: buildFlags,
      args: testArgs,
    };

    this.outputChannel.appendLine(`[debug] launching dlv with overlay: ${overlay.overlayFile}`);
    this.outputChannel.appendLine(`[debug] config: ${JSON.stringify(debugConfig, null, 2)}`);

    const started = await vscode.debug.startDebugging(
      vscode.workspace.workspaceFolders?.[0],
      debugConfig,
    );

    if (!started) {
      this.cleanup(overlay.dir);
    }
  }

  registerCleanupOnSessionEnd(context: vscode.ExtensionContext): void {
    context.subscriptions.push(
      vscode.debug.onDidTerminateDebugSession((session) => {
        if (session.configuration.name === "Go Test Suite Debug") {
          // Clean up all active overlay dirs
          for (const dir of this.activeOverlayDirs) {
            this.cleanup(dir);
          }
          this.activeOverlayDirs.clear();
        }
      }),
    );
  }

  private cleanup(dir: string): void {
    import("node:fs/promises").then(({ rm }) => {
      rm(dir, { recursive: true, force: true }).catch(() => {});
    });
    this.activeOverlayDirs.delete(dir);
  }

  dispose(): void {
    for (const dir of this.activeOverlayDirs) {
      this.cleanup(dir);
    }
  }
}
```

- [ ] **Step 2: Verify build**

Run: `cd vscode-gotest && npm run compile`

Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/debug.ts
git commit -m "feat(vscode): implement debug launcher with overlay + dlv integration"
```

---

## Task 9: Focus/Exclude Code Actions

**Files:**
- Create: `vscode-gotest/src/focusExclude.ts`
- Create: `vscode-gotest/test/unit/focusExclude.test.ts`

- [ ] **Step 1: Implement code actions**

Create `vscode-gotest/src/focusExclude.ts`:

```typescript
import * as vscode from "vscode";
import type { DiscoveryCache } from "./discovery.js";
import type { DiscoverSuite, DiscoverMethod } from "./types.js";

export class FocusExcludeProvider implements vscode.CodeActionProvider {
  static readonly providedCodeActionKinds = [vscode.CodeActionKind.QuickFix];

  constructor(private readonly cache: DiscoveryCache) {}

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range,
  ): vscode.CodeAction[] {
    if (!document.fileName.endsWith("_test.go")) return [];

    const fileName = document.fileName.split("/").pop() || "";
    const line = range.start.line + 1; // 1-based

    const actions: vscode.CodeAction[] = [];

    for (const pkg of this.cache.packages) {
      if (!document.fileName.startsWith(pkg.dir)) continue;

      for (const suite of pkg.suites) {
        if (suite.file === fileName && suite.line === line) {
          actions.push(...this.suiteActions(document, suite, pkg.dir));
        }
        for (const method of suite.methods) {
          if (method.file === fileName && method.line === line) {
            actions.push(...this.methodActions(document, method));
          }
        }
      }
    }

    return actions;
  }

  private suiteActions(
    document: vscode.TextDocument,
    suite: DiscoverSuite,
    pkgDir: string,
  ): vscode.CodeAction[] {
    const actions: vscode.CodeAction[] = [];

    if (suite.focused) {
      actions.push(this.createRenameAction(document, suite, "Unfocus this suite", "F_", ""));
    } else if (suite.excluded) {
      actions.push(this.createRenameAction(document, suite, "Include this suite", "X_", ""));
    } else {
      actions.push(this.createRenameAction(document, suite, "Focus this suite", "", "F_"));
      actions.push(this.createRenameAction(document, suite, "Exclude this suite", "", "X_"));
    }

    return actions;
  }

  private methodActions(
    document: vscode.TextDocument,
    method: DiscoverMethod,
  ): vscode.CodeAction[] {
    const actions: vscode.CodeAction[] = [];

    if (method.focused) {
      actions.push(this.createMethodRenameAction(document, method, "Unfocus this test", "F_", ""));
    } else if (method.excluded) {
      actions.push(this.createMethodRenameAction(document, method, "Include this test", "X_", ""));
    } else {
      actions.push(this.createMethodRenameAction(document, method, "Focus this test", "", "F_"));
      actions.push(this.createMethodRenameAction(document, method, "Exclude this test", "", "X_"));
    }

    return actions;
  }

  private createRenameAction(
    document: vscode.TextDocument,
    suite: DiscoverSuite,
    title: string,
    removePrefix: string,
    addPrefix: string,
  ): vscode.CodeAction {
    const action = new vscode.CodeAction(title, vscode.CodeActionKind.QuickFix);
    const edit = new vscode.WorkspaceEdit();

    const oldName = suite.name;
    const baseName = oldName.replace(/^(F_|X_)/, "");
    const newName = addPrefix + baseName;

    // Replace suite type name at declaration
    const declLine = document.lineAt(suite.line - 1);
    const startCol = declLine.text.indexOf(oldName);
    if (startCol >= 0) {
      const range = new vscode.Range(suite.line - 1, startCol, suite.line - 1, startCol + oldName.length);
      edit.replace(document.uri, range, newName);
    }

    // Replace in method receivers throughout the file
    for (let i = 0; i < document.lineCount; i++) {
      const lineText = document.lineAt(i).text;
      const receiverPattern = `*${oldName}`;
      const idx = lineText.indexOf(receiverPattern);
      if (idx >= 0) {
        const range = new vscode.Range(i, idx + 1, i, idx + 1 + oldName.length);
        edit.replace(document.uri, range, newName);
      }
    }

    action.edit = edit;
    return action;
  }

  private createMethodRenameAction(
    document: vscode.TextDocument,
    method: DiscoverMethod,
    title: string,
    removePrefix: string,
    addPrefix: string,
  ): vscode.CodeAction {
    const action = new vscode.CodeAction(title, vscode.CodeActionKind.QuickFix);
    const edit = new vscode.WorkspaceEdit();

    const oldName = method.name;
    const baseName = oldName.replace(/^(F_|X_)/, "");
    const newName = addPrefix + baseName;

    const line = document.lineAt(method.line - 1);
    const startCol = line.text.indexOf(oldName);
    if (startCol >= 0) {
      const range = new vscode.Range(method.line - 1, startCol, method.line - 1, startCol + oldName.length);
      edit.replace(document.uri, range, newName);
    }

    action.edit = edit;
    return action;
  }
}
```

- [ ] **Step 2: Write unit test for prefix logic**

Create `vscode-gotest/test/unit/focusExclude.test.ts`:

```typescript
import * as assert from "node:assert";
import { describe, it } from "node:test";

describe("Focus/Exclude prefix logic", () => {
  function applyPrefix(name: string, removePrefix: string, addPrefix: string): string {
    const baseName = name.replace(/^(F_|X_)/, "");
    return addPrefix + baseName;
  }

  it("adds F_ prefix to unfocused name", () => {
    assert.strictEqual(applyPrefix("TestFoo", "", "F_"), "F_TestFoo");
  });

  it("removes F_ prefix", () => {
    assert.strictEqual(applyPrefix("F_TestFoo", "F_", ""), "TestFoo");
  });

  it("adds X_ prefix to unfocused name", () => {
    assert.strictEqual(applyPrefix("TestFoo", "", "X_"), "X_TestFoo");
  });

  it("removes X_ prefix", () => {
    assert.strictEqual(applyPrefix("X_TestFoo", "X_", ""), "TestFoo");
  });

  it("adds F_ prefix to suite name", () => {
    assert.strictEqual(applyPrefix("SimpleTestSuite", "", "F_"), "F_SimpleTestSuite");
  });

  it("removes F_ prefix from suite name", () => {
    assert.strictEqual(applyPrefix("F_SimpleTestSuite", "F_", ""), "SimpleTestSuite");
  });
});
```

- [ ] **Step 3: Run test**

Run: `cd vscode-gotest && npx tsx --test test/unit/focusExclude.test.ts`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add vscode-gotest/src/focusExclude.ts vscode-gotest/test/unit/focusExclude.test.ts
git commit -m "feat(vscode): implement focus/exclude code actions with prefix toggling"
```

---

## Task 10: Diagnostics & Status Bar

**Files:**
- Create: `vscode-gotest/src/diagnostics.ts`

- [ ] **Step 1: Implement diagnostics**

Create `vscode-gotest/src/diagnostics.ts`:

```typescript
import * as vscode from "vscode";
import type { DiscoveryCache } from "./discovery.js";

export class FocusDiagnostics {
  private readonly diagnosticCollection: vscode.DiagnosticCollection;
  private readonly statusBarItem: vscode.StatusBarItem;

  constructor(private readonly cache: DiscoveryCache) {
    this.diagnosticCollection = vscode.languages.createDiagnosticCollection("gotest");
    this.statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
    this.statusBarItem.command = "gotest.showFocusedTests";

    this.cache.onDidUpdate(() => this.refresh());
  }

  refresh(): void {
    const enabled = vscode.workspace
      .getConfiguration("gotest")
      .get<boolean>("showFocusWarnings", true);

    this.diagnosticCollection.clear();

    if (!enabled) {
      this.statusBarItem.hide();
      return;
    }

    let focusCount = 0;
    const diagnosticsByFile = new Map<string, vscode.Diagnostic[]>();

    for (const pkg of this.cache.packages) {
      for (const suite of pkg.suites) {
        if (suite.focused) {
          focusCount++;
          const filePath = `${pkg.dir}/${suite.file}`;
          const diag = new vscode.Diagnostic(
            new vscode.Range(suite.line - 1, 0, suite.line - 1, suite.name.length),
            "Focused test suite — will cause CI failure (gotest --ci)",
            vscode.DiagnosticSeverity.Warning,
          );
          diag.source = "gotest";
          const existing = diagnosticsByFile.get(filePath) || [];
          existing.push(diag);
          diagnosticsByFile.set(filePath, existing);
        }

        for (const method of suite.methods) {
          if (method.focused) {
            focusCount++;
            const filePath = `${pkg.dir}/${method.file}`;
            const diag = new vscode.Diagnostic(
              new vscode.Range(method.line - 1, 0, method.line - 1, method.name.length),
              "Focused test — will cause CI failure (gotest --ci)",
              vscode.DiagnosticSeverity.Warning,
            );
            diag.source = "gotest";
            const existing = diagnosticsByFile.get(filePath) || [];
            existing.push(diag);
            diagnosticsByFile.set(filePath, existing);
          }
        }
      }
    }

    for (const [filePath, diagnostics] of diagnosticsByFile) {
      this.diagnosticCollection.set(vscode.Uri.file(filePath), diagnostics);
    }

    if (focusCount > 0) {
      this.statusBarItem.text = `$(warning) gotest: ${focusCount} focused test${focusCount > 1 ? "s" : ""}`;
      this.statusBarItem.tooltip = "Click to show focused tests";
      this.statusBarItem.show();
    } else {
      this.statusBarItem.hide();
    }
  }

  async showFocusedTests(): Promise<void> {
    const items: vscode.QuickPickItem[] = [];

    for (const pkg of this.cache.packages) {
      for (const suite of pkg.suites) {
        if (suite.focused) {
          items.push({
            label: suite.name,
            description: `${pkg.importPath} — ${suite.file}:${suite.line}`,
            detail: "Suite",
          });
        }
        for (const method of suite.methods) {
          if (method.focused) {
            items.push({
              label: method.name,
              description: `${suite.name} — ${method.file}:${method.line}`,
              detail: "Method",
            });
          }
        }
      }
    }

    const selected = await vscode.window.showQuickPick(items, {
      placeHolder: "Focused tests (will fail CI)",
    });

    if (selected?.description) {
      const match = selected.description.match(/(\S+):(\d+)$/);
      if (match) {
        const filePart = selected.description.split(" — ")[1];
        const [file, lineStr] = filePart.split(":");
        // Find full path
        for (const pkg of this.cache.packages) {
          if (selected.description.includes(pkg.importPath)) {
            const uri = vscode.Uri.file(`${pkg.dir}/${file}`);
            const line = parseInt(lineStr, 10) - 1;
            const doc = await vscode.workspace.openTextDocument(uri);
            await vscode.window.showTextDocument(doc, {
              selection: new vscode.Range(line, 0, line, 0),
            });
            break;
          }
        }
      }
    }
  }

  dispose(): void {
    this.diagnosticCollection.dispose();
    this.statusBarItem.dispose();
  }
}
```

- [ ] **Step 2: Verify build**

Run: `cd vscode-gotest && npm run compile`

Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add vscode-gotest/src/diagnostics.ts
git commit -m "feat(vscode): implement focus diagnostics and status bar indicator"
```

---

## Task 11: Wire Extension Entry Point

**Files:**
- Modify: `vscode-gotest/src/extension.ts`
- Modify: `vscode-gotest/package.json` (add commands)

- [ ] **Step 1: Add commands to `package.json`**

Add to the `contributes` section of `package.json`:

```json
"commands": [
  {
    "command": "gotest.runTest",
    "title": "Go Test: Run"
  },
  {
    "command": "gotest.debugTest",
    "title": "Go Test: Debug"
  },
  {
    "command": "gotest.refreshTests",
    "title": "Go Test: Refresh"
  },
  {
    "command": "gotest.showFocusedTests",
    "title": "Go Test: Show Focused Tests"
  }
],
"languages": [
  {
    "id": "go",
    "extensions": [".go"]
  }
]
```

- [ ] **Step 2: Wire all components in `extension.ts`**

Rewrite `vscode-gotest/src/extension.ts`:

```typescript
import * as vscode from "vscode";
import { DiscoveryCache, DiscoveryService } from "./discovery.js";
import { GoTestController } from "./testController.js";
import { TestRunner } from "./runner.js";
import { GoTestCodeLensProvider } from "./codeLens.js";
import { DebugLauncher } from "./debug.js";
import { FocusExcludeProvider } from "./focusExclude.js";
import { FocusDiagnostics } from "./diagnostics.js";

export function activate(context: vscode.ExtensionContext): void {
  const outputChannel = vscode.window.createOutputChannel("Go Test Suites");
  outputChannel.appendLine("Go Test Suites extension activating...");

  const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!workspaceDir) {
    outputChannel.appendLine("No workspace folder found");
    return;
  }

  // Core services
  const cache = new DiscoveryCache();
  const discoveryService = new DiscoveryService(cache, outputChannel);
  const debugLauncher = new DebugLauncher(outputChannel);

  // TestController
  const runner = new TestRunner(
    undefined as unknown as GoTestController, // Will be set after construction
    cache,
    outputChannel,
  );

  const testController = new GoTestController(
    cache,
    outputChannel,
    (request, token) => runner.run(request, token),
    (request, token) =>
      debugLauncher.debug(
        request,
        token,
        (items) => {
          if (!items || items.length === 0) return undefined;
          const first = items[0];
          let current: vscode.TestItem | undefined = first;
          while (current?.parent?.parent) current = current.parent;
          const suite = current?.parent ? current : first;
          const suiteName = suite.label.replace(/^(F_|X_)/, "");

          if (first === suite) return `^Test${suiteName}$`;
          return `^Test${suiteName}$/^${first.label}$`;
        },
        (item) => {
          let current: vscode.TestItem | undefined = item;
          while (current?.parent) current = current.parent;
          const importPath = current?.id;
          if (!importPath) return undefined;
          return cache.getPackage(importPath)?.dir;
        },
      ),
  );

  // Patch runner with controller reference
  Object.assign(runner, { controller: testController });

  // CodeLens
  const codeLensProvider = new GoTestCodeLensProvider(cache);
  context.subscriptions.push(
    vscode.languages.registerCodeLensProvider(
      { language: "go", pattern: "**/*_test.go" },
      codeLensProvider,
    ),
  );

  // Focus/Exclude code actions
  const focusExcludeProvider = new FocusExcludeProvider(cache);
  context.subscriptions.push(
    vscode.languages.registerCodeActionsProvider(
      { language: "go", pattern: "**/*_test.go" },
      focusExcludeProvider,
      { providedCodeActionKinds: FocusExcludeProvider.providedCodeActionKinds },
    ),
  );

  // Diagnostics
  const diagnostics = new FocusDiagnostics(cache);

  // Debug cleanup
  debugLauncher.registerCleanupOnSessionEnd(context);

  // Commands
  context.subscriptions.push(
    vscode.commands.registerCommand("gotest.runTest", (testId: string) => {
      const item = testController.findItem(testId);
      if (item) {
        const request = new vscode.TestRunRequest([item]);
        runner.run(request, new vscode.CancellationTokenSource().token);
      }
    }),
    vscode.commands.registerCommand("gotest.debugTest", (testId: string) => {
      const item = testController.findItem(testId);
      if (item) {
        const request = new vscode.TestRunRequest([item]);
        debugLauncher.debug(
          request,
          new vscode.CancellationTokenSource().token,
          () => undefined,
          () => undefined,
        );
      }
    }),
    vscode.commands.registerCommand("gotest.refreshTests", () => {
      discoveryService.discover(workspaceDir);
    }),
    vscode.commands.registerCommand("gotest.showFocusedTests", () => {
      diagnostics.showFocusedTests();
    }),
  );

  // File watcher
  const watcher = vscode.workspace.createFileSystemWatcher("**/*_test.go");
  const discoverOnSave = () => {
    const enabled = vscode.workspace
      .getConfiguration("gotest")
      .get<boolean>("discoverOnSave", true);
    if (enabled) {
      discoveryService.discover(workspaceDir);
    }
  };
  watcher.onDidChange(discoverOnSave);
  watcher.onDidCreate(discoverOnSave);
  watcher.onDidDelete(discoverOnSave);
  context.subscriptions.push(watcher);

  // Disposables
  context.subscriptions.push(
    cache,
    codeLensProvider,
    diagnostics,
    debugLauncher,
    testController,
    outputChannel,
  );

  // Initial discovery
  discoveryService.discover(workspaceDir);
  outputChannel.appendLine("Go Test Suites extension activated");
}

export function deactivate(): void {}
```

- [ ] **Step 3: Verify build**

Run: `cd vscode-gotest && npm run compile`

Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add vscode-gotest/src/extension.ts vscode-gotest/package.json
git commit -m "feat(vscode): wire all components in extension entry point"
```

---

## Task 12: Integration Test — End-to-End

**Files:**
- Create: `vscode-gotest/test/suite/index.ts`
- Modify: `vscode-gotest/package.json` (test script)

- [ ] **Step 1: Create test runner setup**

Create `vscode-gotest/test/suite/index.ts`:

```typescript
import * as path from "node:path";
import { runTests } from "@vscode/test-electron";

async function main() {
  try {
    const extensionDevelopmentPath = path.resolve(__dirname, "../../");
    const extensionTestsPath = path.resolve(__dirname, "./run.js");

    await runTests({
      extensionDevelopmentPath,
      extensionTestsPath,
      launchArgs: ["--disable-extensions"],
    });
  } catch (err) {
    console.error("Failed to run tests:", err);
    process.exit(1);
  }
}

main();
```

- [ ] **Step 2: Verify full build and structure**

Run:
```bash
cd vscode-gotest && npm run compile && ls dist/
```

Expected: `extension.js` and source maps present

- [ ] **Step 3: Final commit for v1 scaffold**

```bash
git add vscode-gotest/test/
git commit -m "feat(vscode): add integration test scaffold"
```

---

## Task 13: Manual Verification

- [ ] **Step 1: Build the Go CLI with discover/overlay**

Run:
```bash
cd /home/ubuntu/projects/mvrahden/go-test && go build -o gotest ./cmd/gotest/
```

Expected: Binary builds without errors

- [ ] **Step 2: Test `gotest discover`**

Run:
```bash
./gotest discover ./examples/simple_suite
```

Expected: JSON output with `SimpleTestSuite`, methods `TestSucceeds` and `TestFails`, correct file/line info

- [ ] **Step 3: Test `gotest overlay`**

Run:
```bash
./gotest overlay ./examples/simple_suite
```

Expected: JSON output with `overlayFile` path pointing to a file that exists on disk

- [ ] **Step 4: Test debug flow manually**

Run:
```bash
OVERLAY=$(./gotest overlay ./examples/simple_suite | jq -r .overlayFile)
dlv test ./examples/simple_suite --build-flags="-overlay=$OVERLAY" -- -test.run ^TestSimpleTestSuite$
```

Expected: Delve starts successfully, `(dlv)` prompt appears

- [ ] **Step 5: Clean up and commit any fixes**

```bash
rm -f gotest
git status
# If any fixes needed, commit them
```

---

## Summary of Commits

1. `feat(cli): add gotest discover subcommand for IDE integration`
2. `feat(cli): add gotest overlay subcommand for debug tooling`
3. `feat(vscode): scaffold extension project with types and build config`
4. `feat(vscode): implement discovery service and cache`
5. `feat(vscode): implement TestController with resolver and tree builder`
6. `feat(vscode): implement test runner and JSON output parser`
7. `feat(vscode): implement CodeLens provider for suites and methods`
8. `feat(vscode): implement debug launcher with overlay + dlv integration`
9. `feat(vscode): implement focus/exclude code actions with prefix toggling`
10. `feat(vscode): implement focus diagnostics and status bar indicator`
11. `feat(vscode): wire all components in extension entry point`
12. `feat(vscode): add integration test scaffold`
13. Manual verification (fix commits as needed)
