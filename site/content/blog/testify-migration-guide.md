---
title: "Migrating from `testify/suite` to gotest"
date: 2026-07-10
description: "A practical guide to migrating Go test suites from testify/suite to gotest: before-and-after examples, automated migration, and what needs manual review."
tags: ["Migration"]
aliases: ["/blog/testify-migration-guide.html"]
---

`testify/suite` is the most widely used Go test suite framework, and for good reason. It gives you struct-based test grouping and lifecycle hooks on top of the standard library. Many teams have hundreds of suites built on it.

This guide is for teams that have decided to try gotest alongside or in place of their testify suites. It covers what changes, what stays the same, and how to use `gotest migrate` to automate most of the work.

> This is not a case for why you should migrate. If you're still evaluating, [Why gotest Exists]({{< ref "/blog/why-gotest" >}}) and [Code Generation, Not Reflection]({{< ref "/blog/code-generation-not-reflection" >}}) cover the design differences. This post assumes you've already decided to give it a try.

## What maps to what

The structural concepts are the same in both frameworks. Suites are structs, tests are methods, lifecycle hooks run at predictable points. The names change, and the wiring changes, but the mental model carries over.

| testify/suite | gotest | Notes |
|---|---|---|
| `XxxSuite` | `XxxTestSuite` | Struct name must end in `TestSuite` |
| `suite.Suite` embed | *(removed)* | No base type to embed |
| `SetupSuite()` | `BeforeAll(t *gotest.T)` | Receives a `t` parameter |
| `TearDownSuite()` | `AfterAll(t *gotest.T)` | Registered via `t.Cleanup` |
| `SetupTest()` | `BeforeEach(t *gotest.T)` | Receives a `t` parameter |
| `TearDownTest()` | `AfterEach(t *gotest.T)` | Deferred, runs even on `t.Fatal` |
| `func (s *S) TestX()` | `func (s *S) TestX(t *gotest.T)` | Test methods receive `t` |
| `s.Require().Equal(a, b)` | `gotest.Equal(t, a, b)` | Standalone generic functions |
| `s.NoError(err)` | `gotest.NoError(t, err)` | Same for all assertions |
| `suite.Run(t, new(S))` | *(removed)* | Generated automatically |

The biggest conceptual shift is that test methods now receive a `t` parameter. In testify, the test's `*testing.T` is buried inside the embedded `suite.Suite` and accessed via `s.T()`. In gotest, it's an explicit parameter, which means assertions are standalone function calls rather than method calls on the suite.

## Before and after

Here's a complete testify/suite test file and what it looks like after migration:

```go {title="before: user_test.go (testify/suite)"}
package user

import (
    "testing"

    "github.com/stretchr/testify/suite"
)

type UserServiceSuite struct {
    suite.Suite
    db  *TestDB
    svc *UserService
}

func (s *UserServiceSuite) SetupTest() {
    s.db = NewTestDB(s.T())
    s.svc = NewUserService(s.db)
}

func (s *UserServiceSuite) TearDownTest() {
    s.db.Close()
}

func (s *UserServiceSuite) TestCreate() {
    err := s.svc.Create("alice@example.com")
    s.Require().NoError(err)

    user, err := s.svc.Get("alice@example.com")
    s.Require().NoError(err)
    s.Equal("alice@example.com", user.Email)
}

func (s *UserServiceSuite) TestCreateDuplicateEmail() {
    s.svc.Create("alice@example.com")
    err := s.svc.Create("alice@example.com")
    s.Require().Error(err)
    s.Contains(err.Error(), "duplicate")
}

func (s *UserServiceSuite) TestGetNotFound() {
    _, err := s.svc.Get("nobody@example.com")
    s.Require().Error(err)
    s.ErrorIs(err, ErrNotFound)
}

func TestUserServiceSuite(t *testing.T) {
    suite.Run(t, new(UserServiceSuite))
}
```

```go {title="after: user_test.go (gotest)"}
package user

import (
    "github.com/mvrahden/go-test/pkg/gotest"
)

type UserServiceTestSuite struct {
    db  *TestDB
    svc *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.db = NewTestDB(t.T())
    s.svc = NewUserService(s.db)
}

func (s *UserServiceTestSuite) AfterEach(t *gotest.T) {
    s.db.Close()
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    err := s.svc.Create("alice@example.com")
    gotest.NoError(t, err)

    user, err := s.svc.Get("alice@example.com")
    gotest.NoError(t, err)
    gotest.Equal(t, "alice@example.com", user.Email)
}

func (s *UserServiceTestSuite) TestCreateDuplicateEmail(t *gotest.T) {
    s.svc.Create("alice@example.com")
    err := s.svc.Create("alice@example.com")
    gotest.Error(t, err)
    gotest.Contains(t, err.Error(), "duplicate")
}

func (s *UserServiceTestSuite) TestGetNotFound(t *gotest.T) {
    _, err := s.svc.Get("nobody@example.com")
    gotest.ErrorIs(t, err, ErrNotFound)
}
```

Notice what disappeared:

- The `suite.Suite` embed is gone. The struct has only your fields.
- The `func TestUserServiceSuite(t *testing.T) { suite.Run(...) }` boilerplate is gone. The code generator produces this.
- The `testify/suite` import is gone. The only import is `gotest`.

And one behavioral change to be aware of: in the "before" code, `s.Equal(...)`, `s.Contains(...)`, and `s.ErrorIs(...)` are called directly on the suite, which uses assert semantics: the test **continues** after a failure. Only the `s.Require().NoError(...)` calls stop. In the "after" code, all gotest assertions stop on failure. This is usually what you want, but if your tests relied on continuing past a failed assertion, review those cases.

And what changed shape:

- The struct name gained `Test`: `UserServiceSuite` became `UserServiceTestSuite`.
- Lifecycle hooks were renamed and now accept `t`: `SetupTest()` became `BeforeEach(t *gotest.T)`.
- Assertions moved from method calls on the suite to standalone function calls: `s.Require().NoError(err)` became `gotest.NoError(t, err)`.
- Test methods now accept `t`: `TestCreate()` became `TestCreate(t *gotest.T)`.

## What changes beyond the syntax

Once a suite is migrated, several things change beyond the syntax:

- **Compile-time safety.** A typo in a lifecycle hook name (`SetUpTest` instead of `SetupTest`) silently does nothing in testify. In gotest, the code generator validates hook names and signatures at generation time. Wrong name? Clear error with file and line number.
- **No framework in the stack trace.** When a test fails, the stack trace shows your code calling a gotest assertion function. There's no reflection layer, no `suite.Run` orchestration, no `reflect.Value.Call` in between.
- **BDD structure.** Test methods can use `t.When()` and `t.It()` to create labeled subtests. Combined with `gotest spec`, your test hierarchy renders as a behavioral specification. [More on BDD-style tests.]({{< ref "/blog/readable-tests-with-bdd" >}})
- **Process isolation.** Each suite runs as a separate OS process. A panicking test in one suite cannot crash another. Suite-level parallelism is safe by default.
- **Fixtures.** gotest's fixture system supports dependency DAGs, cross-package sharing via serialization, and automatic lifecycle management. [More on fixtures.]({{< ref "/blog/test-fixtures-in-go" >}})
- **No runtime dependency.** The `gotest` package has zero transitive dependencies beyond the standard library. The code generator runs at build time. What executes at test time is standard `go test`.

## Running the migration tool

`gotest migrate` automates the mechanical parts of this transformation. Point it at your packages and it rewrites the source files in place:

```sh
# migrate all packages
$ gotest migrate ./...

# migrate a specific package
$ gotest migrate ./pkg/user

# preview changes without writing (dry run)
$ gotest migrate ./... --dry-run
```

The tool performs an AST-level transformation, not a text find-and-replace. It parses your Go source files, identifies testify/suite patterns, and rewrites them while preserving comments, formatting, and non-suite code in the same file.

### What the tool handles

The migration tool covers the common cases that make up the bulk of a typical migration:

1. **Renames the suite struct** to follow the `*TestSuite` convention.
1. **Renames lifecycle hooks:** `SetupSuite` to `BeforeAll`, `TearDownSuite` to `AfterAll`, `SetupTest` to `BeforeEach`, `TearDownTest` to `AfterEach`.
1. **Transforms assertion calls:** `s.Require().Equal(a, b)` and `s.Equal(a, b)` both become `gotest.Equal(t, a, b)`.
1. **Removes the `suite.Suite` embed** from the struct.
1. **Removes the `suite.Run` boilerplate** function.
1. **Updates imports:** removes `testify/suite`, adds `gotest`.

### What needs manual review

The tool handles the 90% case. For the remaining edge cases, it leaves `// TODO: manual review` comments so you can find and address them:

- **`s.T()` calls outside assertions.** If your suite passes `s.T()` to helper functions, you'll need to replace those with the `t` parameter that test methods now receive.
- **Custom helper methods on the suite.** Methods that aren't lifecycle hooks or test methods are left unchanged. If they call assertions via `s.Require()`, those calls need manual conversion.
- **Testify assertions not in `suite`.** If the same file uses `testify/assert` or `testify/require` directly (not through the suite), those imports and calls aren't touched. Replace them with the equivalent `gotest.*` functions.
- **`BeforeTest` / `AfterTest`.** testify has additional per-test hooks that receive the suite and test names as parameters: `BeforeTest(suiteName, testName string)` and `AfterTest(suiteName, testName string)`. These have no direct gotest equivalent and need manual conversion.
- **`SetupSubTest` / `TearDownSubTest`.** testify/suite has subtest-level hooks that gotest doesn't map to directly. These are flagged for manual review.

## Assertions: the biggest diff

The assertion changes will touch more lines than anything else. In testify, assertions are methods on the suite or on `s.Require()`. In gotest, they're standalone generic functions that take `t` as the first argument.

The good news: it's a mechanical transformation. The assertion names are almost identical, and the argument order is consistent. Here are the most common mappings:

| testify | gotest |
|---|---|
| `s.Equal(expected, actual)` | `gotest.Equal(t, expected, actual)` |
| `s.Require().NoError(err)` | `gotest.NoError(t, err)` |
| `s.Contains(str, sub)` | `gotest.Contains(t, str, sub)` |
| `s.Len(slice, n)` | `gotest.Len(t, slice, n)` |
| `s.True(cond)` | `gotest.True(t, cond)` |
| `s.ErrorIs(err, target)` | `gotest.ErrorIs(t, err, target)` |
| `s.Nil(ptr)` | `gotest.Nil(t, ptr)` |
| `s.NotNil(ptr)` | `gotest.NotNil(t, ptr)` |

`Nil` and `NotNil` deserve a note: testify's `Nil` checks specifically for `nil` using reflection, while gotest's `Zero` checks for the zero value of the type. They behave the same for pointers and interfaces (where `nil` is the zero value), but differ for other types. If your code uses `s.Nil` on non-pointer values, review those call sites manually.

One difference worth noting: testify distinguishes between `s.Assert()` (continues on failure) and `s.Require()` (stops on failure). Calling assertions directly on the suite (`s.Equal(...)`, `s.Contains(...)`) also continues on failure, because the suite embeds `*assert.Assertions`. Only `s.Require().Equal(...)` stops. All gotest assertions stop on failure, like `Require`. This is a deliberate choice: a test that continues after a failed precondition typically produces confusing follow-on errors. If you need soft assertions, you can use `t.Errorf` directly.

Another difference: gotest assertions are generic. `gotest.Equal[V any](t, expected, actual V)` catches type mismatches at compile time. In testify, `s.Equal(42, "42")` compiles and fails at runtime. In gotest, `gotest.Equal(t, 42, "42")` is a compile error.

## Migrating gradually

You don't have to migrate everything at once. gotest suites and `func Test*` functions coexist in the same package. A practical approach for larger codebases:

1. **Start with one package.** Pick a package with a few well-understood suites. Run `gotest migrate ./pkg/user` and verify the tests pass.
1. **Run both side by side.** Unmigrated packages keep using testify. Migrated packages use gotest. Both run via `go test` (or `gotest`) without conflict.
1. **Migrate package by package.** There's no deadline. Each package is independent. A half-migrated codebase works fine.
1. **Remove testify when ready.** Once the last suite is migrated, drop the `testify` dependency from `go.mod`.

The linter can help track progress. `gotest lint` flags `testify/suite` imports in packages that also use gotest suites, catching packages where migration is incomplete.

## Common questions

### Do I need to rewrite all my assertion helpers?

Only the ones that use testify's assertion API. If you have helper functions that accept `*testing.T` and use the standard library's `t.Fatal` or `t.Errorf`, those work unchanged. Functions that call `require.Equal(t, ...)>` need their imports and calls updated to `gotest.Equal(t, ...)`.

### What about testify/mock?

`testify/mock` is a separate package from `testify/suite`. You can migrate your suites to gotest while continuing to use `testify/mock` (or `gomock`, `mockery`, `moq`, or any other mocking tool). Mocking is orthogonal to test organization.

### Can I use `*testing.T` instead of `*gotest.T`?

Yes. All lifecycle hooks and test methods accept either `*gotest.T` or `*testing.T`. Using `*gotest.T` gives you access to `t.When()` and `t.It()` for BDD structure, but it's not required. You can migrate to `*testing.T` first and add BDD structure later.

### What if I have hundreds of suites?

`gotest migrate ./...` processes all packages in one pass. Review the `// TODO: manual review` comments it leaves, fix the edge cases, and run your tests. For large codebases, doing this package by package is safer, but the tool handles batch migration too.
