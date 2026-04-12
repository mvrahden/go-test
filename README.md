# gotest

Structured test suites for Go with lifecycle hooks, nested scopes, and type-safe assertions. Works with `go test`.

```go
func TestOrderService(t *testing.T) {
    gotest.Run(t, func(s *gotest.S) {
        var db *Database

        s.BeforeAll(func(t *gotest.T) { db = setupDB() })
        s.AfterAll(func(t *gotest.T) { db.Close() })
        s.BeforeEach(func(t *gotest.T) { db.Truncate() })

        s.Test("creates an order", func(t *gotest.T) {
            order, err := CreateOrder(db, "item-1")
            gotest.NoError(t, err)
            gotest.Equal(t, "pending", order.Status)
        })

        s.Describe("with premium user", func(s *gotest.S) {
            s.BeforeEach(func(t *gotest.T) { db.SeedPremiumUser() })

            s.Test("applies discount", func(t *gotest.T) {
                order, _ := CreateOrder(db, "item-1")
                gotest.Greater(t, order.Discount, 0.0)
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
- **Type-safe assertions** -- `gotest.Equal(t, expected, actual)` with compile-time type checking
- **Fluent assertions** -- `t.Assert(v).Equal(x)`, `.Contains()`, `.HasLength()`, etc.
- **Parallel support** -- `TestParallel` calls `t.Parallel()` on the subtest
- **Standard tooling** -- works with `go test`, `-run`, `-race`, `-count`, IDE test runners

No code generation. No custom CLI.

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

Two styles, same behavior. All assertions stop the test on failure.

### Package-level functions (type-safe)

Generic type parameters enforce that both values share the same type at compile time:

```go
gotest.Equal(t, "pending", order.Status)  // T = string, compile-time checked
gotest.Equal(t, 42, count)                // T = int
gotest.NoError(t, err)
gotest.Greater(t, balance, 0.0)           // T = float64, must be cmp.Ordered
gotest.Contains(t, items, "needle")
gotest.Len(t, results, 3)
gotest.ElementsMatch(t, []int{3,1,2}, []int{1,2,3})
```

### Fluent assertions

`t.Assert(value)` returns an `AssertContext` for chained checks:

```go
t.Assert(true).IsTrue()
t.Assert(result).Equal(42)
t.Assert(items).HasLength(3)
t.Assert(items).Contains("needle")
t.Assert(body).Empty()
t.Assert(err).NoError()
```

### Full assertion reference

**Package-level (generic, type-safe):**

| Function | Checks |
|---|---|
| `Equal[T](t, expected, actual)` | `reflect.DeepEqual` with same-type constraint |
| `NotEqual[T](t, expected, actual)` | values differ |
| `Zero[T comparable](t, value)` | zero value for type (nil, 0, "", false) |
| `NotZero[T comparable](t, value)` | non-zero |
| `Empty(t, object)` | nil or zero-length container |
| `NotEmpty(t, object)` | has content |
| `True(t, value)` | `value == true` |
| `False(t, value)` | `value == false` |
| `NoError(t, err)` | `err == nil` |
| `Error(t, err)` | `err != nil` |
| `ErrorIs(t, err, target)` | `errors.Is(err, target)` |
| `ErrorAs[E](t, err) E` | `errors.As` with typed return |
| `ErrorContains(t, err, substr)` | error message contains substring |
| `Contains(t, collection, element)` | string/slice/map containment |
| `NotContains(t, collection, element)` | absence |
| `Len(t, object, n)` | `len(object) == n` |
| `ElementsMatch[T](t, a, b)` | same elements regardless of order |
| `Subset[T](t, list, subset)` | all subset elements in list |
| `Greater[T cmp.Ordered](t, a, b)` | `a > b` |
| `GreaterOrEqual[T](t, a, b)` | `a >= b` |
| `Less[T](t, a, b)` | `a < b` |
| `LessOrEqual[T](t, a, b)` | `a <= b` |
| `Regexp[P](t, pattern, str)` | string matches regex |
| `InDelta[T numeric](t, expected, actual, delta)` | within delta |
| `JSONEq(t, expected, actual)` | JSON equivalence ignoring key order |
| `Panics(t, fn) any` | function panics; returns recovered value |
| `Eventually(t, cond, waitFor, tick)` | condition becomes true within timeout |
| `TimeWithin(t, expected, actual, tolerance)` | times within tolerance |
| `TimeIsNow(t, ts, tolerance)` | timestamp near `time.Now()` |
| `Must[T](val, ok) T` | unwrap `(T, error)` or `(T, bool)` pairs |

**Fluent (`t.Assert(v)`):**

| Method | Checks |
|---|---|
| `.Equal(expected)` | deep equality |
| `.NotEqual(expected)` | inequality |
| `.IsTrue()` | boolean true |
| `.IsFalse()` | boolean false |
| `.IsZero()` | zero value |
| `.IsNotZero()` | non-zero |
| `.NoError()` | nil error |
| `.IsError()` | non-nil error |
| `.Empty()` | zero-length or nil |
| `.NotEmpty()` | has content |
| `.Contains(element)` | containment |
| `.NotContains(element)` | absence |
| `.HasLength(n)` | exact length |

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
t.Helper()
t.Errorf(format, args...)
t.FailNow()
```

## License

MIT
