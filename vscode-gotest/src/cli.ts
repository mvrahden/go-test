import * as vscode from "vscode";
import * as path from "node:path";
import { readFile } from "node:fs/promises";

const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";

export interface CliCommand {
  bin: string;
  args: string[];
}

export async function buildCliCommand(subcommandArgs: string[], workspaceDir?: string): Promise<CliCommand> {
  const cliPath = vscode.workspace
    .getConfiguration("gotest")
    .get<string>("cliPath");

  if (cliPath) {
    return { bin: cliPath, args: subcommandArgs };
  }

  const modulePath = vscode.workspace
    .getConfiguration("gotest")
    .get<string>("modulePath") ?? DEFAULT_MODULE_PATH;

  const qualified = await qualifyModulePath(modulePath, workspaceDir);
  return { bin: "go", args: ["run", qualified, ...subcommandArgs] };
}


async function qualifyModulePath(modulePath: string, workspaceDir?: string): Promise<string> {
  if (modulePath.includes("@")) {
    return modulePath;
  }

  const effectiveDir = workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!effectiveDir) {
    return `${modulePath}@latest`;
  }

  const version = await extractVersionFromGoMod(effectiveDir, modulePath);
  return `${modulePath}@${version}`;
}

async function extractVersionFromGoMod(workspaceDir: string, modulePath: string): Promise<string> {
  try {
    const goModPath = path.join(workspaceDir, "go.mod");
    const content = await readFile(goModPath, "utf-8");

    // Find the module root: strip subpath components until we find a match.
    // e.g. "github.com/mvrahden/go-test/cmd/gotest" → try
    //   "github.com/mvrahden/go-test/cmd/gotest",
    //   "github.com/mvrahden/go-test/cmd",
    //   "github.com/mvrahden/go-test", ...
    let candidate = modulePath;
    while (candidate) {
      const pattern = new RegExp(
        `^\\s*${escapeRegExp(candidate)}\\s+(v[^\\s]+)`,
        "m",
      );
      const match = pattern.exec(content);
      if (match) {
        return match[1];
      }
      const lastSlash = candidate.lastIndexOf("/");
      if (lastSlash <= 0) {
        break;
      }
      candidate = candidate.substring(0, lastSlash);
    }
  } catch {
    // go.mod not found or unreadable
  }
  return "latest";
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function formatCliCommand(cmd: CliCommand): string {
  return `${cmd.bin} ${cmd.args.join(" ")}`;
}
