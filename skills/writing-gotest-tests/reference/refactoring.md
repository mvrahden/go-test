# Restructuring ladder (the blue phase)

Safety invariants live in SKILL.md and are non-negotiable. Order the work
so structure settles before names (renaming first means renaming twice):

1. **Cohesion & consolidation** — split suites whose name needs "And" or
   that mix subjects (a suite per production unit reads best in the spec
   view); merge trivial one-test suites orbiting one subject. Same
   assertions over different data → `gotest.Each` table with `Desc` fields,
   OR `t.When`/`t.It` blocks when each variant deserves its own clause
   (both render well). Divergent per-row assertion logic → separate tests.
2. **Lifecycle hoisting** — repeated setup → `BeforeEach`; teardown out of
   `defer` into `AfterEach` (also verifies the teardown's own error);
   expensive read-only state → write-once `BeforeAll`. Goroutine-owning
   resources belong in suite fields so `AfterEach` stops them even when an
   assertion aborts the test mid-way.
3. **Fixture extraction** — near-identical `BeforeAll`s across suites → a
   `*Fixture`; the same fixture rebuilt in several packages → a shared
   fixture; split god-fixtures (see `fixtures.md`).
4. **Parallelization** — the payoff of 2+3, and part of EVERY improvement
   pass (observed: agents skip it unprompted). Recipe and preconditions in
   SKILL.md rule 4; residue that cannot isolate (Setenv, shared writes)
   moves to a small sequential sibling suite.
5. **Spec-language pass** — subject-named suites; behavior-shaped method
   names; one behavior per `It` clause. The renderer strips
   `Test`/`TestSuite` affixes and shows the rest verbatim (no camel-case
   splitting) — sentence-level prose lives in `It`/`When`/`Desc` strings.
   Judge the result by rendering `gotest spec` ONCE at the end (it runs the
   full pipeline) or replay the invariant capture via `spec --input`.
6. **Hygiene** — delete default-restating `SuiteConfig` markers; sized
   timeouts (`IntegrationSuiteConfig` for integration suites); no
   `F_`/`X_` survivors; sleep+assert → `Eventually`; assert behavior, not
   unexported internals.

Smells → moves: setup×2 → BeforeEach · defer teardown → AfterEach · twin
BeforeAlls → Fixture · N-package fixture → shared fixture · same
asserts/diff data → Each or When/It · numbered names (Test1/Test2) →
behavior names · isolated sequential suite → Parallel · sleep+assert →
Eventually · "And"-named suite → split · per-implementation suite copies →
contract suite via scaffold · default-restating config → delete marker.
