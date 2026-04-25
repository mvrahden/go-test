# Testsuite Drop-In Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `testsuite` CLI a production-ready, drop-in augmentation for `go test` that transparently adds test-suite lifecycle support via code generation.

**Architecture:** The CLI is a thin wrapper: split own flags (`-ƒƒ.*`) from everything else, resolve target packages via `packages.Load`, generate ephemeral test harness files, execute `go test` with the original args (streaming output), and clean up generated files via `defer` to guarantee safety on all exit paths including signals. The tool never parses `go test` flags — it delegates entirely to the Go toolchain.

**Tech Stack:** Go 1.22+, `golang.org/x/tools/go/packages`, `text/template`, `go/format`

---

## File Map

### Files to Modify

| File | Responsibility | Task |
|------|---------------|------|
| `examples/go.mod` | Example module dependency config | 1 |
| `pkg/gotest/internal/assert/base_test.go` | Assertion unit tests | 2 |
| `internal/gotestgen/generator_test.go` | Generator golden tests | 3 |
| `internal/gotestrunner/stdlib.go` | `go test` subprocess execution | 4 |
| `internal/gotestgen/generator.go` | Code generation orchestrator | 5 |
| `internal/gotestgen/utils.go` | Package dir resolution | 6 |
| `internal/gotestrunner/suites.go` | Suite file I/O (generate + cleanup) | 7, 8 |
| `internal/gotestgen/static/gotest.suites.tpl` | Generated test harness template | 9 |
| `cmd/testsuite/cli.go` | CLI entrypoint | 12 |
| `cmd/testsuite/args.go` | Argument handling | 11 |
| `cmd/testsuite/exec.go` | Execution pipeline | 12 |
| `internal/testutils/utils.go` | Shared test utilities | 13 |
| `about/git.go` | Build metadata | 14 |

### Files to Delete

| File | Reason | Task |
|------|--------|------|
| `cmd/testsuite/args_unit_test.go` | Replaced by new arg handling tests | 11 |
| `internal/gotestgen/load_cache.go` | Cache replaced by single upfront load | 12 |

### Files to Create

| File | Responsibility | Task |
|------|---------------|------|
| `cmd/testsuite/args_test.go` | Tests for new arg splitting | 11 |
| `cmd/testsuite/discover.go` | Package discovery via `packages.Load` | 11 |
| `cmd/testsuite/discover_test.go` | Tests for package discovery | 11 |
| `cmd/testsuite/cleanup.go` | Filesystem-based cleanup | 12 |

---

## Task 1: Fix `examples/go.mod` Hardcoded Path

The `examples/go.mod` has `replace github.com/mvrahden/go-test => /Users/menno/Projects/github.com/mvrahden/go-test` — a hardcoded absolute path to the original author's machine. All E2E tests fail on any other system.

**Files:**
- Modify: `examples/go.mod:7`

- [ ] **Step 1: Fix the replace directive**

```go
// examples/go.mod — change line 7 from:
replace github.com/mvrahden/go-test => /Users/menno/Projects/github.com/mvrahden/go-test
// to:
replace github.com/mvrahden/go-test => ../
```

- [ ] **Step 2: Run `go mod tidy` in the examples dir**

Run: `cd examples && go mod tidy`
Expected: No errors. `go.sum` may update.

- [ ] **Step 3: Verify the module resolves**

Run: `cd examples && go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add examples/go.mod examples/go.sum
git commit -m "fix: use relative replace directive in examples/go.mod"
```

---

## Task 2: Fix Pointer Regex in Assertion Tests

The `PtrRegex` in `base_test.go` matches standalone pointer addresses (`0xc0000b2010`) but fails when pointers appear inside map representations like `map[1:0x52d4a0]` because the regex requires 9-11 hex chars, while some platform pointer values are shorter (6-7 chars, e.g., `0x52d4a0`).

**Files:**
- Modify: `pkg/gotest/internal/assert/base_test.go:15`

- [ ] **Step 1: Verify the current failure**

Run: `go test ./pkg/gotest/internal/assert/ -run Test_BaseAsserter_IsTrue_Fail -v`
Expected: FAIL — `got: "0x52d500 is not true"` vs `want: "<POINTER_REF> is not true"`

- [ ] **Step 2: Fix the pointer regex to accept variable-length hex addresses**

In `pkg/gotest/internal/assert/base_test.go`, change line 15 from:
```go
PtrRegex = regexp.MustCompile(`0x[[:alnum:]]{9,11}`)
```
to:
```go
PtrRegex = regexp.MustCompile(`0x[0-9a-f]{4,16}`)
```

This matches hex addresses from 4 to 16 chars (covers 32-bit, 64-bit, and all compaction levels). Uses `[0-9a-f]` instead of `[[:alnum:]]` to avoid matching non-hex chars.

- [ ] **Step 3: Run the assertion tests**

Run: `go test ./pkg/gotest/internal/assert/ -v`
Expected: All PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/gotest/internal/assert/base_test.go
git commit -m "fix: broaden pointer regex in assertion tests for variable-length addresses"
```

---

## Task 3: Fix Generator Tests Referencing Deleted Example Dirs

`generator_test.go` has test cases for `examples/my` and `examples/focus_suite` which no longer exist on this branch. Remove those test cases.

**Files:**
- Modify: `internal/gotestgen/generator_test.go:32-33`

- [ ] **Step 1: Remove the stale test cases**

In `internal/gotestgen/generator_test.go`, remove these two lines from the test table (lines 32-33):
```go
		{"my", "github.com/mvrahden/go-test/examples/my", "standard test suites"},
		{"focus_suite", "github.com/mvrahden/go-test/examples/focus_suite", "focus test suites"},
```

The table should only contain:
```go
	for _, tC := range []struct {
		directory   string
		pkgName     string
		description string
	}{
		{"stdlib", "github.com/mvrahden/go-test/examples/stdlib", "stdlib and testsuites"},
		{"simple_suite", "github.com/mvrahden/go-test/examples/simple_suite", "simple test suites"},
	} {
```

- [ ] **Step 2: Run the generator tests**

Run: `go test ./internal/gotestgen/ -run TestGeneratorGoldenExamples -v`
Expected: All PASS (2 test cases).

- [ ] **Step 3: Commit**

```bash
git add internal/gotestgen/generator_test.go
git commit -m "fix: remove generator test cases for deleted example directories"
```

---

## Task 4: Fix Double-Append in `StdlibRunTests`

`StdlibRunTests` appends args twice — once in `exec.Command` and again in the `if` block.

**Files:**
- Modify: `internal/gotestrunner/stdlib.go:7-14`
- Test: (E2E tests cover this after Task 1 is done)

- [ ] **Step 1: Write a unit test for `StdlibRunTests`**

Create the test inline in `internal/gotestrunner/stdlib.go` — wait, we need a test file. We don't have one. Since `StdlibRunTests` shells out to `go test`, a unit test would be slow. Instead, verify by inspection and rely on the E2E tests after Task 1.

Replace the entire function body in `internal/gotestrunner/stdlib.go`:

```go
package gotestrunner

import (
	"os/exec"
)

func StdlibRunTests(args []string) (out []byte, code int, err error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	out, _ = cmd.CombinedOutput()
	return out, cmd.ProcessState.ExitCode(), nil
}
```

This removes the `if len(args) > 0` block that caused the double-append.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/gotestrunner/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/gotestrunner/stdlib.go
git commit -m "fix: remove double-append of args in StdlibRunTests"
```

---

## Task 5: Fix Wrong Error Variable in Generator

`generator.go:89` references `ptestCollected.Errs[0].Err` when it should reference `pxtestCollected.Errs[0].Err`.

**Files:**
- Modify: `internal/gotestgen/generator.go:89`

- [ ] **Step 1: Fix the variable reference**

In `internal/gotestgen/generator.go`, change line 89 from:
```go
			return nil, ptestCollected.Errs[0].Err
```
to:
```go
			return nil, pxtestCollected.Errs[0].Err
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/gotestgen/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/gotestgen/generator.go
git commit -m "fix: use correct error variable for pxtest collection errors"
```

---

## Task 6: Fix `DeterminePkgDir` Root Package Panic

When `pkgPath == modPath` (test suite at module root), the string slice operation panics with index-out-of-range.

**Files:**
- Modify: `internal/gotestgen/utils.go:10-21`
- Modify: `internal/gotestgen/utils_test.go` (add root-package test case)

- [ ] **Step 1: Add a failing test for the root-package case**

In `internal/gotestgen/utils_test.go`, add this test case to the `testCases` slice:
```go
		{desc: "PTest at module root", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc", PackageName: "module_abc", Expected: "/user/xyz/projects"},
		{desc: "PXTest at module root", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc_test", PackageName: "module_abc_test", Expected: "/user/xyz/projects"},
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/gotestgen/ -run Test/PTest_at_module_root -v`
Expected: FAIL — panic: runtime error: index out of range.

- [ ] **Step 3: Fix `DeterminePkgDir`**

Replace the function in `internal/gotestgen/utils.go`:

```go
func DeterminePkgDir(p *packages.Package) string {
	modDir := p.Module.Dir
	modPath := p.Module.Path
	pkgPath := p.PkgPath
	if isPxTest := strings.HasSuffix(p.Name, "_test"); isPxTest {
		pkgPath = pkgPath[:len(pkgPath)-5] // trim "_test"
	}

	if pkgPath == modPath {
		return modDir
	}

	commonPrefix := len(modPath) + 1
	path := pkgPath[commonPrefix:]
	return filepath.Join(modDir, path)
}
```

- [ ] **Step 4: Run all utils tests**

Run: `go test ./internal/gotestgen/ -run ^Test$ -v`
Expected: All PASS (8 test cases now).

- [ ] **Step 5: Commit**

```bash
git add internal/gotestgen/utils.go internal/gotestgen/utils_test.go
git commit -m "fix: handle module-root packages in DeterminePkgDir"
```

---

## Task 7: Fix `SuitesCleanup` Nil-Module Guard

`SuitesCleanup` iterates all packages from `LoadCached` without filtering for `Module != nil`, causing nil-pointer panics on stdlib or non-module packages.

**Files:**
- Modify: `internal/gotestrunner/suites.go:46-49`

- [ ] **Step 1: Add the nil-Module guard**

In `internal/gotestrunner/suites.go`, change the loop at line 46 from:
```go
	for _, pkg := range pkgs {
		pkgPath := gotestgen.DeterminePkgDir(pkg)
```
to:
```go
	for _, pkg := range pkgs {
		if pkg.Module == nil {
			continue
		}
		pkgPath := gotestgen.DeterminePkgDir(pkg)
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/gotestrunner/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/gotestrunner/suites.go
git commit -m "fix: skip non-module packages in SuitesCleanup to prevent nil panic"
```

---

## Task 8: Fix File Permissions for Generated Files

`SuitesGenerate` writes files with `os.ModePerm` (0777). Generated source files should use 0644.

**Files:**
- Modify: `internal/gotestrunner/suites.go:22,28`

- [ ] **Step 1: Change permissions**

In `internal/gotestrunner/suites.go`, change both `os.WriteFile` calls from:
```go
			err := os.WriteFile(testsuiteFile, result.PTest, os.ModePerm)
```
to:
```go
			err := os.WriteFile(testsuiteFile, result.PTest, 0644)
```

And similarly for the PXTest write on line 28.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/gotestrunner/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/gotestrunner/suites.go
git commit -m "fix: use 0644 permissions for generated test files"
```

---

## Task 9: Fix Template — Defer `AfterEach` and `wg.Done()`

`AfterEach` is called sequentially after `testFn`. If `testFn` calls `t.Fatal()` (which triggers `runtime.Goexit()`), `AfterEach` is never reached. In the parallel variant, `wg.Done()` is also skipped, causing a deadlock.

**Files:**
- Modify: `internal/gotestgen/static/gotest.suites.tpl:20-45`

- [ ] **Step 1: Update the sequential `newTestCase` closure**

In `internal/gotestgen/static/gotest.suites.tpl`, replace lines 20-30 (the `newTestCase` block) with:

```
{{ if $ts.TestCases -}}
  newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
    return func(tt *gotest.T) {
      t := tt.T()
      t.Run(desc, func(it *testing.T) {
        ttt := gotest.NewT(it)
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        testFn(ttt)
      })
    }}
{{- end }}
```

- [ ] **Step 2: Update the parallel `newParallelTestCase` closure**

Replace lines 31-45 (the `newParallelTestCase` block) with:

```
{{- if $ts.HasParallelTestCases }}
  newParallelTestCase := func(desc string, wg *sync.WaitGroup, testFn gotest.TestCase) gotest.TestCase {
    wg.Add(1)
    return func(tt *gotest.T) {
      t := tt.T()
      t.Run(desc, func(it *testing.T) {
        it.Parallel()
        defer wg.Done()
        ttt := gotest.NewT(it)
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        testFn(ttt)
      })
    }}
  wg := &sync.WaitGroup{}
{{- end }}
```

Key changes: `defer s.AfterEach(ttt)` replaces the sequential call; `defer wg.Done()` is added before `AfterEach` so it runs after `AfterEach` (defers are LIFO).

- [ ] **Step 3: Update golden files**

The golden files in `examples/*/testdata/` and `internal/cmd/testgen/testdata/*/` must be regenerated to match the new template output. Run the generator and capture the new output:

Run: `go test ./internal/cmd/testgen/ -run TestE2E_CLI -v`
Expected: FAIL — golden files don't match the new template output.

Copy the actual output to replace the golden files. The diff should show only the addition of `defer` keywords.

For each golden file containing `newTestCase` or `newParallelTestCase`, update the body to use `defer` for `AfterEach` and `wg.Done()`.

The affected golden files are:
- `examples/simple_suite/testdata/gotestgen_ptest.golden`
- `examples/simple_suite/testdata/gotestgen_pxtest.golden`
- `examples/stdlib/testdata/gotestgen_ptest.golden`
- `examples/stdlib/testdata/gotestgen_pxtest.golden`
- `internal/cmd/testgen/testdata/testsuite/ƒƒ_psuite_test.go.golden`
- `internal/cmd/testgen/testdata/testsuite/ƒƒ_pxsuite_test.go.golden`

- [ ] **Step 4: Run generator tests**

Run: `go test ./internal/gotestgen/ -run TestGeneratorGoldenExamples -v`
Expected: PASS.

Run: `go test ./internal/cmd/testgen/ -run TestE2E_CLI -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gotestgen/static/gotest.suites.tpl
git add examples/simple_suite/testdata/ examples/stdlib/testdata/
git add internal/cmd/testgen/testdata/
git commit -m "fix: defer AfterEach and wg.Done in generated template to prevent deadlock"
```

---

## Task 10: Fix Exit Code Propagation for Generation Errors

When `fanOutJob` reports an error, the collector prints it but skips the `maxCode` update. The process exits 0 even when generation fails.

**Files:**
- Modify: `cmd/testsuite/exec.go:27-31`

- [ ] **Step 1: Set exit code on generation errors**

In `cmd/testsuite/exec.go`, change the error branch in `collectorFunc` from:
```go
			if r.Error != nil {
				fmt.Fprintf(os.Stdout, "FAIL  in:     %s\n", r.Dir)
				fmt.Fprintf(os.Stdout, "      due to the following error:\n")
				fmt.Fprintf(os.Stdout, " -->  %s\n", r.Error)
				continue
			}
```
to:
```go
			if r.Error != nil {
				fmt.Fprintf(os.Stderr, "FAIL  in:     %s\n", r.Dir)
				fmt.Fprintf(os.Stderr, "      due to the following error:\n")
				fmt.Fprintf(os.Stderr, " -->  %s\n", r.Error)
				if maxCode < r.Code {
					maxCode = r.Code
				}
				continue
			}
```

Two changes: (1) errors go to stderr instead of stdout (they're not test output), (2) `maxCode` is updated so the process exits non-zero.

- [ ] **Step 2: Verify compilation**

Run: `go build ./cmd/testsuite/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add cmd/testsuite/exec.go
git commit -m "fix: propagate exit code for generation errors and write errors to stderr"
```

---

## Task 11: Replace Flag Parser with Package Discovery

The current `parseNArgs` tries to parse `go test` flags to separate package names — an impossible problem (boolean vs value flags, custom flags, new Go versions). Replace it with a simpler approach: extract own flags, identify package patterns heuristically, resolve via `packages.Load`, pass everything else through.

**Files:**
- Rewrite: `cmd/testsuite/args.go`
- Delete: `cmd/testsuite/args_unit_test.go`
- Create: `cmd/testsuite/args_test.go`
- Create: `cmd/testsuite/discover.go`
- Create: `cmd/testsuite/discover_test.go`

- [ ] **Step 1: Write tests for the new `SplitArgs` function**

Create `cmd/testsuite/args_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitArgs(t *testing.T) {
	for _, tc := range []struct {
		desc         string
		inArgs       []string
		expectOwn    []string
		expectGoTest []string
	}{
		{
			desc:         "empty",
			inArgs:       nil,
			expectOwn:    nil,
			expectGoTest: nil,
		},
		{
			desc:         "only go test args",
			inArgs:       []string{"-v", "./...", "-race", "-count=1"},
			expectOwn:    nil,
			expectGoTest: []string{"-v", "./...", "-race", "-count=1"},
		},
		{
			desc:         "only own args",
			inArgs:       []string{"-ƒƒ.internal.debug"},
			expectOwn:    []string{"-ƒƒ.internal.debug"},
			expectGoTest: nil,
		},
		{
			desc:         "mixed args",
			inArgs:       []string{"-ƒƒ.internal.debug", "-v", "./...", "-race"},
			expectOwn:    []string{"-ƒƒ.internal.debug"},
			expectGoTest: []string{"-v", "./...", "-race"},
		},
		{
			desc:         "own args interleaved",
			inArgs:       []string{"-v", "-ƒƒ.internal.debug", "./...", "-ƒƒ.clean"},
			expectOwn:    []string{"-ƒƒ.internal.debug", "-ƒƒ.clean"},
			expectGoTest: []string{"-v", "./..."},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			own, goTest := SplitArgs(tc.inArgs)
			require.Equal(t, tc.expectOwn, own)
			require.Equal(t, tc.expectGoTest, goTest)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/testsuite/ -run TestSplitArgs -v`
Expected: FAIL — `SplitArgs` signature has changed.

- [ ] **Step 3: Rewrite `args.go`**

Replace `cmd/testsuite/args.go` entirely:

```go
package main

import (
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
)

type ExecConfig struct {
	GoTestArgs  []string // complete args passed to `go test` verbatim
	PackageDirs []string // resolved filesystem directories to scan for suites
}

// SplitArgs separates the tool's own flags (-ƒƒ.*) from go test args.
func SplitArgs(inArgs []string) (ownArgs, goTestArgs []string) {
	for _, arg := range inArgs {
		if strings.HasPrefix(arg, "-ƒƒ.") {
			ownArgs = append(ownArgs, arg)
		} else {
			goTestArgs = append(goTestArgs, arg)
		}
	}
	return ownArgs, goTestArgs
}

// ParseOwnArgs processes the tool's own flags.
func ParseOwnArgs(args []string) {
	DEBUG = slices.Contains(args, "-ƒƒ.internal.debug")
	CLEAN = slices.Contains(args, "-ƒƒ.clean")
}

// RunGoTest executes `go test` with the given args.
var RunGoTest = gotestrunner.StdlibRunTests
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/testsuite/ -run TestSplitArgs -v`
Expected: PASS.

- [ ] **Step 5: Write tests for package discovery**

Create `cmd/testsuite/discover_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractPackagePatterns(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		args     []string
		expected []string
	}{
		{
			desc:     "explicit relative path",
			args:     []string{"-v", "./...", "-race"},
			expected: []string{"./..."},
		},
		{
			desc:     "explicit named package",
			args:     []string{"-v", "github.com/foo/bar", "-race"},
			expected: []string{"github.com/foo/bar"},
		},
		{
			desc:     "no package defaults to dot",
			args:     []string{"-v", "-race"},
			expected: []string{"."},
		},
		{
			desc:     "multiple packages",
			args:     []string{"./pkg/a", "./pkg/b", "-v"},
			expected: []string{"./pkg/a", "./pkg/b"},
		},
		{
			desc:     "stops at -args",
			args:     []string{"-v", "./...", "-args", "-custom", "./not/a/pkg"},
			expected: []string{"./..."},
		},
		{
			desc:     "flag values not mistaken for packages",
			args:     []string{"-run", "TestFoo", "./...", "-timeout", "30s"},
			expected: []string{"./..."},
		},
		{
			desc:     "no args at all defaults to dot",
			args:     nil,
			expected: []string{"."},
		},
		{
			desc:     "bare relative path",
			args:     []string{"-v", "./cmd/testsuite"},
			expected: []string{"./cmd/testsuite"},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := ExtractPackagePatterns(tc.args)
			require.Equal(t, tc.expected, result)
		})
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./cmd/testsuite/ -run TestExtractPackagePatterns -v`
Expected: FAIL — `ExtractPackagePatterns` is not defined.

- [ ] **Step 7: Implement `discover.go`**

Create `cmd/testsuite/discover.go`:

```go
package main

import (
	"strings"
)

// ExtractPackagePatterns identifies Go package patterns from a mixed
// list of go test arguments. It uses a simple heuristic: a token is a
// package pattern if it doesn't start with "-" and it looks like a
// path (starts with ".", "/", or contains "/"). Tokens after "-args"
// are ignored. If no patterns are found, defaults to ".".
func ExtractPackagePatterns(goTestArgs []string) []string {
	var patterns []string
	for _, arg := range goTestArgs {
		if arg == "-args" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if looksLikePackagePattern(arg) {
			patterns = append(patterns, arg)
		}
	}
	if len(patterns) == 0 {
		return []string{"."}
	}
	return patterns
}

// looksLikePackagePattern returns true if the token looks like a Go
// package pattern rather than a flag value. Package patterns start
// with ".", "/", or contain "/" (like "github.com/foo/bar").
func looksLikePackagePattern(s string) bool {
	if strings.HasPrefix(s, ".") {
		return true
	}
	if strings.HasPrefix(s, "/") {
		return true
	}
	if strings.Contains(s, "/") {
		return true
	}
	return false
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./cmd/testsuite/ -run TestExtractPackagePatterns -v`
Expected: PASS.

- [ ] **Step 9: Delete old args test file**

```bash
rm cmd/testsuite/args_unit_test.go
```

- [ ] **Step 10: Verify all cmd/testsuite tests pass**

Run: `go test ./cmd/testsuite/ -v`
Expected: All PASS.

- [ ] **Step 11: Commit**

```bash
git add cmd/testsuite/args.go cmd/testsuite/args_test.go
git add cmd/testsuite/discover.go cmd/testsuite/discover_test.go
git rm cmd/testsuite/args_unit_test.go
git commit -m "refactor: replace go-test flag parser with heuristic package discovery"
```

---

## Task 12: Redesign Execution Pipeline

Replace the fan-out pipeline with a linear generate→run→cleanup model. Key changes: (1) single upfront `packages.Load` instead of per-package calls, (2) deferred cleanup for signal safety, (3) streaming output, (4) filesystem-based cleanup.

**Files:**
- Rewrite: `cmd/testsuite/cli.go`
- Rewrite: `cmd/testsuite/exec.go`
- Modify: `internal/gotestrunner/stdlib.go` (streaming)
- Modify: `internal/gotestrunner/suites.go` (filesystem cleanup)
- Create: `cmd/testsuite/cleanup.go`
- Delete: `internal/gotestgen/load_cache.go`

- [ ] **Step 1: Create `cleanup.go` — filesystem-based cleanup**

Create `cmd/testsuite/cleanup.go`:

```go
package main

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
)

// CleanupGeneratedFiles removes all generated suite files from the
// given directories. It scans by filename match rather than relying
// on in-memory state, so it works even after crashes.
func CleanupGeneratedFiles(dirs []string) {
	for _, dir := range dirs {
		os.Remove(filepath.Join(dir, about.PSuite))
		os.Remove(filepath.Join(dir, about.PXSuite))
	}
}
```

- [ ] **Step 2: Rewrite `stdlib.go` for streaming output**

Replace `internal/gotestrunner/stdlib.go`:

```go
package gotestrunner

import (
	"os"
	"os/exec"
)

// StdlibRunTests runs `go test` with the given args, streaming
// stdout and stderr directly to the parent process.
func StdlibRunTests(args []string) (code int, err error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return 2, err
	}
	return 0, nil
}
```

Note: the return signature changes from `([]byte, int, error)` to `(int, error)` — no buffered output needed when streaming.

- [ ] **Step 3: Update `SuitesGenerate` to accept a list of dirs**

Modify `internal/gotestrunner/suites.go` to simplify:

```go
package gotestrunner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

// SuitesGenerate generates suite harness files for the given package
// pattern and returns the list of directories where files were written.
func SuitesGenerate(pkgPattern string) (dirs []string, err error) {
	results, err := testgen.GenerateSuites(pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed generating suites: %w", err)
	}

	for _, result := range results {
		if len(result.PTest) > 0 {
			testsuiteFile := filepath.Join(result.AbsPath, about.PSuite)
			err := os.WriteFile(testsuiteFile, result.PTest, 0644)
			if err != nil {
				return dirs, fmt.Errorf("failed writing ptest: %w", err)
			}
		}
		if len(result.PXTest) > 0 {
			testsuiteFile := filepath.Join(result.AbsPath, about.PXSuite)
			err := os.WriteFile(testsuiteFile, result.PXTest, 0644)
			if err != nil {
				return dirs, fmt.Errorf("failed writing pxtest: %w", err)
			}
		}
		dirs = append(dirs, result.AbsPath)
	}
	return dirs, nil
}
```

The function now returns the dirs it wrote to, so the caller can pass them to cleanup. The old `SuitesCleanup` function is removed (replaced by `CleanupGeneratedFiles` in the CLI package).

- [ ] **Step 4: Rewrite `exec.go` — linear pipeline with defer**

Replace `cmd/testsuite/exec.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// Run is the main execution pipeline: generate → run → cleanup.
// Cleanup is deferred so it runs on all exit paths including signals.
func Run(cfg ExecConfig) int {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Phase 1: Generate suite files for all target packages
	var allDirs []string
	for _, pattern := range cfg.PackageDirs {
		dirs, err := gotestrunner.SuitesGenerate(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allDirs = append(allDirs, dirs...)
	}

	// Defer cleanup BEFORE running tests — guarantees cleanup on
	// signal, panic, or any early return.
	if !DEBUG {
		defer CleanupGeneratedFiles(allDirs)
	}

	// Phase 2: Run go test with original args, streaming output.
	// Check for cancellation (signal received during generation).
	select {
	case <-ctx.Done():
		return 130 // standard exit code for SIGINT
	default:
	}

	code, err := gotestrunner.StdlibRunTests(cfg.GoTestArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}
```

- [ ] **Step 5: Rewrite `cli.go`**

Replace `cmd/testsuite/cli.go`:

```go
package main

import (
	"fmt"
	"os"
)

var (
	DEBUG bool
	CLEAN bool
)

func main() {
	ownArgs, goTestArgs := SplitArgs(os.Args[1:])
	ParseOwnArgs(ownArgs)

	if CLEAN {
		patterns := ExtractPackagePatterns(goTestArgs)
		runClean(patterns)
		return
	}

	patterns := ExtractPackagePatterns(goTestArgs)
	cfg := ExecConfig{
		GoTestArgs:  goTestArgs,
		PackageDirs: patterns,
	}

	os.Exit(Run(cfg))
}

func runClean(patterns []string) {
	for _, pattern := range patterns {
		dirs, err := resolvePatternToDirs(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: could not resolve %s: %s\n", pattern, err)
			continue
		}
		CleanupGeneratedFiles(dirs)
	}
}

// resolvePatternToDirs uses the generator to resolve a package
// pattern to filesystem directories.
func resolvePatternToDirs(pattern string) ([]string, error) {
	// Reuse the same packages.Load path as generation
	results, err := gotestrunner.SuitesGenerate(pattern)
	if err != nil {
		return nil, err
	}
	// SuitesGenerate returns dirs it wrote to, but for clean mode
	// we don't want to generate. This is a placeholder — see step 7.
	return results, nil
}
```

Wait — `runClean` needs to resolve patterns to dirs without generating. We need a separate resolution path. Let me fix this.

- [ ] **Step 6: Add a resolve-only path for clean mode**

Update `cmd/testsuite/cli.go` to handle clean mode properly. The clean mode should walk the filesystem under the resolved dirs and remove any files matching `about.PSuiteRegex`:

```go
func runClean(patterns []string) {
	for _, pattern := range patterns {
		// For clean mode, walk directories looking for orphaned files
		// rather than running the full generator pipeline.
		err := filepath.WalkDir(pattern, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip inaccessible paths
			}
			if !d.IsDir() && about.PSuiteRegex.MatchString(d.Name()) {
				fmt.Fprintf(os.Stdout, "removing %s\n", path)
				return os.Remove(path)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: clean failed for %s: %s\n", pattern, err)
		}
	}
}
```

Add these imports to `cli.go`:
```go
import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
)
```

Remove the `resolvePatternToDirs` function — it's not needed.

- [ ] **Step 7: Remove `load_cache.go` and update imports**

Delete `internal/gotestgen/load_cache.go`.

In `internal/gotestgen/generator.go`, replace the `LoadCached` call with a direct `packages.Load`:

Change `loadPackages` function:
```go
func loadPackages(targetPkg string) ([]*loadResult, error) {
	totalFoundPkgs, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return nil, err
	}
```

Remove any references to `LoadCached` from `internal/gotestrunner/suites.go` (the old `SuitesCleanup` is already removed in step 3).

- [ ] **Step 8: Verify compilation of all packages**

Run: `go build ./...`
Expected: Clean build. There may be compilation errors from removed types — fix any references to the old `ExecConfig` fields (`MaxConcurrency`, `CWD`, `NArgs`, `PackageNameList`, `SuitesGenerate`, `SuitesRun`, `SuitesCleanup`, `SuiteGeneratorFunc`, `SuiteRunnerFunc`, `SuiteCleanupFunc`).

- [ ] **Step 9: Run all tests**

Run: `go test ./... 2>&1`
Expected: The unit tests pass. E2E tests may need golden file updates due to output changes (stderr vs stdout for errors). Fix as needed.

- [ ] **Step 10: Commit**

```bash
git add cmd/testsuite/ internal/gotestrunner/ internal/gotestgen/
git rm internal/gotestgen/load_cache.go
git commit -m "refactor: redesign execution pipeline with streaming, defer cleanup, and signal safety"
```

---

## Task 13: Fix Timestamp Regex in Test Utilities

The regex `(\d\.\d+s)` only matches single-digit-second durations. Tests taking ≥10s produce `10.123s` which won't match.

**Files:**
- Modify: `internal/testutils/utils.go:22`

- [ ] **Step 1: Fix the regex**

In `internal/testutils/utils.go`, change line 22 from:
```go
	timestampRegex = regexp.MustCompile(`(\d\.\d+s)`)
```
to:
```go
	timestampRegex = regexp.MustCompile(`(\d+\.\d+s)`)
```

The `+` after `\d` allows matching multi-digit seconds.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/testutils/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/testutils/utils.go
git commit -m "fix: broaden timestamp regex to handle multi-digit seconds"
```

---

## Task 14: Replace `init()` Git Shelling with Build-Time Injection

The `init()` in `about/git.go` runs 4 git subprocesses on every import. Replace with build-time `-ldflags` injection.

**Files:**
- Modify: `about/git.go:21-45`

- [ ] **Step 1: Remove the `init()` function and shell imports**

Replace `about/git.go`:

```go
package about

import "regexp"

const (
	PSuite  = "ƒƒ_psuite_test.go"
	PXSuite = "ƒƒ_pxsuite_test.go"
)

var PSuiteRegex = regexp.MustCompile(`ƒƒ_p(x)?suite_test\.go$`)

const (
	Application = "go-test"
	Repo        = "github.com/mvrahden/go-test"
)

// Set via -ldflags at build time. Defaults to "dev" for `go run`.
var (
	Version = "dev"
)
```

- [ ] **Step 2: Update `util.go` to use the new `Version` var**

Replace `about/util.go`:

```go
package about

func ShortInfo() string {
	return Application + " (" + Repo + ")"
}

func LongInfo() string {
	return Repo + " (" + Version + "/" + GoVersion + " " + GoOS + " " + GoArch + ")"
}
```

- [ ] **Step 3: Delete `about/go.go` if `GoVersion`, `GoOS`, `GoArch` are unused elsewhere**

Check whether `GoVersion`, `GoOS`, `GoArch` are used outside `util.go`:

Run: `grep -rn "GoVersion\|GoArch\|GoOS" --include="*.go" | grep -v "about/"`
Expected: If no results, delete `about/go.go` and remove the references from `util.go`. If they are used, keep them.

If unused outside `about/`, simplify `util.go`:
```go
package about

func ShortInfo() string {
	return Application + " (" + Repo + ")"
}

func LongInfo() string {
	return Repo + " (" + Version + ")"
}
```

And delete `about/go.go`.

- [ ] **Step 4: Update `about/git_test.go`**

The test only tests `PSuiteRegex`, not the git variables. Verify it still compiles:

Run: `go test ./about/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add about/
git commit -m "refactor: replace runtime git shelling with build-time version injection"
```

---

## Task 15: Reject Generic Test Suite Structs

If a user writes `type MyTestSuite[T any] struct{}`, the generated code will fail to compile. Detect this early and provide a clear error.

**Files:**
- Modify: `internal/gotestast/spec.go:227-253` (`DetermineTestSuite`)
- Modify: `internal/gotestast/spec_test.go` (add test)

- [ ] **Step 1: Add a test case for generic suites**

This requires a full Go package to parse, so it's best tested via the collector. Instead, add a check in `DetermineTestSuite` and test it at the generator level.

In `internal/gotestast/spec.go`, after line 246 (after the regex match), add a check for type parameters:

```go
	if ts.TypeParams != nil && ts.TypeParams.NumFields() > 0 {
		return nil, ts.Pos(), fmt.Errorf("test suite %q must not have type parameters (generics are not supported)", ts.Name.Name)
	}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/gotestast/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/gotestast/spec.go
git commit -m "feat: reject generic test suite structs with clear error message"
```

---

## Task 16: Run Full Test Suite and Fix Remaining Issues

After all previous tasks, run the complete test suite and fix any remaining integration issues.

**Files:**
- Various (as needed)

- [ ] **Step 1: Run all unit tests**

Run: `go test ./... -short 2>&1`
Expected: Identify any remaining failures.

- [ ] **Step 2: Run E2E tests**

Run: `go test ./e2e/ -v -timeout 120s`
Expected: Identify failures. The E2E tests may need adjustments because:
- `StdlibRunTests` signature changed (no `[]byte` return)
- `SuitesCleanup` was removed from `gotestrunner`
- Golden files may need updating

Fix each failure in order.

- [ ] **Step 3: Run the gotest package E2E test**

Run: `go test ./pkg/gotest/ -v -timeout 120s`
Expected: May need golden file update for `t.golden` due to template changes.

- [ ] **Step 4: Update golden files as needed**

For each golden file mismatch, verify the actual output is correct (not just different), then replace the golden file with the new output.

- [ ] **Step 5: Final verification**

Run: `go test ./... -timeout 120s`
Expected: All PASS.

Run: `go vet ./...`
Expected: No issues.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "fix: resolve remaining integration issues after pipeline redesign"
```

---

## Task 17: Verify Drop-In Behavior End-to-End

Verify the tool works as a transparent `go test` wrapper for the most common invocations.

**Files:**
- Modify: `e2e/cli_pmain_test.go` (add new test cases)

- [ ] **Step 1: Add E2E test for `-v ./...` (most common invocation)**

In `e2e/cli_pmain_test.go`, add a test case to the existing table:
```go
		{basedir: ".", pkgName: "github.com/mvrahden/go-test/examples/...", goldenName: "test_all.txt", debug: false},
```

This tests the recursive walk with `-v` flag — the invocation that was broken by the boolean flag parser.

- [ ] **Step 2: Add E2E test for exit code**

Add a new test function to `e2e/cli_pmain_test.go` that verifies the exit code is non-zero when tests fail:

```go
func Test_TestsuiteCLI_ExitCode(t *testing.T) {
	tmp := t.TempDir()
	testutils.CopyModuleUnderTestToTmp(t, tmp, "./..", ".git", "go.work")
	testutils.ActivateTests(t, tmp)
	testutils.HackGoWork(t, tmp)

	cmd := exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/testsuite",
		filepath.Join(tmp, "examples", "simple_suite"), "-v")
	cmd.Dir = filepath.Join(tmp, "examples")
	_ , err := cmd.CombinedOutput()

	// simple_suite has intentional test failures
	require.Error(t, err)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok)
	require.NotZero(t, exitErr.ExitCode())
}
```

- [ ] **Step 3: Run the new tests**

Run: `go test ./e2e/ -v -timeout 120s`
Expected: All PASS.

- [ ] **Step 4: Commit**

```bash
git add e2e/
git commit -m "test: add E2E tests for common invocations and exit code verification"
```
