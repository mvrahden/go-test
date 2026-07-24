---
title: "Organizing Go Tests Beyond `func TestX`"
date: 2026-07-06
description: "Go's testing simplicity is a feature. Until your project outgrows it. A look at why flat test functions, table-driven tests, and subtests each solve part of the problem, and what happens when you need all three at once."
tag: "Patterns"
readTime: 10
aliases: ["/blog/organizing-go-tests.html"]
---

Go's testing package is deliberately simple. No annotations, no test classes, no dependency injection: just functions that start with `Test` and a `*testing.T`. This simplicity is one of Go's genuine strengths. But it creates a gap that every growing project eventually falls into.

This post walks through the progression most Go projects follow as their test suites grow: flat functions, table-driven tests, subtests, and suites. Each solves a real problem. Each leaves something on the table. Understanding that progression makes it easier to see where your own project sits, and what it might need next.

## Flat functions: where everyone starts

A new Go project's test file usually looks like this:

```go {title="user_service_test.go"}
func TestCreateUser(t *testing.T) {
    db := setupTestDB(t)
    err := db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
}

func TestCreateUserDuplicateEmail(t *testing.T) {
    db := setupTestDB(t)
    db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})
    err := db.CreateUser(User{Name: "Bob", Email: "alice@example.com"})
    if err == nil {
        t.Fatal("expected error for duplicate email")
    }
}

func TestGetUser(t *testing.T) {
    db := setupTestDB(t)
    db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})
    user, err := db.GetUser("alice@example.com")
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if user.Name != "Alice" {
        t.Errorf("expected Alice, got %s", user.Name)
    }
}

func TestGetUserNotFound(t *testing.T) {
    db := setupTestDB(t)
    _, err := db.GetUser("nobody@example.com")
    if err == nil {
        t.Fatal("expected error for missing user")
    }
}

func TestDeleteUser(t *testing.T) {
    db := setupTestDB(t)
    db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})
    err := db.DeleteUser("alice@example.com")
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
}
```

Five functions, and `setupTestDB(t)` appears in every one. There's no visible relationship between the "create" tests and the "get" tests; they're just an alphabetical list. Assertions are manual `if`/`t.Fatal` checks with inconsistent formatting.

This works fine at 10 test functions. At 50, you're scrolling to find what you need. At 200, you're `grep`-ing your own test files.

## Table-driven tests: less code, same structure

The first escape hatch most Go developers reach for is the table-driven pattern:

```go
func TestCreateUser(t *testing.T) {
    tests := []struct {
        name    string
        user    User
        wantErr bool
    }{
        {"valid user", User{Name: "Alice", Email: "alice@example.com"}, false},
        {"missing name", User{Email: "alice@example.com"}, true},
        {"invalid email", User{Name: "Alice", Email: "not-an-email"}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            db := setupTestDB(t)
            err := db.CreateUser(tt.user)
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

Better. Less repetition within a group of related cases. `t.Run` gives each case a name so failures point to the right row.

But table-driven tests solve repetition, not organization. `TestCreateUser` and `TestGetUser` and `TestDeleteUser` are still unrelated flat functions sitting in a file together. And the pattern starts to strain when cases need different assertion logic: the struct grows fields like `wantCode`, `wantBody`, `setupFn`, `checkFn`, and the table becomes harder to read than the tests it replaced.

## Subtests: hierarchy without lifecycle

Go 1.7 introduced `t.Run`, and with it came the ability to nest tests:

```go
func TestUserService(t *testing.T) {
    db := setupTestDB(t)
    t.Cleanup(func() { db.Close() })

    t.Run("Create", func(t *testing.T) {
        t.Run("valid user", func(t *testing.T) {
            err := db.CreateUser(User{Name: "Alice"})
            if err != nil {
                t.Fatal(err)
            }
        })
        t.Run("duplicate email", func(t *testing.T) {
            err := db.CreateUser(User{Name: "Alice"})
            // this fails: Create/valid_user already inserted Alice
            if err == nil {
                t.Fatal("expected error")
            }
        })
    })

    t.Run("Delete", func(t *testing.T) {
        err := db.DeleteUser("alice@example.com")
        // depends on what Create tests left behind
        if err != nil {
            t.Fatal(err)
        }
    })
}
```

Now there's visible structure: `TestUserService/Create/valid_user` reads like a path. But this pattern has real problems:

- **Shared state leaks across subtests.** The `db` variable is shared between "Create" and "Delete." If the "Create" subtests modify the database, "Delete" inherits that state. Tests become order-dependent.
- **No per-group lifecycle.** There's no way to reset `db` before each subtest. `t.Cleanup` runs once, at the end. You'd need to call `setupTestDB(t)` inside every leaf `t.Run`, which brings back the repetition you were trying to eliminate.
- **Closures stack up fast.** Three levels of nesting means three levels of `func(t *testing.T) {`. At scale, the indentation alone makes the file hard to navigate.
- **Parallelism is manual.** Want to run subtests in parallel? You need `t.Parallel()` in every leaf, and careful synchronization on the shared `db`.

Subtests give you hierarchy. They don't give you lifecycle.

## What a suite actually provides

The pattern most projects eventually need is a **test suite**: a group of tests organized around a single subject, with lifecycle hooks that guarantee each test starts from a known state.

A suite gives you four things that flat functions and subtests don't:

1. **Grouping by subject.** All tests for `UserService` live on one struct.
1. **Lifecycle hooks.** `BeforeEach` runs before every test, so each test gets fresh state.
1. **Isolation.** Test methods can't accidentally share state through closures.
1. **Discoverability.** The struct name tells you what's under test. Methods tell you what it does.

In Go, the most established suite framework is `testify/suite`:

```go {title="testify/suite approach"}
type UserServiceSuite struct {
    suite.Suite
    db *TestDB
}

func (s *UserServiceSuite) SetupTest() {
    s.db = setupTestDB(s.T())
}

func (s *UserServiceSuite) TearDownTest() {
    s.db.Close()
}

func (s *UserServiceSuite) TestCreateUser() {
    err := s.db.CreateUser(User{Name: "Alice"})
    s.NoError(err)
}

func (s *UserServiceSuite) TestCreateUserDuplicate() {
    s.db.CreateUser(User{Name: "Alice", Email: "a@b.com"})
    err := s.db.CreateUser(User{Name: "Bob", Email: "a@b.com"})
    s.Error(err)
}

func TestUserServiceSuite(t *testing.T) {
    suite.Run(t, new(UserServiceSuite))
}
```

This solves the organizational problem. `SetupTest` runs before each test method. State lives on the struct, not in closures. Each method is a self-contained test.

But it comes with trade-offs:

- **Reflection-based discovery.** `suite.Run` uses `reflect` to find and invoke test methods at runtime. That means test failures don't always map cleanly to stack traces, and refactoring tools can't always follow the call chain.
- **Runtime dependency.** Your test binary imports `github.com/stretchr/testify`, a third-party module with its own release cycle and transitive dependencies.
- **Framework coupling.** Every suite must embed `suite.Suite`. Every assertion goes through `s.NoError`, `s.Equal`, methods on the embedded struct, not type-safe generic functions.
- **No BDD structure.** Test methods are flat; there's no equivalent of "when X, it should Y" nesting within a method.

## A different angle: suites as a compile-time concern

Here's the observation that changes the equation: **the suite pattern is an organizational concern, not a runtime concern.**

What suites need at runtime is ordinary Go code: `t.Run` calls, `t.Cleanup` calls, struct initialization. Everything the stdlib already provides. The reason frameworks like testify/suite use reflection is to *discover* test methods and *wire up* lifecycle hooks. But discovery and wiring are compile-time activities. They can happen before `go test` runs.

That's the approach [gotest](https://github.com/mvrahden/go-test) takes. You write test suites as plain Go structs with naming conventions. A code generator reads your source, discovers the structure, and generates the lifecycle wiring that you'd write by hand. What runs is standard `go test`. No reflection. No framework orchestration at runtime.

```go {title="gotest approach"}
type UserServiceTestSuite struct {
    db *TestDB
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.db = setupTestDB(t)
}

func (s *UserServiceTestSuite) TestCreateUser(t *gotest.T) {
    t.When("the input is valid", func(w *gotest.T) {
        err := s.db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})

        w.It("succeeds without error", func(it *gotest.T) {
            gotest.NoError(it, err)
        })
    })

    t.When("the email already exists", func(w *gotest.T) {
        s.db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})
        err := s.db.CreateUser(User{Name: "Bob", Email: "alice@example.com"})

        w.It("returns an error", func(it *gotest.T) {
            gotest.Error(it, err)
        })
    })
}

func (s *UserServiceTestSuite) TestDeleteUser(t *gotest.T) {
    t.When("the user exists", func(w *gotest.T) {
        s.db.CreateUser(User{Name: "Alice", Email: "alice@example.com"})
        err := s.db.DeleteUser("alice@example.com")

        w.It("succeeds without error", func(it *gotest.T) {
            gotest.NoError(it, err)
        })
    })
}
```

A few things to notice:

- **No framework embedding.** The struct has only your fields. No `suite.Suite`, no interface to satisfy.
- **Lifecycle via naming conventions.** `BeforeEach` is recognized by name. It runs before every `Test*` method, giving each test a fresh `db`.
- **BDD structure inside methods.** `t.When` and `t.It` create labeled subtests via `t.Run`. They're just method calls: no DSL, no magic.
- **Type-safe assertions.** `gotest.NoError`, `gotest.Equal`, and others are generic functions. Pass the wrong type and the compiler catches it.

Running `gotest ./...` generates the `t.Run` nesting, lifecycle calls, and process isolation behind the scenes, then invokes `go test`. The generated code is injected via Go's `-overlay` flag; it never touches your source tree.

## Tests that explain themselves

There's a useful consequence of this structure. Because test methods, `When` blocks, and `It` blocks all have descriptive names, gotest can render them as a behavioral specification:

{{< terminal title="gotest spec ./..." >}}
<span class="t-prompt">$</span> <span class="t-cmd">gotest spec ./...</span>

<span class="t-suite">UserService</span>
  <span class="t-test">CreateUser</span>
    <span class="t-when">when the input is valid</span>
      <span class="t-pass">✓</span> succeeds without error <span class="t-time">(5ms)</span>
    <span class="t-when">when the email already exists</span>
      <span class="t-pass">✓</span> returns an error <span class="t-time">(<1ms)</span>
  <span class="t-test">DeleteUser</span>
    <span class="t-when">when the user exists</span>
      <span class="t-pass">✓</span> succeeds without error <span class="t-time">(3ms)</span>
<span class="t-summary">1 suite, 3 behaviors: <span class="t-pass">3 passed</span>, 0 failed, 0 skipped</span>
{{< /terminal >}}

This isn't generated documentation; it's a direct rendering of your test structure. If a test is missing, the spec has a gap. If a test fails, the spec shows it. The test *is* the specification.

This spec output is also machine-readable. `gotest spec --format json` produces structured JSON that you can feed into documentation pipelines, AI-assisted workflows, or CI reporting.

## Where this leaves you

Every Go project moves through a progression:

1. **Flat functions.** Simple, but no organization.
1. **Table-driven tests.** Reduce repetition within a group.
1. **Subtests.** Add hierarchy, but no lifecycle.
1. **Suites.** Add lifecycle, grouping, and isolation.

The question at step 4 is how you get suites. Runtime reflection (testify/suite) works and is battle-tested. Compile-time code generation (gotest) gives you the same organizational benefits with type safety, no reflection, and BDD structure, at the cost of a code generation step.

Neither is universally right. But if you've been writing Go tests long enough to feel the limits of flat functions and subtests, it's worth understanding the trade-offs. The test structure you choose is the one you'll read, debug, and maintain for the life of the project.
