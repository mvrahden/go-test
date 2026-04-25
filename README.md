# gotest

Go tests that write themselves, organize themselves, and explain themselves.

`gotest` closes the gap between `func TestX(t *testing.T)` and a well-organized test suite through code generation. You write structs, name them well, and the tool handles the rest. No runtime dependencies. No reflection. No lock-in. Just standard Go tests with lifecycle management and structured organization.

## Install

```bash
go install github.com/mvrahden/go-test/cmd/gotest@latest
```

## 30-Second Example

Write a test suite struct:

```go
// user_service_suite_test.go
package user

import "github.com/mvrahden/go-test/pkg/gotest"

type UserServiceTestSuite struct {
    svc *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.svc = NewUserService()
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.It("creates a user with valid input", func(it *gotest.T) {
        err := s.svc.Create("alice@example.com")
        gotest.NoError(it, err)
    })

    t.When("email already exists", func(w *gotest.T) {
        w.It("returns ErrDuplicate", func(it *gotest.T) {
            s.svc.Create("alice@example.com")
            err := s.svc.Create("alice@example.com")
            gotest.ErrorIs(it, err, ErrDuplicate)
        })
    })
}
```

Run:

```bash
gotest ./... -v
```

Output is standard `go test` output:

```
=== RUN   TestUserServiceTestSuite
=== RUN   TestUserServiceTestSuite/TestCreate
=== RUN   TestUserServiceTestSuite/TestCreate/creates_a_user_with_valid_input
=== RUN   TestUserServiceTestSuite/TestCreate/email_already_exists
=== RUN   TestUserServiceTestSuite/TestCreate/email_already_exists/returns_ErrDuplicate
--- PASS: TestUserServiceTestSuite (0.01s)
```

No generated code leaks into your workflow. `gotest` generates it before tests run and cleans it up after.

## Features

### Lifecycle Hooks

```go
func (s *MySuite) BeforeAll(t *gotest.T)  {} // once before all tests
func (s *MySuite) AfterAll(t *gotest.T)   {} // once after all tests
func (s *MySuite) BeforeEach(t *gotest.T) {} // before each test method
func (s *MySuite) AfterEach(t *gotest.T)  {} // after each test method
```

All hooks are optional. `AfterAll` runs via `t.Cleanup` (LIFO). `AfterEach` is deferred, so it runs even on `t.Fatal()`.

### Focus and Exclude

```go
type F_UserServiceTestSuite struct { ... }  // F_ prefix: only this suite runs
type X_BrokenTestSuite struct { ... }       // X_ prefix: this suite is skipped

func (s *MySuite) F_TestCreate(t *gotest.T) {} // focus a single test
func (s *MySuite) X_TestFlaky(t *gotest.T)  {} // exclude a single test
```

Use `--ci` in CI to fail the build if any `F_` prefix slipped through:

```bash
gotest --ci ./... -v -race
```

### BDD Vocabulary

```go
func (s *Suite) TestCreate(t *gotest.T) {
    t.When("input is valid", func(w *gotest.T) {
        w.It("creates the record", func(it *gotest.T) {
            // ...
        })
    })
}
```

`When` groups context. `It` specifies behavior. Both map to `t.Run` under the hood.

### Parallel Tests

```go
type UserServiceTestSuiteParallel struct { ... } // suite-level parallel

func (s *Suite) TestParallelCreate(t *gotest.T) {} // TestParallel prefix: test-level parallel
```

### Type-Safe Assertions

Functional API with compile-time type safety:

```go
gotest.Equal(t, expected, actual)            // [T any] — cross-type comparison is a compile error
gotest.NoError(t, err)
gotest.ErrorIs(t, err, target)
gotest.Contains(t, haystack, needle)
gotest.Greater(t, a, b)                      // [T cmp.Ordered]
gotest.Len(t, collection, 3)
gotest.True(t, condition)
gotest.Eventually(t, func() bool { ... }, 5*time.Second, 100*time.Millisecond)
```

Fluent API for quick exploration:

```go
t.Assert(result).Equal(expected)
t.Assert(items).HasLength(3)
t.Assert(err).NoError()
t.Assert(ok).IsTrue()
```

Works with both `*gotest.T` (suites) and `*testing.T` (standalone tests).

### Scaffold

Generate a test suite skeleton from any Go type:

```bash
gotest scaffold ./pkg/user.UserService
# Generated: pkg/user/user_service_suite_test.go
```

### Migrate from testify/suite

```bash
gotest migrate ./...
# Migrated 12 suites across 8 packages:
#   pkg/user/user_test.go: UserSuite → UserTestSuite
```

Renames lifecycle methods, rewrites assertions, removes testify imports.

## How It Works

```
you write:          gotest generates:         go test runs:
                    (hidden, auto-cleaned)

MySuite struct      ƒƒ_psuite_test.go        func TestMySuite(t *testing.T)
  BeforeAll()   →     BeforeAll wrapper    →    t.Cleanup(AfterAll)
  TestFoo()           TestFoo wrapper            BeforeAll()
  AfterAll()          t.Run("TestFoo",...)       t.Run("TestFoo", ...)
                                                 ...
```

The generated code is what a careful developer would write by hand: `t.Run`, `t.Cleanup`, `defer`, `sync.WaitGroup`. No reflection, no interface dispatch.

## Naming Conventions

| Convention | Meaning |
|---|---|
| `*TestSuite` suffix | Test suite struct |
| `*TestSuiteParallel` suffix | Parallel test suite |
| `BeforeAll` / `AfterAll` | Suite-level lifecycle |
| `BeforeEach` / `AfterEach` | Test-level lifecycle |
| `Test*` method | Test case |
| `TestParallel*` method | Parallel test case |
| `F_` prefix | Focus (run only this) |
| `X_` prefix | Exclude (skip this) |

## Commands

```bash
gotest ./... -v -race          # generate, test, cleanup (default)
gotest clean ./...             # remove orphaned generated files
gotest scaffold ./pkg/user.Svc # generate suite skeleton from type
gotest migrate ./...           # convert testify/suite to go-test
gotest version                 # print version
gotest help                    # show help
```

All `go test` flags work unchanged: `-race`, `-cover`, `-count`, `-run`, `-json`, `-short`, `-timeout`, `-v`.

## License

MIT
