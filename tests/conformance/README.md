# Conformance suites

These suites are the behavior catalog of gotest in executable form. Every
`When`/`It` here is one sentence of the contract; `gotest spec --static` reads
the sentences from source and `make catalog` indexes them in
`docs/design/catalog.json`. A CI step fails when the committed index differs
from a fresh render, and a conventions suite fails when any behavior is marked
incomplete, so the index is always the whole, current list.

## Naming

- One package per catalog section: `cli`, `lifecycle`, `fixtures`,
  `discovery`, `assertions`, `bench`, `fuzz`, `spec`, `lint`, `migrate`.
- Suite and method names mirror the headings of `docs/design/spec.md`
  (`CLIFlagsTestSuite` under "CLI Interface › Flags").
- `When` names the branch, not the connective: `t.When("the run is stopped")`.
- `It` names one observable: a verdict, an event, an output or a resource
  fact, with the numbers it pins in the text.
- Input universes are `Each` tables whose `Desc` is the cell name; literal
  tables only, so the static reading lists every row.

## Oracles

`internal/testkit` runs the real CLI: `RunPackage` over a testdata package,
`RunCLI` for a bare invocation, `Run.Verdict` for a test's last verdict and
`Run.MarkerOrder` for the order of markers a fixture stamped into
`GOTEST_CANARY_DIR/markers`. Testdata packages live under
`tests/conformance/testdata`.

## Specifying a behavior

A behavior is stated once, in `specs/<capability>/spec.md`, and written in
this order:

1. **Spec first.** Add a scenario under its requirement in the spec. The
   binding suite in `tests/conventions` goes red: the scenario has no clause.
2. **Then the clause.** Write the conformance `When`/`It` whose labels equal
   the scenario's WHEN text and each of its THEN and AND texts. A clause is an
   `It` directly under a `When`; the rows of an `Each` table are not clauses,
   and nothing under a suite the `X_` prefix disables is one.
3. **Then the behavior.** Make the clause pass.

Guides (`README.md`, `AGENTS.md`, `docs/`, the agent skill) link the
requirement, for example
[the version requirement](../../specs/cli/spec.md#requirement-version), and
never restate it. Rewording a promise changes the spec, the `When`/`It` label
and any drill `Clause:` line in one commit.

## Verdict clauses and the drill

A ring-0 twin in `tests/canary` — the same check written with plain Go, no
gotest assertion — is required exactly when the mutant lives in what a test
binary links (`pkg/gotest`, `pkg/gotestruntime`, `internal/gotestgen`).
Conformance suites assert with that code, so a mutant there could pass its own
test; the drill then judges without the conformance suites, and the mutant's
`Expect:` lines match the twin's output. Any other clause is judged by its own
conformance suite: the drill adds the packages whose specs state the patch's
`Clause:` lines, and the `Expect:` line is that It's failure line as the judge
prints it.
