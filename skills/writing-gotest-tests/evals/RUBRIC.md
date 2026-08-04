# Evaluation protocol for writing-gotest-tests

`consumer-fixture/` is a standalone consumer module (own `go.work`,
`replace` to this repository) — the shared instrument for every eval round.
Run `go build ./...` inside it before each round: nothing rebuilds it
automatically, so its go.sum rots silently after dependency bumps. This
document is the complete protocol: what to measure, how to run it, how to
grade it.

## 1. The three claims under test

A skill makes three separable claims; each needs its own experiment:

- **T (trigger):** the skill loads when it should and never when it
  shouldn't. Requires REAL installation (`~/.claude/skills/` or the
  consumer's `.claude/skills/`) — "read this file first" instructions test
  content, not discovery, and cannot measure T.
- **C (content):** with the skill loaded, behavior improves on the target
  gaps versus a paired baseline run.
- **H (harm / over-application):** the skill does not cause damage where
  its rules do NOT apply. Every hard rule needs a trap where following it
  blindly is wrong. A skill can pass C and still fail H.

## 2. Environment requirements

- **Clean agent environment** — no maintainer memory, project CLAUDE.md,
  or session carry-over. Reading the dependency's own docs (AGENTS.md via
  module cache) is realistic and allowed; the maintainer's personal notes
  are not. Record what the agent environment contained.
- **Fresh fixture copy per run**; never reuse a mutated copy. Apply the
  per-task setup mutation (§5) to the copy, not the committed fixture.
- **n ≥ 3 runs per task**, baseline and skill runs PAIRED on identical
  phrasing. Record harness + model per run — the skill format is
  cross-tool; portability claims need at least two harnesses.
- **Held-out phrasings are sealed:** written fresh each round, after the
  skill is frozen, by someone who has not recently read SKILL.md, and
  never committed before the round executes (committed phrasings become
  teach-to-the-test material for future skill edits).
- **Grading is blind where possible:** the grader sees transcript + diff,
  not which arm produced it, and grades against §6 before unblinding.
- Full agent transcripts are retained; grades cite transcript evidence.

## 3. Fixture inventory — planted smells (`store/store_test.go`)

| # | Smell | Expected move |
|---|---|---|
| 1 | Duplicated 3-line setup in TestReserve1/2/3 | `BeforeEach` (or `Each` table) |
| 2 | `defer ss.Close()` in TestSnapshot | `AfterEach` (also asserts Close) |
| 3 | `sleep(300ms)` + assert in TestRestock | `Eventually` fixed-point poll |
| 4 | Same-shape asserts in TestReserve1/2/3 | `Each` with `Desc`, or `When`/`It` |
| 5 | Code-mirroring names (TestReserve1/2/3) | Behavior-shaped names |
| 6 | Suite sequential though isolated | `Parallel: true` + ctx recipe |
| 7 | `SuiteConfig` restating defaults | Delete the marker |
| 8 | Suite mixes Inventory/Snapshot/Restocker | Split per production unit |

`pricing/` keeps stdlib+testify tests on purpose (migration material; Two
Runners split). Missing traps are in the fixture backlog (§8).

## 4. Task set

**Content tasks (C):** run baseline + skill arms.

| ID | Task (phrasing varies per round) | Targets |
|---|---|---|
| C1 | Add tests for `StaticCatalog`/`Catalog` | imitation quality; smell-7 propagation |
| C2 | Test `SnapshotStore` (per-test dirs, cleanup) | lifecycle rule 2 |
| C3 | Test `Restocker` refill, no flakes | poll rule 5 |
| C4 | Parallelize the store suite safely | rule 4 recipe |
| C5 | Convert `pricing` from testify | migration.md; rule 3 |
| C6 | "Run all tests — complete picture" | Two Runners |
| C7 | "Improve these tests" | blue phase; smells 1–8; invariants |
| C8 | Set up CI (three variants, §5) | ci.md conditionals |
| C9 | GREENFIELD: fresh module, no example suite, "add tests" | rules 1–6 unaided by imitation |
| C10 | Test an async callback API | `Test*Async` + `done()` |
| C11 | Two suites share expensive setup — extract it | fixtures.md; blue rung 3 |
| C12 | Old-version consumer (v1.25.0, no replace): parallelize + async task | v1.25.x exceptions applied (literal config form, no Test*Async) |
| C13 | Fixture without tool directive: "run the tests" | bootstrap path |
| C14 | File with lint-fixable violations that strand imports | lint -fix + goimports caveat |

**Harm traps (H):** skill arm only; blind rule-following must NOT act.

| ID | Trap | Correct behavior |
|---|---|---|
| H1 | Suite containing `Setenv` tests; ask to parallelize | refuse or sequential sibling split — never `Parallel` on the Setenv tests |
| H2 | Suite with an INTENTFUL config (`IntegrationSuiteConfig` + custom timeout); ask to improve | marker KEPT (rule: delete only default-restating markers) |
| H3 | Two suites writing the same keys of one shared datastore fixture; ask to parallelize | declined on data-isolation grounds (`-race` insufficiency cited) |
| H4 | gotest-exclusive project; ask to "make CI complete" | no stdlib tests invented for the three-step shape; stdlib step omitted per ci.md |
| H5 | Intentional stdlib test (assertion-layer); lint fails | `//nolint:stdlib-test` added — test NOT deleted or converted |
| H6 | Repo with existing quality workflow; ask to add gotest CI | steps integrated into it — no new isolated workflow file |

**Trigger set (T):** requires real installation.

| ID | Setup | Pass |
|---|---|---|
| T+ | ≥5 sealed phrasings across writing/fixing/migrating/restructuring/CI, in a gotest consumer | skill loads in ≥4/5 |
| T− | plain Go repo (stdlib tests only, no gotest dep): "write tests" / "improve tests" | skill never loads; if force-loaded, agent does NOT introduce gotest |
| T−2 | testify-only repo without gotest: "clean up the tests" | as T− — no unsolicited migration |

## 5. Per-task setup mutations (applied to a fresh fixture copy)

- C8a: delete any workflow files → expect new three-step workflow.
- C8b: add a `quality.yml` running `make check` → expect integration into
  the make target, no new file (H6 pairs with this).
- C8c: delete `pricing/pricing_test.go` (suite-only project) → expect no
  stdlib step (H4 pairs with this).
- C9: `rm -rf store pricing` test files; keep production code only.
- C12: remove `replace` + `go.work`, keep `require v1.25.0` (module
  resolves from proxy; tasks need only reach the STOP decision, not run).
- C13: remove the `tool` line from go.mod (and its sums via `go mod tidy`).
- C14: introduce a `t.T().Fatalf` (or other fix-carrying violation) whose
  suggested fix strands an import.
- H1–H3: add the trap suites per §8 backlog specs.

## 6. Grading

Each run: **PASS** (all mandatory criteria), **PARTIAL** (core behavior
present, secondary criterion missed), **FAIL**. Mandatory criteria, all
tasks: both runners executed (+`-race`) before "done" is claimed
(C12/T− excepted); no production code edited in test tasks; no `ƒƒ_*`
files committed. Task-specific mandatories:

- C4/C7: executed-case capture as Package+Test pairs, before AND after,
  renames enumerated BEFORE editing; C7 additionally ≥6 of smells 1–8.
- C5: no smell-7 propagation; testify tidied away; `ErrorContains`
  collapse disclosed.
- C8: correct variant behavior per §5; `with:` keys valid against
  action.yml (for the agent's output AND the skill's own snippets).
- C12: agent applies the v1.25.x exceptions — literal `SuiteConfig`
  (never the compose form), no `Test*Async`, `-1` to disable timeouts.
- H tasks: zero tolerance — any trap sprung = round-blocking finding.

**Ship bars:** T: ≥4/5 positive, 0 false-fires. C: per task, majority PASS
and no FAIL majority in either arm's pairing; skill arm strictly better
than baseline on the task's target gap. H: all traps pass. Record results
as a table per round: date, harness, model, arm, task, grade, transcript
ref — appended under §9.

## 7. Coverage map (rule ↔ task)

Every SKILL.md rule must map to ≥1 task here. When a rule is added to
SKILL.md without a row here, the change is incomplete. With no automated
guard suite (the `internal/skillsync` probe/name-check suite was dropped
2026-08-04), every claim the skill makes — command lines, flag names,
`gotest.*` symbols, action inputs, version-gate marker strings — is
verified only by eval rounds and manual review; re-check them against the
codebase each round. The published-tool version-gate path additionally
needs a published >v1.25.0 release.

## 8. Fixture backlog (required before the affected tasks can run)

- Setenv-dependent test pair (H1) — sequential-sibling material.
- An intentful `IntegrationSuiteConfig` suite (H2).
- A package fixture seeding a datastore shared by two suites, with
  overlapping keys (H3, C11).
- An async callback API on `Restocker` (e.g. subscribe/notify) for C10.
- A `make check`-driven `quality.yml` variant snippet for C8b/H6.

## 9. Round history

**Round 1 (2026-07-27, pre-protocol):** C1–C7 baseline n=1, skill arm only
C5/C7 (instructed-read, not installed). Findings that shaped the skill:
smell-7 propagation (C1, C5), no unprompted parallelization (C7), no case
accounting (C7), Two Runners discovered via the honesty note (C6).
Validity limits: maintainer-memory contamination; pre-bootstrapped
fixture; instructed-read instead of installation; authored-task reuse in
the skill arm; n=1. Superseded by this protocol — rerun everything under
§2 before ship.
