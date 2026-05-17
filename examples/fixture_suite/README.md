# Fixture Suite

Fixtures with suite-scoped lifecycle.

A fixture is a struct with `BeforeAll`/`AfterAll` that runs once per suite.
Suites embed the fixture pointer to access its state.
`FixtureConfig()` returns a preset like `ContainerFixtureConfig()` to control timeouts and retries.
