# cart — Shopping Cart Operations

Demonstrates basic assertions and error handling using a shopping cart with add, remove, discount, and checkout operations. Both suites run their methods in parallel: `BeforeEach` returns a fresh cart per test instead of storing one on the suite struct.

## Structure

- **cart.go** — Cart with catalog pricing, quantity tracking, and discount logic
- **suite_test.go** — `ShoppingCartTestSuite` (parallel, returning `BeforeEach`) with 9 test methods
- **suite_ext_test.go** — `ShoppingCartTestSuite` (external package variant, same shape)

## Features

`Equal` · `Contains` · `Empty` · `InDelta` · `GreaterOrEqual` · `ErrorIs` · `NoError` · `Len` · `SuiteConfig` with `Parallel` · returning `BeforeEach`
