---
title: "Readable Tests with BDD-Style Go"
date: 2026-07-08
description: "Test names like TestCreateUser_WhenEmailInvalid_ReturnsError are hard to scan. BDD-style labeled subtests turn Go tests into living documentation."
tags: ["Testing"]
aliases: ["/blog/readable-tests-with-bdd.html"]
---

When a test fails in CI, the first thing you read is its name. And for most Go projects, that name looks like this:

{{< gotest-output title="go test -v output" >}}
=== RUN   TestCreateUser_WhenEmailInvalid_ReturnsError
--- FAIL: TestCreateUser_WhenEmailInvalid_ReturnsError (0.01s)
{{< /gotest-output >}}

You can decode it. `TestCreateUser` is the subject, `WhenEmailInvalid` is the condition, `ReturnsError` is the expectation. But it takes a moment. And when you're scanning twenty failures across five packages, those moments add up.

This post looks at why Go test output is hard to read at scale, how BDD-style structure helps, and what it takes to turn test runs into something that reads like a specification.

## The naming problem

Go's test runner requires test function names to start with `Test` and use CamelCase or underscores. This forces you to encode three separate ideas, namely the subject, the condition, and the expectation, into a single identifier:

```go
func TestCreateUser_WhenEmailInvalid_ReturnsError(t *testing.T) { ... }
func TestCreateUser_WhenEmailAlreadyExists_ReturnsDuplicate(t *testing.T) { ... }
func TestCreateUser_WhenInputValid_CreatesUser(t *testing.T) { ... }
func TestCreateUser_WhenInputValid_SendsWelcomeEmail(t *testing.T) { ... }
func TestDeleteUser_WhenUserExists_SoftDeletes(t *testing.T) { ... }
func TestDeleteUser_WhenUserNotFound_ReturnsNotFound(t *testing.T) { ... }
```

Six test functions, but the structure is invisible. You have to read every name to understand that there are two subjects (`CreateUser` and `DeleteUser`), three conditions for create, two for delete, and that two of the create expectations share the same condition.

Subtests improve this somewhat:

```go
func TestCreateUser(t *testing.T) {
    t.Run("when email is invalid", func(t *testing.T) {
        t.Run("returns error", func(t *testing.T) {
            // ...
        })
    })
    t.Run("when input is valid", func(t *testing.T) {
        t.Run("creates the user", func(t *testing.T) {
            // ...
        })
    })
}
```

The code itself is more structured. But the output isn't:

{{< gotest-output title="go test -v output" >}}
=== RUN   TestCreateUser
=== RUN   TestCreateUser/when_email_is_invalid
=== RUN   TestCreateUser/when_email_is_invalid/returns_error
--- PASS: TestCreateUser/when_email_is_invalid/returns_error (0.00s)
--- PASS: TestCreateUser/when_email_is_invalid (0.00s)
=== RUN   TestCreateUser/when_input_is_valid
=== RUN   TestCreateUser/when_input_is_valid/creates_the_user
--- PASS: TestCreateUser/when_input_is_valid/creates_the_user (0.01s)
--- PASS: TestCreateUser/when_input_is_valid (0.01s)
--- PASS: TestCreateUser (0.02s)
PASS
{{< /gotest-output >}}

Every subtest name is repeated three times: once in `=== RUN`, again in its own `--- PASS`, and again in its parent's `--- PASS`. The nesting is encoded by slash-separated paths, not indentation. For a single test function with four subtests, that's twelve lines of output. Scale to a real test suite and the signal-to-noise ratio collapses.

## What BDD structure looks like

Behavior-driven development (BDD) introduces a simple idea: describe *what* a system does in terms of *contexts* and *expectations*. A test for a user service might read:

> UserService → Create → when email is valid → creates the user

The structure is always the same: subject, behavior, context, expectation. Each level adds specificity. The key insight is that contexts and expectations are separate concepts; they shouldn't be jammed into a single function name.

In gotest, two methods on `*gotest.T` express this structure:

- `t.When(label, fn)`: establishes a context. "When the email is invalid," "when the user already exists."
- `t.It(label, fn)`: states an expectation. "It creates the user," "it returns an error."

Both are thin wrappers around `t.Run`; they create standard Go subtests. The difference is semantic: `When` is for conditions, `It` is for assertions. Here's what the user service example looks like:

```go {title="service_test.go"}
type UserServiceTestSuite struct {
    svc *UserService
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.When("email is valid", func(w *gotest.T) {
        user := User{Name: "Alice", Email: "alice@example.com"}
        err := s.svc.Create(user)

        w.It("creates the user", func(it *gotest.T) {
            gotest.NoError(it, err)
        })
        w.It("sends a welcome email", func(it *gotest.T) {
            gotest.Equal(it, s.svc.EmailsSent(), 1)
        })
    })

    t.When("email already exists", func(w *gotest.T) {
        _ = s.svc.Create(User{Email: "bob@example.com"})
        err := s.svc.Create(User{Email: "bob@example.com"})

        w.It("returns ErrDuplicate", func(it *gotest.T) {
            gotest.ErrorIs(it, err, ErrDuplicate)
        })
    })
}

func (s *UserServiceTestSuite) TestDelete(t *gotest.T) {
    t.It("soft-deletes the user", func(it *gotest.T) {
        s.svc.Create(User{Email: "charlie@example.com"})
        err := s.svc.Delete("charlie@example.com")
        gotest.NoError(it, err)
    })
}
```

A few things to notice:

- **Setup runs inside `When` blocks.** The condition and its setup live together. `When("email already exists", ...)` creates the duplicate inside the block, not in a separate fixture.
- **Multiple `It` blocks per `When`.** "creates the user" and "sends a welcome email" share the same setup but assert different things.
- **Suite and method names carry meaning.** `UserServiceTestSuite` becomes the subject. `TestCreate` becomes the behavior. `When`/`It` provide context and expectation.
- **You can skip `When`.** `TestDelete` goes straight to `It` because the test doesn't need a conditional context.

## Tests as specification output

The structure in the code is only half the value. The other half is what you see when the tests run. `gotest spec` transforms the test output into an indented tree:

{{< spec title="gotest spec ./..." >}}
<span class="t-suite">UserService</span>
  <span class="t-test">Create</span>
    <span class="t-when">when email is valid</span>
      <span class="t-pass">✓</span> creates the user <span class="t-time">(8ms)</span>
      <span class="t-pass">✓</span> sends a welcome email <span class="t-time">(120ms)</span>
    <span class="t-when">when email already exists</span>
      <span class="t-pass">✓</span> returns ErrDuplicate <span class="t-time">(<1ms)</span>
  <span class="t-test">Delete</span>
    <span class="t-pass">✓</span> soft-deletes the user <span class="t-time">(5ms)</span>

<span class="t-summary">1 suite, 4 passed in 0.34s</span>
{{< /spec >}}

Compare this with the `go test -v` output for the same tests. The spec output is:

- **8 lines** instead of 20+. No repetition, no `=== RUN` / `--- PASS` noise.
- **Hierarchical.** 2-space indentation shows structure at a glance. Suite name is bold. Method names are bold. Contexts are dimmed. Expectations get checkmarks.
- **Human-readable names.** `UserServiceTestSuite` becomes "UserService" (suffix stripped). `TestCreate` becomes "Create" (prefix stripped). No CamelCase, no underscores.

When a test fails, the tree structure tells you exactly where:

{{< spec title="gotest spec ./..." >}}
<span class="t-suite">UserService</span>
  <span class="t-test">Create</span>
    <span class="t-when">when email is valid</span>
      <span class="t-pass">✓</span> creates the user <span class="t-time">(8ms)</span>
      <span class="t-fail">✗</span> sends a welcome email <span class="t-time">(120ms)</span>
    <span class="t-when">when email already exists</span>
      <span class="t-pass">✓</span> returns ErrDuplicate <span class="t-time">(<1ms)</span>

<span class="t-summary">1 suite, 2 passed, <span class="t-fail">1 failed</span> in 0.34s</span>
{{< /spec >}}

The red cross at "sends a welcome email" under "when email is valid" is a sentence: *UserService Create, when email is valid, fails to send a welcome email.* You know what's broken without reading any code.

## When and It can nest arbitrarily

Both `When` and `It` delegate to `t.Run`, so they nest to any depth. This is useful when a single context has sub-conditions:

```go
func (s *OrderServiceTestSuite) TestCheckout(t *gotest.T) {
    t.When("the cart is not empty", func(w *gotest.T) {
        w.When("payment succeeds", func(w *gotest.T) {
            w.It("creates an order", func(it *gotest.T) {
                // ...
            })
            w.It("clears the cart", func(it *gotest.T) {
                // ...
            })
        })
        w.When("payment fails", func(w *gotest.T) {
            w.It("does not create an order", func(it *gotest.T) {
                // ...
            })
        })
    })
}
```

The spec output reflects the nesting:

{{< spec title="gotest spec ./..." >}}
<span class="t-suite">OrderService</span>
  <span class="t-test">Checkout</span>
    <span class="t-when">when the cart is not empty</span>
      <span class="t-when">when payment succeeds</span>
        <span class="t-pass">✓</span> creates an order <span class="t-time">(12ms)</span>
        <span class="t-pass">✓</span> clears the cart <span class="t-time">(3ms)</span>
      <span class="t-when">when payment fails</span>
        <span class="t-pass">✓</span> does not create an order <span class="t-time">(2ms)</span>

<span class="t-summary">1 suite, 3 passed in 0.21s</span>
{{< /spec >}}

Each level of `When` narrows the context. The spec reads as a decision tree: checkout, when the cart is not empty, when payment succeeds, creates an order. You can trace any path from root to leaf and get a complete behavioral statement.

## The spec command

`gotest spec` works by running `gotest test` with `-json` output, then transforming the structured test events into the indented tree. It accepts the same package patterns as `go test`:

```sh
# spec output for all packages
gotest spec ./...

# verbose mode: show durations for passing tests
gotest spec -v ./...

# filter by test name
gotest spec --run UserService ./...

# disable color for CI logs
gotest spec --no-color ./...
```

The rendering strips naming conventions automatically:

- `UserServiceTestSuite` → **UserService** (drops `TestSuite` suffix)
- `TestCreate` → **Create** (drops `Test` prefix)
- Underscores in `When`/`It` labels → spaces

Suite and method names are bold. Contexts are dimmed. Passing expectations get a green checkmark, failing ones a red cross. The summary line at the bottom shows total counts and duration.

## Tests that document themselves

The deeper value of spec output is that it doubles as documentation. When someone new joins the team and asks "what does the order service do?", you can point them at `gotest spec ./pkg/order/...` instead of a wiki page that's three sprints out of date.

The spec output stays accurate because it's generated from the tests. If the behavior changes, the tests change, and the spec reflects it. There's no separate document to keep in sync.

This works best when test names are written as behavioral descriptions rather than implementation details. Compare:

- **Implementation-focused:** `t.It("calls repository.Save", ...)`
- **Behavior-focused:** `t.It("persists the user", ...)`

The first tells you what the code does internally. The second tells you what the system does for its users. When the spec output reads like a behavioral contract, you've turned your test suite into a living specification.
