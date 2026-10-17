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
