# Test Suite Improvements Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve test speed, coverage, and CI reliability across the go-test project.

**Architecture:** Five independent changes: (1) short-circuit stdlib package loading, (2) cache module setup across gotestgen tests, (3) add t.Parallel() to gotestgen tests, (4) extract and unit-test pure functions from cmd/gotest, (5) enable workspace-dependent tests in CI.

**Tech Stack:** Go 1.24, golang.org/x/tools/go/packages, fsnotify, GitHub Actions

---

### Task 1: Short-circuit stdlib package detection in loadPackages

The `loadPackages` function in `internal/gotestgen/generator.go` calls `packages.Load()` for all targets including stdlib packages like `net/http`, then filters out packages with `Module == nil`. For stdlib packages this wastes seconds on type-checking. The fix: after loading, if all results have `Module == nil`, return early with an empty slice — before the expensive filter/reduce pipeline.

**Files:**
- Modify: `internal/gotestgen/generator.go:66-105`
- Test: `internal/gotestgen/generator_test.go` (existing file)

- [ ] **Step 1: Write a test for stdlib early return**

Add to `internal/gotestgen/generator_test.go`:

```go
func TestGenerate_StdlibPackage_ReturnsEmpty(t *testing.T) {
	res, err := Generate("strings")
	gotest.NoError(t, err)
	gotest.Empty(t, res)
}
```

- [ ] **Step 2: Run the test — verify it passes but is slow**

Run: `go test ./internal/gotestgen/ -run TestGenerate_StdlibPackage -v -count=1`

This test already passes (stdlib returns empty) but takes several seconds. Record the time.

- [ ] **Step 3: Add early return in loadPackages for non-module packages**

In `internal/gotestgen/generator.go`, in the `loadPackages` function, replace the current filter chain (lines 75-82) with an early-return check:

```go
// filter all packages with Go-Module support
loadedTestPkgs := slices.Filter(totalFoundPkgs, func(item *packages.Package, index int) bool {
	return item.Module != nil
})
if len(loadedTestPkgs) == 0 {
	return nil, nil
}
// filter all test-related packages
loadedTestPkgs = slices.Filter(loadedTestPkgs, func(item *packages.Package, index int) bool {
	return strings.HasSuffix(item.ID, ".test]")
})
```

This is a minimal change — we just add `if len(loadedTestPkgs) == 0 { return nil, nil }` after the module filter to skip the rest of the pipeline.

- [ ] **Step 4: Run the test — verify it passes faster**

Run: `go test ./internal/gotestgen/ -run TestGenerate_StdlibPackage -v -count=1`

Expect: PASS in under 1 second.

- [ ] **Step 5: Run the full stdlib E2E tests to confirm no regressions**

Run: `go test ./internal/cmd/testgen/ -run TestE2E_NoTestSuites -v -count=1`

Expect: All 4 subtests pass, and the two stdlib tests are significantly faster.

- [ ] **Step 6: Commit**

```bash
git add internal/gotestgen/generator.go internal/gotestgen/generator_test.go
git commit -m "perf: early return in loadPackages when no module packages found"
```

---

### Task 2: Cache module setup in gotestgen test helper

All 49 tests using `loadTestPkgWithGotest` independently create a temp directory, write `go.mod`, run `go mod tidy`, then call `packages.Load`. The `go mod tidy` call is the main bottleneck. Fix: run `go mod tidy` once via `sync.Once` and copy the resolved `go.mod`/`go.sum` to each test's temp directory.

**Files:**
- Modify: `internal/gotestgen/collector_test.go:49-88`

- [ ] **Step 1: Add shared module cache using sync.Once**

Replace the `loadTestPkgWithGotest` function in `internal/gotestgen/collector_test.go` (lines 49-88) with a cached version:

```go
var sharedMod struct {
	once   sync.Once
	goMod  []byte
	goSum  []byte
	err    error
}

func initSharedMod() {
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		sharedMod.err = err
		return
	}

	dir, err := os.MkdirTemp("", "gotest-shared-mod-*")
	if err != nil {
		sharedMod.err = err
		return
	}
	defer os.RemoveAll(dir)

	goMod := []byte("module testpkg\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + modRoot + "\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), goMod, 0644); err != nil {
		sharedMod.err = err
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "stub.go"), []byte("package testpkg\n"), 0644); err != nil {
		sharedMod.err = err
		return
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		sharedMod.err = fmt.Errorf("go mod tidy: %w\n%s", err, out)
		return
	}

	sharedMod.goMod, _ = os.ReadFile(filepath.Join(dir, "go.mod"))
	sharedMod.goSum, _ = os.ReadFile(filepath.Join(dir, "go.sum"))
}

// loadTestPkgWithGotest loads a package that imports gotest.T using the full
// packages.Load machinery. Module resolution is cached across tests.
func loadTestPkgWithGotest(t *testing.T, src string) *packages.Package {
	t.Helper()

	sharedMod.once.Do(initSharedMod)
	if sharedMod.err != nil {
		t.Fatal(sharedMod.err)
	}

	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "test.go"), []byte(src), 0644))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), sharedMod.goMod, 0644))
	if len(sharedMod.goSum) > 0 {
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), sharedMod.goSum, 0644))
	}

	pkgs, err := packages.Load(&packages.Config{
		Mode: packageEvalMode,
		Dir:  dir,
	}, ".")
	gotest.NoError(t, err)
	gotest.True(t, len(pkgs) > 0, "expected at least one package loaded")

	pkg := pkgs[0]
	gotest.True(t, len(pkg.Errors) == 0, "expected no package errors, got: %v", pkg.Errors)
	return pkg
}
```

Also add the necessary imports to the import block: `"fmt"` and `"sync"`.

- [ ] **Step 2: Run the collector tests to verify they still pass**

Run: `go test ./internal/gotestgen/ -run "TestCollector|TestValidation|TestApply" -v -count=1`

Expect: All tests pass. The first test takes ~2s (the one-time `go mod tidy`), subsequent tests should be much faster.

- [ ] **Step 3: Run the renderer and shared fixture integration tests**

Run: `go test ./internal/gotestgen/ -run "TestRenderer|TestSharedFixture_Integration" -v -count=1`

Expect: All tests pass with similar speedup.

- [ ] **Step 4: Run the full gotestgen test suite and compare total time**

Run: `go test ./internal/gotestgen/ -v -count=1 2>&1 | tail -5`

Expect: Significant reduction in total test time (from ~120s to ~30-40s).

- [ ] **Step 5: Commit**

```bash
git add internal/gotestgen/collector_test.go
git commit -m "perf: cache go mod tidy across gotestgen tests via sync.Once"
```

---

### Task 3: Add t.Parallel() to independent gotestgen tests

Now that module setup is cached and each test uses its own `t.TempDir()`, the tests are safe to parallelize.

**Files:**
- Modify: `internal/gotestgen/collector_test.go`
- Modify: `internal/gotestgen/renderer_test.go`
- Modify: `internal/gotestgen/sharedfixture_integration_test.go`

- [ ] **Step 1: Add t.Parallel() to collector tests**

Add `t.Parallel()` as the first line inside every `func Test*(t *testing.T)` function in `internal/gotestgen/collector_test.go` that calls `loadTestPkgWithGotest`. Do NOT add it to `TestCollector_NilPackage` (line 616) or the `TestValidation_*` / `TestApplyTestSuiteSpecs_*` tests that don't call `loadTestPkgWithGotest`.

The tests to add `t.Parallel()` to (add it as the first statement in each function body):
- `TestCollector_FixtureCollection_PackageFixture`
- `TestCollector_FixtureCollection_PackageFixtureAllMethods`
- `TestCollector_FixtureCollection_SharedFixture`
- `TestCollector_FixtureCollection_SharedFixtureWithAfterAll`
- `TestCollector_FixtureEmbeddingInTestSuite`
- `TestCollector_NoFixtureEmbedding`
- `TestCollector_FixtureToFixtureEmbedding`
- `TestCollector_SharedFixture_BeforeEachDisallowed`
- `TestCollector_SharedFixture_AfterEachDisallowed`
- `TestCollector_SharedFixture_WrongBeforeAllSignature`
- `TestCollector_SuiteEmbedsMultipleFixtures`
- `TestCollector_PackageFixture_WrongBeforeAllSignature`
- `TestCollector_StdlibT_SuiteDetected`
- `TestCollector_StdlibT_LifecycleHooks`
- `TestCollector_StdlibT_MixedMethodSignatures`
- `TestCollector_StdlibT_WrongParamType`
- `TestCollector_GotestT_NotUsesStdlibT`
- `TestCollector_SharedFixtureNotTreatedAsParent`
- `TestCollector_FixtureConfig_Detected`
- `TestCollector_SharedFixtureConfig_Detected`
- `TestCollector_FixtureConfig_AbsentIsNil`
- `TestCollector_SuiteConfig_Detected`
- `TestCollector_SuiteConfig_AbsentIsFalse`
- `TestCollector_FixtureConfig_InvalidSignature_WithParams`
- `TestCollector_FixtureConfig_InvalidSignature_WrongReturnType`
- `TestCollector_SuiteConfig_InvalidSignature_WithParams`
- `TestCollector_SuiteConfig_InvalidSignature_WrongReturnType`

- [ ] **Step 2: Add t.Parallel() to renderer tests**

Add `t.Parallel()` as the first statement in every `func Test*(t *testing.T)` function in `internal/gotestgen/renderer_test.go`.

- [ ] **Step 3: Add t.Parallel() to shared fixture integration tests**

Add `t.Parallel()` as the first statement in these functions in `internal/gotestgen/sharedfixture_integration_test.go`:
- `TestSharedFixture_Integration_DiscoverFromRealPackage`
- `TestSharedFixture_Integration_DiscoverFromRealPackage_MultipleFixtures`
- `TestSharedFixture_Integration_DiscoverWithHydrate`

Do NOT add to tests that don't call `loadTestPkgWithGotest`.

- [ ] **Step 4: Run all gotestgen tests and verify they pass**

Run: `go test ./internal/gotestgen/ -v -count=1 2>&1 | tail -5`

Expect: All pass, with noticeably faster total time due to parallelism.

- [ ] **Step 5: Commit**

```bash
git add internal/gotestgen/collector_test.go internal/gotestgen/renderer_test.go internal/gotestgen/sharedfixture_integration_test.go
git commit -m "perf: parallelize gotestgen tests with t.Parallel()"
```

---

### Task 4: Extract and unit-test pure functions from cmd/gotest

The `cmd/gotest` package has 6.3% coverage. Several pure functions have no tests: `isGoFile`, `dirsToPatterns`, `replacePatterns` (watch.go), `parseMinFlag` (cli.go), and `looksLikePackagePattern` (args.go). These are all pure logic with no I/O dependencies.

**Files:**
- Create: `cmd/gotest/watch_test.go`
- Create: `cmd/gotest/cli_test.go`
- Modify: `cmd/gotest/args_test.go` (add test for `looksLikePackagePattern`)

- [ ] **Step 1: Write tests for watch.go pure functions**

Create `cmd/gotest/watch_test.go`:

```go
package main

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestIsGoFile(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		name   string
		expect bool
	}{
		{desc: "go file", name: "main.go", expect: true},
		{desc: "test file", name: "main_test.go", expect: true},
		{desc: "path with go file", name: "/tmp/foo/bar.go", expect: true},
		{desc: "not a go file", name: "main.py", expect: false},
		{desc: "go in middle", name: "foo.go.bak", expect: false},
		{desc: "empty", name: "", expect: false},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotest.Equal(t, tc.expect, isGoFile(tc.name))
		})
	}
}

func TestDirsToPatterns(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		dirs    map[string]bool
		lenWant int
	}{
		{desc: "single dir", dirs: map[string]bool{"pkg/foo": true}, lenWant: 1},
		{desc: "multiple dirs", dirs: map[string]bool{"pkg/foo": true, "cmd/bar": true}, lenWant: 2},
		{desc: "empty", dirs: map[string]bool{}, lenWant: 0},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := dirsToPatterns(tc.dirs)
			gotest.Len(t, result, tc.lenWant)
			for _, p := range result {
				gotest.True(t, len(p) > 2 && p[:2] == "./", "expected ./ prefix, got: %s", p)
			}
		})
	}
}

func TestReplacePatterns(t *testing.T) {
	for _, tc := range []struct {
		desc        string
		original    []string
		newPatterns []string
		expected    []string
	}{
		{
			desc:        "replaces package pattern",
			original:    []string{"-v", "./pkg/foo", "-race"},
			newPatterns: []string{"./cmd/bar"},
			expected:    []string{"-v", "-race", "./cmd/bar"},
		},
		{
			desc:        "no patterns to replace",
			original:    []string{"-v", "-race"},
			newPatterns: []string{"./pkg/new"},
			expected:    []string{"-v", "-race", "./pkg/new"},
		},
		{
			desc:        "multiple patterns replaced",
			original:    []string{"-v", "./pkg/a", "./pkg/b", "-race"},
			newPatterns: []string{"./changed"},
			expected:    []string{"-v", "-race", "./changed"},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := replacePatterns(tc.original, tc.newPatterns)
			gotest.Equal(t, tc.expected, result)
		})
	}
}
```

- [ ] **Step 2: Run watch tests to verify they pass**

Run: `go test ./cmd/gotest/ -run "TestIsGoFile|TestDirsToPatterns|TestReplacePatterns" -v -count=1`

Expect: All pass.

- [ ] **Step 3: Write tests for parseMinFlag**

Create `cmd/gotest/cli_test.go`:

```go
package main

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestParseMinFlag(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		args   []string
		expect int
	}{
		{desc: "no flag", args: []string{"--debug"}, expect: 0},
		{desc: "equals syntax", args: []string{"--min=80"}, expect: 80},
		{desc: "space syntax", args: []string{"--min", "90"}, expect: 90},
		{desc: "empty args", args: nil, expect: 0},
		{desc: "invalid value", args: []string{"--min=abc"}, expect: 0},
		{desc: "min at end no value", args: []string{"--min"}, expect: 0},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotest.Equal(t, tc.expect, parseMinFlag(tc.args))
		})
	}
}
```

- [ ] **Step 4: Run cli tests to verify they pass**

Run: `go test ./cmd/gotest/ -run TestParseMinFlag -v -count=1`

Expect: All pass.

- [ ] **Step 5: Add looksLikePackagePattern tests to args_test.go**

Append to `cmd/gotest/args_test.go`:

```go
func TestLooksLikePackagePattern(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		input  string
		expect bool
	}{
		{desc: "relative path", input: "./pkg/foo", expect: true},
		{desc: "absolute path", input: "/usr/local/pkg", expect: true},
		{desc: "named package", input: "github.com/foo/bar", expect: true},
		{desc: "flag", input: "-v", expect: false},
		{desc: "bare word", input: "strings", expect: false},
		{desc: "dot only", input: ".", expect: true},
		{desc: "dot-slash", input: "./...", expect: true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotest.Equal(t, tc.expect, looksLikePackagePattern(tc.input))
		})
	}
}
```

- [ ] **Step 6: Run all cmd/gotest tests to verify**

Run: `go test ./cmd/gotest/ -v -count=1`

Expect: All tests pass including existing args tests and new tests.

- [ ] **Step 7: Commit**

```bash
git add cmd/gotest/watch_test.go cmd/gotest/cli_test.go cmd/gotest/args_test.go
git commit -m "test: add unit tests for pure functions in cmd/gotest"
```

---

### Task 5: Enable workspace-dependent tests in CI

The `go.work` file is gitignored (intentionally — it's a local development convenience). Three tests skip when it's absent: `TestGeneratorGoldenExamples`, `TestSharedFixture_E2E_MultiPackage`, and `TestSharedFixture_E2E_DumpGolden` (this one also requires `DUMP_GOLDEN=1` so it will remain skipped). The fix: add a CI step to create the workspace before running tests.

**Files:**
- Modify: `.github/workflows/test.yml`

- [ ] **Step 1: Add workspace setup step to CI**

In `.github/workflows/test.yml`, add a step between `actions/setup-go@v5` (line 16-18) and the Test step (line 19-20):

```yaml
      - name: Init workspace
        run: |
          go work init . ./examples
```

- [ ] **Step 2: Verify the CI file is valid YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"`

Expect: No errors.

- [ ] **Step 3: Verify the skipped tests pass locally with go.work**

Run: `go test ./internal/gotestgen/ -run "TestGeneratorGoldenExamples|TestSharedFixture_E2E_MultiPackage" -v -count=1`

Expect: Both tests run (not skipped) and pass.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/test.yml
git commit -m "ci: init go workspace to enable golden and e2e tests"
```
