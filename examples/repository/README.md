# repository — Fixtures & Integration Testing

Demonstrates the fixture system and integration test configuration using a user repository backed by a `DatabaseFixture` with full lifecycle management.

## Structure

- **repository.go** — User type and repository operations
- **fixtures.go** — `DatabaseFixture` with `BeforeAll`/`AfterAll`/`BeforeEach`/`AfterEach` and `FixtureConfig`
- **suite_test.go** — `UserRepositoryTestSuite` bound to `DatabaseFixture`
- **suite_ext_test.go** — `UserRepositoryTestSuite` (external package variant)
- **orders/** — Sub-package: order lifecycle with `BeforeAll` on suite and `FailFast`
- **inventory/** — Sub-package: stock management with comparison assertions

## Features

`SuiteConfig` · `IntegrationSuiteConfig` · `FixtureConfig` · fixture `BeforeAll`/`AfterAll`/`BeforeEach`/`AfterEach` · `FailFast` · `True` · `False`
