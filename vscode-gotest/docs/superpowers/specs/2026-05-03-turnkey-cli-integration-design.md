# Turn-Key CLI Integration

## Goal

Refactor the VSCode extension to delegate test execution, coverage, and debug preparation to the `gotest` CLI as single turn-key commands, eliminating the extension's 3-subprocess orchestration (overlay + shared-setup + go test).

## Architecture

Three execution paths, each with a clear CLI boundary:

| Path | CLI Command | CLI Owns | Extension Owns |
|------|-------------|----------|----------------|
| **Test Run** | `gotest -json -count=1 [-run=F] [-coverprofile=P] <pkg>` | overlay, shared fixtures, go test | spawning CLI, parsing JSON, reading coverprofile, mapping to TestItems |
| **Coverage** | Same (always with `-coverprofile`) | Same | Same + `go tool cover -func` + CoverageStore |
| **Debug** | `gotest prepare <pkg>` (new, blocks until SIGTERM) | overlay, shared fixtures, lifetime mgmt | spawning CLI, reading prep JSON, DebugConfiguration, dlv launch, kill on session end |

Existing subcommands (`discover`, `watch`, `generate`, `clean`, `scaffold`, `migrate`, `spec`) remain unchanged.

## CLI Changes

### 1. New `prepare` subcommand

File: `cmd/gotest/prepare.go`

```
gotest prepare [-tags=<tags>] <package>
```

Behavior:
1. Parse package patterns and tags from args
2. Call `generateOverlay(patterns)` (existing internal function)
3. If shared fixtures present, call `startSharedFixtures(ctx, tmpDir, fixtures)`
4. Output single JSON line to stdout: `{"overlayFile": "...", "dir": "...", "stateFile": "..."}`
   - `stateFile` omitted when no shared fixtures
5. Block on signal channel (SIGINT/SIGTERM)
6. On signal: teardown shared fixtures, clean up overlay dir, exit 0

Register in `knownSubcommands` (args.go) and dispatch switch (cli.go).

### 2. Drop `overlay` and `shared-setup` subcommands

The extension was the sole consumer of both. After this refactor, no caller remains.

- Delete `cmd/gotest/overlay.go`
- Delete `cmd/gotest/sharedsetup.go`
- Remove `"overlay"` and `"shared-setup"` from `knownSubcommands` in `args.go`
- Remove their cases from the dispatch switch in `cli.go`
- Adjust `overlay_test.go` — the first test uses `overlayOutput` struct from `overlay.go` and calls `gotestgen.Generate` directly. Rename to `generate_test.go`, replace `overlayOutput` usage with inline struct or `prepareOutput`, keep the core logic that exercises `gotestgen.Generate` + `gotestrunner.WriteOverlay`. The second test verifies empty-results behavior and needs no struct changes.

The internal functions `generateOverlay()` (exec.go) and `startSharedFixtures()` (sharedfixture.go) remain — they are used by `Run()`, `watchRunOnce()`, and the new `runPrepare()`.

### 3. Fix `watchRunOnce()` shared fixture gap

`watch.go:watchRunOnce()` calls `generateOverlay()` but ignores `overlay.sharedFixtures`. Tests that use shared fixtures silently fail during watch because `GOTEST_SHARED_STATE_FILE` is never set.

Fix: after `generateOverlay()`, check `overlay.sharedFixtures` and call `startSharedFixtures()` if non-empty. Pass `GOTEST_SHARED_STATE_FILE` via `extraEnv`. Teardown in defer.

### 4. Context-aware `StdlibRunTests` / `StdlibRunTestsJSON`

File: `internal/gotestrunner/stdlib.go`, `internal/gotestrunner/json.go`

Currently these functions don't accept a `context.Context`. When the extension SIGTERMs the gotest process, the signal handler cancels the context (via `signal.NotifyContext`), but the child `go test` process keeps running because `exec.Command` doesn't propagate cancellation.

Change: add `context.Context` as first parameter, use `exec.CommandContext`. All callers (`Run()`, `runWithSpec()`, `watchRunOnce()`) already have a context available.

## Extension Changes

### 1. Simplify `runner.ts`

Replace the per-package body (overlay subprocess + shared-setup subprocess + go test spawn + triple cleanup) with:

1. Build CLI args: `["-json", "-count=1", importPath]`
   - Add `-coverprofile=<tmpFile>` if coverOnRun enabled
   - Add `-run <filter>` if filter present
   - Append `testFlags` from config
2. Call `buildCliCommand(cliArgs, workspaceDir, outputChannel)` — resolves gotest binary, appends `-tags` from config
3. Spawn via `spawnTestProcess(cmd.bin, cmd.args, workspaceDir, token, ...)`
4. Parse JSON events, apply results
5. If coverOnRun: read coverprofile from temp file, run `go tool cover -func`, update CoverageStore

Cleanup: only the coverprofile temp dir (extension-managed). Overlay and shared fixture cleanup is handled by the CLI process exiting.

Remove imports: `execFile`/`promisify` (no more subprocess calls for overlay), `startSharedSetup`/`SharedSetupProcess`, `OverlayOutput`.

### 2. Simplify `coverage.ts`

Same treatment for both `run()` and `runPackage()`:

1. Build CLI args with `-coverprofile=<tmpFile>` always included
2. Spawn gotest via `buildCliCommand` + `spawnTestProcess`
3. After exit: read coverprofile, run `go tool cover -func`, update CoverageStore

Remove imports: `execFile`/`promisify`, `startSharedSetup`/`SharedSetupProcess`, `OverlayOutput`.

The pure functions (`parseCoverProfile`, `parseFuncCoverage`, `buildFileCoverages`, `runGoToolCoverFunc`) and `CoverageRunner.copyCoverageSummary()` remain unchanged.

### 3. Refactor `debug.ts`

Replace `gotest overlay` call with `gotest prepare`:

1. Call `buildCliCommand(["prepare", pkgDir], workspaceDir, outputChannel)`
2. Spawn as long-running process (not via `spawnTestProcess` — need to read first line then keep process alive)
3. Read first JSON line from stdout → parse as `PrepareOutput`
4. Build DebugConfiguration:
   - `buildFlags: "-overlay=<overlayFile> <extraBuildFlags>"`
   - `env: { GOTEST_SHARED_STATE_FILE: stateFile }` (if stateFile present)
   - `args: ["-test.run", filter]` (if filter present)
5. Launch `vscode.debug.startDebugging(workspaceFolder, debugConfig)`
6. Track the prepare child process (not just overlay dir)
7. On `onDidTerminateDebugSession`: SIGTERM the prepare process (triggers CLI-side teardown + cleanup)

This fixes the current gap where debug sessions have no shared fixture support.

Remove: overlay dir tracking (`overlayDirs` set, `cleanupOverlayDir`, `cleanupAllOverlays`). Replace with prepare process tracking.

### 4. Delete `sharedFixtures.ts`

The CLI owns the shared fixture lifecycle for all three paths. The extension no longer needs to spawn or manage `gotest shared-setup`.

### 5. Update `types.ts`

Remove:
- `SharedFixtureInfo` — no longer needed extension-side
- `OverlayOutput` — no longer needed (extension doesn't call `overlay`)

Add:
```typescript
export interface PrepareOutput {
  overlayFile: string;
  dir: string;
  stateFile?: string;
}
```

### 6. Improve `spawnTestProcess` error reporting

Currently `spawnTestProcess` resolves with stdout regardless of exit code, and discards stderr (logs it but doesn't return it). When gotest fails during overlay generation (before go test starts), stdout is empty and the caller has no error context.

Change: return a result object `{ stdout: string; stderr: string; exitCode: number }` instead of bare string. Callers can then detect empty stdout + non-zero exit and produce meaningful error messages for test items.

## Files Changed

### CLI
| File | Action |
|------|--------|
| `cmd/gotest/prepare.go` | Create |
| `cmd/gotest/overlay.go` | Delete |
| `cmd/gotest/sharedsetup.go` | Delete |
| `cmd/gotest/args.go` | Modify (remove overlay/shared-setup, add prepare) |
| `cmd/gotest/cli.go` | Modify (remove overlay/shared-setup cases, add prepare) |
| `cmd/gotest/watch.go` | Modify (wire shared fixtures in watchRunOnce) |
| `cmd/gotest/overlay_test.go` | Rename to `generate_test.go`, replace `overlayOutput` with inline struct |
| `internal/gotestrunner/stdlib.go` | Modify (add context parameter) |
| `internal/gotestrunner/json.go` | Modify (add context parameter) |
| `cmd/gotest/exec.go` | Modify (pass context to StdlibRunTests) |

### Extension
| File | Action |
|------|--------|
| `src/runner.ts` | Modify (simplify to single CLI spawn) |
| `src/coverage.ts` | Modify (simplify to single CLI spawn) |
| `src/debug.ts` | Modify (use prepare, add shared fixture env) |
| `src/sharedFixtures.ts` | Delete |
| `src/types.ts` | Modify (drop SharedFixtureInfo/OverlayOutput, add PrepareOutput) |
| `src/runnerUtils.ts` | Modify (improve spawnTestProcess return type) |

### No change
`extension.ts`, `cli.ts`, `testController.ts`, `coverageStore.ts`, `outputParser.ts`, `discovery.ts`, `watch.ts`

## Known Limitations

**Build tags in code generation**: `generateOverlay()` calls `gotestgen.GenerateWithSharedFixtures(pattern)` without forwarding build tags. This is a pre-existing issue affecting all paths (default mode, watch, and the new prepare). Build tags reach `go test` but not the code generation step. Out of scope for this refactor.
