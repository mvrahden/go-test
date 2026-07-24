---
title: "BDD in Go: What Behavior-Driven Development Actually Means"
date: 2026-07-13
description: "BDD in Go isn't syntax — it's communication. How Go's type system maps to behavior-driven development without describe/context/it ceremony or a DSL."
tags: ["Philosophy"]
keywords: ["bdd golang", "go bdd framework", "ginkgo alternative", "behavior driven development go"]
---

BDD — Behavior-Driven Development — is one of the most misunderstood ideas in testing. Most developers encounter it as syntax: `describe`, `context`, `it`. They learn the keywords, use them in a few test files, and assume they are doing BDD. But BDD is not syntax. It is a communication practice. And Go's type system makes it possible to do BDD without the ceremony that other languages require.

This is not a tutorial on `When` and `It` — that is covered in [Readable Tests with BDD-Style Go]({{< ref "/blog/readable-tests-with-bdd" >}}). This post is about the idea behind BDD, why most implementations of it carry unnecessary weight, and how Go's type system maps to BDD concepts in a way that is unique among programming languages.

## BDD is about communication, not keywords

Dan North introduced BDD in 2006 as a way to reframe TDD. The insight was not "use `describe`/`it` instead of test functions." It was: write tests that describe what the system does in terms that stakeholders understand. The tests become a shared specification — a contract between developers, QA, and product that is executable and always up to date.

This is a genuinely powerful idea. When it works, the test suite is not just a safety net. It is the documentation. A new developer reads the spec output and understands what the system does before reading a line of implementation code. A product manager can look at the test names and confirm that the feature matches the requirement.

But somewhere along the way, the practice got confused with its tooling. In JavaScript, BDD means Jest or Mocha with `describe`/`it` blocks. In Ruby, it means RSpec. In Go, it means Ginkgo. Each ecosystem built a framework around the idea, and the framework became the idea. Developers started asking "which BDD framework should I use?" instead of "how should I structure my tests so they communicate intent?"

The distinction matters because the frameworks carry baggage. Deeply nested closures, string-based descriptions, runtime-only structure, framework-specific lifecycle hooks. These are implementation choices, not requirements of BDD. The question is whether Go offers a better way to express the same ideas.

## Go's type system IS the BDD vocabulary

In gotest, every BDD concept maps to a Go language construct:

| BDD Concept | gotest Mapping | Go Construct |
|---|---|---|
| Subject | Suite type name | `type UserServiceTestSuite struct` |
| Capability | Method name | `func (s *...) TestCreate(t *gotest.T)` |
| Context | When block | `t.When("email is valid", ...)` |
| Behavior | It block | `t.It("creates the user", ...)` |
| Variants | Each loop | `gotest.Each(t, entries)` |

This is not a surface-level analogy. Each row represents a structural correspondence between a BDD concept and a Go language feature. The suite type *is* the subject. The method name *is* the capability. The type system participates in the specification, which means the compiler participates too.

Here is what that looks like in practice:

```go {title="user_service_test.go"}
type UserServiceTestSuite struct {
    service      *UserService
    existingUser *User
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.When("email is valid", func(t *gotest.T) {
        user, err := s.service.Create("alice@example.com")
        t.It("creates the user", func(t *gotest.T) {
            gotest.NoError(t, err)
            gotest.NotNil(t, user)
        })
        t.It("assigns an ID", func(t *gotest.T) {
            gotest.NotEmpty(t, user.ID)
        })
    })
    t.When("email is duplicate", func(t *gotest.T) {
        _, err := s.service.Create(s.existingUser.Email)
        t.It("returns ErrDuplicate", func(t *gotest.T) {
            gotest.ErrorIs(t, err, ErrDuplicate)
        })
    })
}
```

And the spec output reads:

{{< spec title="gotest spec" >}}
UserService
  Create
    when email is valid
      <span class="t-pass">✓</span> creates the user
      <span class="t-pass">✓</span> assigns an ID
    when email is duplicate
      <span class="t-pass">✓</span> returns ErrDuplicate
{{< /spec >}}

That output is a behavioral specification. Not because it uses special keywords, but because the test structure — type, method, When, It — maps directly to subject, capability, context, behavior. The specification emerges from Go's own constructs.

## Why there is no Describe

Ginkgo provides eight keywords before you write a single assertion: `Describe`, `Context`, `It`, `BeforeEach`, `JustBeforeEach`, `AfterEach`, `JustAfterEach`, `BeforeSuite`. These are all functions that take a string description and a closure. The entire test structure lives inside nested closures.

gotest has two: `When` and `It`.

The reason is not minimalism for its own sake. It is that `Describe` is redundant when you have types. Consider how Ginkgo structures a test:

```go {title="Ginkgo"}
var _ = Describe("UserService", func() {
    Describe("Create", func() {
        Context("when email is valid", func() {
            It("creates the user", func() {
                // ...
            })
        })
    })
})
```

In gotest, the struct name *is* the outer `Describe`. The method name *is* the inner `Describe`. The type system already provides two levels of grouping that Ginkgo builds from strings and closures. `When` and `It` are the only additional vocabulary needed because Go already has the rest.

This is not a criticism of Ginkgo — Ginkgo is a well-designed framework that serves its community well. But it is worth understanding why the same structure needs fewer keywords in Go. The answer is that Go's type system does real work that closure-based frameworks must replicate with strings.

Fewer keywords means less to learn, less to argue about ("should this be a `Context` or a `Describe`?"), and a structure that the compiler participates in. You cannot misspell a method name without a compile error. You can misspell a string inside a `Describe()` call and nothing warns you.

## The struct as subject

In BDD, the "subject under test" is the thing whose behavior you are specifying. In RSpec, it is a string: `describe UserService do`. In Jest, it is a string: `describe('UserService', () => {})`. In both cases, the subject exists only at runtime, as a label.

In gotest, the subject is a Go type:

```go
type UserServiceTestSuite struct {
    db      *sql.DB
    service *UserService
    admin   *User
}
```

The suite type name is not decoration. It is a type with structure. It has fields that hold dependencies and state. It has lifecycle methods (`BeforeAll`, `BeforeEach`) that establish context. It has test methods that describe capabilities. And it is scoped to a package — the natural boundary of a Go module.

This is more structured than string-based `Describe` blocks. The type system enforces that every test belongs to a subject, every subject has a clear boundary, and lifecycle is bound to scope. You cannot accidentally share state between two suites because they are different types. You cannot forget to set up a dependency because the struct field is right there, visible in the type definition.

## The method as capability

Each test method on a suite describes one capability of the subject. Not a scenario, not a use case — a capability. The naming convention is `TestVerbNoun`: `TestCreate`, `TestDelete`, `TestSearchByTag`.

Inside the method, `When` blocks establish context (preconditions), and `It` blocks assert behavior (postconditions). This maps exactly to the Given-When-Then structure that BDD is built on:

- **Given**: Suite state — the fields set up in `BeforeEach`. The database is seeded, the service is initialized, the dependencies are wired.
- **When**: The `t.When(...)` block — additional context plus the action being tested. "When email is valid," "when the user does not exist."
- **It**: The `t.It(...)` block — the expected outcome. "It creates the user," "it returns ErrNotFound."

```go {title="order_service_test.go"}
func (s *OrderServiceTestSuite) BeforeEach(t *gotest.T) {
    // Given: a seeded catalog and an authenticated user
    s.catalog = seedCatalog(s.db)
    s.user = createTestUser(s.db)
    s.service = NewOrderService(s.db, s.catalog)
}

func (s *OrderServiceTestSuite) TestPlace(t *gotest.T) {
    t.When("stock is sufficient", func(t *gotest.T) {
        order, err := s.service.Place(s.user.ID, s.catalog.Items[0].ID, 1)
        t.It("creates the order", func(t *gotest.T) {
            gotest.NoError(t, err)
            gotest.NotNil(t, order)
        })
        t.It("decrements stock", func(t *gotest.T) {
            stock := s.catalog.StockFor(s.catalog.Items[0].ID)
            gotest.Equal(t, 9, stock)
        })
    })
    t.When("stock is insufficient", func(t *gotest.T) {
        _, err := s.service.Place(s.user.ID, s.catalog.Items[0].ID, 9999)
        t.It("returns ErrInsufficientStock", func(t *gotest.T) {
            gotest.ErrorIs(t, err, ErrInsufficientStock)
        })
    })
}
```

The Given-When-Then structure is not imposed by a framework. It emerges from the combination of `BeforeEach` (Given), `When` (When), and `It` (Then). Each piece is a Go construct with a clear purpose, not a framework abstraction layered on top of the language.

## Data-driven BDD with Each

Table-driven tests are one of Go's great strengths. `gotest.Each` integrates them into BDD structure, turning each table entry into a variant in the behavioral specification:

```go {title="auth_test.go"}
func (s *AuthTestSuite) TestValidateEmail(t *gotest.T) {
    type entry struct {
        Email   string
        Valid   bool
    }

    for t, e := range gotest.Each(t, []entry{
        {"alice@example.com", true},
        {"bob@company.org", true},
        {"not-an-email", false},
        {"@missing-local", false},
    }) {
        t.When("checking the format", func(t *gotest.T) {
            result := validateEmail(e.Email)
            t.It("matches expected validity", func(t *gotest.T) {
                gotest.Equal(t, e.Valid, result)
            })
        })
    }
}
```

Each entry becomes a row in the behavior specification — same structure, different data. This is how BDD handles parameterized behavior. The table is not a workaround for the framework; it is a first-class part of the specification.

In closure-based BDD frameworks, parameterized tests are awkward. You either generate `It` blocks in a loop (fighting closure-over-loop-variable bugs) or use a separate table-test mechanism that lives outside the BDD structure. In gotest, the table and the BDD structure are the same thing because both are Go code.

## BDD at the project level

When every suite follows this convention — struct = subject, method = capability, When = context, It = behavior — the spec output for the entire project reads as a complete behavioral specification:

{{< spec title="gotest spec ./..." >}}
UserService
  Create
    when email is valid
      <span class="t-pass">✓</span> creates the user
      <span class="t-pass">✓</span> assigns an ID
    when email is duplicate
      <span class="t-pass">✓</span> returns ErrDuplicate
  Delete
    when user exists
      <span class="t-pass">✓</span> removes the user
    when user does not exist
      <span class="t-pass">✓</span> returns ErrNotFound

OrderService
  Place
    when stock is sufficient
      <span class="t-pass">✓</span> creates the order
      <span class="t-pass">✓</span> decrements stock
    when stock is insufficient
      <span class="t-pass">✓</span> returns ErrInsufficientStock
{{< /spec >}}

This is not a testing report. It is a specification document — generated from tests that are also the implementation verification. The tests and the spec are the same artifact. A new team member reads this output and knows what the system does. A product manager reads it and confirms the feature coverage. This is what North meant by BDD: the tests *are* the specification. For how to export this output as markdown and treat it as a real documentation artifact, see [Go Tests as Living Documentation]({{< ref "/blog/tests-as-documentation" >}}).

And because the structure comes from Go types and methods, it is not fragile. Rename a struct and the spec updates. Add a method and a new capability appears. Delete a `When` block and the context disappears from the spec. The specification cannot drift from the implementation because they are the same code.

## BDD vs. "BDD-flavored testing"

An honest distinction is worth making. Most teams using Ginkgo, Jest, or RSpec are doing BDD-flavored testing, not BDD. True BDD starts with the specification — written collaboratively with stakeholders — and derives tests from it. The spec comes first. The code makes it pass. BDD-flavored testing starts with code and makes the tests readable after the fact.

Both are valuable. BDD-flavored testing produces better test output, encourages better test organization, and makes test failures more meaningful. It is a real improvement over flat test functions with opaque names. You do not need to practice "true BDD" to benefit from structured, readable tests.

gotest does not pretend otherwise. It gives you tools to make tests that read as specifications. Whether you practice full BDD (specification-first, stakeholder-collaborative) or write well-structured tests that happen to be readable, the same syntax works. The struct-method-When-It structure is useful either way — the value does not depend on your development process.

The important thing is that the structure exists. Flat test functions with names like `TestCreateUser_WhenEmailInvalid_ReturnsError` encode the same information, but they encode it in a string that only humans parse. With types and methods, the compiler parses it too. And with `When`/`It`, the test runner renders it as a tree. The information is the same; the structure makes it accessible.

## What the compiler gives you

In string-based BDD frameworks, the specification exists only at runtime. The `Describe` strings are concatenated into a path, the `It` strings become leaf labels, and the hierarchy is assembled when the tests execute. Nothing checks it before then.

When BDD structure maps to Go types, the compiler participates:

- **Misspelled method name?** Compile error. In a closure-based framework, a misspelled `Describe` string silently creates a different spec path.
- **Wrong lifecycle signature?** The code generator catches it at generation time with a clear error and line number. In a reflection-based framework, the method is silently ignored.
- **Duplicate test names?** Two methods with the same name on the same type is a compile error. Two `It` blocks with the same string in the same `Describe` is... nothing. It runs both, and the output is confusing.
- **Refactoring?** Rename a struct and your IDE updates every reference. Rename a string inside `Describe()` and you update that one call — every cross-reference is a manual search.

These are not minor ergonomic improvements. They change the failure mode from "silent wrong behavior at runtime" to "loud error before the test runs." In a practice that is fundamentally about communication, catching miscommunication early matters.

## BDD without a DSL

BDD in Go does not need a DSL. It needs types, methods, and two keywords. The language already provides the structure that other ecosystems build from scratch with closures and strings. The suite struct is the subject. The method is the capability. `When` is the context. `It` is the behavior. The compiler checks what string-based frameworks cannot.

This is not an argument against other BDD frameworks — they solve real problems in languages that lack Go's structural tools. It is an argument that Go developers have an opportunity to do BDD with less ceremony and more compiler support than any other language offers. The type system is not an obstacle to BDD. It is the best implementation of it.

For the practical syntax guide to `When`, `It`, and spec output, see [Readable Tests with BDD-Style Go]({{< ref "/blog/readable-tests-with-bdd" >}}). If you would rather feel the difference than read about it, [Your First Go Test Suite in 10 Minutes]({{< ref "/blog/zero-to-suite" >}}) takes you from install to spec output in one sitting. For the complete API, see the [reference docs]({{< ref "/reference" >}}).
