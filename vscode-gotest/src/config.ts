import * as vscode from "vscode";

// Deliberately not reusing cli.ts's scopedConfig: reading a setting should not
// pull the whole toolchain-resolution graph (and its module-load side effects)
// into every module that wants a number.
function scoped(workspaceDir?: string): vscode.WorkspaceConfiguration {
  const scope = workspaceDir ? vscode.Uri.file(workspaceDir) : undefined;
  return vscode.workspace.getConfiguration("gotest", scope);
}

// The CLI's own force-kill backstop, mirrored from
// internal/gotestrunner/process.go: GracefulShutdownDelay = 5m30s. On a stop
// line (or a SIGTERM) the CLI cancels its run, which stops each suite and then
// waits this long before killing it — long enough to cover a 5-minute
// ContainerFixtureConfig teardown or an IntegrationSuiteConfig AfterAll.
//
// Ours has to outlast it. At our timeout the CLI is sent `force`, its second
// Ctrl-C, which releases the fixtures at once and kills what still runs two
// seconds later; forcing early cuts the CLI's teardown short and leaks whatever
// the fixture was holding.
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

// How long a CLI sent `force` gets before its process tree is killed: it ends
// within about four seconds of the line, so a CLI still running then took
// neither line.
export const CONTROL_FORCE_FALLBACK_MS = 5_000;

// Seconds a cancelled run gets after its `stop` line before it is sent `force`.
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

// `gotest spec --render-only` is a read, like discovery: it renders JSON it was
// handed, but on a cold cache it still pays the same `go run` compile first. It
// shares discovery's budget rather than introducing a sixth number.
export function specRenderTimeoutSeconds(workspaceDir?: string): number {
  return discoveryTimeoutSeconds(workspaceDir);
}
