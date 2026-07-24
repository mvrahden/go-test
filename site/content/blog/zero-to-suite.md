---
title: "Your First Go Test Suite in 10 Minutes: A gotest Tutorial"
date: 2026-07-12
description: "Go test suite tutorial: from go install to a running gotest suite with lifecycle hooks, BDD structure, and readable spec output — in 10 minutes."
tags: ["Tutorial"]
keywords: ["go test suite tutorial", "gotest getting started", "go testing framework", "go bdd tutorial"]
cta_text: "Scaffold your first suite today."
howto:
  name: "Write your first gotest suite"
  steps:
    - name: "Install gotest"
      text: "Install the gotest binary with go install and verify it with gotest version."
    - name: "Set up a project"
      text: "Create a small Go module with a counter package to test."
    - name: "Write the suite"
      text: "Declare a struct whose name ends in TestSuite, add a BeforeEach hook, and write Test methods that use gotest's generic assertions."
    - name: "Run the suite"
      text: "Run gotest ./... — it generates bridge code via Go's -overlay flag and executes standard go test."
    - name: "Add BDD structure"
      text: "Group setup and assertions with t.When() and t.It() so the tests read like a specification."
    - name: "Render the spec output"
      text: "Run gotest spec ./... to render the test hierarchy as a readable behavioral tree."
    - name: "Focus a test"
      text: "Prefix a method or suite name with F_ to run only that item, or X_ to skip it."
    - name: "Use watch mode"
      text: "Run gotest watch ./... to re-run affected packages automatically on every save."
---

Plain `go test` carries you a long way — until you need setup that runs before every test, related cases grouped under a shared context, and output you can actually read. At that point most of us start hand-rolling lifecycle helpers and squinting at walls of `--- PASS` lines. In the next 10 minutes you'll go from `go install` to a structured test suite with lifecycle hooks, BDD-style grouping, and spec output — all running on standard `go test`.

No prior gotest knowledge required. You need Go 1.24+ and a terminal.

## Install gotest

gotest is a single binary. Install it with `go install`:

```sh
go install github.com/mvrahden/go-test/cmd/gotest@latest
```

Verify the installation:

```sh
gotest version
```

## Set up a project

Create a small Go module to work with. We will build a `counter` package and test it:

```sh
mkdir counter-demo && cd counter-demo
go mod init counter-demo
```

Write a simple counter type:

```go {title="counter.go"}
package counter

type Counter struct {
    value int
}

func New() *Counter             { return &Counter{} }
func (c *Counter) Value() int   { return c.value }
func (c *Counter) Inc()         { c.value++ }
func (c *Counter) Dec()         { c.value-- }
func (c *Counter) Reset()       { c.value = 0 }
```

Nothing special so far. Now let's test it.

## Write a test suite

A gotest suite is a Go struct whose name ends in `TestSuite`. Test methods are pointer-receiver methods whose names start with `Test`. That is the entire API — naming conventions.

Add the gotest library to your module and create a test file:

```sh
go get github.com/mvrahden/go-test/pkg/gotest
```

```go {title="counter_test.go"}
package counter

import "github.com/mvrahden/go-test/pkg/gotest"

type CounterTestSuite struct {
    c *Counter
}

func (s *CounterTestSuite) BeforeEach(t *gotest.T) {
    s.c = New()
}

func (s *CounterTestSuite) TestInc(t *gotest.T) {
    s.c.Inc()
    gotest.Equal(t, 1, s.c.Value())
}

func (s *CounterTestSuite) TestDec(t *gotest.T) {
    s.c.Inc()
    s.c.Inc()
    s.c.Dec()
    gotest.Equal(t, 1, s.c.Value())
}

func (s *CounterTestSuite) TestReset(t *gotest.T) {
    s.c.Inc()
    s.c.Inc()
    s.c.Inc()
    s.c.Reset()
    gotest.Equal(t, 0, s.c.Value())
}
```

A few things to notice:

- **`CounterTestSuite`** ends in `TestSuite`, so gotest recognizes it as a suite.
- **`BeforeEach`** runs before every test method. Each test gets a fresh `Counter` — no shared state between tests.
- **Assertions** are standalone generic functions: `gotest.Equal(t, expected, actual)`. They stop the test on failure (like testify's `Require`). Type mismatches are caught at compile time.
- **No boilerplate.** No `suite.Run(t, new(...))`, no embedded base struct, no registration call. The struct name and method names are the entire declaration.

## Run the suite

Use the `gotest` command instead of `go test`:

```sh
gotest ./...
```

Behind the scenes, gotest reads your source files with `go/parser`, generates bridge functions that connect your suite to `go test`, injects them via Go's `-overlay` flag, and runs `go test`. The generated code never touches your source tree. What runs is standard `go test`, what you see is standard `go test` output.

You should see all three tests pass:

{{< terminal title="output" >}}
ok   counter-demo   0.3s
{{< /terminal >}}

## Add BDD structure

Flat test methods work, but as the number of tests grows, grouping related assertions under a shared context makes them easier to read. gotest provides `t.When()` for context and `t.It()` for behavioral expectations. Both are thin wrappers around `t.Run`.

Let's rewrite the suite with BDD structure:

```go {title="counter_test.go"}
package counter

import "github.com/mvrahden/go-test/pkg/gotest"

type CounterTestSuite struct {
    c *Counter
}

func (s *CounterTestSuite) BeforeEach(t *gotest.T) {
    s.c = New()
}

func (s *CounterTestSuite) TestInc(t *gotest.T) {
    t.When("incrementing once", func(w *gotest.T) {
        s.c.Inc()

        w.It("has value 1", func(it *gotest.T) {
            gotest.Equal(it, 1, s.c.Value())
        })
    })

    t.When("incrementing three times", func(w *gotest.T) {
        s.c.Inc()
        s.c.Inc()
        s.c.Inc()

        w.It("has value 3", func(it *gotest.T) {
            gotest.Equal(it, 3, s.c.Value())
        })
    })
}

func (s *CounterTestSuite) TestDec(t *gotest.T) {
    t.When("decrementing from 2", func(w *gotest.T) {
        s.c.Inc()
        s.c.Inc()
        s.c.Dec()

        w.It("has value 1", func(it *gotest.T) {
            gotest.Equal(it, 1, s.c.Value())
        })
    })

    t.When("decrementing past zero", func(w *gotest.T) {
        s.c.Dec()

        w.It("goes negative", func(it *gotest.T) {
            gotest.Equal(it, -1, s.c.Value())
        })
    })
}

func (s *CounterTestSuite) TestReset(t *gotest.T) {
    t.When("resetting after increments", func(w *gotest.T) {
        s.c.Inc()
        s.c.Inc()
        s.c.Inc()
        s.c.Reset()

        w.It("returns to zero", func(it *gotest.T) {
            gotest.Equal(it, 0, s.c.Value())
        })
    })
}
```

The structure reads like a specification: *Counter → Inc → when incrementing once → it has value 1*. Each `When` block establishes a context (the setup and action), and each `It` block makes an assertion about the outcome.

Run it again:

```sh
gotest ./...
```

## Render the spec output

The test names encode a behavioral specification. The `gotest spec` command renders it as a readable tree:

```sh
gotest spec ./...
```

Output:

{{< spec title="gotest spec ./..." >}}
Counter
  Inc
    when incrementing once
      <span class="t-pass">✓</span> has value 1
    when incrementing three times
      <span class="t-pass">✓</span> has value 3
  Dec
    when decrementing from 2
      <span class="t-pass">✓</span> has value 1
    when decrementing past zero
      <span class="t-pass">✓</span> goes negative
  Reset
    when resetting after increments
      <span class="t-pass">✓</span> returns to zero
{{< /spec >}}

This is generated from the test names and `When`/`It` labels. The suite name is stripped of its `TestSuite` suffix, method names lose their `Test` prefix, and the hierarchy is indented. The result is a behavioral specification that non-developers can read.

> You can also run `gotest ./... --spec` to see spec output instead of the normal test results. The `--spec` flag redirects the entire run through the spec renderer.

## Add a second suite

Multiple suites can coexist in the same package. Each suite runs in its own OS process, so they are completely isolated from each other. Let's add a suite that tests boundary behavior:

```go {title="boundary_test.go"}
package counter

import "github.com/mvrahden/go-test/pkg/gotest"

type BoundaryTestSuite struct {
    c *Counter
}

func (s *BoundaryTestSuite) BeforeEach(t *gotest.T) {
    s.c = New()
}

func (s *BoundaryTestSuite) TestNewCounter(t *gotest.T) {
    t.It("starts at zero", func(it *gotest.T) {
        gotest.Equal(it, 0, s.c.Value())
    })
}

func (s *BoundaryTestSuite) TestResetIdempotent(t *gotest.T) {
    t.When("resetting a counter that is already zero", func(w *gotest.T) {
        s.c.Reset()

        w.It("stays at zero", func(it *gotest.T) {
            gotest.Equal(it, 0, s.c.Value())
        })
    })
}
```

Run `gotest spec ./...` again and both suites appear in the output:

{{< spec title="gotest spec ./..." >}}
Boundary
  NewCounter
    <span class="t-pass">✓</span> starts at zero
  ResetIdempotent
    when resetting a counter that is already zero
      <span class="t-pass">✓</span> stays at zero

Counter
  Inc
    when incrementing once
      <span class="t-pass">✓</span> has value 1
    when incrementing three times
      <span class="t-pass">✓</span> has value 3
  Dec
    when decrementing from 2
      <span class="t-pass">✓</span> has value 1
    when decrementing past zero
      <span class="t-pass">✓</span> goes negative
  Reset
    when resetting after increments
      <span class="t-pass">✓</span> returns to zero
{{< /spec >}}

Both suites ran in separate processes, concurrently. Neither can affect the other, even if one panics or leaks goroutines.

## Focus on one test

During development, you often want to run just one suite or one test method. Prefix the name with `F_`:

```go
func (s *CounterTestSuite) F_TestDec(t *gotest.T) {
    // only this test method runs
}
```

Only focused items run. Remove the `F_` prefix when you're done. If you forget and push it, `gotest --ci` will fail the build — the CI guard catches committed focus prefixes.

Similarly, `X_` excludes a test:

```go
func (s *CounterTestSuite) X_TestReset(t *gotest.T) {
    // this test is skipped
}
```

Both prefixes work on suites too: `F_CounterTestSuite` runs only that suite.

## Use watch mode

For a tight feedback loop during development, use `gotest watch`:

```sh
gotest watch ./...
```

Every time you save a file, gotest re-runs only the affected packages. Combine with `F_` focus to iterate on a single test in under a second.

## What to explore next

You've covered the core workflow: suites, lifecycle hooks, BDD structure, spec output, focus, and watch mode. Here's where to go deeper:

- **Fixtures.** Package-scoped resources with lifecycle management and DAG-based dependencies. See [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}).
- **Assertions.** The full assertion API: `Equal`, `NoError`, `Contains`, `Len`, `InDelta`, `ErrorIs`, `Eventually`, `Consistently`, and more. See the [Reference]({{< ref "/reference#assertions" >}}).
- **Migration.** If you have existing testify/suite tests, `gotest migrate ./...` converts them automatically. See [Migrating from testify/suite]({{< ref "/blog/testify-migration-guide" >}}).
- **Parallel execution.** Suite-level parallelism is on by default (each suite is its own process). For method-level parallelism within a suite, see [the lifecycle reference]({{< ref "/reference#lifecycle" >}}).
- **CI integration.** Failure-focused summaries, GitHub PR annotations, and coverage gates. See [gotest in CI]({{< ref "/blog/gotest-in-ci" >}}).
- **VS Code extension.** Test Explorer, CodeLens, coverage gutters, watch mode, and debug support. [Install from the Marketplace](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest).
