# orders — Suite Lifecycle & FailFast

Demonstrates `BeforeAll` on the suite (shared state across test methods) and
`FailFast` configuration using an order placement and lifecycle workflow.

## Structure

- **orders.go** — Order store with place, confirm, and ship operations
- **suite_test.go** — `OrderRepositoryTestSuite` with `BeforeAll` and `SuiteConfig{FailFast: true}`

## Features

`BeforeAll` · `SuiteConfig` · `FailFast` · `Error` · `ErrorIs` · `NoError` · `True` · `Equal`
