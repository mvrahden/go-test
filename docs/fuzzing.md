# Fuzzing

Fuzz methods live on the same suites as tests — same struct, same lifecycle, own subcommand.

## Struct arguments

Go's fuzzing engine accepts exactly fifteen argument types — and the hand-written wire and message types you most want to fuzz are structs.
`f.Fuzz` accepts a struct — or any named type over a native one, like `type Priority int` — and codegen fans it out: one engine argument per leaf field, reassembled into your type before every execution.
The engine still sees only types it understands, but it sees the *fields*, so its mutator moves each one independently instead of chewing on one opaque blob.
The supported set is deliberately strict: structs of exported fields with simple shapes.
Maps, unexported fields (which excludes `time.Time` and protobuf-generated types), interfaces, and recursion are refused at generation time with a suggested alternative — the full table is below.

```go
func (s *UserServiceTestSuite) FuzzCreate(f *gotest.F) {
    f.Add(CreateUserRequest{Email: "a@b.c", Age: 30})
    f.Fuzz(func(t *gotest.T, req CreateUserRequest) {
        gotest.NoError(t, req.Validate())
    })
}
```

What fans out:

- `string`, `bool` and `[]byte` fields pass through unchanged.
- Numbers ride as fixed-width bytes, which gives the mutator its richest operators: bit flips, interesting values, boundary values. Why that beats a native number is in [ARCHITECTURE.md](../ARCHITECTURE.md), "Code Generation". A bare number argument (`func(t *gotest.T, n int)`) rides the same way, so its corpus entries on disk are bytes, not a readable number.
- Nested structs, pointers and small arrays flatten into the same tuple; a variable-length slice of non-bytes rides as one packed `[]byte`.
- Every position of a multi-argument callback fans on its own: `f.Fuzz(func(t *gotest.T, h Header, n int))` fans `Header` into its leaves and `n` into one leaf. Two values as arguments and two values in a struct reach the engine identically; use a struct when the values form a concept.

Seeds stay plain Go literals.
`f.Add` buffers them and `f.Fuzz` explodes each one through the target's own fan; a seed of the wrong type is rejected with the position named (`seed #1: value 1: f.Add was given []byte, but this fuzz target takes CreateUserRequest`).

Crashers stay readable.
A failing execution prints the decoded value beside the failure, `CreateUserRequest{Email: "a@\x00", Age: -1}` rather than corpus bytes, and `gotest fuzz promote` splices that literal back into the method as a typed `f.Add(...)` seed.
Every shape the fan accepts renders as a literal, pointers included (`&[]int{5}[0]`, since `&5` is not Go and `new(5)` needs Go 1.26).

**A struct target's corpus files are bound to its field order.**
Fields map to leaves in declaration order, so an entry on disk means whatever the current field list says:

- Adding or removing a field changes the leaf count. Go's engine rejects every old entry, and gotest names the drift first: a pre-flight check on plain runs and on `gotest fuzz` prints which entry no longer fits, what it holds and what the target now takes.
- Swapping two fields of the same kind is silent: the entry loads and quietly becomes a different test.

The rule that follows: keep crashers of struct targets as `f.Add` seeds, never as committed corpus files. `gotest fuzz promote` writes the seed for each crasher the latest `gotest fuzz` session wrote; an older corpus file is deleted, or its input written as a typed `f.Add` seed by hand.
A promoted seed is source and survives any change to the encoding.
The `fuzz-struct-corpus` lint rule flags on-disk entries for a shape-bound target: delete them or write them as typed seeds.

Refused at generation time, with the alternative named in the message, because generated code that cannot round-trip faithfully would lie:

| Refused | Do this instead |
|---|---|
| structs with unexported fields | fuzz the constructor's input, or declare a local wrapper struct |
| `map` fields | fuzz a slice of key/value pairs and build the map in the callback |
| interfaces, channels, funcs | — |
| recursive types | — |
| `time.Time` and friends (unexported internals) | fuzz an `int64` and convert in the callback |

Each rejection names the field path and its type, e.g. `fuzz target FuzzUserServiceTestSuite_FuzzCreate: CreateUserRequest.mu (sync.Mutex) is not fuzzable — unexported fields cannot be set — fuzz the constructor's input, or declare a local wrapper struct`.
Generation is the only place this can be caught: `go vet` checks direct `(*testing.F).Fuzz` calls only, and `f.Fuzz` takes `any`.

## Seed harvesting

At generation time, gotest mines each `Fuzz*` method's package for literal primitive values that already flow into the function under fuzz — table-test rows (`gotest.Each`) and direct call-site literals in `_test.go` files — and injects them as additional `f.Add(...)` seeds in the generated wrapper, on top of whatever you added by hand.
Your existing valid-input examples become a starting corpus for free — coverage-guided mutation from real inputs finds interesting states far faster than starting from `""`.

Harvesting only scans test files, never production code, and only lifts literal shapes — basic literals, `true`/`false`, a unary-minus of one, or a single-arg conversion of one — never computed expressions.
It's on by default; disable it for one run with `--no-harvest`, or persistently with `fuzz: harvest: false` in `.gotest.yml` (see [Project Configuration](../README.md#project-configuration)).

## Seed replay

```bash
gotest ./...
```

```
=== RUN   FuzzParserTestSuite_FuzzParse
=== RUN   FuzzParserTestSuite_FuzzParse/seed#0
--- PASS: FuzzParserTestSuite_FuzzParse (0.00s)
    --- PASS: FuzzParserTestSuite_FuzzParse/seed#0 (0.00s)
```

Every seed added via `f.Add`, plus any crasher corpus already discovered under `testdata/fuzz/`, replays as an ordinary subtest under a plain run — `gotest ./...`, `gotest spec`, `gotest watch`, and `gotest summary` all do this, at zero extra cost, with no `-fuzz` flag involved.
For a **pass-through** target, committing a crasher's corpus file makes it a permanent regression test; for a **shape-bound** target, run `gotest fuzz promote` after the session that found it instead — its corpus files are bound to the type's field order (the `fuzz-struct-corpus` lint rule will remind you), while a promoted `f.Add` literal is ordinary Go source.
A user-supplied `-run` filter wins over this widening — fuzz seeds only replay if a generated `Fuzz<Suite>_<Method>` name matches the filter on its own merit. `-run '^TestParserTestSuite$'` therefore runs the suite's tests and replays none of its seeds; name the wrapper too, or drop the anchors ([A -run pattern in each suite process](../specs/cli/spec.md#requirement-a--run-pattern-in-each-suite-process)).

In the spec view the target sits under its suite as a property beside the examples — `✓ Parse  2 seeds` — and opens up to name the entry when a seed or corpus file fails; the trailer counts it as a fuzz target, never as a stdlib test.
`gotest spec --static` lists fuzz targets the same way, read from source, so the declared specification states the properties too; on the wire the node's `kind` is `fuzz`.

One caveat to state plainly: suite fuzz methods exist only through gotest's generated wrappers.
Plain `go test` (and external fuzzing infrastructure like OSS-Fuzz, which needs on-disk `FuzzXxx` functions) never sees them, and promoted seeds replay only under gotest runs.
If a target must also run under stock tooling, write it as a top-level `func FuzzXxx(*testing.F)` — `gotest.NewF(f, nil, nil, nil)` and `f.Fuzz` work there too, as plain library calls bound by reflection, though with native argument types only: fans exist solely in generated suite wrappers.

## Running

```bash
gotest fuzz ./...                     # one minute, shared across all targets
gotest fuzz --for=5m ./...            # about five minutes across all targets
gotest fuzz --for=1m --jobs=2 ./...   # two targets at a time
gotest fuzz --target=FuzzParserTestSuite_FuzzParse ./pkg/parser
```

`gotest fuzz` finds every generated `Fuzz<Suite>_<Method>` target and runs each as its own `go test -fuzz=...` process.
This is the one gotest subcommand that does not reuse the compiled suite binary: `cmd/go` weaves fuzz instrumentation in only when `-fuzz` is present on that `go test` invocation, so a binary from `go test -c` would fuzz without coverage guidance.
Each target pays its own compile and gets a real search in return.

The budget:

- `--for` is the session's wall clock, one minute by default. Each target's `-fuzztime` share is `--for × min(--jobs, targets) / targets`, so concurrent waves add back up to about `--for`. Shares never drop below 10s; a small `--for` on many targets stretches the session instead, and gotest says so. The resolved schedule and the deadline it implies print before the search starts.
- `--for` is the only clock. The deadline follows it with headroom for builds. `gotest fuzz` ignores the `timeout` and `setup-timeout` keys of `.gotest.yml`, which the other commands take, and refuses `--timeout` given as a flag rather than let two clocks compete ([Config defaults a command cannot take](../specs/cli/spec.md#requirement-config-defaults-a-command-cannot-take)).
- `--for=0` removes the budget: targets fuzz until interrupted. Only the first `--jobs` targets ever run then, since each holds its slot, and gotest says so up front.
- `--jobs` caps concurrent targets (default `max(1, GOMAXPROCS/2)`). A target whose slot never opens before the deadline is reported as `[<Func>] skipped: session ended before this target started`, and the closing summary names the targets that did not get their full share.
- `--target=<Fuzz...>` narrows a session to one wrapper; an unmatched name lists the available ones, and `-run`, `-skip` and `-bench` are refused in its favour. This is what the VS Code extension invokes: fuzz methods get **Fuzz** / **Debug Seeds** CodeLenses, budgeted cancellable sessions, crasher notifications wired to triage/promote/debug, and Test Explorer items whose runs replay seeds (see `vscode-gotest/README.md`).

Ending by deadline or interrupt is the normal end of an open-ended search, not a failure.
The session exits 0 unless something was found: a failing target, or a new crasher file, detected by comparing each target's `testdata/fuzz/<Func>/` directory before and after, so a crash counts even when the deadline killed the process mid-report.

Output streams live, line by line, each line prefixed `[<Func>] `:

```
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 0s, gathering baseline coverage: 0/76 completed
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 0s, gathering baseline coverage: 76/76 completed, now fuzzing with 6 workers
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 3s, execs: 393973 (131314/sec), new interesting: 0 (total: 76)
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 10s, execs: 1407280 (131111/sec), new interesting: 1 (total: 77)
[FuzzNotificationServiceTestSuite_FuzzTrim] PASS
[FuzzNotificationServiceTestSuite_FuzzTrim] ok  	github.com/mvrahden/go-test/examples/notification	10.115s
```

On a crashing input the session exits 1 and names each new corpus file, `[<Func>] new crasher: <dir>/testdata/fuzz/<Func>/<hash>`, followed by the two commands that handle it: `gotest fuzz triage` to see the decoded input and `gotest fuzz promote` to keep it as a typed seed that replays on every ordinary run.
A target that fails without writing a new file (a failing seed or existing corpus entry) is called out as such; it reproduces on a regular `gotest` run.

Every session closes with one line, `fuzzed 5 targets in 20.3s: 7,012,345 execs, 3 new interesting inputs, no crashers`.
Under GitHub Actions the same session lands in the step summary as a table with one row per target.

"Interesting" inputs are the ones that reached new coverage.
Go keeps them in its build cache, under `fuzz/<package>/<Func>/` in `go env GOCACHE`, never beside your code.
The next session resumes from them, which is why a second run starts with a larger baseline; `go clean -fuzzcache` starts over.
They are the search's own memory, not something to commit: the seeds you write and the crashers you promote are the corpus that travels with the code.
The engine's `To re-run: go test -run=...` advice is dropped from the stream, since plain `go test` cannot see a generated target; the hint after a crasher names the working commands.

With no `Fuzz*` methods in the matched packages, `gotest fuzz` prints `no fuzz targets found` and exits 0 without invoking `go test`; patterns that match no package at all exit 2.

## Triage and promote

```bash
gotest fuzz triage ./...     # re-run every discovered crasher, report pass/fail
gotest fuzz promote ./...    # splice crashers into f.Add(...) seeds
```

`gotest fuzz triage` scans each target's `testdata/fuzz/<Func>/` directory (a plain filesystem scan — no `go test -fuzz` invoked) and re-runs every corpus entry found there via `go test -run='^<Func>/<hash>$'`, printing its decoded input and either the panic/failure cause or `status: no longer failing`. A re-run that produced no exit status of its own — it was terminated, or never started — reports `status: unverified` and fails the triage: nothing was proven about that crasher.
Exits 1 if any crasher's re-run still fails, 0 otherwise.

`gotest fuzz promote` does the same discovery, then splices each crasher the latest `gotest fuzz` session of its target wrote, with its exact values, into its originating `Fuzz*` method as a permanent `f.Add(...)` seed — an AST-level source edit via `internal/refactor`, inserted after the method's last existing `f.Add` call — and deletes the crasher file once the splice succeeds.
The crasher is now a committed regression test that replays for free on every ordinary run, same as any seed under [Seed replay](#seed-replay).
A crasher whose originating method can't be located with confidence is skipped with a warning and left in place; promote never partially edits source.
Which files it takes and what it refuses is stated in [the promote requirements](../specs/fuzz/spec.md#requirement-promote-takes-corpus-literals-only).

Struct-typed targets work the same way: triage prints the decoded struct on the `input:` line, and promote splices it as a typed `f.Add(T{...})` seed rather than an opaque byte string.
See [Struct arguments](#struct-arguments) for the shapes that carry a literal and the raw-bytes fallback for those that don't.

## Known limitation

Top-level stdlib fuzz functions — a bare `func FuzzXxx(f *testing.F)` not attached to any suite — are not discovered or run by the gotest runner at all.
They only run via `go test -run`/`go test -fuzz` directly; `gotest ./...`, `gotest fuzz`, and every other gotest subcommand silently ignore them.
Only `Fuzz*` methods on `*TestSuite` structs participate in seed replay and the `gotest fuzz` orchestrator.

## Worked example

[`examples/fuzzing`](../examples/fuzzing) fuzzes a message broker's wire-frame codec: a struct-typed round-trip target, a `[]byte` target asserting the decoder never panics on adversarial input, a string idempotence target, and two two-argument targets — one over two strings, one mixing a struct with a string.
Its README records a real crash-to-regression-test cycle — the fuzzer finding a framing bug, `triage` decoding it, `promote` splicing it back as a typed seed.
