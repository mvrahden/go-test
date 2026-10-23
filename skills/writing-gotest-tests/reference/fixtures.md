# Fixtures — the DAG

Package fixtures are structs named `*Fixture` with lifecycle hooks
`BeforeAll(ctx context.Context) error` / `AfterAll(ctx context.Context)
error` — note the signatures differ from suite hooks, which take
`(t *gotest.T)`. Suites reference fixtures as pointer fields; the generator
walks the type graph, wires the DAG, and orders setup/teardown. Fixtures may
depend on other fixtures (pointer fields again) — layer infrastructure
(container → migrations → seed data) instead of building god-fixtures, so
suites depend only on what they use.

A package fixture lives as long as the suite that binds it: set up after the
suite's `SuiteGuard` (v1.32+; before it below — see `versions.md`) and
before its `BeforeAll`, torn down after its `AfterAll` —
again for each `Fuzz*` method's seed replay. It is not shared across
suites; setup that must happen once per run belongs in a shared fixture. A
suite sets up only the fixtures it binds and their dependencies (v1.32+),
so an unrelated fixture never fails it.

Unlike a suite's, a fixture's `AfterAll` runs only after its `BeforeAll`
succeeded (or overran): a `BeforeAll` that fails midway must release what
it acquired before returning its error. When a fixture's setup fails, the
suite's own `BeforeAll` and `AfterAll` never run.

Package fixtures may also define per-test hooks — `BeforeEach(ctx
context.Context) error` / `AfterEach(ctx context.Context) error` — with the
same signatures as the All hooks. They wrap the suite's: fixture
`BeforeEach` → suite `BeforeEach` → test → suite `AfterEach` → fixture
`AfterEach`. A fixture's `AfterEach` runs only when every fixture
`BeforeEach` up to its own in dependency order returned without an error
(v1.34+; see `versions.md`), so a `BeforeEach` that returns an error must
release what it acquired before returning. A suite with `Fuzz*` methods
refuses fixtures that have them.

Shared fixtures (`*SharedFixture`) span packages: gotest sets them up once
and transfers state by JSON-serializing the fixture's EXPORTED FIELDS into
a state file each test process reads, so those fields must survive
`encoding/json`. A shared fixture has no `BeforeEach`/`AfterEach`: the
generator refuses one that declares them; put per-test work in a package
fixture that binds the shared one, or in the suite's hooks. Inside
a shared fixture, one pointer field per parent shared fixture TYPE: only
one instance of a parent ever exists, so a second field of the same type
is a generation error — keep one wired field and derive the other
reference in `BeforeAll`. `Hydrate(ctx) error` runs in each test process
to reconstruct live handles from that state; `Dehydrate(ctx) error` is
per-test-process CLEANUP (deferred), not a serializer; define both or
neither. Below v1.33 also define `AfterAll` on every shared fixture: one
without it passes generation and then fails to start. Serialize
connection INFO (addresses, DSNs), never live handles — reconstruct them
in `Hydrate` and keep them in fields assigned there (the "Hydrate-local"
pattern). Shared fixture types must live in a NON-internal package: the
setup subprocess cannot import `internal/` paths.

Timeouts: `FixtureConfig()` marker; a zero `Timeout` means the default
(see `config.md`). All shared fixture setup together is also capped by
`--setup-timeout` (default 2m), so a 5m `ContainerFixtureConfig()` needs
a larger one.
A DECLARED `Timeout` is a verdict, not advice: it bounds the hook's
context AND fails the fixture when `BeforeAll` or `AfterAll` returns after
it — even when the hook ignores its context. A hook that never returns is
not failed by it; other deadlines end it and fail the run: for a package
fixture the test binary's `-timeout` (gotest passes 10m unless you set
one), for a shared fixture `--setup-timeout` (default 2m) on `BeforeAll`
and a force-kill after its teardown budget on `AfterAll`. No marker →
context bounds only;
a fixture is never failed against a number its author did not write. A
setup that returns success only after overrunning its budget is not
retried (the work completed; a retry would build a second set of
resources) but its `AfterAll` still runs, because what it built exists.

Parallel suites + fixtures: a shared fixture means a shared live resource
across processes. `-race` cannot see two tests mutating the same rows.
Parallelize only with per-test keys/schemas/transactions or read-only use.

## Fixture windows (v1.27+)

Shared fixture start is never speculative: a fixture is resident exactly
while a scheduled suite that DECLARES it (as a pointer field, directly or
through the fixture DAG) needs it. Two consequences:

- A fixture no scheduled suite requires never starts — and can therefore
  never fail the run. Do not "test" a fixture by leaving it wired to
  nothing; nothing will execute it.
- A shared fixture you read without declaring may be absent: never
  started, or already torn down at the bulk→exclusive-tail barrier.
  Reaching one through a package-level variable or another struct
  compiles and then lies at run time. Declare a pointer field for every
  fixture a suite touches (or reach it through a declared fixture); the
  `shared-fixture-undeclared` lint rule (integrity tier) flags undeclared
  reads. Fixture self-tests that construct the fixture locally are
  exempt.
