# Fuzzing (v1.29+)

A fuzz target is a `Fuzz*` method on the suite, so it shares the suite's
struct and `BeforeEach`/`AfterEach` (interposed around every execution,
seeds included). It takes `*gotest.F` only; `*testing.F` is rejected.

```go
func (s *ParserTestSuite) FuzzParse(f *gotest.F) {
	f.Add(`{"a":1}`)                      // typed seeds, before Fuzz
	f.Fuzz(func(t *gotest.T, in string) { // any arity after the *gotest.T
		doc, err := s.p.Parse(in)
		if err != nil {
			return // rejecting invalid input is fine
		}
		out, err := doc.Marshal()
		gotest.NoError(t, err)
		gotest.JSONEq(t, in, out)     // the property
	})
}
```

Rules the linter enforces (`gotest lint`): assert a property
(`fuzz-no-oracle`); add at least one seed (`fuzz-seed`); read no
`time.Now`/`math/rand`/`os.Getenv` in the target (`fuzz-determinism`); no
IO in the suite's per-execution hooks (`fuzz-hook-io`); typed seeds, never
raw `[]byte` on a non-byte position (`fuzz-raw-seed`); no on-disk corpus
entries for a struct-typed target (`fuzz-struct-corpus`).

## Arguments

Native engine types pass through. A struct, a named type over a native
one, a pointer, a fixed-size array or a slice fans out at generation time
into one engine argument per leaf field and is reassembled before each
execution, so the mutator works field by field. Refused at generation
time, with the alternative named: unexported fields (fuzz the constructor's
input or a local wrapper struct), maps (a slice of pairs), interfaces,
channels, funcs, recursive types, `time.Time` (an `int64`). Two values as
arguments and two values in a struct fuzz identically; choose what reads
better.

## Running and the crasher loop

- `go tool gotest ./...` replays every seed and committed corpus entry as
  ordinary subtests — no flag, no cost. `-run` filters are never widened.
- `go tool gotest fuzz ./...` searches for one minute by default (`--for`
  sets the budget, `--for=0` removes it); `--target=<FuzzSuite_Method>`
  narrows to one wrapper. Exit 0 means the budget ran out with nothing
  found; exit 1 means a finding, and the session names each new
  `testdata/fuzz/<Func>/<hash>` file.
- `go tool gotest fuzz triage ./...` re-runs each crasher and prints the
  decoded input and the cause; `go tool gotest fuzz promote ./...` splices
  it into the method as a typed `f.Add(...)` seed and deletes the file.
  Promote is the durable form: a struct target's corpus file is bound to
  its field order, a promoted literal is source. After promoting, fix the
  bug the seed now reproduces; the seed stays as the regression test.
- "New interesting inputs" in the session line live in Go's build cache
  (`fuzz/<package>/<Func>/` under `go env GOCACHE`), resume the next
  session, and are not committed; `go clean -fuzzcache` starts over.
- Seed harvesting is on by default: literal arguments from table tests and
  call sites in `_test.go` files become extra seeds at generation time
  (`--no-harvest`, or `fuzz: harvest: false` in `.gotest.yml`).
- In CI, the action's `fuzz: true` input runs a session after the tests,
  `fuzz-for` sets its budget (see `reference/ci.md`).

Generated wrappers are named `Fuzz<Suite>_<Method>`; plain `go test -fuzz`
cannot see them, so a target that must also run under stock tooling has to
be a top-level `func FuzzX(*testing.F)` using `gotest.NewF(f, nil, nil, nil)`
with native argument types — and gotest itself will not run that one.
