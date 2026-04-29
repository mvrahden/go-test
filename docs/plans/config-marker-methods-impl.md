# Config Marker Methods — Implementation Plan

**Goal:** Add `FixtureConfig` / `SuiteConfig` marker methods with sensible defaults always applied, per the spec at `docs/plans/config-marker-methods.md`.

**Architecture:** New `pkg/gotest/config.go` defines the config types and defaults. AST layer detects optional marker methods. Templates always emit config-aware code — using the user's method when present, framework defaults otherwise. `NewTWithDeadline` in `pkg/gotest/t.go` provides per-test-case context deadlines.

**Tech Stack:** Go, `go/types` AST inspection, `text/template`

---

## File Map

- **Create:** `pkg/gotest/config.go` — `FixtureConfig`, `SuiteConfig` structs + preset constructors
- **Modify:** `pkg/gotest/t.go` — add `ctx` field, `NewTWithDeadline`, update `Context()`
- **Modify:** `internal/gotestast/fixture.go:29-41` — add `Config *types.Func` field to `FixtureSpec`
- **Modify:** `internal/gotestast/fixture.go:108-194` — add `FixtureConfig` case in `DetermineFixtureHarness`
- **Modify:** `internal/gotestast/spec.go:30` — add `SuiteConfig` to `IS_TEST_SUITE_METHOD` regex
- **Modify:** `internal/gotestast/spec.go:216-222` — add `Config *types.Func` field to `TestSuiteHarness`
- **Modify:** `internal/gotestast/spec.go:300-421` — add `SuiteConfig` case in `DetermineTestSuiteHarness`
- **Modify:** `internal/gotestgen/renderer.go:30-39` — add `HasConfig` to `FixtureViewModel`
- **Modify:** `internal/gotestgen/renderer.go:94-132` — add `"time"` import
- **Modify:** `internal/gotestgen/renderer.go:176-240` — set `HasConfig` in `buildFixtureViewModels`
- **Modify:** `internal/gotestgen/static/gotest.fixture.tpl` — config-aware fixture code generation
- **Modify:** `internal/gotestgen/static/gotest.suites.tpl` — config-aware suite code generation
- **Modify:** `examples/fixture_suite/` — update golden files to match new generated output
- **Modify:** `examples/nested_fixture/` — update golden files
- **Test:** `internal/gotestgen/renderer_test.go` — new tests for config-aware rendering
- **Test:** `internal/gotestgen/collector_test.go` — new tests for config detection
- **Test:** `internal/gotestast/fixture_test.go` — new tests for `FixtureConfig` detection
- **Test:** `internal/gotestast/spec_test.go` — new tests for `SuiteConfig` detection

---

### Task 1: Config types and defaults

**Files:**
- Create: `pkg/gotest/config.go`

- [ ] **Step 1: Create `pkg/gotest/config.go`**

```go
package gotest

import "time"

type FixtureConfig struct {
	Timeout    time.Duration
	Retries    int
	RetryDelay time.Duration
}

type SuiteConfig struct {
	Timeout      time.Duration
	SetupTimeout time.Duration
	Retries      int
	FailFast     bool
}

func DefaultFixtureConfig() FixtureConfig {
	return FixtureConfig{Timeout: 2 * time.Minute}
}

func ContainerFixtureConfig() FixtureConfig {
	return FixtureConfig{Timeout: 5 * time.Minute, Retries: 1, RetryDelay: 5 * time.Second}
}

func DefaultSuiteConfig() SuiteConfig {
	return SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second}
}

func IntegrationSuiteConfig() SuiteConfig {
	return SuiteConfig{Timeout: 2 * time.Minute, SetupTimeout: 5 * time.Minute}
}
```

- [ ] **Step 2: Build**

Run: `go build ./pkg/gotest/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/gotest/config.go
git commit -m "feat: add FixtureConfig and SuiteConfig types with defaults"
```

---

### Task 2: `NewTWithDeadline` and context override

**Files:**
- Modify: `pkg/gotest/t.go`

- [ ] **Step 1: Add `ctx` field to `T` struct and update `Context()`**

In `pkg/gotest/t.go`, add `ctx context.Context` field to the `T` struct and make `Context()` prefer it when non-nil:

```go
type T struct {
	t         *testing.T
	ctx       context.Context
	collector *collectingT
}

func (t *T) Context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return t.t.Context()
}
```

- [ ] **Step 2: Add `NewTWithDeadline` constructor**

```go
func NewTWithDeadline(t *testing.T, timeout time.Duration) *T {
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return &T{t: t, ctx: ctx}
}
```

- [ ] **Step 3: Build and test**

Run: `go build ./pkg/gotest/... && go test ./pkg/gotest/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/gotest/t.go
git commit -m "feat: add NewTWithDeadline constructor with context override"
```

---

### Task 3: AST detection — `FixtureConfig` on fixtures

**Files:**
- Modify: `internal/gotestast/fixture.go`
- Test: `internal/gotestast/fixture_test.go`

- [ ] **Step 1: Add `Config` field to `FixtureSpec`**

In `internal/gotestast/fixture.go`, add to `FixtureSpec` struct after line 39:

```go
Config     *types.Func   // FixtureConfig() method, may be nil
```

- [ ] **Step 2: Add `FixtureConfig` to the name guard in `DetermineFixtureHarness`**

At line 123, change:
```go
if name != "BeforeAll" && name != "AfterAll" && name != "BeforeEach" && name != "AfterEach" {
```
to:
```go
if name != "BeforeAll" && name != "AfterAll" && name != "BeforeEach" && name != "AfterEach" && name != "FixtureConfig" {
```

- [ ] **Step 3: Add `FixtureConfig` case before the `switch f.Kind` block**

After the receiver-type match (line 148) and before `switch f.Kind` (line 153), add an early return for `FixtureConfig`:

```go
if name == "FixtureConfig" {
	if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () gotest.FixtureConfig", methodID)
	}
	resType := sig.Results().At(0).Type().String()
	if !strings.HasSuffix(resType, "/gotest.FixtureConfig") {
		return m.Pos(), fmt.Errorf("unsupported return type for %q: expected gotest.FixtureConfig, got %s", methodID, resType)
	}
	f.Config = m
	return -1, nil
}
```

- [ ] **Step 4: Write test for `FixtureConfig` detection**

In `internal/gotestast/fixture_test.go` (or `internal/gotestgen/collector_test.go` where `loadTestPkgWithGotest` lives), add a test:

```go
func TestCollector_FixtureConfig_Detected(t *testing.T) {
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.True(t, result.Fixtures[0].Config != nil, "expected Config to be set")
}
```

Also add a test that fixtures without `FixtureConfig()` have `Config == nil`:

```go
func TestCollector_FixtureConfig_AbsentIsNil(t *testing.T) {
	src := `package testpkg

import "context"

type PlainFixture struct{}

func (f *PlainFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.True(t, result.Fixtures[0].Config == nil, "expected Config to be nil")
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/gotestast/... ./internal/gotestgen/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/gotestast/fixture.go internal/gotestgen/collector_test.go
git commit -m "feat: detect FixtureConfig marker method in AST"
```

---

### Task 4: AST detection — `SuiteConfig` on suites

**Files:**
- Modify: `internal/gotestast/spec.go`
- Test: `internal/gotestgen/collector_test.go`

- [ ] **Step 1: Add `SuiteConfig` to `IS_TEST_SUITE_METHOD` regex**

At line 30, change:
```go
IS_TEST_SUITE_METHOD = &regexpW{regexp2.MustCompile(`^(?:BeforeAll|AfterAll|BeforeEach|AfterEach|(?:X_|F_)?(Test(?!Parallel)|TestParallel).+)$`, regexp2.ECMAScript)}
```
to:
```go
IS_TEST_SUITE_METHOD = &regexpW{regexp2.MustCompile(`^(?:BeforeAll|AfterAll|BeforeEach|AfterEach|SuiteConfig|(?:X_|F_)?(Test(?!Parallel)|TestParallel).+)$`, regexp2.ECMAScript)}
```

- [ ] **Step 2: Add `Config` field to `TestSuiteHarness`**

At line 216-222, add:
```go
type TestSuiteHarness struct {
	BeforeAll  *TestSuiteMethod
	BeforeEach *TestSuiteMethod
	AfterAll   *TestSuiteMethod
	AfterEach  *TestSuiteMethod
	TestCases  []*TestSuiteMethod
	Config     *types.Func // SuiteConfig() method, may be nil
}
```

- [ ] **Step 3: Add rendering accessor to `TestSuiteSpec`**

Add after the existing rendering accessor methods (around line 198):

```go
func (ts *TestSuiteSpec) HasConfig() bool { return ts.th.Config != nil }
```

- [ ] **Step 4: Add `SuiteConfig` case in `DetermineTestSuiteHarness`**

In the switch block at line 356-368, add a case before `default`:

```go
case "SuiteConfig":
	if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () gotest.SuiteConfig", methodID)
	}
	resType := sig.Results().At(0).Type().String()
	if !strings.HasSuffix(resType, "/gotest.SuiteConfig") {
		return m.Pos(), fmt.Errorf("unsupported return type for %q: expected gotest.SuiteConfig, got %s", methodID, resType)
	}
	s.th.Config = m
	return -1, nil
```

Note: The `SuiteConfig` case must return before the `detectParamT` / test-case logic runs (it has no `*gotest.T` / `*testing.T` parameter).

- [ ] **Step 5: Write tests**

```go
func TestCollector_SuiteConfig_Detected(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.DefaultSuiteConfig()
}
func (s *MyTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, result.Suites[0].HasConfig(), "expected HasConfig() to be true")
}

func TestCollector_SuiteConfig_AbsentIsFalse(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, !result.Suites[0].HasConfig(), "expected HasConfig() to be false")
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/gotestast/... ./internal/gotestgen/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/gotestast/spec.go internal/gotestgen/collector_test.go
git commit -m "feat: detect SuiteConfig marker method in AST"
```

---

### Task 5: Renderer — `HasConfig` on `FixtureViewModel` + `"time"` import

**Files:**
- Modify: `internal/gotestgen/renderer.go`

- [ ] **Step 1: Add `HasConfig` to `FixtureViewModel`**

At line 30-39, add field:
```go
type FixtureViewModel struct {
	Identifier     string
	HasConfig      bool
	BeforeAll      bool
	AfterAll       bool
	BeforeEach     bool
	AfterEach      bool
	ChildSuites    []*gotestast.TestSuiteSpec
	ChildFixtures  []*FixtureViewModel
	SharedFixtures []SharedFixtureRef
}
```

- [ ] **Step 2: Set `HasConfig` in `buildFixtureViewModels`**

At line 183-191, add:
```go
vm := &FixtureViewModel{
	Identifier:     f.Identifier(),
	HasConfig:      f.Config != nil,
	BeforeAll:      f.BeforeAll != nil,
	// ... rest unchanged
}
```

- [ ] **Step 3: Always add `"time"` import in `renderFileHeader`**

At line 104-116, add `"time"` to the base imports list:
```go
imports := []Import{
	{Path: "testing"},
	{Path: "time"},
	{Path: about.Repo + "/pkg/gotest"},
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/gotestgen/...`
Expected: Existing tests may fail since generated output now includes `"time"` import. That's expected — golden files and string-match tests will be updated in Task 8.

- [ ] **Step 5: Commit**

```bash
git add internal/gotestgen/renderer.go
git commit -m "feat: add HasConfig to FixtureViewModel and time import"
```

---

### Task 6: Template — config-aware fixture code generation

**Files:**
- Modify: `internal/gotestgen/static/gotest.fixture.tpl`

- [ ] **Step 1: Update root fixture template**

Replace the root fixture function (lines 17-43 approximately) to always emit config-aware code. The key changes:

After `fixture := &{{ $f.Identifier }}{}`:
```
{{- if $f.HasConfig }}
    ƒcfg := fixture.FixtureConfig()
{{- else }}
    ƒcfg := gotest.DefaultFixtureConfig()
{{- end }}
```

Replace the `t.Cleanup` block:
```
    t.Cleanup(func() {
{{- if $f.AfterAll }}
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        if err := fixture.AfterAll(ctx); err != nil {
            t.Errorf("{{ $f.Identifier }}.AfterAll failed: %v", err)
        }
{{- end }}
    })
```

Replace the `BeforeAll` call with retry loop:
```
    var ƒerr error
    ƒattempts := 1 + ƒcfg.Retries
    for ƒi := range ƒattempts {
        ctx := t.Context()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        ƒerr = fixture.BeforeAll(ctx)
        if ƒerr == nil {
            break
        }
        if ƒi < ƒattempts-1 {
            t.Logf("{{ $f.Identifier }}.BeforeAll attempt %d/%d failed: %v", ƒi+1, ƒattempts, ƒerr)
            if ƒcfg.RetryDelay > 0 {
                time.Sleep(ƒcfg.RetryDelay)
            }
        }
    }
    if ƒerr != nil {
        t.Fatalf("{{ $f.Identifier }}.BeforeAll failed after %d attempt(s): %v", ƒattempts, ƒerr)
    }
```

- [ ] **Step 2: Update child fixture template (Level 2 nesting)**

Apply the same pattern to the child fixture section (lines 142-278). Use `child` instead of `fixture`:

```
{{- if $cf.HasConfig }}
        ƒcfg_child := child.FixtureConfig()
{{- else }}
        ƒcfg_child := gotest.DefaultFixtureConfig()
{{- end }}
```

And apply the same retry loop and timeout wrapping for `child.BeforeAll` and `child.AfterAll`.

- [ ] **Step 3: Verify template compiles**

Run: `go build ./internal/gotestgen/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/gotestgen/static/gotest.fixture.tpl
git commit -m "feat: config-aware fixture template with retry and timeout"
```

---

### Task 7: Template — config-aware suite code generation

**Files:**
- Modify: `internal/gotestgen/static/gotest.suites.tpl`
- Modify: `internal/gotestgen/static/gotest.fixture.tpl` (suite sections within fixture template)

- [ ] **Step 1: Update standalone suite template (`gotest.suites.tpl`)**

After `s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{}`, add config resolution:

```
{{- if $ts.HasConfig }}
  ƒcfg := s.{{ $ts.Identifier }}.SuiteConfig()
{{- else }}
  ƒcfg := gotest.DefaultSuiteConfig()
{{- end }}
```

In `newTestCase`, add deadline wrapping after `ttt := gotest.NewT(it)`:
```
        ttt := gotest.NewT(it)
        if ƒcfg.Timeout > 0 {
            ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
        }
```

Do the same for `newParallelTestCase`.

After the test case loop, add FailFast check:
```
  for _, tc := range testCases {
    tc(tt)
    if ƒcfg.FailFast && t.Failed() {
      break
    }
  }
```

- [ ] **Step 2: Update fixture-bound suite sections in `gotest.fixture.tpl`**

Both the root fixture child-suite section and the nested fixture child-suite section need the same treatment: config resolution, timeout wrapping in `newTestCase`/`newParallelTestCase`, FailFast in the loop.

- [ ] **Step 3: Verify template compiles**

Run: `go build ./internal/gotestgen/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/gotestgen/static/gotest.suites.tpl internal/gotestgen/static/gotest.fixture.tpl
git commit -m "feat: config-aware suite template with timeout and failfast"
```

---

### Task 8: Update golden files and fix existing tests

**Files:**
- Modify: `examples/fixture_suite/testdata/gotestgen_ptest.golden`
- Modify: `examples/nested_fixture/testdata/gotestgen_ptest.golden`
- Modify: `examples/simple_suite/testdata/gotestgen_ptest.golden`
- Modify: `examples/parallel_suite/testdata/gotestgen_ptest.golden`
- Modify: `examples/focus_exclude/testdata/gotestgen_ptest.golden`
- Modify: `examples/generic_suite/testdata/gotestgen_ptest.golden`
- Modify: `examples/stdlib/testdata/gotestgen_ptest.golden`
- Modify: `internal/gotestgen/renderer_test.go`

- [ ] **Step 1: Regenerate golden files**

Run the code generator against each example to produce new golden output:

```bash
go run ./cmd/gotest gen ./examples/fixture_suite
go run ./cmd/gotest gen ./examples/nested_fixture
go run ./cmd/gotest gen ./examples/simple_suite
go run ./cmd/gotest gen ./examples/parallel_suite
go run ./cmd/gotest gen ./examples/focus_exclude
go run ./cmd/gotest gen ./examples/generic_suite
go run ./cmd/gotest gen ./examples/stdlib
```

Then copy the generated `*_gotest_test.go` output to the corresponding golden files. Alternatively, if the golden tests fail, inspect the diff and update the golden files to match the new expected output.

- [ ] **Step 2: Update `renderer_test.go` string assertions**

The existing renderer tests check for specific strings in the generated output. Update them to expect the config-aware code. Key changes:

- `TestRenderer_FixtureWithChildSuite`: expect `gotest.DefaultFixtureConfig()`, retry loop, timeout wrapping
- `TestRenderer_FixtureWithoutAfterAll`: expect `gotest.DefaultFixtureConfig()`, retry loop
- `TestRenderer_FixtureWithBeforeAfterEach`: expect config, timeout in AfterAll cleanup
- `TestRenderer_MixedFixtureBoundAndStandalone`: expect config in both fixture and standalone sections
- Standalone suite tests: expect `gotest.DefaultSuiteConfig()`, timeout wrapping in `newTestCase`

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add examples/ internal/gotestgen/renderer_test.go
git commit -m "test: update golden files and renderer tests for config defaults"
```

---

### Task 9: New renderer tests for config marker methods

**Files:**
- Test: `internal/gotestgen/renderer_test.go`

- [ ] **Step 1: Test fixture with `FixtureConfig()` marker**

```go
func TestRenderer_FixtureWithConfig(t *testing.T) {
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CFGFixture struct{}

func (f *CFGFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *CFGFixture) AfterAll(ctx context.Context) error  { return nil }
func (f *CFGFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}

type CFGTestSuite struct {
	*CFGFixture
}

func (s *CFGTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)

	// Should call user's FixtureConfig(), not DefaultFixtureConfig()
	gotest.Contains(t, output, "fixture.FixtureConfig()")
	gotest.True(t, !strings.Contains(output, "gotest.DefaultFixtureConfig()"), "should use user config, not default")

	// Should have retry loop
	gotest.Contains(t, output, "ƒattempts := 1 + ƒcfg.Retries")
	gotest.Contains(t, output, "fixture.BeforeAll(ctx)")

	// Should have timeout wrapping in AfterAll cleanup
	gotest.Contains(t, output, "context.WithTimeout(ctx, ƒcfg.Timeout)")
}
```

- [ ] **Step 2: Test fixture without `FixtureConfig()` uses default**

```go
func TestRenderer_FixtureWithoutConfig_UsesDefault(t *testing.T) {
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PlainFixture struct{}

func (f *PlainFixture) BeforeAll(ctx context.Context) error { return nil }

type PlainTestSuite struct {
	*PlainFixture
}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)

	// Should call DefaultFixtureConfig()
	gotest.Contains(t, output, "gotest.DefaultFixtureConfig()")
	gotest.True(t, !strings.Contains(output, "fixture.FixtureConfig()"), "should use default, not user config")
}
```

- [ ] **Step 3: Test suite with `SuiteConfig()` marker**

```go
func TestRenderer_SuiteWithConfig(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type ConfiguredTestSuite struct{}

func (s *ConfiguredTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 10_000_000_000, FailFast: true}
}
func (s *ConfiguredTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)

	// Should call user's SuiteConfig()
	gotest.Contains(t, output, "s.ConfiguredTestSuite.SuiteConfig()")
	gotest.True(t, !strings.Contains(output, "gotest.DefaultSuiteConfig()"), "should use user config")

	// Should have timeout wrapping and failfast
	gotest.Contains(t, output, "gotest.NewTWithDeadline(it, ƒcfg.Timeout)")
	gotest.Contains(t, output, "ƒcfg.FailFast && t.Failed()")
}
```

- [ ] **Step 4: Test suite without `SuiteConfig()` uses default**

```go
func TestRenderer_SuiteWithoutConfig_UsesDefault(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)

	// Should call DefaultSuiteConfig()
	gotest.Contains(t, output, "gotest.DefaultSuiteConfig()")
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/gotestgen/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/gotestgen/renderer_test.go
git commit -m "test: add renderer tests for config marker methods"
```

---

### Task 10: End-to-end verification

- [ ] **Step 1: Run the full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 2: Run the generator against the fixture_suite example and inspect output**

```bash
go run ./cmd/gotest gen ./examples/fixture_suite
cat examples/fixture_suite/*_gotest_test.go
```

Verify the generated code:
- Contains `gotest.DefaultFixtureConfig()` (since the example fixture has no `FixtureConfig()` method)
- Contains retry loop with `ƒattempts`
- Contains timeout wrapping with `context.WithTimeout`
- Contains `gotest.DefaultSuiteConfig()` (since the example suite has no `SuiteConfig()` method)
- Contains `gotest.NewTWithDeadline` in `newTestCase`
- Contains `ƒcfg.FailFast && t.Failed()` check

- [ ] **Step 3: Run the generated tests**

```bash
cd examples/fixture_suite && go test -v -count=1 ./...
```
Expected: All tests pass

- [ ] **Step 4: Commit any final fixes**

```bash
git add -A
git commit -m "fix: end-to-end adjustments for config marker methods"
```
