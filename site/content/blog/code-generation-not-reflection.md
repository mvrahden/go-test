---
title: "Code Generation, Not Reflection: How gotest Discovers Your Tests"
date: 2026-07-09
description: "Most Go test frameworks use reflection to find suites at runtime. gotest takes a different approach: AST-based code generation with overlay filesystem injection."
tag: "Internals"
readTime: 11
aliases: ["/blog/code-generation-not-reflection.html"]
---

Every Go test framework that supports suites needs to solve the same problem: how do you connect a struct full of test methods to the `go test` runner? The runner only knows about top-level `func Test*(t *testing.T)` functions. Your suite is a struct with methods. Something has to bridge the gap.

Most frameworks use reflection. gotest uses code generation. This post explains why, and what the code generation pipeline actually does.

## The reflection approach

In a reflection-based framework like testify/suite, the bridge looks like this:

```go
func TestUserSuite(t *testing.T) {
    suite.Run(t, new(UserSuite))
}
```

You write a `func Test*` that the runner can find, and inside it you call `suite.Run`, which uses the `reflect` package to:

1. Enumerate all methods on `UserSuite`
1. Filter for methods starting with `Test`
1. Call each one as a subtest via `t.Run`
1. Look for `SetupTest`, `TearDownTest`, etc. and call them at the right time

This works. But it has costs:

- **Boilerplate.** Every suite needs a `func Test*(t *testing.T) { suite.Run(t, new(MySuite)) }` wrapper. It's the same line every time, but you have to write it.
- **Runtime discovery.** Method lookup happens when the test runs, not when it compiles. A typo in a lifecycle method name (`SetUpTest` instead of `SetupTest`) silently does nothing.
- **Opaque wiring.** The connection between your struct and the test runner is hidden inside `suite.Run`. You can't inspect the generated subtests, set breakpoints on the lifecycle logic, or see what gets called in what order without reading the framework source.

## The code generation approach

gotest takes a different path. Instead of discovering tests at runtime with reflection, it discovers them at build time by reading your source code's AST (abstract syntax tree). Then it generates the bridge code that a developer would otherwise write by hand.

The pipeline has three stages:

```diagram
AST discovery → code generation → overlay injection
```

### Stage 1: AST discovery

gotest uses Go's `go/parser` and `go/ast` packages to walk your source files. It looks for naming conventions:

- **Structs ending in `TestSuite`**: recognized as test suites
- **Methods starting with `Test`** on suite types: recognized as test cases
- **`BeforeEach`, `AfterEach`, `BeforeAll`, `AfterAll`**: recognized as lifecycle hooks
- **Structs ending in `Fixture`**: recognized as package fixtures
- **Structs ending in `SharedFixture`**: recognized as cross-package fixtures

This is purely static analysis. No code runs. No `reflect` import. The AST walker reads struct declarations and method signatures from the parse tree, the same way a linter would.

The key advantage: errors are caught before your tests run. If a lifecycle method has the wrong signature, say `BeforeEach()` with no parameters instead of `BeforeEach(t *gotest.T)`, gotest reports the error at generation time with the file and line number, not as a silent no-op at runtime.

### Stage 2: Code generation

From the AST analysis, gotest generates a `gotest_psuite_test.go` file for each package that contains suites. Here's what gets generated for a simple suite:

```go {title="your code"}
type UserServiceTestSuite struct {
    svc *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.svc = NewUserService()
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) { /* ... */ }
func (s *UserServiceTestSuite) TestDelete(t *gotest.T) { /* ... */ }
```

```go {title="generated bridge (simplified)"}
func TestUserServiceTestSuite(t *testing.T) {
    s := &UserServiceTestSuite{}

    t.Run("TestCreate", func(t *testing.T) {
        s.BeforeEach(gotest.NewT(t))
        s.TestCreate(gotest.NewT(t))
    })

    t.Run("TestDelete", func(t *testing.T) {
        s.BeforeEach(gotest.NewT(t))
        s.TestDelete(gotest.NewT(t))
    })
}
```

The generated code is what you'd write by hand if you were wiring up the suite yourself: a top-level `func Test*` that creates the struct, then runs each test method as a subtest with the lifecycle hooks in the right places.

For suites with fixtures, the generated code also includes fixture initialization with proper DAG ordering, `sync.Once` semantics for one-time setup, and `t.Cleanup` registration for teardown.

### Stage 3: Overlay injection

Here's the part that makes code generation practical: the generated files never touch your source tree.

Go 1.16 introduced the `-overlay` flag for the `go` toolchain. It takes a JSON file that maps virtual file paths to actual file paths on disk. When the compiler encounters a file reference, it checks the overlay map first. This means a file can appear to exist at `pkg/user/gotest_psuite_test.go` while actually living in a temp directory.

```go {title="overlay.json"}
{
  "Replace": {
    "pkg/user/gotest_psuite_test.go": "/tmp/gotest-overlay/0/gotest_psuite_test.go",
    "pkg/order/gotest_psuite_test.go": "/tmp/gotest-overlay/1/gotest_psuite_test.go"
  }
}
```

gotest writes the generated files to a content-addressable cache directory, creates the overlay JSON, and passes it to `go test -overlay=overlay.json`. After the run, nothing is left behind. Your source directory stays clean, your git status stays unchanged, and pull requests don't include generated code.

The cache uses SHA-256 hashing of the generated content. If you run tests twice without changing any suite code, the second run reuses the cached overlay; no regeneration needed.

## What you gain

The code generation approach has several concrete advantages over reflection:

### Compile-time errors

If your `BeforeEach` method takes `(ctx context.Context)` instead of `(t *gotest.T)`, gotest tells you at generation time:

```go
user_test.go:12: BeforeEach: expected signature func(t *gotest.T), got func(ctx context.Context)
```

With reflection, this would either panic at runtime or, worse, silently skip the hook because the framework doesn't recognize the method signature.

### Inspectable output

You can run `gotest generate ./...` to write the generated files to disk and read them. The generated code is standard Go; you can set breakpoints in it, step through the lifecycle logic, and see exactly what gets called in what order. There's no framework magic at runtime, just direct method calls.

### No registration ceremony

With reflection-based frameworks, every suite needs a top-level `func Test*` that calls `suite.Run`. With code generation, the bridge function is generated automatically. You write the suite struct and its methods, and that's it.

```go {title="reflection-based (every suite needs this)"}
func TestUserSuite(t *testing.T) {
    suite.Run(t, new(UserSuite))
}
```

```go {title="code generation (nothing to write)"}
// The bridge is generated. Just write your suite.
```

### Zero runtime overhead

The generated code compiles to direct function calls. No `reflect.ValueOf`, no `reflect.Method`, no interface dispatch. At test execution time, there's no framework code in the call stack: just your suite methods being called directly.

## The pipeline in practice

When you run `gotest ./...`, the full pipeline executes automatically:

1. **Load packages**: discover which packages match the pattern
1. **AST discovery**: walk source files for suites, fixtures, lifecycle hooks
1. **Validate**: check method signatures, fixture consistency, dependency cycles
1. **Generate**: produce bridge code for each package
1. **Cache**: write generated files to the content-addressable cache
1. **Overlay**: create the overlay JSON mapping
1. **Compile & run**: invoke `go test -overlay=...`

Steps 1--6 are fast; they operate on parse trees, not compiled code. And with caching, steps 4--6 are skipped entirely on repeat runs if the source hasn't changed.

For large projects, gotest also uses streaming compilation: test packages start running as soon as they're compiled, without waiting for all packages to finish compiling. This overlaps compilation time with execution time, reducing wall-clock duration for multi-package test runs.

## Inspecting the generated code

If you're curious about what gotest generates for your project, you can write the files to disk:

```sh
# Write generated files alongside your source
gotest generate ./...

# Look at what was generated
cat pkg/user/gotest_psuite_test.go

# Clean up when done
gotest clean
```

The generated files are standard Go test files. They import your package, reference your types, and call your methods. Reading them is the most direct way to understand what the framework does, because the framework *is* the generated code.

## The trade-off

Code generation isn't free. It adds a build step: the AST discovery and generation that runs before `go test`. If you run `go test` directly without `gotest`, you'll get a compilation error because the generated bridge file doesn't exist.

This is a deliberate design choice. A missing generated file produces a clear compiler error that tells you what to do. The alternative, silently doing nothing when the bridge is absent, would be worse.

The code generation approach aligns with Go's broader philosophy: explicit over implicit, compile-time over runtime, readable code over framework magic. The generated code is what a careful developer would write by hand. gotest just writes it for you.
