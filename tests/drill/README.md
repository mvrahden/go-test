# Mutation drill

The drill is a test of the tests. gotest runs its own test suites, so a bug
in gotest's core could hide a failure from gotest itself. The checks that
close that loophole (the census, the ring-0 suites) are only
worth trusting if they demonstrably catch such bugs. `make drill` plants one
deliberate bug at a time and requires those checks to fail.

## What a run does

For every `mutants/<name>.patch`, `drill.sh`:

1. copies the repository to a scratch directory;
2. applies the patch, which introduces one bug in a core component;
3. runs the ring-0 test packages in the copy with
   `gotest summary`;
4. expects that run to exit non-zero.

Each run uses `go run ./cmd/gotest` from inside the copy, so the CLI,
runtime and assertion code under judgement are the copy's own.

The drill exits 0 only when every mutant was caught. It runs in CI
(`quality.yml`) on every push and pull request.

## Reading a failure

- `DRILL  <name>: SURVIVED` — the bug went unnoticed. A check has a hole;
  the log tail shows what the run reported. Fix the check, not the mutant.
- `DRILL  <name>: patch does not apply, refresh it` — the code the mutant
  targets has changed. Re-create the patch against the current source.

## The mutants

| Patch | Bug it plants | Check that must catch it |
|---|---|---|
| `harness-drops-methods` | the suites template omits every other method | census: declared methods have no verdict |

## Adding a mutant

1. Pick a bug that one of the checks claims to catch, confined to one file.
2. Make the change in a scratch copy of the repository and save it as a
   unified diff with `a/` and `b/` path prefixes:
   `diff -u a/<path> b/<path> > tests/drill/mutants/<name>.patch`.
3. Run `make drill`. The new mutant must be reported as caught; add it to
   the table above with the check that caught it.

A patch that stops applying fails the drill, so a mutant cannot silently
stop testing anything.
