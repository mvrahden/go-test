# inventory — Comparison Assertions

Demonstrates the full set of ordered comparison assertions using stock level management with reserve and restock operations. The suite runs its methods in parallel: `BeforeEach` returns a fresh stock level per test and `AfterEach` receives the same context back.

## Structure

- **inventory.go** — Stock level with available quantity, reserve, and restock
- **suite_test.go** — `InventoryTestSuite` (parallel) with returning `BeforeEach` and a context-taking `AfterEach`

## Features

`Less` · `Greater` · `LessOrEqual` · `GreaterOrEqual` · `NotEqual` · `Equal` · `True` · `False` · `SuiteConfig` with `Parallel` · returning `BeforeEach` · `AfterEach`
