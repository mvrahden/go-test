# datarace — Race Conditions, Panics & Crash Diagnostics

Demonstrates how the test runner surfaces runtime failures: data races detected by `-race`, panics inside test methods, and panics inside `BeforeEach` fixtures.

## Structure

- **counter.go** — Unsynchronized `Counter`, mutex-guarded `SafeCounter`, panic-triggering helpers
- **suite_test.go** — `CounterTestSuite` (data race), `SafeCounterTestSuite` (safe variant), `PanicTestSuite` (in-test panic), `FixturePanicTestSuite` (fixture panic)

## Features

`-race` diagnostics · `BeforeEach` · `sync.Mutex` · `sync.WaitGroup` · panic stack traces
