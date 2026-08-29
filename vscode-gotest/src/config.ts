import * as vscode from "vscode";

// Deliberately not reusing cli.ts's scopedConfig: reading a setting should not
// pull the whole toolchain-resolution graph (and its module-load side effects)
// into every module that wants a number.
function scoped(workspaceDir?: string): vscode.WorkspaceConfiguration {
  const scope = workspaceDir ? vscode.Uri.file(workspaceDir) : undefined;
  return vscode.workspace.getConfiguration("gotest", scope);
}

// The CLI's own force-kill backstop, mirrored from
// internal/gotestrunner/process.go: GracefulShutdownDelay = 5m30s. On SIGTERM
// the CLI cancels its root context, which SIGTERMs the test process group and
// then waits this long before killing it — long enough to cover a 5-minute
// ContainerFixtureConfig teardown or an IntegrationSuiteConfig AfterAll.
//
// Ours has to outlast it. We SIGKILL the whole process group, so killing early
// takes the CLI's teardown with it and leaks whatever the fixture was holding.
const CLI_GRACEFUL_SHUTDOWN_S = 330;

// Enough margin for the CLI to finish its own force-kill and exit. Keep this
// above CLI_GRACEFUL_SHUTDOWN_S; the two numbers are a contract, not a
// preference.
export const DEFAULT_FORCE_KILL_TIMEOUT_S = CLI_GRACEFUL_SHUTDOWN_S + 30;

// A discovery run compiles the CLI on a cold cache and then loads every package
// in the workspace, so a monorepo can legitimately spend minutes here. The
// budget only stops a hung child from blocking discovery forever.
export const DEFAULT_DISCOVERY_TIMEOUT_S = 120;

// `discover` reads; it has no fixtures to tear down, so nothing is lost by
// killing it promptly once it has already ignored SIGTERM.
export const CAPTURE_FORCE_KILL_GRACE_MS = 5_000;

// Seconds to wait after SIGTERM before SIGKILL for a cancelled test run.
export function forceKillTimeoutSeconds(workspaceDir?: string): number {
  return (
    scoped(workspaceDir).get<number>("forceKillTimeout") ??
    DEFAULT_FORCE_KILL_TIMEOUT_S
  );
}

// Seconds to wait for `gotest discover` before giving up.
export function discoveryTimeoutSeconds(workspaceDir?: string): number {
  return (
    scoped(workspaceDir).get<number>("discoveryTimeout") ??
    DEFAULT_DISCOVERY_TIMEOUT_S
  );
}
