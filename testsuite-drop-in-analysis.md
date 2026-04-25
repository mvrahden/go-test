# Making `go-test` a Drop-In `go test` Augmentation

Revised analysis — challenges earlier findings, identifies a fundamental design
flaw in the arg-parsing approach, and proposes a simpler architecture.

---

## 0. Current State: Tests Already Broken

Before discussing the path forward, the branch has **pre-existing test failures**:

1. **E2E tests fail** — `examples/go.mod` has a hardcoded `replace` directive
   pointing to `/Users/menno/Projects/github.com/mvrahden/go-test`. This path
   doesn't exist on any machine except the original author's. The E2E harness
   (`HackGoWork`) tries to work around this with a `go.work` setup, but the
   `replace` directive in `examples/go.mod` takes precedence. Every E2E test
   fails with module resolution errors.

2. **Assertion tests fail** — The `maskPtr` regex in `base_test.go` matches
   pointer addresses like `0xc0000b2010` but fails when pointers appear inside
   map representations like `map[1:0x52d4a0]` (no space before the pointer).
   5 assertion test cases fail.

3. **Generator tests will fail** — `generator_test.go` references
   `examples/my` and `examples/focus_suite` which were deleted on this branch.

These must be fixed before any forward progress.

---

## 1. Fundamental Design Challenge: The Arg Parser Is the Wrong Approach

The previous analysis listed boolean-flag mishandling as a bug to fix. On
deeper analysis, **the entire arg-parsing strategy is flawed** and should be
replaced rather than patched.

### Why the Current Approach Can't Work

The tool needs to extract package patterns from the command line to know which
directories to scan for suite structs. It does this by parsing `go test` args
to distinguish flags from package names.

This is an **impossible problem** to solve correctly by reimplementing `go test`
flag parsing:

1. **`go test` accepts both `-flag` and `--flag` forms.** Not handled.

2. **`go test` accepts `-flag value` and `-flag=value` for value flags, but
   boolean flags like `-v` ALSO accept `-v=true`/`-v=false`.** The parser
   would need a complete flag type registry.

3. **`go test` forwards unrecognized flags to the test binary.** A flag like
   `-myCustomFlag` could be either boolean or value-taking, and the CLI has
   no way to know without understanding the test binary's flag definitions.

4. **Build flags overlap with test flags.** `-v` means verbose for both
   `go build` and `go test`. `-cover` is a build flag AND a test flag.
   `-json` can appear in both contexts.

5. **`-args` stops flag processing** — anything after `-args` is passed raw
   to the test binary.

6. **New Go versions add new flags.** Go 1.21 added `-fullpath`. Go 1.22 added
   `-skip`. Any hardcoded flag list becomes stale.

7. **Custom test flags** registered via `flag.String` etc. in test code are
   parsed by `go test` if prefixed with `-test.`. The tool can't know these.

**The "fix with a known-flag set" approach** from the prior analysis is a
patch on a fundamentally broken strategy. It creates a maintenance burden
(tracking every Go release for new flags) and can never handle custom test
binary flags.

### The Right Approach: Use `go list`

Instead of parsing flags to extract package patterns, use the Go toolchain
itself:

```
go list -find ./...          → lists all packages matching the pattern
go list -find -json ./...    → with filesystem paths
```

Or even simpler: `packages.Load` (which the tool already uses for code
generation) handles all pattern resolution, including `./...`, named
packages, build tags, `go.work` workspaces, and vendored dependencies.

**Proposed architecture:**

```
testsuite [go-test-flags...] [packages...]

Step 1: Pass the ENTIRE arg list to `go list` (or packages.Load) to
        discover which directories to scan. Let the Go toolchain parse
        its own flags.

Step 2: For each discovered directory, generate suite files.

Step 3: Pass the ENTIRE original arg list to `go test`. Don't parse
        or modify it.

Step 4: Clean up generated files.
```

The CLI becomes a thin wrapper that doesn't need to understand `go test`
flags at all. It only needs to:
- Extract its OWN flags (the `-ƒƒ.*` namespace — already handled)
- Pass everything else through untouched

For package discovery, the simplest correct approach is to call
`go list -find <everything-that-isn't-a-ƒƒ-flag>` and let the Go
toolchain figure out which tokens are packages and which are flags.
If `go list` fails, the user's `go test` invocation would also fail,
so the error is appropriate.

**Alternatively**, if passing flags to `go list` feels fragile (it handles
a subset of build flags, not test flags): you can extract package patterns
more simply by knowing that Go package patterns are the **non-flag positional
arguments** (don't start with `-`) that appear **before** `-args`. Boolean
vs value flags don't matter for this — you only need to identify which
non-`-` tokens are package patterns. The heuristic: any positional arg
that looks like a path (`./`, `../`, contains `/`) or matches `...` is a
package pattern. Everything else is a flag value. This is imperfect but
far more robust than full flag parsing.

**Best approach**: since `packages.Load` in the generator already resolves
package patterns, the CLI can simply pass each non-flag token as a candidate
to `packages.Load` and let it succeed or fail. No flag parsing needed.

---

## 2. Confirmed Critical Bugs

### 2.1 Double-Append in `StdlibRunTests`

**Confirmed.** `internal/gotestrunner/stdlib.go:8-11`:

```go
cmd := exec.Command("go", append([]string{"test"}, args...)...)  // args included once
if len(args) > 0 {
    cmd.Args = append(cmd.Args, args...)  // args included AGAIN
}
```

Result: `go test -v ./... -v ./...`. This is surprising — the E2E tests
somehow pass despite this. Investigation shows: the E2E tests DO fail (see
section 0), so this bug was never caught.

### 2.2 `AfterEach` Not Deferred — Deadlock Risk

**Confirmed.** `internal/gotestgen/static/gotest.suites.tpl:25-28`:

```go
s.BeforeEach(ttt)
testFn(ttt)       // t.Fatal() → runtime.Goexit() → skips remaining
s.AfterEach(ttt)  // never reached
```

This is worse than a missing teardown. In the parallel variant (lines 38-43),
`wg.Done()` is also not deferred. A `t.Fatal()` inside a parallel test case
causes:
- `AfterEach` skipped
- `wg.Done()` skipped
- `wg.Wait()` in `t.Cleanup` (line 64) blocks forever
- **The entire test suite hangs with no output**

The template ALSO does not defer `wg.Done()` in the non-fatal case. If
`AfterEach` itself panics, `wg.Done()` is skipped and the suite deadlocks.

**Fix:** Both `AfterEach` and `wg.Done()` must be deferred (in reverse order
so `wg.Done()` runs last).

### 2.3 Wrong Error Variable — Confirmed Panic

**Confirmed.** `internal/gotestgen/generator.go:89`:

```go
return nil, ptestCollected.Errs[0].Err  // should be pxtestCollected
```

Copy-paste bug. Panics when pxtest has errors but ptest doesn't.

### 2.4 `DeterminePkgDir` Root Package Panic

**Confirmed.** `internal/gotestgen/utils.go:18`:

```go
commonPrefix := len(modPath) + 1  // e.g. 28 + 1 = 29
path := pkgPath[commonPrefix:]    // pkgPath is 28 chars → panic
```

When test suites live at the module root (`pkgPath == modPath`).

### 2.5 `SuitesCleanup` Nil-Module Panic

**Confirmed.** `internal/gotestrunner/suites.go:46-47`:

`SuitesCleanup` iterates ALL packages from `LoadCached` (which returns the
raw `packages.Load` result) without the `Module != nil` filter that
`loadPackages` in `generator.go:44` applies. Any stdlib or non-module package
in the results causes a nil-pointer dereference in `DeterminePkgDir`.

### 2.6 Exit Code Not Set for Generation Errors

**Confirmed.** `cmd/testsuite/exec.go:27-31`:

```go
if r.Error != nil {
    fmt.Fprintf(os.Stdout, ...)
    continue  // ← skips maxCode update
}
```

The tool exits 0 even if code generation fails for every package. The user
sees "FAIL" messages printed to stdout but gets a success exit code.

---

## 3. Challenged Findings from Previous Analysis

### 3.1 "Boolean flags eat the next token" — Correct but Wrong Fix

The previous analysis proposed a hardcoded boolean flag set. This is correct
about the symptom but wrong about the fix. See section 1 — the right fix is
to eliminate the flag parser entirely, not to maintain a flag registry.

### 3.2 `NArgs` Contains Package Names — Not Actually a Bug

The previous analysis flagged this as inconsistent. Re-reading the code:
`NArgs` is passed directly to `go test`, which needs the package names.
The comment "excluding package names" is wrong, but the behavior is correct.
The fix is to update the comment, not the code. `pkgNameIndexes` is dead
code and should be removed.

### 3.3 `init()` in `about/git.go` — Less Severe Than Stated

The previous analysis called this a performance and security concern. On
re-examination:
- The `init()` runs once per process, not per package. The 4 git commands
  add ~50ms total startup cost. Measurable but not severe.
- The git info is not used for anything functional — it's only in
  `ShortInfo()` and `LongInfo()` which appear in the generated file header
  comment. If they fail, the defaults (`"local"`) are fine.
- **However**, for a distributable tool, build-time injection via `-ldflags`
  is still the right approach. The `init()` queries the USER's git repo,
  not the go-test repo — the version info is meaningless.

### 3.4 "Streaming output" — Needs Nuance

The previous analysis said to pipe stdout/stderr directly. This is almost
right but ignores a subtlety: the current architecture fans out generation
results through a collector channel (exec.go:25-43). If we stream `go test`
output directly, the collector goroutine becomes unnecessary for the test-run
phase (it's still needed for generation/cleanup error reporting). The fix
should also address the fact that generation errors currently go to stdout
(exec.go:28-30) but should go to stderr, since they're not test output.

### 3.5 Generic Test Suites — Lower Priority Than Stated

The previous analysis flagged generics support as needed. In practice, generic
test suites are an extremely rare pattern — there's no meaningful use case for
parameterizing a test suite struct over a type. This should be handled with a
clear error message in `DetermineTestSuite` (check `ts.TypeParams != nil`),
not by implementing full generics support.

### 3.6 Missing: `examples/go.mod` Hardcoded Replace Directive

The previous analysis missed this entirely. The `examples/go.mod` contains:

```
replace github.com/mvrahden/go-test => /Users/menno/Projects/github.com/mvrahden/go-test
```

This is a hardcoded absolute path to the author's macOS machine. **Every E2E
test that touches the examples module fails on any other machine.** This is
the #1 blocker before any other work can proceed. The fix is to use a relative
path:

```
replace github.com/mvrahden/go-test => ../
```

Or better: let `HackGoWork` handle the module resolution entirely and remove
the `replace` directive.

### 3.7 Missing: Process-Level Package Cache Incompatibility

The `LoadCached` function uses a process-level `sync.Mutex`-guarded map.
`SuitesGenerate` and `SuitesCleanup` both call `LoadCached`, which works
because they run in the same process. But this creates an implicit coupling:
cleanup only works if generation ran in the same process. If the process
crashes between generation and cleanup (or the user runs `go test` manually
after generation), the cache is empty and cleanup silently does nothing —
the generated files are orphaned.

The cleanup should not depend on the cache. It should scan the filesystem
for files matching `PSuiteRegex` instead.

### 3.8 Missing: Code Generation Invalidation

If a user changes their test suite struct (adds/removes methods, renames a
test), the generated file from a previous run may be stale. The current
generate-run-cleanup model handles this because it regenerates on every run.
But if the debug flag is used (`-ƒƒ.internal.debug`), or a crash orphans
files, the stale generated code sits in the source tree and compiles — it
may call methods that no longer exist or miss new ones. There's no
timestamp/hash check for staleness.

### 3.9 Missing: File Permission Issue

`SuitesGenerate` writes files with `os.ModePerm` (0777). Generated test files
should use `0644` (read-write for owner, read-only for others). `0777` creates
executable test source files, which is unusual and may trigger linter warnings.

### 3.10 Missing: Template Initialization Order Bug

In `renderer.go:21-25`:

```go
headerTpl = template.Must(template.New("header").ParseFS(templates, "static/header.*"))
gotestTpl = template.Must(template.New("gotest").Funcs(tplFuncs).ParseFS(templates, "static/gotest.*"))
tplFuncs  = map[string]any{
    "hasSuffix": strings.HasSuffix,
}
```

`gotestTpl` uses `tplFuncs`, but `tplFuncs` is declared AFTER `gotestTpl` in
the `var` block. Go initializes package-level variables in declaration order
within a `var` block. So `gotestTpl` is initialized with a nil `tplFuncs`.
However, `template.Funcs` is called before `ParseFS`, and `template.Funcs`
with a nil map is a no-op (it doesn't panic, but it doesn't register
`hasSuffix`). If the template uses `hasSuffix` (it does, on line 15 of
`gotest.suites.tpl`), this causes a **template execution error** at runtime.

**Wait — but the generator tests pass for the `stdlib` and `simple_suite`
examples.** Let me reconsider: `hasSuffix` is only called when a suite has
`TestSuiteParallel` suffix. The existing test examples don't have parallel
suites. So this bug exists but is latent — it only manifests when someone
actually uses `TestSuiteParallel`.

**Correction:** Go's `var` block initialization order is actually by
dependency analysis, not textual order. Since `gotestTpl` references
`tplFuncs`, Go will initialize `tplFuncs` first. So this is NOT a bug —
Go handles this correctly. The textual order is misleading but functional.

---

## 4. Revised Architecture for Drop-In Compatibility

### 4.1 Simplified CLI Design

```
testsuite [ƒƒ-flags] [go-test-args...]

The only flags the CLI owns are in the -ƒƒ.* namespace.
Everything else is passed through to `go test` verbatim.
```

No subcommands needed for MVP. The `clean` functionality should be a flag:
`-ƒƒ.clean` that removes orphaned generated files and exits.

### 4.2 Package Discovery Without Flag Parsing

Replace the current `parseNArgs` with:

```go
func discoverPackages(goTestArgs []string) ([]string, error) {
    // 1. Find the -args boundary (if any)
    argsIdx := slices.Index(goTestArgs, "-args")
    searchArgs := goTestArgs
    if argsIdx >= 0 {
        searchArgs = goTestArgs[:argsIdx]
    }

    // 2. Extract candidate package patterns:
    //    non-flag tokens (don't start with -)
    var candidates []string
    for _, arg := range searchArgs {
        if !strings.HasPrefix(arg, "-") {
            candidates = append(candidates, arg)
        }
    }

    // 3. If no candidates, default to "."
    if len(candidates) == 0 {
        candidates = []string{"."}
    }

    // 4. Resolve each candidate via go list
    //    This handles ./..., named packages, build tags, etc.
    return candidates, nil
}
```

This is imperfect — a flag value like `-run TestFoo` would make `TestFoo` a
candidate. But `packages.Load("TestFoo")` will return zero packages (it's not
a valid package pattern), so it's harmless. The fallback is safe.

### 4.3 Execution Pipeline

```
main()
├── 1. Split ƒƒ-flags from go-test-args
├── 2. Discover package directories (via go list / packages.Load)
├── 3. Generate suite files for each directory
├── 4. defer: cleanup suite files (runs on ALL exit paths including signals)
├── 5. Exec `go test` with original go-test-args
│      Stream stdout/stderr directly
│      Capture exit code
└── 6. os.Exit(exit code)
```

Key changes from current design:
- **Step 4 uses defer**, not a post-run phase. This handles Ctrl+C, panics,
  and all error paths.
- **Step 5 streams** instead of buffering.
- **No fan-out for generation.** `packages.Load` with `./...` already resolves
  all packages in one call. The fan-out was needed because the current design
  calls `packages.Load` per-package. With a single upfront load, generation
  is a simple loop.
- **Cleanup scans the filesystem** for files matching `PSuiteRegex` in the
  discovered directories, rather than relying on the process-level cache.

### 4.4 Signal Safety

```go
func main() {
    // ... parse args, discover packages, generate ...

    // Register cleanup BEFORE running go test
    generatedDirs := generateSuites(packages)
    defer cleanupSuites(generatedDirs)

    // Forward signals to go test subprocess
    ctx, cancel := signal.NotifyContext(context.Background(),
        os.Interrupt, syscall.SIGTERM)
    defer cancel()

    code := runGoTest(ctx, args)
    os.Exit(code)
}
```

`defer cleanupSuites(...)` runs even if `runGoTest` is interrupted. The
`signal.NotifyContext` ensures the `go test` subprocess is killed on
signal, and the deferred cleanup still runs.

**Caveat:** `os.Exit` does NOT run defers. The deferred cleanup must run
before `os.Exit`. Use `runtime.Goexit()` or restructure to return from
`main()` with the exit code set via a wrapper.

---

## 5. Revised Priority Roadmap

### Phase 0 — Unblock Development (Day 1)

1. Fix `examples/go.mod` replace directive (hardcoded path → relative)
2. Fix `maskPtr` regex in assertion tests (handle pointers inside maps)
3. Remove/fix generator test cases referencing deleted example dirs
4. Verify all tests pass on this machine

### Phase 1 — Fix Critical Bugs (Week 1)

1. Fix double-append in `StdlibRunTests`
2. Fix `AfterEach`/`wg.Done()` via `defer` in template
3. Fix wrong error variable in `generator.go:89`
4. Fix `DeterminePkgDir` root-package panic
5. Fix `SuitesCleanup` nil-Module guard
6. Fix exit code propagation for generation errors
7. Fix file permissions (0777 → 0644)

### Phase 2 — Redesign CLI for Drop-In Use (Week 2-3)

1. Replace flag parser with `go list`-based package discovery
2. Implement streaming output (pipe stdout/stderr directly)
3. Add deferred cleanup for signal safety
4. Default to `.` when no package specified
5. Handle `-args` boundary
6. Add `-ƒƒ.clean` flag for orphaned file cleanup
7. Replace git `init()` with build-time `-ldflags`
8. Make cleanup filesystem-based, not cache-based

### Phase 3 — Testing & Confidence (Week 3-4)

1. Add E2E test: package with zero suites (passthrough)
2. Add E2E test: `-v ./...` (the most common invocation)
3. Add E2E test: `-race ./...`
4. Add E2E test: multiple suites in one package
5. Add E2E test: focus/exclude (`F_`/`X_` prefixes)
6. Add E2E test: parallel suites and test cases
7. Add E2E test: exit code verification
8. Add E2E test: signal interruption cleanup
9. Reject generic suite structs with a clear error
10. Set up GitHub Actions CI

### Phase 4 — Ship It (Week 4-5)

1. Choose binary name
2. Set up goreleaser
3. Write README with install + usage guide
4. Cross-platform golden file handling
5. Tag v0.1.0
