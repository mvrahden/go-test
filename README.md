# gotest

Structured test suites for Go with lifecycle hooks, nested scopes, and fluent assertions. Works with `go test`. Zero dependencies.

```go
func TestOrderService(t *testing.T) {
    gotest.Run(t, func(s *gotest.S) {
        var db *Database

        s.BeforeAll(func(t *gotest.T) { db = setupDB() })
        s.AfterAll(func(t *gotest.T) { db.Close() })
        s.BeforeEach(func(t *gotest.T) { db.Truncate() })

        s.Test("creates an order", func(t *gotest.T) {
            order, err := CreateOrder(db, "item-1")
            t.Assert(err).IsNil()
            t.Assert(order.Status).Equals("pending")
        })

        s.Describe("with premium user", func(s *gotest.S) {
            s.BeforeEach(func(t *gotest.T) { db.SeedPremiumUser() })

            s.Test("applies discount", func(t *gotest.T) {
                order, _ := CreateOrder(db, "item-1")
                t.Assert(order.Discount > 0).IsTrue()
            })
        })
    })
}
```

```
go test ./...
```

## Install

```
go get github.com/mvrahden/go-test/gotest
```

Requires Go 1.23+.

## Why

Go's `testing` package has no concept of test suites. If you need shared setup/teardown across a group of tests, you end up with either boilerplate `TestMain` + global state, or runtime reflection libraries that lose type safety.

`gotest` gives you:

- **Lifecycle hooks** -- `BeforeAll`, `AfterAll`, `BeforeEach`, `AfterEach`
- **Nested scopes** -- `Describe` blocks with inherited hooks
- **Focus/skip** -- `FTest`/`XTest` for development, `FDescribe`/`XDescribe` for groups
- **Fluent assertions** -- `t.Assert(v).Equals(x)`, `IsNil()`, `Contains()`, etc.
- **Parallel support** -- `TestParallel` calls `t.Parallel()` on the subtest
- **Standard tooling** -- works with `go test`, `-run`, `-race`, `-count`, IDE test runners

No code generation. No custom CLI. No external dependencies.

## Lifecycle

```
BeforeAll    -- once, before any test in this scope
  BeforeEach -- before each test
    Test
  AfterEach  -- after each test (via defer, guaranteed even on failure)
AfterAll     -- once, after all tests complete
```

Hooks are registered as closures. Shared state lives in variables scoped to the `Run` callback:

```go
gotest.Run(t, func(s *gotest.S) {
    var conn *grpc.ClientConn

    s.BeforeAll(func(t *gotest.T) {
        conn, _ = grpc.Dial("localhost:50051")
    })
    s.AfterAll(func(t *gotest.T) {
        conn.Close()
    })

    s.Test("ping", func(t *gotest.T) {
        // conn is available here via closure
    })
})
```

## Nesting

`Describe` creates a child scope. Child scopes inherit parent `BeforeEach`/`AfterEach` hooks. Parent hooks run first; cleanup unwinds in reverse:

```go
gotest.Run(t, func(s *gotest.S) {
    s.BeforeEach(func(t *gotest.T) { /* parent setup */ })

    s.Describe("admin users", func(s *gotest.S) {
        s.BeforeEach(func(t *gotest.T) { /* child setup */ })

        s.Test("can delete", func(t *gotest.T) {
            // execution order: parent BeforeEach -> child BeforeEach -> test
            //                  -> child AfterEach -> parent AfterEach
        })
    })
})
```

Nesting maps to `t.Run` subtests, so `go test -run` works naturally:

```
TestOrderService/creates_an_order
TestOrderService/with_premium_user/applies_discount
```

## Focus and Exclude

During development, prefix with `F` to focus or `X` to exclude:

```go
s.FTest("only this runs", func(t *gotest.T) { ... })
s.XTest("this is skipped", func(t *gotest.T) { ... })
s.FDescribe("only this group", func(s *gotest.S) { ... })
s.XDescribe("skip this group", func(s *gotest.S) { ... })
```

When any item is focused, everything else in that scope is skipped. Excluded items are always skipped. Remove the `F`/`X` prefix before committing.

## Assertions

`t.Assert(value)` returns a fluent context:

| Method | Checks |
|---|---|
| `.IsTrue()` | `value == true` |
| `.IsFalse()` | `value == false` |
| `.Equals(x)` | `reflect.DeepEqual(value, x)` |
| `.IsNil()` | `value` is nil (handles typed nils) |
| `.IsNotNil()` | `value` is not nil |
| `.IsZero()` | `value` is the zero value for its type |
| `.HasLength(n)` | `len(value) == n` (string, slice, map, array, channel) |
| `.IsEmpty()` | `len(value) == 0` |
| `.Contains(x)` | substring (string) or element (slice/array) |

Assertion failures use `t.Errorf` -- they report the failure and continue, allowing multiple assertions per test. Access `t.T()` for the underlying `*testing.T` when you need `Fatal`, `Skip`, or other stdlib methods.

## Parallel Tests

```go
s.TestParallel("fast check", func(t *gotest.T) {
    // runs with t.Parallel()
})
```

`BeforeEach` hooks run before the parallel barrier, so setup completes before the test is released to run concurrently.

## API

```go
// Entry point
gotest.Run(t *testing.T, fn func(*S))

// Suite builder (S)
s.BeforeAll(fn func(*T))
s.AfterAll(fn func(*T))
s.BeforeEach(fn func(*T))
s.AfterEach(fn func(*T))
s.Test(name string, fn func(*T))
s.TestParallel(name string, fn func(*T))
s.Describe(name string, fn func(*S))
s.FTest / s.XTest           // focused / excluded test
s.FDescribe / s.XDescribe   // focused / excluded group

// Test context (T)
t.T() *testing.T             // underlying stdlib type
t.Assert(v any) *AssertContext
t.It(name string, fn func(*T))
```

## License

MIT
