import * as vscode from "vscode";

const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";

export interface CliCommand {
  bin: string;
  args: string[];
}

/**
 * Build a CLI command for invoking gotest.
 *
 * Uses `go run <modulePath>` by default so the binary version
 * is resolved from the project's go.mod. If the user sets
 * `gotest.cliPath` to an explicit binary path, that is used directly.
 */
export function buildCliCommand(subcommandArgs: string[]): CliCommand {
  const cliPath = vscode.workspace
    .getConfiguration("gotest")
    .get<string>("cliPath");

  if (cliPath) {
    return { bin: cliPath, args: subcommandArgs };
  }

  const modulePath = vscode.workspace
    .getConfiguration("gotest")
    .get<string>("modulePath") ?? DEFAULT_MODULE_PATH;

  return { bin: "go", args: ["run", modulePath, ...subcommandArgs] };
}

export function formatCliCommand(cmd: CliCommand): string {
  return `${cmd.bin} ${cmd.args.join(" ")}`;
}
