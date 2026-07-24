---
title: "Contract Testing Go Interfaces with Generic Test Suites"
date: 2026-07-21
description: "Go interface contract testing with generics: define the contract in one test suite, instantiate it for every implementation, and catch drift as it happens."
tags: ["Patterns"]
keywords: ["go interface contract testing", "go generics test suite", "go test multiple implementations"]
cta_text: "Turn your interfaces into verified contracts."
---

Go interfaces define contracts. An `io.Reader` promises `Read(p []byte) (n int, err error)`. A `Storage` interface promises `Get`, `Put`, `Delete`. But who verifies that every implementation actually honors the contract? Usually, each implementation has its own tests, written independently, checking slightly different things. The contract drifts.

Generic suites fix this. Write one test suite parameterized by a type, then instantiate it for every implementation. The same behaviors are verified against every implementation. If you add a contract requirement, every implementation is tested. If an implementation deviates, the specific behavior that fails is named in the spec.

## The problem: duplicated conformance tests

The typical pattern without generics looks like this. Two implementations of the same interface, two separate test suites, testing the same contract independently:

```go {title="memory_test.go"}
type MemoryStorageTestSuite struct {
    store *MemoryStorage
}

func (s *MemoryStorageTestSuite) BeforeEach(t *gotest.T) {
    s.store = NewMemoryStorage()
}

func (s *MemoryStorageTestSuite) TestPut(t *gotest.T) {
    t.When("key is new", func(t *gotest.T) {
        err := s.store.Put("key", "value")
        t.It("stores the value", func(t *gotest.T) {
            gotest.NoError(t, err)
            v, _ := s.store.Get("key")
            gotest.Equal(t, "value", v)
        })
    })
}
```

```go {title="redis_test.go"}
type RedisStorageTestSuite struct {
    store *RedisStorage
}

func (s *RedisStorageTestSuite) BeforeEach(t *gotest.T) {
    s.store = NewRedisStorage(testRedisAddr)
}

func (s *RedisStorageTestSuite) TestPut(t *gotest.T) {
    t.When("key is new", func(t *gotest.T) {
        err := s.store.Put("key", "value")
        t.It("stores the value", func(t *gotest.T) {
            gotest.NoError(t, err)
            // Slightly different: this test also checks TTL
            // The contract is already drifting
        })
    })
}
```

The problems are structural:

- **Each implementation tests different aspects of the same contract.** One suite checks error values, another checks side effects, a third checks neither. The contract is implicit and fragmented.
- **Adding a new contract requirement means updating every implementation's tests.** If the interface gains a `List` method, every suite needs a new `TestList`. In practice, some suites get updated and some do not.
- **Subtle differences in test assertions mask contract violations.** If the memory implementation returns `nil` on delete-nonexistent and the Redis implementation returns `ErrNotFound`, no test catches the inconsistency because they are checking different things.
- **New implementations start from scratch.** When someone adds an S3 backend, they write a new test suite from memory, guessing at what the contract requires.

## The solution: generic suites

A generic suite is parameterized by the interface it tests. The type parameter constrains the suite to implementations of that interface, and the suite defines the contract once:

```go {title="storage_contract_test.go"}
type StorageContractTestSuite[T Storage] struct {
    factory func() T
    store   T
}

func (s *StorageContractTestSuite[T]) BeforeEach(t *gotest.T) {
    s.store = s.factory()
}

func (s *StorageContractTestSuite[T]) TestPut(t *gotest.T) {
    t.When("key is new", func(t *gotest.T) {
        err := s.store.Put("key", "value")
        t.It("stores the value without error", func(t *gotest.T) {
            gotest.NoError(t, err)
        })
        t.It("makes the value retrievable", func(t *gotest.T) {
            v, err := s.store.Get("key")
            gotest.NoError(t, err)
            gotest.Equal(t, "value", v)
        })
    })

    t.When("key already exists", func(t *gotest.T) {
        s.store.Put("key", "original")
        err := s.store.Put("key", "updated")
        t.It("overwrites the value", func(t *gotest.T) {
            gotest.NoError(t, err)
            v, _ := s.store.Get("key")
            gotest.Equal(t, "updated", v)
        })
    })
}

func (s *StorageContractTestSuite[T]) TestGet(t *gotest.T) {
    t.When("key does not exist", func(t *gotest.T) {
        _, err := s.store.Get("missing")
        t.It("returns ErrNotFound", func(t *gotest.T) {
            gotest.ErrorIs(t, err, ErrNotFound)
        })
    })
}

func (s *StorageContractTestSuite[T]) TestDelete(t *gotest.T) {
    t.When("key exists", func(t *gotest.T) {
        s.store.Put("key", "value")
        err := s.store.Delete("key")
        t.It("removes the value", func(t *gotest.T) {
            gotest.NoError(t, err)
            _, err := s.store.Get("key")
            gotest.ErrorIs(t, err, ErrNotFound)
        })
    })

    t.When("key does not exist", func(t *gotest.T) {
        err := s.store.Delete("missing")
        t.It("returns no error", func(t *gotest.T) {
            gotest.NoError(t, err)
        })
    })
}
```

The contract is defined once. Every behavior is named. Every assertion is the same regardless of the backing implementation.

## Instantiating for each implementation

Type aliases create concrete suites from the generic definition:

```go {title="aliases_test.go"}
type MemoryStorageTestSuite = StorageContractTestSuite[*MemoryStorage]
type RedisStorageTestSuite = StorageContractTestSuite[*RedisStorage]
type SQLStorageTestSuite = StorageContractTestSuite[*SQLStorage]
```

Each alias is a full, independent test suite. The code generator picks up each alias and generates the test wiring for it. The spec output shows each implementation separately:

{{< spec title="gotest spec ./..." >}}
MemoryStorage (StorageContract)
  Put
    when key is new
      <span class="t-pass">✓</span> stores the value without error
      <span class="t-pass">✓</span> makes the value retrievable
    when key already exists
      <span class="t-pass">✓</span> overwrites the value
  Get
    when key does not exist
      <span class="t-pass">✓</span> returns ErrNotFound
  Delete
    when key exists
      <span class="t-pass">✓</span> removes the value
    when key does not exist
      <span class="t-pass">✓</span> returns no error

RedisStorage (StorageContract)
  Put
    when key is new
      <span class="t-pass">✓</span> stores the value without error
      <span class="t-pass">✓</span> makes the value retrievable
    ...
{{< /spec >}}

Same contract, same behaviors, different implementations. If Redis handles delete-nonexistent differently from memory, the specific behavior that diverges is named.

## Providing implementation-specific setup

The `factory` field in the generic suite is set during suite initialization. Each alias can provide its own factory through `BeforeAll` or the suite's struct fields.

For simple cases (in-memory), the factory is trivial. For infrastructure-backed implementations, use [fixtures]({{< ref "/blog/test-fixtures-in-go" >}}):

```go {title="redis_setup_test.go"}
type RedisStorageTestSuite = StorageContractTestSuite[*RedisStorage]

// The RedisStorageTestSuite uses a RedisFixture for connection management
type redisSetup struct {
    Redis *RedisFixture
}
```

Or initialize the factory directly in a test helper. The point is that the contract tests are the same; only the setup differs. When multiple contract suites share the same backing infrastructure, [Advanced Go Test Fixtures]({{< ref "/blog/advanced-fixture-patterns" >}}) covers patterns for composing and reusing that setup across suites.

## A real example: search index contract

The gotest codebase itself uses this pattern. The search index is generic over any type that satisfies the `Indexable` constraint:

```go {title="index_contract_test.go"}
type Indexable interface {
    comparable
    SearchText() string
    Label() string
}

type IndexContractTestSuite[T Indexable] struct{}

func (s *IndexContractTestSuite[T]) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{Parallel: true}
}

func (s *IndexContractTestSuite[T]) TestEmptyIndex(t *gotest.T) {
    t.When("the index has no items", func(t *gotest.T) {
        idx := newIndex[T]()
        t.It("returns no search results", func(t *gotest.T) {
            gotest.Empty(t, idx.Search("anything"))
        })
        t.It("has no labels", func(t *gotest.T) {
            gotest.Empty(t, idx.Labels())
        })
        t.It("reports zero items", func(t *gotest.T) {
            gotest.Zero(t, len(idx.All()))
        })
    })
}

// Instantiated for two types
type ArticleIndexTestSuite = IndexContractTestSuite[Article]
type ProductIndexTestSuite = IndexContractTestSuite[Product]
```

Two types, same contract, full parallel execution. The `SuiteConfig` method enables method-level parallelism, so all behaviors within a single suite run concurrently. Because each type alias produces an independent suite, the cross-suite parallelism is automatic.

## Constraints and trade-offs

Generic suites are not universally applicable. The pattern has real constraints that are worth understanding before adopting it.

### Same-package only

Generic aliases must be in the same package as the generic suite. This is a Go compiler constraint: type aliases with type parameters cannot cross package boundaries the same way non-generic aliases can. In gotest terms, this means the contract suite and its aliases live in the `ptest` (white-box) package, not `pxtest` (black-box). If the contract suite and the implementations are in different packages, the aliases must live alongside the generic suite definition.

### Factory pattern

The generic suite needs a way to create instances. A `factory` field or a `NewT()` constraint on the type parameter are the common approaches. The factory pattern is more flexible because it does not impose any constructor signature on the implementations, but it requires initialization in `BeforeAll` or a fixture.

### Implementation-specific behaviors

Some implementations have behaviors beyond the contract. Redis has TTL. SQL has transactions. File-based storage has permission errors. These do not belong in the contract suite. Test them in separate, implementation-specific suites. The contract suite covers the shared contract; additional suites cover the extras.

This is a feature, not a limitation. It forces a clear distinction between "what the interface promises" and "what a specific implementation offers." That distinction is valuable even if you never use generic suites.

## Scaffold support

gotest can generate a contract suite from an interface:

```sh
gotest scaffold io.ReadCloser
```

This generates a generic suite with test method stubs for each method on the interface. You fill in the behavioral assertions; it handles the boilerplate. The generated suite follows the same naming conventions as any other gotest suite, so the code generator picks it up without additional configuration.

## When to use contract testing

Good fits:

- **Multiple implementations of the same interface** — storage backends, transport layers, serializers. If you have two or more implementations, a contract suite pays for itself immediately.
- **Plugin systems** — verify that third-party plugins honor the host interface. Ship the contract suite as part of your SDK so plugin authors can run it against their implementations.
- **Library interfaces** — if you define an interface for users to implement, provide a contract suite they can run. This is the clearest form of documentation: executable behavioral requirements.
- **Refactoring migrations** — verifying that a new implementation matches the old one's behavior. Run both through the same contract suite, and the spec output shows exactly where they diverge.

Not needed:

- **Single implementation** — just test it directly. A contract suite adds indirection without benefit when there is only one implementation.
- **Trivial interfaces** — a one-method interface rarely needs a full contract suite. The overhead of the generic machinery is not justified when the contract is a single function call.
- **Implementation-specific behavior** — use a regular suite for behaviors that do not belong to the contract. A contract suite should test what the interface promises, not what a particular implementation happens to do.

## When contract testing pays off

Interfaces are promises. Generic suites turn those promises into verified contracts. When you add a new implementation, you get a complete conformance report from the start. When you add a new contract requirement, every implementation is tested. The contract and the tests are the same artifact.

The pattern is not complicated. It is a generic struct, a type parameter constrained to an interface, and a set of type aliases. The rest is standard gotest: `BeforeEach` for setup, `When`/`It` for structure, standalone assertions for verification. The generator handles the wiring.

For more on the patterns that support this approach, see the [reference docs]({{< ref "/reference" >}}), [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}) for the setup side of contract suites, and [Advanced Go Test Fixtures]({{< ref "/blog/advanced-fixture-patterns" >}}) for when implementations need heavier infrastructure.
