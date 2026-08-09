# Fixtures — the DAG

Package fixtures are structs named `*Fixture` with lifecycle hooks
`BeforeAll(ctx context.Context) error` / `AfterAll(ctx context.Context)
error` — note the signatures differ from suite hooks, which take
`(t *gotest.T)`. Suites reference fixtures as pointer fields; the generator
walks the type graph, wires the DAG, and orders setup/teardown. Fixtures may
depend on other fixtures (pointer fields again) — layer infrastructure
(container → migrations → seed data) instead of building god-fixtures, so
suites depend only on what they use.

Package fixtures may also define per-test hooks — `BeforeEach(ctx
context.Context) error` / `AfterEach(ctx context.Context) error` — with the
same signatures as the All hooks.

Shared fixtures (`*SharedFixture`) span packages: gotest sets them up once
and transfers state by JSON-serializing the fixture's EXPORTED FIELDS into
a state file each test process reads. One pointer field per parent shared
fixture TYPE: only one instance of a parent ever exists, so a second field
of the same type is a generation error (it used to leave the first field
nil) — keep one wired field and derive the other reference in `BeforeAll`. `Hydrate(ctx) error` runs in each
test process to reconstruct live handles from that state; `Dehydrate(ctx)
error` is per-test-process CLEANUP (deferred), not a serializer. Serialize
connection INFO (addresses, DSNs), never live handles — reconstruct them
in `Hydrate` and keep them in fields assigned there (the "Hydrate-local"
pattern). Shared fixture types must live in a NON-internal package: the
setup subprocess cannot import `internal/` paths.

Timeouts: `FixtureConfig()` marker, literal semantics (see `config.md`).
A DECLARED `Timeout` is a verdict, not advice: it bounds the hook's
context AND fails the fixture when `BeforeAll` or `AfterAll` outlives it —
even when the hook ignores its context. No marker → context bounds only;
a fixture is never failed against a number its author did not write. A
setup that returns success only after overrunning its budget is not
retried (the work completed; a retry would build a second set of
resources) but its `AfterAll` still runs, because what it built exists.

Parallel suites + fixtures: a shared fixture means a shared live resource
across processes. `-race` cannot see two tests mutating the same rows.
Parallelize only with per-test keys/schemas/transactions or read-only use.
