# Conformance suites

These suites are the behavior catalog of gotest in executable form. Every
`When`/`It` here is one sentence of the contract; `gotest spec --static` reads
the sentences from source and `make catalog` writes them to
`docs/design/catalog.md` and `docs/design/catalog.json`. A CI step fails when
the committed catalog differs from a fresh render, and a conventions suite
fails when any behavior is marked incomplete, so the catalog is always the
whole, current list.

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

## Quoting a clause in prose

`docs/design/spec.md`, `AGENTS.md`, `README.md` and the agent skill state a
behavior only by quoting a clause as a blockquote:

> when the version subcommand runs → exits 0 and prints the version on stdout

The text before the arrow is a `When` label as the catalog renders it (the
connective included) and the text after it is the `It` label directly under
it. A conventions suite looks every quote up in `docs/design/catalog.json`,
so a sentence that no longer matches a clause fails the build.

Only an `It` directly under a `When` is a quotable clause. The rows of an
`Each` table are pinned in prose by a link to their catalog section, never by
a quote. Nothing under a suite the `X_` prefix disables is a clause.

## Verdict clauses and the drill

A clause whose outcome is a verdict — an exit code, a pass or fail, a row of
the event stream — ships with a ring-0 twin in `tests/canary`: the same check
written with plain Go, no gotest assertion. Conformance suites assert with
gotest, so a bug in the assertion engine could pass its own test; the twin is
what the mutation drill holds the kernel to. The mutant's `Expect:` lines
match the twin's output, and its `Clause:` line records which catalog
sentence the twin protects.
