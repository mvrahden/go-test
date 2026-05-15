# Parallel Suite

Method-level parallel execution.

Setting `SuiteConfig{Parallel: true}` runs each test method concurrently.
A returning `BeforeEach` provides per-test state isolation by yielding a
context value that is passed to the test method and `AfterEach`.
