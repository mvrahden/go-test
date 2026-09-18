# Mutation drill

The drill is a test of the tests. gotest runs its own test suites, so a bug
in gotest's core could hide a failure from gotest itself. The checks that
close that loophole (the census, the ring-0 suites and the canary) are only
worth trusting if they demonstrably catch such bugs. `make drill` plants one
deliberate bug at a time and requires those checks to fail.
The checks themselves are described in `ARCHITECTURE.md` under "Testing gotest", and its Glossary defines the terms used here.

## What a run does

For every `mutants/<name>.patch`, `drill.sh`:

1. copies the repository to a scratch directory;
2. applies the patch, which introduces one bug in a core component, with
   exact context and no fuzz (a line offset is accepted: the context still
   matches verbatim);
3. builds the copy and compiles the test binaries of the drill packages;
4. runs the ring-0 test packages and the canary in the copy with
   `gotest summary`;
5. expects that run to exit non-zero, and every `Expect:` line of the patch
   to match a line of its log.

The `gotest` that judges each run is built from the unmodified tree before
any patch is applied. The copy's test binaries still link the copy's mutated
runtime and assertion code, and the canary inside the copy builds and tests
the copy's own CLI. This is what lets a bug in the CLI's exit-code path be
caught: a mutated CLI cannot be trusted to report on itself.

The drill exits 0 only when every mutant was caught. It runs in CI
(`quality.yml`) on every push and pull request.

## Reading a failure

- `DRILL  <name>: SURVIVED` — the bug went unnoticed. A check has a hole;
  the log tail shows what the run reported. Fix the check, not the mutant.
- `DRILL  <name>: patch does not apply, refresh it` — the code the mutant
  targets has changed. Re-create the patch against the current source.
- `DRILL  <name>: mutant does not compile, refresh it` — the patch applies
  but breaks the build; the judge would exit 2 on that and hide whether any
  check fired. Re-create the patch.
- `DRILL  <name>: caught by the wrong check` — the run went red, but not
  through the check the patch's `Expect:` lines name. The mutant may have
  landed somewhere else, or the named check lost its hold; the unmatched
  expectations and the first failures follow.
- `DRILL  <name>: no Expect: line` — the patch does not name its check.
- `DRILL  could not build the judge` — the unmodified tree does not compile.

## The mutants

| Patch | Bug it plants | Check that must catch it |
|---|---|---|
| `assert-always-passes` | `CheckEqual` reports success for every input | ring-0 assert suite: its raw checks see the wrong verdict string |
| `empty-always-true` | `IsEmpty` reports every value as empty | ring-0 predicates suite, and the canary's `Empty` row |
| `fail-is-noop` | `fail()` in `pkg/gotest` records nothing | canary: the fixtures that must fail stay green |
| `harness-drops-methods` | the suites template omits the first method of each suite | census: declared methods have no verdict, exercised by the canary's passing fixture |
| `bench-harness-drops-methods` | the bench template omits the first benchmark of each suite | bench census, exercised by the canary's benchmark fixture |
| `fuzz-harness-drops-target` | the fuzz template omits the first fuzz target of each suite | canary: the fuzzing fixture's golden list (the fixture is red, so the census stands down) |
| `exit-code-zero` | `WorstExitCode` always returns 0 | canary: exit codes differ from the golden list |
| `tree-fail-is-pass` | `BuildTree` classifies `fail` events as `pass` | ring-0 tree suite (the canary reads raw events, not the tree) |
| `it-skips-body` | `gotest.T.It` opens the subtest and never runs its closure | canary: the behavior rows under every `It` vanish from the golden list |

## Adding a mutant

1. Pick a bug that one of the checks claims to catch, confined to one file.
2. Make the change in a scratch copy of the repository and save it as a
   unified diff with `a/` and `b/` path prefixes:
   `diff -u a/<path> b/<path> > tests/drill/mutants/<name>.patch`.
3. Run the copy's checks and read the log. Put one `Expect: <ERE>` line per
   signal of the named check above the diff, each matching a line of the
   log with colors stripped (a census verdict, a ring-0 suite's FAIL line,
   a canary golden mismatch), not merely a failure.
4. Run `make drill`. The new mutant must be reported as caught; add it to
   the table above with the check its `Expect:` lines name.

A patch that stops applying exactly, stops compiling, or is caught by
another check fails the drill, so a mutant cannot silently stop testing
anything.
