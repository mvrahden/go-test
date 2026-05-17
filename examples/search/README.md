# search — Generics, Parallel Execution & Collection Assertions

Demonstrates generic test suites with type alias instantiation, parallel test
execution, returning `BeforeEach` for per-test isolation, and collection assertions.

## Structure

- **index.go** — Generic `Indexable` interface, `Article`/`Product` types, full-text search index
- **suite_test.go** — `ArticleSearchTestSuite` (parallel, returning `BeforeEach`), `IndexContractTestSuite[T]` (generic), type aliases `ArticleIndexTestSuite` and `ProductIndexTestSuite`
- **suite_ext_test.go** — `SearchResultTestSuite` (external package variant)

## Features

`SuiteConfig{Parallel: true}` · returning `BeforeEach` · generic suites · type alias instantiation · `ElementsMatch` · `Subset` · `Zero` · `NotZero` · `NotContains`
