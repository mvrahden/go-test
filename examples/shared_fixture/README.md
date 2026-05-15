# Shared Fixture

Cross-package fixture sharing.

Shared fixtures (`PostgresSharedFixture`, `RedisSharedFixture`) are defined
in a root package and consumed by suites in subpackages (`api/`, `web/`).
State is serialized to JSON between the CLI runner and each test binary.
`Hydrate`/`Dehydrate` hooks restore non-serializable resources (e.g. pool
handles) on the consumer side.
