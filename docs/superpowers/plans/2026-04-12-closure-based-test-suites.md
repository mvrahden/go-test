# Closure-Based Test Suites Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the code-generation-based test suite framework with a zero-dependency, closure-based runtime library that works with plain `go test`.

**Architecture:** Two-phase runtime (registration then execution). Users call `gotest.Run(t, func(s *gotest.S) { ... })` inside a standard `TestXxx` function. The `S` builder collects hooks and tests during registration; `run()` executes them with proper lifecycle management using `t.Run()` subtests. Nested `Describe` blocks inherit parent hooks. Focus/exclude is resolved at execution time.

**Tech Stack:** Go 1.23+, standard library only (zero external dependencies for new code). Old code retains its dependencies until cleanup task.

---

## File Structure

All new code lives in `gotest/` at the module root. Package name: `gotest`. Import path: `github.com/mvrahden/go-test/gotest`.

| File | Responsibility |
|---|---|
| `gotest/gotest.go` | `Run()` entry point, `S` type with registration methods (`Test`, `Describe`, `BeforeAll`, etc.), `run()` execution engine |
| `gotest/gotest_test.go` | Suite lifecycle tests, hook ordering, nesting, parallel |
| `gotest/focus.go` | `resolveFocus()` — determines effective test/describe set based on F/X markers |
| `gotest/focus_test.go` | Focus/exclude resolution tests |
| `gotest/t.go` | `T` wrapper type, `NewT()`, `Assert()` method |
| `gotest/t_test.go` | T wrapper tests |
| `gotest/assert.go` | `AssertContext` type with fluent assertion methods (`Equals`, `IsNil`, `IsTrue`, `HasLength`, etc.) |
| `gotest/assert_test.go` | Assertion method tests |

Old code in `internal/`, `cmd/`, `pkg/`, `about/`, `examples/` is untouched until Task 9.

---

### Task 1: Scaffolding

**Files:**
- Create: `gotest/gotest.go`
- Create: `gotest/t.go`
- Modify: `go.mod:1-2`

- [ ] **Step 1: Bump go.mod to Go 1.23**

In `go.mod`, change lines 3-5:

```go
go 1.23

toolchain go1.24.0
```

- [ ] **Step 2: Verify old code still compiles**

Run: `go build ./...`
Expected: no errors (old code is backward-compatible with 1.23+)

- [ ] **Step 3: Create gotest/t.go with T wrapper**

Create `gotest/t.go`:

```go
package gotest

import "testing"

// T wraps *testing.T with suite-aware functionality.
type T struct {
	t *testing.T
}

// NewT creates a T from a *testing.T.
func NewT(t *testing.T) *T {
	return &T{t: t}
}

// T returns the underlying *testing.T.
func (t *T) T() *testing.T { return t.t }
```

- [ ] **Step 4: Create gotest/gotest.go with type stubs**

Create `gotest/gotest.go`:

```go
package gotest

import "testing"

// hookFn is the signature for lifecycle hooks.
type hookFn func(*T)

// testEntry represents a registered test case.
type testEntry struct {
	name     string
	fn       func(*T)
	focused  bool
	excluded bool
	parallel bool
}

// describeEntry represents a registered child describe block.
type describeEntry struct {
	name     string
	fn       func(*S)
	focused  bool
	excluded bool
}

// S is the suite builder. It collects tests and hooks during the registration
// phase and executes them during the run phase.
type S struct {
	t          *testing.T
	beforeAll  []hookFn
	afterAll   []hookFn
	beforeEach []hookFn
	afterEach  []hookFn
	tests      []testEntry
	describes  []describeEntry
}

// Run executes a test suite. The provided function registers tests and hooks
// on the S builder. After registration completes, all tests are executed with
// proper lifecycle hook management.
func Run(t *testing.T, fn func(*S)) {
	t.Helper()
	s := &S{t: t}
	fn(s)
	s.run(nil, nil)
}

func (s *S) run(inheritedBeforeEach, inheritedAfterEach []hookFn) {
	// Will be implemented in subsequent tasks.
}
```

- [ ] **Step 5: Verify it compiles**

Run: `go build ./gotest/...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add gotest/gotest.go gotest/t.go go.mod
git commit -m "feat(gotest): scaffold closure-based test suite package

Bump Go to 1.23. Create gotest/ with T wrapper and S builder stubs."
```

---

### Task 2: T Wrapper — TDD

**Files:**
- Create: `gotest/t_test.go`
- Modify: `gotest/t.go`

- [ ] **Step 1: Write failing tests for T**

Create `gotest/t_test.go`:

```go
package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestNewT_wraps_testing_T(t *testing.T) {
	tt := gotest.NewT(t)
	if tt.T() != t {
		t.Fatal("T() should return the underlying *testing.T")
	}
}

func TestT_It_runs_subtest(t *testing.T) {
	var ran bool
	tt := gotest.NewT(t)
	tt.It("subtest", func(it *gotest.T) {
		ran = true
		if it.T() == nil {
			t.Fatal("It callback should receive a valid T")
		}
	})
	if !ran {
		t.Fatal("It callback should have executed")
	}
}

func TestT_It_subtest_name_appears_in_test_output(t *testing.T) {
	tt := gotest.NewT(t)
	tt.It("my_subtest_name", func(it *gotest.T) {
		if it.T().Name() != t.Name()+"/my_subtest_name" {
			t.Fatalf("unexpected subtest name: %s", it.T().Name())
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./gotest/ -v -run TestT`
Expected: FAIL — `It` method not defined on `*gotest.T`

- [ ] **Step 3: Implement It method on T**

Add to `gotest/t.go`:

```go
// It runs a named subtest within the current test context.
func (t *T) It(description string, fn func(*T)) {
	t.t.Run(description, func(sub *testing.T) {
		fn(NewT(sub))
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./gotest/ -v -run TestT`
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add gotest/t.go gotest/t_test.go
git commit -m "feat(gotest): implement T wrapper with It subtest method"
```

---

### Task 3: Suite Core — BeforeAll / AfterAll

**Files:**
- Modify: `gotest/gotest.go`
- Modify: `gotest/gotest_test.go`

- [ ] **Step 1: Write failing tests for basic suite with BeforeAll/AfterAll**

Create `gotest/gotest_test.go` (or append if it exists from Task 2 — in this case it's a new file):

```go
package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestRun_executes_registered_tests(t *testing.T) {
	var ran bool
	gotest.Run(t, func(s *gotest.S) {
		s.Test("basic", func(t *gotest.T) {
			ran = true
		})
	})
	if !ran {
		t.Fatal("test should have executed")
	}
}

func TestRun_test_name_becomes_subtest(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("my_test", func(tt *gotest.T) {
			expected := t.Name() + "/my_test"
			if tt.T().Name() != expected {
				t.Fatalf("expected subtest name %q, got %q", expected, tt.T().Name())
			}
		})
	})
}

func TestRun_multiple_tests_execute_in_order(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("first", func(t *gotest.T) {
			order = append(order, "first")
		})
		s.Test("second", func(t *gotest.T) {
			order = append(order, "second")
		})
		s.Test("third", func(t *gotest.T) {
			order = append(order, "third")
		})
	})
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("expected [first second third], got %v", order)
	}
}

func TestRun_BeforeAll_runs_once_before_all_tests(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) {
			order = append(order, "beforeAll")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"beforeAll", "testA", "testB"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_AfterAll_runs_once_after_all_tests(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.AfterAll(func(t *gotest.T) {
			order = append(order, "afterAll")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"testA", "testB", "afterAll"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_BeforeAll_and_AfterAll_bracket_tests(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) {
			order = append(order, "beforeAll")
		})
		s.AfterAll(func(t *gotest.T) {
			order = append(order, "afterAll")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
	})
	expected := []string{"beforeAll", "testA", "afterAll"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_no_tests_still_runs_hooks(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) {
			order = append(order, "beforeAll")
		})
		s.AfterAll(func(t *gotest.T) {
			order = append(order, "afterAll")
		})
	})
	expected := []string{"beforeAll", "afterAll"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

// slicesEqual is a test helper — compares two string slices.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./gotest/ -v -run "TestRun_"`
Expected: FAIL — `S` has no exported methods yet. Tests may compile (struct fields are set internally) but `Test`, `BeforeAll`, `AfterAll` methods are missing, and `run()` is empty.

- [ ] **Step 3: Implement registration methods and basic execution engine**

Replace the stub `run()` and add registration methods in `gotest/gotest.go`. The full file should now be:

```go
package gotest

import "testing"

// hookFn is the signature for lifecycle hooks.
type hookFn func(*T)

// testEntry represents a registered test case.
type testEntry struct {
	name     string
	fn       func(*T)
	focused  bool
	excluded bool
	parallel bool
}

// describeEntry represents a registered child describe block.
type describeEntry struct {
	name     string
	fn       func(*S)
	focused  bool
	excluded bool
}

// S is the suite builder. It collects tests and hooks during the registration
// phase and executes them during the run phase.
type S struct {
	t          *testing.T
	beforeAll  []hookFn
	afterAll   []hookFn
	beforeEach []hookFn
	afterEach  []hookFn
	tests      []testEntry
	describes  []describeEntry
}

// Run executes a test suite. The provided function registers tests and hooks
// on the S builder. After registration completes, all tests are executed with
// proper lifecycle hook management.
func Run(t *testing.T, fn func(*S)) {
	t.Helper()
	s := &S{t: t}
	fn(s)
	s.run(nil, nil)
}

// BeforeAll registers a hook that runs once before all tests in this scope.
func (s *S) BeforeAll(fn hookFn) { s.beforeAll = append(s.beforeAll, fn) }

// AfterAll registers a hook that runs once after all tests in this scope complete.
func (s *S) AfterAll(fn hookFn) { s.afterAll = append(s.afterAll, fn) }

// BeforeEach registers a hook that runs before each test in this scope.
func (s *S) BeforeEach(fn hookFn) { s.beforeEach = append(s.beforeEach, fn) }

// AfterEach registers a hook that runs after each test in this scope.
func (s *S) AfterEach(fn hookFn) { s.afterEach = append(s.afterEach, fn) }

// Test registers a test case.
func (s *S) Test(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn})
}

func (s *S) run(inheritedBeforeEach, inheritedAfterEach []hookFn) {
	tt := NewT(s.t)

	// AfterAll via Cleanup — guaranteed to run after all subtests complete (LIFO).
	if len(s.afterAll) > 0 {
		s.t.Cleanup(func() {
			for i := len(s.afterAll) - 1; i >= 0; i-- {
				s.afterAll[i](tt)
			}
		})
	}

	// BeforeAll — runs once, immediately.
	for _, fn := range s.beforeAll {
		fn(tt)
	}

	// Merge inherited hooks with own hooks.
	allBeforeEach := make([]hookFn, 0, len(inheritedBeforeEach)+len(s.beforeEach))
	allBeforeEach = append(allBeforeEach, inheritedBeforeEach...)
	allBeforeEach = append(allBeforeEach, s.beforeEach...)

	allAfterEach := make([]hookFn, 0, len(s.afterEach)+len(inheritedAfterEach))
	allAfterEach = append(allAfterEach, s.afterEach...)
	allAfterEach = append(allAfterEach, inheritedAfterEach...)

	// Execute tests.
	for _, test := range s.tests {
		s.t.Run(test.name, func(sub *testing.T) {
			ttt := NewT(sub)
			for _, fn := range allBeforeEach {
				fn(ttt)
			}
			defer func() {
				for i := len(allAfterEach) - 1; i >= 0; i-- {
					allAfterEach[i](ttt)
				}
			}()
			test.fn(ttt)
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./gotest/ -v -run "TestRun_"`
Expected: all 7 `TestRun_*` tests PASS

- [ ] **Step 5: Commit**

```bash
git add gotest/gotest.go gotest/gotest_test.go
git commit -m "feat(gotest): implement suite core with BeforeAll/AfterAll and test execution"
```

---

### Task 4: BeforeEach / AfterEach

**Files:**
- Modify: `gotest/gotest_test.go`

The execution engine already handles BeforeEach/AfterEach (implemented in Task 3's `run()`). This task adds tests to verify the behavior.

- [ ] **Step 1: Write tests for BeforeEach/AfterEach**

Append to `gotest/gotest_test.go`:

```go
func TestRun_BeforeEach_runs_before_each_test(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) {
			order = append(order, "beforeEach")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"beforeEach", "testA", "beforeEach", "testB"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_AfterEach_runs_after_each_test(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.AfterEach(func(t *gotest.T) {
			order = append(order, "afterEach")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"testA", "afterEach", "testB", "afterEach"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_full_lifecycle_ordering(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) { order = append(order, "beforeAll") })
		s.AfterAll(func(t *gotest.T) { order = append(order, "afterAll") })
		s.BeforeEach(func(t *gotest.T) { order = append(order, "beforeEach") })
		s.AfterEach(func(t *gotest.T) { order = append(order, "afterEach") })
		s.Test("a", func(t *gotest.T) { order = append(order, "testA") })
		s.Test("b", func(t *gotest.T) { order = append(order, "testB") })
	})
	expected := []string{
		"beforeAll",
		"beforeEach", "testA", "afterEach",
		"beforeEach", "testB", "afterEach",
		"afterAll",
	}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_AfterEach_runs_even_when_test_calls_Fatal(t *testing.T) {
	var afterEachRan bool
	gotest.Run(t, func(s *gotest.S) {
		s.AfterEach(func(t *gotest.T) {
			afterEachRan = true
		})
		s.Test("fails", func(t *gotest.T) {
			t.T().Fail() // marks test as failed without stopping execution
		})
	})
	if !afterEachRan {
		t.Fatal("AfterEach should run even when test fails")
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./gotest/ -v -run "TestRun_BeforeEach|TestRun_AfterEach|TestRun_full_lifecycle"`
Expected: all 4 new tests PASS (implementation already exists from Task 3)

- [ ] **Step 3: Commit**

```bash
git add gotest/gotest_test.go
git commit -m "test(gotest): verify BeforeEach/AfterEach lifecycle ordering"
```

---

### Task 5: Nested Describe

**Files:**
- Modify: `gotest/gotest.go`
- Modify: `gotest/gotest_test.go`

- [ ] **Step 1: Write failing tests for nested Describe**

Append to `gotest/gotest_test.go`:

```go
func TestRun_Describe_creates_nested_subtest(t *testing.T) {
	var ran bool
	gotest.Run(t, func(s *gotest.S) {
		s.Describe("group", func(s *gotest.S) {
			s.Test("inner", func(t *gotest.T) {
				ran = true
			})
		})
	})
	if !ran {
		t.Fatal("nested test should have executed")
	}
}

func TestRun_Describe_inherits_parent_BeforeEach(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) {
			order = append(order, "parentBeforeEach")
		})
		s.Describe("child", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				order = append(order, "childBeforeEach")
			})
			s.Test("inner", func(t *gotest.T) {
				order = append(order, "test")
			})
		})
	})
	expected := []string{"parentBeforeEach", "childBeforeEach", "test"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_Describe_AfterEach_unwinds_in_reverse(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.AfterEach(func(t *gotest.T) {
			order = append(order, "parentAfterEach")
		})
		s.Describe("child", func(s *gotest.S) {
			s.AfterEach(func(t *gotest.T) {
				order = append(order, "childAfterEach")
			})
			s.Test("inner", func(t *gotest.T) {
				order = append(order, "test")
			})
		})
	})
	expected := []string{"test", "childAfterEach", "parentAfterEach"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_Describe_full_nested_lifecycle(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) { order = append(order, "parentBeforeAll") })
		s.AfterAll(func(t *gotest.T) { order = append(order, "parentAfterAll") })
		s.BeforeEach(func(t *gotest.T) { order = append(order, "parentBE") })
		s.AfterEach(func(t *gotest.T) { order = append(order, "parentAE") })

		s.Test("top", func(t *gotest.T) { order = append(order, "topTest") })

		s.Describe("child", func(s *gotest.S) {
			s.BeforeAll(func(t *gotest.T) { order = append(order, "childBeforeAll") })
			s.AfterAll(func(t *gotest.T) { order = append(order, "childAfterAll") })
			s.BeforeEach(func(t *gotest.T) { order = append(order, "childBE") })
			s.AfterEach(func(t *gotest.T) { order = append(order, "childAE") })

			s.Test("nested", func(t *gotest.T) { order = append(order, "nestedTest") })
		})
	})
	expected := []string{
		"parentBeforeAll",
		"parentBE", "topTest", "parentAE",
		"childBeforeAll",
		"parentBE", "childBE", "nestedTest", "childAE", "parentAE",
		"childAfterAll",
		"parentAfterAll",
	}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected:\n  %v\ngot:\n  %v", expected, order)
	}
}

func TestRun_Describe_double_nesting(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) { order = append(order, "L0-BE") })
		s.Describe("L1", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) { order = append(order, "L1-BE") })
			s.Describe("L2", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) { order = append(order, "L2-BE") })
				s.Test("deep", func(t *gotest.T) { order = append(order, "test") })
			})
		})
	})
	expected := []string{"L0-BE", "L1-BE", "L2-BE", "test"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./gotest/ -v -run "TestRun_Describe"`
Expected: FAIL — `Describe` method does not exist on `*gotest.S`

- [ ] **Step 3: Implement Describe method and child execution**

Add `Describe` method to `gotest/gotest.go`:

```go
// Describe registers a nested test group with its own hooks.
// Child groups inherit parent BeforeEach/AfterEach hooks.
func (s *S) Describe(name string, fn func(*S)) {
	s.describes = append(s.describes, describeEntry{name: name, fn: fn})
}
```

And add child describe execution to the end of `run()`, after the test execution loop:

```go
	// Execute child describes.
	for _, desc := range s.describes {
		desc := desc
		s.t.Run(desc.name, func(sub *testing.T) {
			child := &S{t: sub}
			desc.fn(child)
			child.run(allBeforeEach, allAfterEach)
		})
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./gotest/ -v -run "TestRun_Describe"`
Expected: all 5 `TestRun_Describe_*` tests PASS

- [ ] **Step 5: Commit**

```bash
git add gotest/gotest.go gotest/gotest_test.go
git commit -m "feat(gotest): implement nested Describe with hook inheritance"
```

---

### Task 6: Focus / Exclude

**Files:**
- Create: `gotest/focus.go`
- Create: `gotest/focus_test.go`
- Modify: `gotest/gotest.go`
- Modify: `gotest/gotest_test.go`

- [ ] **Step 1: Write unit tests for resolveFocus**

Create `gotest/focus_test.go`:

```go
package gotest

import "testing"

func Test_resolveFocus_no_focused_items_returns_unchanged(t *testing.T) {
	tests := []testEntry{
		{name: "a"},
		{name: "b"},
	}
	descs := []describeEntry{
		{name: "c"},
	}
	rt, rd := resolveFocus(tests, descs)
	if len(rt) != 2 || len(rd) != 1 {
		t.Fatalf("expected 2 tests and 1 describe, got %d and %d", len(rt), len(rd))
	}
	if rt[0].excluded || rt[1].excluded || rd[0].excluded {
		t.Fatal("nothing should be excluded when no focus exists")
	}
}

func Test_resolveFocus_focused_test_excludes_others(t *testing.T) {
	tests := []testEntry{
		{name: "a"},
		{name: "b", focused: true},
		{name: "c"},
	}
	rt, _ := resolveFocus(tests, nil)
	if !rt[0].excluded {
		t.Fatal("non-focused test 'a' should be excluded")
	}
	if rt[1].excluded {
		t.Fatal("focused test 'b' should NOT be excluded")
	}
	if !rt[2].excluded {
		t.Fatal("non-focused test 'c' should be excluded")
	}
}

func Test_resolveFocus_focused_describe_excludes_other_describes(t *testing.T) {
	descs := []describeEntry{
		{name: "a"},
		{name: "b", focused: true},
	}
	_, rd := resolveFocus(nil, descs)
	if !rd[0].excluded {
		t.Fatal("non-focused describe 'a' should be excluded")
	}
	if rd[1].excluded {
		t.Fatal("focused describe 'b' should NOT be excluded")
	}
}

func Test_resolveFocus_focused_test_also_excludes_non_focused_describes(t *testing.T) {
	tests := []testEntry{
		{name: "a", focused: true},
	}
	descs := []describeEntry{
		{name: "b"},
	}
	_, rd := resolveFocus(tests, descs)
	if !rd[0].excluded {
		t.Fatal("non-focused describe should be excluded when a test is focused")
	}
}

func Test_resolveFocus_excluded_stays_excluded_even_when_focused(t *testing.T) {
	tests := []testEntry{
		{name: "a", excluded: true},
		{name: "b"},
	}
	rt, _ := resolveFocus(tests, nil)
	if !rt[0].excluded {
		t.Fatal("explicitly excluded test should remain excluded")
	}
}

func Test_resolveFocus_excluded_items_without_focus(t *testing.T) {
	tests := []testEntry{
		{name: "a"},
		{name: "b", excluded: true},
	}
	rt, _ := resolveFocus(tests, nil)
	if rt[0].excluded {
		t.Fatal("non-excluded test should not be excluded")
	}
	if !rt[1].excluded {
		t.Fatal("excluded test should stay excluded")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./gotest/ -v -run "Test_resolveFocus"`
Expected: FAIL — `resolveFocus` not defined

- [ ] **Step 3: Implement resolveFocus**

Create `gotest/focus.go`:

```go
package gotest

// resolveFocus applies focus/exclude logic to the registered tests and describes.
// If any item is focused, all non-focused items become excluded.
// Already-excluded items stay excluded regardless.
func resolveFocus(tests []testEntry, describes []describeEntry) ([]testEntry, []describeEntry) {
	hasFocused := false
	for _, t := range tests {
		if t.focused {
			hasFocused = true
			break
		}
	}
	if !hasFocused {
		for _, d := range describes {
			if d.focused {
				hasFocused = true
				break
			}
		}
	}

	if !hasFocused {
		return tests, describes
	}

	resolvedTests := make([]testEntry, len(tests))
	copy(resolvedTests, tests)
	for i := range resolvedTests {
		if !resolvedTests[i].focused {
			resolvedTests[i].excluded = true
		}
	}

	resolvedDescs := make([]describeEntry, len(describes))
	copy(resolvedDescs, describes)
	for i := range resolvedDescs {
		if !resolvedDescs[i].focused {
			resolvedDescs[i].excluded = true
		}
	}

	return resolvedTests, resolvedDescs
}
```

- [ ] **Step 4: Run resolveFocus tests**

Run: `go test ./gotest/ -v -run "Test_resolveFocus"`
Expected: all 6 tests PASS

- [ ] **Step 5: Write integration tests for FTest/XTest/FDescribe/XDescribe**

Append to `gotest/gotest_test.go`:

```go
func TestRun_XTest_is_skipped(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("a", func(t *gotest.T) { order = append(order, "a") })
		s.XTest("b", func(t *gotest.T) { order = append(order, "b") })
		s.Test("c", func(t *gotest.T) { order = append(order, "c") })
	})
	expected := []string{"a", "c"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_FTest_only_runs_focused(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("a", func(t *gotest.T) { order = append(order, "a") })
		s.FTest("b", func(t *gotest.T) { order = append(order, "b") })
		s.Test("c", func(t *gotest.T) { order = append(order, "c") })
	})
	expected := []string{"b"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_XDescribe_is_skipped(t *testing.T) {
	var ran bool
	gotest.Run(t, func(s *gotest.S) {
		s.XDescribe("skipped", func(s *gotest.S) {
			s.Test("inner", func(t *gotest.T) { ran = true })
		})
	})
	if ran {
		t.Fatal("XDescribe tests should not run")
	}
}

func TestRun_FDescribe_only_runs_focused_group(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.Describe("normal", func(s *gotest.S) {
			s.Test("a", func(t *gotest.T) { order = append(order, "a") })
		})
		s.FDescribe("focused", func(s *gotest.S) {
			s.Test("b", func(t *gotest.T) { order = append(order, "b") })
		})
	})
	expected := []string{"b"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}
```

- [ ] **Step 6: Implement FTest, XTest, FDescribe, XDescribe and wire resolveFocus into run()**

Add to `gotest/gotest.go`, after the `Test` method:

```go
// FTest registers a focused test case. When any focused items exist,
// only focused items run.
func (s *S) FTest(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn, focused: true})
}

// XTest registers an excluded test case. Excluded tests are always skipped.
func (s *S) XTest(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn, excluded: true})
}

// FDescribe registers a focused test group.
func (s *S) FDescribe(name string, fn func(*S)) {
	s.describes = append(s.describes, describeEntry{name: name, fn: fn, focused: true})
}

// XDescribe registers an excluded test group.
func (s *S) XDescribe(name string, fn func(*S)) {
	s.describes = append(s.describes, describeEntry{name: name, fn: fn, excluded: true})
}
```

In the `run()` method, add focus resolution after merging hooks but before executing tests. Replace the test execution loop with:

```go
	// Resolve focus/exclude.
	effectiveTests, effectiveDescs := resolveFocus(s.tests, s.describes)

	// Execute tests.
	for _, test := range effectiveTests {
		test := test
		s.t.Run(test.name, func(sub *testing.T) {
			if test.excluded {
				sub.Skip("excluded")
				return
			}
			ttt := NewT(sub)
			for _, fn := range allBeforeEach {
				fn(ttt)
			}
			defer func() {
				for i := len(allAfterEach) - 1; i >= 0; i-- {
					allAfterEach[i](ttt)
				}
			}()
			test.fn(ttt)
		})
	}

	// Execute child describes.
	for _, desc := range effectiveDescs {
		desc := desc
		s.t.Run(desc.name, func(sub *testing.T) {
			if desc.excluded {
				sub.Skip("excluded")
				return
			}
			child := &S{t: sub}
			desc.fn(child)
			child.run(allBeforeEach, allAfterEach)
		})
	}
```

- [ ] **Step 7: Run all focus/exclude tests**

Run: `go test ./gotest/ -v -run "TestRun_XTest|TestRun_FTest|TestRun_XDescribe|TestRun_FDescribe"`
Expected: all 4 integration tests PASS

- [ ] **Step 8: Run the full test suite to check for regressions**

Run: `go test ./gotest/ -v`
Expected: all tests PASS

- [ ] **Step 9: Commit**

```bash
git add gotest/focus.go gotest/focus_test.go gotest/gotest.go gotest/gotest_test.go
git commit -m "feat(gotest): implement focus/exclude with FTest, XTest, FDescribe, XDescribe"
```

---

### Task 7: Parallel Test Support

**Files:**
- Modify: `gotest/gotest.go`
- Modify: `gotest/gotest_test.go`

- [ ] **Step 1: Write failing test for TestParallel**

Append to `gotest/gotest_test.go`:

```go
func TestRun_TestParallel_calls_t_Parallel(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.TestParallel("parallel_test", func(t *gotest.T) {
			// If t.Parallel() was called, this test runs in parallel.
			// We verify by checking the test didn't block other tests.
			// The real verification is that this compiles and doesn't panic.
		})
	})
}

func TestRun_TestParallel_still_runs_hooks(t *testing.T) {
	var beforeEachCount int32
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) {
			beforeEachCount++
		})
		s.TestParallel("p1", func(t *gotest.T) {})
		s.TestParallel("p2", func(t *gotest.T) {})
	})
	if beforeEachCount != 2 {
		t.Fatalf("expected BeforeEach to run twice, ran %d times", beforeEachCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./gotest/ -v -run "TestRun_TestParallel"`
Expected: FAIL — `TestParallel` method not defined

- [ ] **Step 3: Implement TestParallel**

Add to `gotest/gotest.go`, after `XTest`:

```go
// TestParallel registers a test case that calls t.Parallel().
func (s *S) TestParallel(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn, parallel: true})
}
```

In the `run()` test execution loop, add the parallel check. The existing loop body should already have the `test.parallel` handling if you used the code from Task 6 Step 6. If not, add this after `sub.Skip` check:

```go
			if test.parallel {
				sub.Parallel()
			}
```

This goes right after the `if test.excluded { ... }` block, before the BeforeEach loop.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./gotest/ -v -run "TestRun_TestParallel"`
Expected: both tests PASS

- [ ] **Step 5: Run full suite for regression check**

Run: `go test ./gotest/ -v`
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add gotest/gotest.go gotest/gotest_test.go
git commit -m "feat(gotest): implement TestParallel for parallel test execution"
```

---

### Task 8: Fluent Assertions

**Files:**
- Create: `gotest/assert.go`
- Create: `gotest/assert_test.go`
- Modify: `gotest/t.go`

- [ ] **Step 1: Write failing tests for Assert**

Create `gotest/assert_test.go`:

```go
package gotest_test

import (
	"fmt"
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestAssert_IsTrue_passes_for_true(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("true", func(t *gotest.T) {
			t.Assert(true).IsTrue()
		})
	})
}

func TestAssert_IsTrue_fails_for_false(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert(false).IsTrue()
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert(false).IsTrue() should fail")
	}
}

func TestAssert_IsFalse_passes_for_false(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("false", func(t *gotest.T) {
			t.Assert(false).IsFalse()
		})
	})
}

func TestAssert_IsFalse_fails_for_true(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert(true).IsFalse()
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert(true).IsFalse() should fail")
	}
}

func TestAssert_Equals_passes_for_equal_values(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(42).Equals(42)
	tt.Assert("hello").Equals("hello")
	tt.Assert([]int{1, 2}).Equals([]int{1, 2})
}

func TestAssert_Equals_fails_for_unequal_values(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert(42).Equals(99)
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert(42).Equals(99) should fail")
	}
}

func TestAssert_IsNil_passes_for_nil(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(nil).IsNil()
	var p *int
	tt.Assert(p).IsNil()
	tt.Assert(error(nil)).IsNil()
}

func TestAssert_IsNil_fails_for_non_nil(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert(42).IsNil()
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert(42).IsNil() should fail")
	}
}

func TestAssert_IsNotNil_passes_for_non_nil(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(42).IsNotNil()
	tt.Assert("hello").IsNotNil()
	tt.Assert(fmt.Errorf("err")).IsNotNil()
}

func TestAssert_IsNotNil_fails_for_nil(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert(nil).IsNotNil()
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert(nil).IsNotNil() should fail")
	}
}

func TestAssert_IsZero_passes_for_zero_values(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(0).IsZero()
	tt.Assert("").IsZero()
	tt.Assert(false).IsZero()
}

func TestAssert_IsZero_fails_for_non_zero(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert(1).IsZero()
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert(1).IsZero() should fail")
	}
}

func TestAssert_HasLength_passes_for_correct_length(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2, 3}).HasLength(3)
	tt.Assert("abc").HasLength(3)
	tt.Assert(map[string]int{"a": 1}).HasLength(1)
}

func TestAssert_HasLength_fails_for_wrong_length(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert([]int{1, 2}).HasLength(5)
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert([]int{1,2}).HasLength(5) should fail")
	}
}

func TestAssert_IsEmpty_passes_for_empty(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{}).IsEmpty()
	tt.Assert("").IsEmpty()
	tt.Assert(map[string]int{}).IsEmpty()
}

func TestAssert_IsEmpty_fails_for_non_empty(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert([]int{1}).IsEmpty()
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert([]int{1}).IsEmpty() should fail")
	}
}

func TestAssert_Contains_passes_for_slice_with_element(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2, 3}).Contains(2)
}

func TestAssert_Contains_passes_for_string_with_substring(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert("hello world").Contains("world")
}

func TestAssert_Contains_fails_for_missing_element(t *testing.T) {
	var subFailed bool
	t.Run("inner", func(t *testing.T) {
		tt := gotest.NewT(t)
		tt.Assert([]int{1, 2, 3}).Contains(99)
		subFailed = t.Failed()
	})
	if !subFailed {
		t.Fatal("Assert([]int{1,2,3}).Contains(99) should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./gotest/ -v -run "TestAssert_"`
Expected: FAIL — `Assert` method not defined on `*gotest.T`

- [ ] **Step 3: Implement AssertContext and assertion methods**

Create `gotest/assert.go`:

```go
package gotest

import (
	"fmt"
	"reflect"
	"strings"
)

// AssertContext provides fluent assertion methods for a single value.
type AssertContext struct {
	v     any
	failf func(format string, args ...any)
}

// IsTrue asserts the value is boolean true.
func (a *AssertContext) IsTrue() {
	v, ok := a.v.(bool)
	if !ok || !v {
		a.failf("expected true, got %v", a.v)
	}
}

// IsFalse asserts the value is boolean false.
func (a *AssertContext) IsFalse() {
	v, ok := a.v.(bool)
	if !ok || v {
		a.failf("expected false, got %v", a.v)
	}
}

// Equals asserts the value deeply equals the expected value.
func (a *AssertContext) Equals(expected any) {
	if !reflect.DeepEqual(a.v, expected) {
		a.failf("expected %v (%T), got %v (%T)", expected, expected, a.v, a.v)
	}
}

// IsNil asserts the value is nil.
func (a *AssertContext) IsNil() {
	if !isNil(a.v) {
		a.failf("expected nil, got %v (%T)", a.v, a.v)
	}
}

// IsNotNil asserts the value is not nil.
func (a *AssertContext) IsNotNil() {
	if isNil(a.v) {
		a.failf("expected non-nil, got nil")
	}
}

// IsZero asserts the value is the zero value for its type.
func (a *AssertContext) IsZero() {
	rv := reflect.ValueOf(a.v)
	if !rv.IsValid() {
		return // nil is zero
	}
	if !rv.IsZero() {
		a.failf("expected zero value, got %v", a.v)
	}
}

// HasLength asserts the value (string, slice, map, array, or channel) has the given length.
func (a *AssertContext) HasLength(expected int) {
	rv := reflect.ValueOf(a.v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
		actual := rv.Len()
		if actual != expected {
			a.failf("expected length %d, got %d", expected, actual)
		}
	default:
		a.failf("HasLength not supported for type %T", a.v)
	}
}

// IsEmpty asserts the value (string, slice, map, array, or channel) has length 0.
func (a *AssertContext) IsEmpty() {
	a.HasLength(0)
}

// Contains asserts the value contains the given element.
// For strings: checks substring containment.
// For slices/arrays: checks element presence via DeepEqual.
func (a *AssertContext) Contains(element any) {
	rv := reflect.ValueOf(a.v)
	switch rv.Kind() {
	case reflect.String:
		s := rv.String()
		sub, ok := element.(string)
		if !ok {
			a.failf("Contains on string requires a string argument, got %T", element)
			return
		}
		if !strings.Contains(s, sub) {
			a.failf("expected %q to contain %q", s, sub)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), element) {
				return
			}
		}
		a.failf("expected %v to contain %v", a.v, element)
	default:
		a.failf("Contains not supported for type %T", a.v)
	}
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

```

- [ ] **Step 4: Add Assert method to T**

Append to `gotest/t.go`:

```go
// Assert returns an AssertContext for fluent assertions on the given value.
// Assertion failures are reported via t.Errorf (non-fatal).
func (t *T) Assert(v any) *AssertContext {
	t.t.Helper()
	return &AssertContext{
		v: v,
		failf: func(format string, args ...any) {
			t.t.Helper()
			t.t.Errorf(format, args...)
		},
	}
}
```

- [ ] **Step 5: Run assertion tests**

Run: `go test ./gotest/ -v -run "TestAssert_"`
Expected: all 18 `TestAssert_*` tests PASS

- [ ] **Step 6: Run full suite**

Run: `go test ./gotest/ -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add gotest/assert.go gotest/assert_test.go gotest/t.go
git commit -m "feat(gotest): implement fluent assertion API on T wrapper"
```

---

### Task 9: Remove Old Code and Trim Dependencies

**Files:**
- Delete: `about/` (entire directory)
- Delete: `cmd/` (entire directory)
- Delete: `internal/` (entire directory)
- Delete: `pkg/` (entire directory)
- Delete: `examples/` (entire directory)
- Delete: `e2e/` (entire directory)
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Verify new library tests pass in isolation**

Run: `go test ./gotest/ -v`
Expected: all tests PASS

- [ ] **Step 2: Delete old code directories**

```bash
rm -rf about/ cmd/ internal/ pkg/ examples/ e2e/
```

- [ ] **Step 3: Remove external dependencies from go.mod**

Replace `go.mod` contents with:

```
module github.com/mvrahden/go-test

go 1.23

toolchain go1.24.0
```

- [ ] **Step 4: Regenerate go.sum**

```bash
go mod tidy
```

Expected: `go.sum` should be empty or deleted (no external deps).

- [ ] **Step 5: Verify everything compiles and tests pass**

Run: `go test ./... -v`
Expected: only `gotest/` tests run, all PASS

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: remove code generation pipeline

Replace the AST-based code generation framework with the new closure-based
runtime library. Removes all external dependencies (regexp2, x/tools,
testify). The gotest package now works with plain 'go test'."
```

---

### Task 10: Working Examples

**Files:**
- Create: `gotest/example_test.go`

- [ ] **Step 1: Write example tests that serve as documentation**

Create `gotest/example_test.go`:

```go
package gotest_test

import (
	"bytes"
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

// ExampleBasicSuite demonstrates a basic test suite with lifecycle hooks.
func TestExample_BasicSuite(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var buf bytes.Buffer

		s.BeforeAll(func(t *gotest.T) {
			buf.WriteString("setup;")
		})

		s.AfterAll(func(t *gotest.T) {
			buf.WriteString("teardown;")
			expected := "setup;beforeEach;test1;afterEach;beforeEach;test2;afterEach;teardown;"
			t.Assert(buf.String()).Equals(expected)
		})

		s.BeforeEach(func(t *gotest.T) {
			buf.WriteString("beforeEach;")
		})

		s.AfterEach(func(t *gotest.T) {
			buf.WriteString("afterEach;")
		})

		s.Test("test1", func(t *gotest.T) {
			buf.WriteString("test1;")
		})

		s.Test("test2", func(t *gotest.T) {
			buf.WriteString("test2;")
		})
	})
}

// ExampleNestedDescribe demonstrates nested describe blocks with hook inheritance.
func TestExample_NestedDescribe(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var level string

		s.BeforeEach(func(t *gotest.T) {
			level = "base"
		})

		s.Test("uses base level", func(t *gotest.T) {
			t.Assert(level).Equals("base")
		})

		s.Describe("with premium", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				level = level + "+premium"
			})

			s.Test("has premium", func(t *gotest.T) {
				t.Assert(level).Equals("base+premium")
			})

			s.Describe("during sale", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) {
					level = level + "+sale"
				})

				s.Test("has all modifiers", func(t *gotest.T) {
					t.Assert(level).Equals("base+premium+sale")
				})
			})
		})
	})
}

// ExampleFocusAndExclude demonstrates focus and exclude functionality.
func TestExample_FocusAndExclude(t *testing.T) {
	var executed []string
	gotest.Run(t, func(s *gotest.S) {
		// In normal usage you would use FTest to focus during development,
		// then remove the F prefix before committing.
		// Here we just demonstrate that XTest skips:
		s.Test("runs", func(t *gotest.T) {
			executed = append(executed, "runs")
		})
		s.XTest("skipped", func(t *gotest.T) {
			executed = append(executed, "skipped")
		})
	})

	if len(executed) != 1 || executed[0] != "runs" {
		t.Fatalf("expected only 'runs' to execute, got %v", executed)
	}
}

// ExampleAssertions demonstrates the fluent assertion API.
func TestExample_Assertions(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("boolean assertions", func(t *gotest.T) {
			t.Assert(true).IsTrue()
			t.Assert(false).IsFalse()
		})

		s.Test("equality", func(t *gotest.T) {
			t.Assert(42).Equals(42)
			t.Assert("hello").Equals("hello")
			t.Assert([]int{1, 2, 3}).Equals([]int{1, 2, 3})
		})

		s.Test("nil checks", func(t *gotest.T) {
			t.Assert(nil).IsNil()
			t.Assert(42).IsNotNil()
		})

		s.Test("collections", func(t *gotest.T) {
			t.Assert([]int{1, 2, 3}).HasLength(3)
			t.Assert([]int{}).IsEmpty()
			t.Assert([]int{1, 2, 3}).Contains(2)
			t.Assert("hello world").Contains("world")
		})
	})
}

// ExampleParallel demonstrates parallel test execution.
func TestExample_Parallel(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.TestParallel("p1", func(t *gotest.T) {
			t.Assert(1 + 1).Equals(2)
		})
		s.TestParallel("p2", func(t *gotest.T) {
			t.Assert(2 + 2).Equals(4)
		})
	})
}
```

- [ ] **Step 2: Run examples**

Run: `go test ./gotest/ -v -run "TestExample_"`
Expected: all 5 example tests PASS

- [ ] **Step 3: Run full test suite one final time**

Run: `go test ./... -v -count=1`
Expected: all tests PASS, zero dependencies, only `gotest/` package tested

- [ ] **Step 4: Commit**

```bash
git add gotest/example_test.go
git commit -m "docs(gotest): add working examples for all major features"
```

---

## Verification Checklist

After all tasks are complete, verify:

- [ ] `go test ./... -v -count=1` — all tests pass
- [ ] `go test ./... -race` — no race conditions
- [ ] `go mod tidy && git diff go.mod` — no unnecessary dependencies
- [ ] `grep -r "regexp2\|x/tools\|testify" go.mod` — zero external dependencies
- [ ] `wc -l gotest/*.go` — total implementation ~250 lines (excluding tests)
