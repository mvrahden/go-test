# Nested Fixture

Fixture composition.

Fixtures can embed other fixtures to form a hierarchy. `APIFixture` embeds
`InfraFixture`, so suites using `APIFixture` transitively access infra
state. Suites at different levels of the hierarchy share the underlying
fixture instances.
