# Conformance suites

These suites verify [`specs/`](../../specs/README.md), where gotest's behavior
is stated. A `When` label and an `It` label directly under it are the key that
binds a test to a scenario: they equal the scenario's WHEN and one of its THEN
or AND texts. `gotest scenarios` binds them from source, the same way it binds
any module's specs, so nothing is generated or committed. The binding suite
in `tests/conventions` (`make scenarios`) runs it over `specs/` and
`./tests/conformance/...` and adds this repository's rules: a scenario binds
in its own capability's package only, the labels equal the spec's text as
written, every `When` holding an `It` is some scenario's WHEN, and no behavior
is one the static reading could not list.

## Naming

- One package per capability in `specs/`: `tests/conformance/<capability>`
  verifies `specs/<capability>/spec.md` (`tests/conformance/cli` verifies
  `specs/cli/spec.md`).
- A suite groups requirements of its capability (`CLIFlagsTestSuite` in
  `cli`) and a method verifies scenarios of one requirement. Names bind
  nothing; the labels do.
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
   and nothing under a suite the `X_` prefix disables is one. `make scenarios`
   goes green; `go run ./cmd/gotest scenarios --unstated ./tests/conformance/...`
   lists what is still unbound, with the clause at fault.
3. **Then the behavior.** Make the clause pass, and give the clause a drill
   mutant with a `Clause:` line (`tests/drill/README.md`).

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
