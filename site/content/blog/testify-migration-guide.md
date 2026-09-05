---
title: "Migrating from `testify/suite` to gotest"
date: 2026-07-10
description: "A practical guide to migrating Go test suites from testify/suite to gotest: before-and-after examples, automated migration, and what needs manual review."
tags: ["Migration"]
keywords: ["migrate from testify", "testify suite alternative", "testify to gotest", "gotest migrate", "go test suite migration"]
cta_text: "Run gotest migrate on your first testify suite today."
cta_command: "gotest migrate ./..."
howto:
  name: "Migrate a testify/suite codebase to gotest"
  steps:
    - name: "Start from a clean tree"
      text: "Commit or stash first — the tool rewrites files in place, so git diff is your preview and git checkout your undo."
    - name: "Migrate one package"
      text: "Run gotest migrate ./pkg/user on a package with a few well-understood suites and verify the tests still pass."
    - name: "Resolve manual-review TODOs"
      text: "Search for the // TODO(gotest-migrate) comments the tool leaves for direct suite assertion calls like s.Equal, unmapped assertions, and BeforeTest/AfterTest/SetupSubTest/TearDownSubTest hooks."
    - name: "Run testify and gotest side by side"
      text: "Leave unmigrated packages on testify; they keep running under go test while migrated suites run under gotest, without conflict."
    - name: "Track progress with the linter"
      text: "Run gotest lint to flag remaining testify imports, catching packages where migration is incomplete."
    - name: "Drop testify"
      text: "Once the last suite is migrated, remove the testify dependency from go.mod."
aliases: ["/blog/testify-migration-guide.html"]
---

`gotest migrate ./...` rewrites most of a testify/suite codebase automatically: struct renames, lifecycle hooks, assertion calls, imports, and the `suite.Run` boilerplate. The catch is a handful of patterns it can't convert safely and leaves for manual review. This guide shows what the tool converts, what it leaves for you, and how to migrate package by package without breaking the rest of the codebase.

`testify/suite` is the most widely used Go test suite framework, and for good reason. It gives you struct-based test grouping and lifecycle hooks on top of the standard library. Many teams have hundreds of suites built on it. This guide is for teams that have decided to try gotest alongside or in place of those suites.

> This is not a case for why you should migrate. If you're still evaluating, [Why Go's testing Package Needs a Suite Layer]({{< ref "/blog/why-gotest" >}}) and [Code Generation vs Reflection in Go Test Frameworks]({{< ref "/blog/code-generation-not-reflection" >}}) cover the design differences. This post assumes you've already decided to give it a try. If you're starting fresh rather than migrating, [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}) is the better entry point.

## What won't migrate automatically

`gotest migrate` handles the mechanical bulk of the transformation. Four patterns are flagged with `// TODO(gotest-migrate)` comments — or left to the compiler — instead of being silently rewritten:

- **Assertions called directly on the suite.** `s.Equal(a, b)`, `s.Contains(...)` — calls through the embedded `suite.Suite` — are left in place with a TODO naming the `gotest.*` replacement.
- **Custom suite helpers.** Methods that aren't lifecycle hooks or test methods keep their shape; the tool doesn't add a `t *gotest.T` parameter to them, so rewritten assertions inside them need you to thread `t` through.
- **`BeforeTest` / `AfterTest` / `SetupSubTest` / `TearDownSubTest`.** testify's per-test-name and subtest-level hooks have no direct gotest equivalent.
- **Testify assertions outside suite methods.** `assert.*`/`require.*` calls in standalone functions aren't converted.

Each of these gets a detailed treatment in the "What needs manual review" section below. The tool rewrites files in place, so run it on a clean working tree — `git diff` is your preview, `git checkout` your undo.

## testify to gotest: what maps to what

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

## Before and after: a full suite migration

Here's a complete testify/suite test file and what it looks like after migration — the tool's rewrite plus the handful of TODO fixes it flags (the direct `s.Equal`/`s.Contains`/`s.ErrorIs` calls below):

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
- Lifecycle hooks were renamed and now accept `t`: `SetupTest()` became `BeforeEach(t *gotest.T)`. [Go Test Lifecycle]({{< ref "/blog/go-test-lifecycle" >}}) covers the full hook ordering and its guarantees.
- Assertions moved from method calls on the suite to standalone function calls: `s.Require().NoError(err)` became `gotest.NoError(t, err)`.
- Test methods now accept `t`: `TestCreate()` became `TestCreate(t *gotest.T)`.

## What changes beyond the syntax

Once a suite is migrated, several things change beyond the syntax:

- **Generation-time safety.** A typo in a lifecycle hook name (`SetUpTest` instead of `SetupTest`) silently does nothing in testify. In gotest, a hook with the wrong signature is a generation-time error with file and line number, and `gotest lint` flags near-miss hook names like `BeforEach` before they silently become ordinary methods.
- **No framework in the stack trace.** When a test fails, the stack trace shows your code calling a gotest assertion function. There's no reflection layer, no `suite.Run` orchestration, no `reflect.Value.Call` in between.
- **BDD structure.** Test methods can use `t.When()` and `t.It()` to create labeled subtests. Combined with `gotest spec`, your test hierarchy renders as a behavioral specification. [More on BDD-style tests.]({{< ref "/blog/readable-tests-with-bdd" >}})
- **Process isolation.** Each suite runs as a separate OS process. A panicking test in one suite cannot crash another. Suite-level parallelism is safe by default.
- **Fixtures.** gotest's fixture system supports dependency DAGs, cross-package sharing via serialization, and automatic lifecycle management. [More on fixtures]({{< ref "/blog/test-fixtures-in-go" >}}), and [Advanced Go Test Fixtures]({{< ref "/blog/advanced-fixture-patterns" >}}) for the DAG and sharing patterns.
- **No runtime dependency.** The `gotest` package has zero transitive dependencies beyond the standard library. The code generator runs at build time. What executes at test time is standard `go test`.

## Running the migration tool

`gotest migrate` automates the mechanical parts of this transformation. Point it at your packages and it rewrites the source files in place:

```sh
# migrate all packages
$ gotest migrate ./...

# migrate a specific package
$ gotest migrate ./pkg/user
```

There is no dry-run mode — the tool writes rewritten files directly. Run it on a clean git tree and use `git diff` to review every change (and `git checkout` to back out).

The tool performs an AST-level transformation, not a text find-and-replace. It parses your Go source files, identifies testify/suite patterns, and rewrites them while preserving comments, formatting, and non-suite code in the same file.

### What the tool handles

The migration tool covers the common cases that make up the bulk of a typical migration:

1. **Renames the suite struct** to follow the `*TestSuite` convention.
1. **Renames lifecycle hooks:** `SetupSuite` to `BeforeAll`, `TearDownSuite` to `AfterAll`, `SetupTest` to `BeforeEach`, `TearDownTest` to `AfterEach`.
1. **Transforms assertion calls:** `s.Require().Equal(a, b)` and `s.Assert().Equal(a, b)` become `gotest.Equal(t, a, b)`, as do `require.Equal(s.T(), a, b)`-style calls inside suite methods. Direct `s.Equal(a, b)` calls are annotated with a TODO instead (see below).
1. **Removes the `suite.Suite` embed** from the struct.
1. **Removes the `suite.Run` boilerplate** function.
1. **Updates imports:** removes `testify/suite`, adds `gotest`.

### What needs manual review

The tool handles the 90% case. For the remaining edge cases, it leaves `// TODO(gotest-migrate)` comments so you can find and address them:

- **Assertions called directly on the suite.** `s.Equal(...)`, `s.Contains(...)` — testify's assert-flavored calls through the embedded `suite.Suite` — are left as written, each with a TODO naming the `gotest.*` replacement. Since the embedding is removed, they won't compile until you convert them.
- **Custom helper methods on the suite.** Methods that aren't lifecycle hooks or test methods don't get a `t *gotest.T` parameter added. Assertion calls inside them are still rewritten to `gotest.*(t, ...)`, so you need to thread a `t` parameter through yourself. The same applies to `s.T()` handoffs, which the tool rewrites to `t.T()` everywhere.
- **Testify assertions outside suite methods.** `assert.*` or `require.*` calls in standalone functions aren't converted — but the testify imports in a migrated file are removed, so the compiler will point you at every remaining call. Replace them with the equivalent `gotest.*` functions. (Inside suite methods, `assert.Equal(s.T(), ...)`-style calls are converted automatically.)
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

`Nil` and `NotNil` deserve a note: gotest's versions are type-guarded. They accept only nilable types — pointers, interfaces, slices, maps, channels, functions — and fail with a guard error on anything else, pointing you to `Zero`/`NotZero` for comparable value types. testify's `Nil` accepts any value and reports a plain assertion failure. If your code calls `s.Nil` on non-nilable values, switch those call sites to `gotest.Zero`.

One difference worth noting: testify distinguishes between `s.Assert()` (continues on failure) and `s.Require()` (stops on failure). Calling assertions directly on the suite (`s.Equal(...)`, `s.Contains(...)`) also continues on failure, because the suite embeds `*assert.Assertions`. Only `s.Require().Equal(...)` stops. All gotest assertions stop on failure, like `Require`. This is a deliberate choice: a test that continues after a failed precondition typically produces confusing follow-on errors. If you need soft assertions, you can use `t.Errorf` directly.

Another difference: gotest assertions are generic. `gotest.Equal[V any](t, expected, actual V)` catches type mismatches at compile time. In testify, `s.Equal(42, "42")` compiles and fails at runtime. In gotest, `gotest.Equal(t, 42, "42")` is a compile error.

## Migrating gradually, package by package

You don't have to migrate everything at once. gotest suites and `func Test*` functions coexist in the same package. A practical approach for larger codebases:

1. **Start with one package.** Pick a package with a few well-understood suites. Run `gotest migrate ./pkg/user` and verify the tests pass.
1. **Run both side by side.** Unmigrated packages keep using testify under `go test`. Migrated suites run under `gotest` — the two runners partition the work and ignore each other's tests, so a complete run is both commands: `go test ./...` for the stdlib/testify half, `gotest ./...` for the suites.
1. **Migrate package by package.** There's no deadline. Each package is independent. A half-migrated codebase works fine.
1. **Remove testify when ready.** Once the last suite is migrated, drop the `testify` dependency from `go.mod`.

The linter can help track progress. `gotest lint` flags every remaining testify import ("testify import ... — consider migrating to gotest"), which makes half-migrated packages easy to list. Wiring that check into your pipeline keeps half-migrated packages from lingering; [Go Tests in GitHub Actions]({{< ref "/blog/gotest-in-ci" >}}) covers the CI setup.

## Common questions

### Do I need to rewrite all my assertion helpers?

Only the ones that use testify's assertion API. If you have helper functions that accept `*testing.T` and use the standard library's `t.Fatal` or `t.Errorf`, those work unchanged. Functions that call `require.Equal(t, ...)` need their imports and calls updated to `gotest.Equal(t, ...)`.

### What about testify/mock?

`testify/mock` is a separate package from `testify/suite`. You can migrate your suites to gotest while continuing to use `testify/mock` (or `gomock`, `mockery`, `moq`, or any other mocking tool). Mocking is orthogonal to test organization.

### Can I use `*testing.T` instead of `*gotest.T`?

Yes. All lifecycle hooks and test methods accept either `*gotest.T` or `*testing.T`. Using `*gotest.T` gives you access to `t.When()` and `t.It()` for BDD structure, but it's not required. You can migrate to `*testing.T` first and add BDD structure later.

### What if I have hundreds of suites?

`gotest migrate ./...` processes all packages in one pass. Review the `// TODO(gotest-migrate)` comments it leaves, fix the edge cases, and run your tests. For large codebases, doing this package by package is safer, but the tool handles batch migration too.
