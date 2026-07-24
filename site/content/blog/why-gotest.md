---
title: "Why gotest Exists"
date: 2026-07-05
description: "Go's testing package is one of the best things about the language. But it leaves gaps that every growing project eventually hits. The motivation behind gotest and the five principles that shaped its design."
tag: "Philosophy"
readTime: 8
aliases: ["/blog/why-gotest.html"]
---

Go's `testing` package is one of the best things about the language. No framework to install. No annotations to learn. Write a function that starts with `Test`, accept a `*testing.T`, and run `go test`. It works, and it scales further than most people expect.

But it does leave gaps. Not because the design is wrong, but because `testing` is deliberately minimal. It gives you a test runner and an assertion-free `T` type. Everything else is left to the developer: organizing tests into groups, managing setup and teardown, sharing expensive infrastructure, expressing what a test *means* rather than just what it does.

gotest exists to fill those gaps without replacing what works. This post explains the problems it targets, the principles behind its design, and why those principles lead to code generation rather than a runtime framework.

## The gaps

Most Go projects hit the same set of problems as their test suite grows past a few dozen files:

- **No structural grouping.** The stdlib gives you `func TestX` and `t.Run` for subtests. But subtests are closures inside flat functions, not first-class groups. If you have 30 test functions for `UserService` and 20 for `OrderService` in the same package, they are an alphabetical list. There is no way to say "these tests belong together" at the structural level.
- **No lifecycle hooks.** There is no built-in "run this before each test" or "run this once before the suite." `TestMain` gives you one entry point per package, but it runs before all tests and has no per-test granularity. `t.Cleanup` runs after a test, but there is no corresponding setup hook.
- **No fixture management.** If three test functions need a Postgres container, each one either starts its own (slow) or they share a package-level variable (fragile). The stdlib has no concept of "this resource has a lifecycle and these tests depend on it."
- **No cross-package sharing.** `go test` runs each package as a separate OS process. There is no way for `pkg/user` and `pkg/order` to share a database container through Go code. Most teams fall back to Makefiles or CI scripts to start infrastructure before running tests.
- **No structure in output.** Test names like `TestCreateUser_WhenEmailInvalid_ReturnsError` encode meaning in underscores. But `go test -v` renders them as flat lines. There is no hierarchical view that shows what a test suite covers at a glance.

These are not theoretical concerns. They are the reasons most Go projects beyond a certain size end up either adopting a framework like testify/suite or building an ad-hoc system of helper functions, global variables, and `TestMain` orchestration.

## What existing frameworks do

testify/suite is the most widely used answer to these problems. It gives you struct-based test suites with `SetupTest`/`TearDownTest` lifecycle hooks, and it works. But it makes trade-offs that gotest is designed to avoid:

- **Runtime reflection.** testify/suite discovers test methods with `reflect` at runtime. A typo in a lifecycle method name (`SetUpTest` instead of `SetupTest`) silently does nothing. The wrong method signature silently does nothing. You find out when your test passes for the wrong reason, not when you compile.
- **Registration boilerplate.** Every suite needs a `func TestX(t *testing.T) { suite.Run(t, new(MySuite)) }` wrapper. It is one line, but it is one line per suite that exists only to satisfy the framework.
- **Framework coupling.** Every suite must embed `suite.Suite`. Assertions go through `s.Equal`, `s.NoError`, which are methods on the embedded struct, not standalone functions. Removing the framework means rewriting every assertion in every test.
- **No fixture graph.** testify/suite has no concept of fixture dependencies. If suite A and suite B both need a database, and the database needs a container, the developer manages that wiring manually.

testify/suite is a good tool. gotest is not a reaction against it. But looking at the trade-offs above, they are not independent problems. They share a root cause.

## A different starting point

testify/suite discovers tests at runtime through reflection. Once you choose reflection, the rest follows: you need a base type to reflect on (framework coupling), the developer must connect each suite to the test runner (registration boilerplate), and method signatures cannot be validated until the code actually runs (silent failures).

gotest asks a different question: what if discovery happens before the tests run? The suite struct is already in the source code. The method names are already there. A tool that reads Go source files can find everything reflection finds, without running any code, and produce the bridge functions that connect suites to `go test`. If that tool runs at build time, everything it produces is ordinary Go.

That premise shaped five design principles. They are not aspirations; they are constraints that every feature in gotest must satisfy.

## Principle 1: Standard Go output, always

Every generated test is a `func Test*(t *testing.T)`. Every line of output is standard `go test` output. Every CI system, IDE, coverage tool, and profiler works unchanged.

This is the highest-priority principle, and the others are subordinate to it. gotest does not replace `go test`. It generates the bridge code that connects your suite structs to `go test`'s entry point convention. What actually runs is `go test`, with standard `func Test*(t *testing.T)` functions that the generator produced.

This means there is no lock-in at the output level. CI pipelines that parse `go test -json` output do not need to know gotest exists. Coverage tools see standard Go test functions. Race detector, profiler, debugger: all unchanged. The generated code is the same code a careful developer would write by hand; gotest automates the wiring, not the execution.

## Principle 2: The naming IS the API

gotest has no configuration files, no struct tags, no annotations, and no registration calls. The entire API is naming conventions:

- A struct whose name ends in `TestSuite` is a test suite.
- A method whose name starts with `Test` is a test case.
- A method named `BeforeEach` runs before every test.
- A struct whose name ends in `Fixture` is a package fixture.
- A struct whose name ends in `SharedFixture` is a cross-package shared fixture.
- A method prefixed with `F_` is focused (only focused tests run). A method prefixed with `X_` is excluded.

The goal is that a developer reads the naming conventions once and never opens documentation again. If you can read Go, you can read a gotest suite. There is nothing to decode, no interface to look up, no config to cross-reference.

In practice, a suite looks like this:

```go {title="user_test.go"}
type UserServiceTestSuite struct {
    db   *sql.DB
    svc  *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.db = setupTestDB(t.T())
    s.svc = NewUserService(s.db)
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.When("email is valid", func(w *gotest.T) {
        w.It("creates the user", func(it *gotest.T) {
            err := s.svc.Create("alice@example.com")
            gotest.NoError(it, err)
        })
    })
}
```

There is no registration call, no embedded base type, no interface to implement. The struct name ends in `TestSuite`, so the generator recognizes it. The method name starts with `Test`, so it becomes a test case. `BeforeEach` runs before every test method. Everything else is plain Go.

The code generator reads these source files with `go/parser`, walks the AST looking for the naming patterns, and produces the lifecycle wiring. Discovery is purely static: no code runs during the generation step.

## Principle 3: Zero runtime cost

At test execution time, there is no framework orchestration code in your call stack. The only runtime component is the thin `gotest.T` wrapper that provides `When`/`It` methods and standalone assertion functions like `gotest.Equal`. Everything else is generated: struct initialization, `t.Run` calls, `t.Cleanup` calls, direct method invocations. No reflection, no interface dispatch, no type assertions.

This has practical consequences. Stack traces point to your code, not to framework internals. Refactoring tools can follow the call chain because every call is a direct call. The `gotest` package that your tests import is a thin layer of helper types and functions; it has no transitive dependencies beyond the standard library.

The code generator runs before `go test` and produces Go source files. Those files are injected via Go's `-overlay` flag, a built-in mechanism that maps virtual file paths to real files on disk. The compiler sees the generated files; your source directory does not. After the run, nothing is left behind. Your `git status` stays clean.

This extends to isolation. Each suite runs in its own OS process: a generated test binary, compiled and executed separately. A panicking test in one suite cannot crash another. Memory is isolated by the OS, not by convention. Goroutine leaks, global state mutations, and port conflicts stay contained to the process that caused them.

This structural isolation is what makes suite-level parallelism safe by default: suites run concurrently without any opt-in, because they cannot interfere with each other.

## Principle 4: Invisible until needed

A developer who has never heard of gotest can read a test suite struct and understand what it does. The struct has fields, methods with descriptive names, and assertions that are standalone function calls. There is nothing to decode.

A developer who runs `go test` directly (without the CLI) gets a compilation error for the missing generated file, not silent wrong behavior. The error is clear and points to the solution.

And a developer who decides to stop using gotest can look at the generated code to see exactly what to write by hand. The generated file is a complete, readable Go test file. It is not a binary artifact or a compressed intermediate representation. It is the code you would have written yourself if you had the patience.

## Principle 5: Adopt incrementally, eject freely

Existing `func Test*` tests coexist with suites in the same package. You do not need to convert everything at once. A single suite can live next to 50 flat test functions, and `gotest` handles both.

Ejecting is real work, but it is straightforward work. You replace `*gotest.T` parameters with `*testing.T`, swap gotest assertions for your preferred alternative, and write the `func Test*` entry points that the generator was producing for you. The generated code shows exactly what those entry points look like. There is no data to migrate, no configuration to unwind, no runtime state to reconstruct.

This is a deliberate contrast with frameworks that require embedding a base type or implementing an interface. Those create a structural dependency: your test code *is* framework code. With gotest, your test code is Go code that happens to follow naming conventions. The conventions are what the tool reads; they are not what the code depends on.

## Why code generation

The principles above point toward code generation almost by necessity. If you want no reflection, you need a static discovery step. If you want standard `go test` output, you need standard test functions. If you want naming conventions instead of interfaces, you need a tool that reads source code and produces source code.

Code generation also solves the error-reporting problem that plagues reflection-based frameworks. If a lifecycle method has the wrong signature, the code generator rejects it at generation time with a clear error message, file name, and line number. With reflection, the method is silently ignored at runtime. You discover the problem when your `BeforeEach` never runs and your tests pass for the wrong reason.

The overlay filesystem injection is what makes this practical. Without it, code generation would mean generated files in your source tree: files to `.gitignore`, files that clutter your editor, files that go stale if you forget to regenerate. The `-overlay` flag removes all of that. The generated code exists only during the test run, in a content-addressable cache that handles invalidation automatically.

## What follows from this

These principles are not abstract. They directly shaped every feature in gotest:

- **Test suites** are plain structs with `TestSuite` suffix, not types that embed a framework base. Lifecycle hooks are methods with conventional names, not interface implementations. [More on organizing tests.]({{< ref "/blog/organizing-go-tests" >}})
- **Parallel execution** is safe by default at the suite level: each suite runs as a separate OS process, so suites cannot interfere with each other. Method-level parallelism is opt-in via `SuiteConfig{Parallel: true}`, with a returning `BeforeEach` giving each test its own isolated state. [More in the reference.]({{< ref "/reference#lifecycle" >}})
- **Fixtures** are structs with `Fixture` suffix. Dependencies between fixtures are expressed as pointer fields, forming a DAG that the generator resolves automatically. Shared fixtures cross the process boundary through JSON serialization. [More on fixtures.]({{< ref "/blog/test-fixtures-in-go" >}})
- **BDD structure** uses `t.When()` and `t.It()` method calls that map directly to `t.Run`. The `gotest spec` command renders this structure as a behavioral specification, because the hierarchy is already there in the test code. [More on BDD-style tests.]({{< ref "/blog/readable-tests-with-bdd" >}})
- **AST-based discovery** uses `go/parser` to read source files without importing or compiling them. This keeps the generation step fast and means the generator never executes user code. [More on code generation.]({{< ref "/blog/code-generation-not-reflection" >}})

Each of these deserves a deeper look. The common thread is the same: gotest is not trying to replace Go's testing model. It is trying to generate the code that Go's testing model requires you to write by hand once your project outgrows flat functions and subtests.
