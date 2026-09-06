# Canary

The canary checks gotest's verdicts from outside gotest. Every other test in
this repository reports its result through gotest's own runner, so a bug in
that runner could turn a failure into a pass without any test noticing. The
canary closes that gap: it builds the `gotest` binary, runs it over seven
small fixture packages written to fail in specific ways, and compares what
the binary reports with a checked-in expectation. It checks with plain Go
only, so nothing under test takes part in the verdict.

## Fixtures

Each directory under `testdata/` is one package with one suite:

| Fixture | Behavior it pins |
|---|---|
| `passing` | a green suite exits 0 and every method and behavior gets `pass` |
| `failing` | one failing assertion per assertion family, each reported as `fail` |
| `failnow` | a failed assertion halts the method; the statement after it never runs |
| `aftereach` | `AfterEach` runs even when the test failed |
| `lifecycle` | `BeforeAll`, the methods and `AfterAll` run in that order |
| `broken` | a package that does not compile exits 2 |
| `panicking` | a panic fails its method and the run is red |

`testdata/expected.txt` is the golden list. A `<fixture> exit <code>` line
gives the exit code; `<fixture> <action> <test>` lines list every `pass`,
`fail` or `skip` event the `-json` stream must carry, with `-` for the
package-level event.

Fixtures that must prove something ran (a teardown, an order) write a
marker file into the directory named by `GOTEST_CANARY_DIR`; the suite reads
it back after the run.

## Updating the golden list

Change `expected.txt` only when the behavior it pins has deliberately
changed. A failing check prints the rows it got and the rows it wanted side
by side; copy the new rows from there.
