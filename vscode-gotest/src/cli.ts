import * as vscode from "vscode";
import * as path from "node:path";
import { readFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { resolveGoBinary, fileExists, clearGoBinaryCache } from "./goBinary.js";

export { resolveGoBinary } from "./goBinary.js";

const execFileAsync = promisify(execFile);
const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";
const MIN_CLI_VERSION = "v1.14.0";

export interface CliCommand {
  bin: string;
  args: string[];
}

let versionWarningShown = false;

export async function validateGoBinary(
  log?: vscode.LogOutputChannel,
  workspaceDir?: string,
): Promise<string | undefined> {
  const goBin = await resolveGoBinary(log, workspaceDir);
  try {
    const { stdout } = await execFileAsync(goBin, ["version"], {
      timeout: 5_000,
      cwd: workspaceDir,
    });
    log?.debug(`[go] binary validated: ${stdout.trim()}`);
    return goBin;
  } catch {
    log?.warn(`[go] binary "${goBin}" failed validation`);
    clearGoBinaryCache();
    return undefined;
  }
}

export async function buildCliCommand(
  subcommandArgs: string[],
  workspaceDir?: string,
  log?: vscode.LogOutputChannel,
): Promise<CliCommand> {
  const config = scopedConfig(workspaceDir);

  const buildTags = config.get<string>("buildTags", "").trim();
  if (buildTags) {
    subcommandArgs = [...subcommandArgs, `-tags=${buildTags}`];
  }

  const modulePath = config.get<string>("modulePath") ?? DEFAULT_MODULE_PATH;
  const effectiveDir =
    workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  // 1. Explicit cliPath override — only trusted when user-provided and version-valid
  const cliPath = config.get<string>("cliPath", "").trim();
  if (cliPath) {
    const resolved = resolveCliPath(cliPath, workspaceDir);
    if (await fileExists(resolved)) {
      const version = await queryBinaryVersion(resolved, log);
      if (version && compareVersions(version, MIN_CLI_VERSION) >= 0) {
        log?.debug(`[cli] cliPath override: ${resolved} (${version})`);
        return { bin: resolved, args: subcommandArgs };
      }
      if (version) {
        log?.warn(
          `[cli] cliPath "${resolved}" is ${version}, requires >= ${MIN_CLI_VERSION} — falling back to go run`,
        );
      } else {
        log?.warn(
          `[cli] cliPath "${resolved}" failed version check — falling back to go run`,
        );
      }
    } else {
      log?.debug(`[cli] cliPath "${resolved}" not found, probing alternatives`);
    }
  }

  // 2. Workspace IS the gotest module (development / go.work overlap)
  if (effectiveDir) {
    const declaredModule = await readModuleDeclaration(effectiveDir);
    if (
      declaredModule &&
      (modulePath === declaredModule ||
        modulePath.startsWith(declaredModule + "/"))
    ) {
      const goBin = await resolveGoBinary(log, workspaceDir);
      const relPath = "./" + modulePath.slice(declaredModule.length + 1);
      log?.debug(`[cli] workspace is gotest module: ${goBin} run ${relPath}`);
      return {
        bin: goBin,
        args: ["run", relPath, ...subcommandArgs],
      };
    }
  }

  // 3–4. Project-pinned version from go.mod
  if (effectiveDir && !modulePath.includes("@")) {
    const version = await extractVersionFromGoMod(effectiveDir, modulePath);
    if (version !== "latest") {
      if (compareVersions(version, MIN_CLI_VERSION) >= 0) {
        const goBin = await resolveGoBinary(log, workspaceDir);

        // 3. Replace directive → go run without version (respects go.mod resolution)
        if (await hasReplaceDirective(effectiveDir, modulePath)) {
          log?.debug(
            `[cli] go.mod has replace directive: ${goBin} run ${modulePath}`,
          );
          return {
            bin: goBin,
            args: ["run", modulePath, ...subcommandArgs],
          };
        }

        // 4. Pinned version → go run module@version
        const qualified = `${modulePath}@${version}`;
        log?.debug(`[cli] using go.mod: ${goBin} run ${qualified}`);
        return {
          bin: goBin,
          args: ["run", qualified, ...subcommandArgs],
        };
      }
      log?.warn(`[cli] go.mod pins ${version}, requires >= ${MIN_CLI_VERSION}`);
      showVersionWarning(version, effectiveDir, log);
    }
  }

  // 5. Fallback: go run @latest
  const goBin = await resolveGoBinary(log, workspaceDir);
  const qualified = modulePath.includes("@")
    ? modulePath
    : `${modulePath}@latest`;
  log?.debug(`[cli] using fallback: ${goBin} run ${qualified}`);
  return { bin: goBin, args: ["run", qualified, ...subcommandArgs] };
}

async function readModuleDeclaration(
  workspaceDir: string,
): Promise<string | undefined> {
  try {
    const goModPath = path.join(workspaceDir, "go.mod");
    const content = await readFile(goModPath, "utf-8");
    const match = /^\s*module\s+(\S+)/m.exec(content);
    return match?.[1];
  } catch {
    return undefined;
  }
}

function resolveCliPath(cliPath: string, workspaceDir?: string): string {
  if (path.isAbsolute(cliPath)) {
    return cliPath;
  }
  if (workspaceDir) {
    return path.resolve(workspaceDir, cliPath);
  }
  const wsFolder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (wsFolder) {
    return path.resolve(wsFolder, cliPath);
  }
  return cliPath;
}

function showVersionWarning(
  version: string,
  effectiveDir: string,
  log?: vscode.LogOutputChannel,
): void {
  if (versionWarningShown) {
    return;
  }
  versionWarningShown = true;
  vscode.window
    .showWarningMessage(
      `gotest: go.mod pins gotest ${version}, but >= ${MIN_CLI_VERSION} is required.`,
      "Upgrade",
    )
    .then(async (choice) => {
      if (choice !== "Upgrade") return;
      try {
        const goBin = await resolveGoBinary(log, effectiveDir);
        const args = ["get", `${DEFAULT_MODULE_PATH}@latest`];
        log?.info(`[cli] upgrading: ${goBin} ${args.join(" ")}`);
        await execFileAsync(goBin, args, {
          cwd: effectiveDir,
          timeout: 30_000,
        });
        log?.info("[cli] upgrade complete");
        vscode.window.showInformationMessage(
          "gotest: gotest dependency upgraded to latest.",
        );
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        log?.error(`[cli] upgrade failed: ${msg}`);
        vscode.window.showErrorMessage(`gotest: upgrade failed. ${msg}`);
      }
    });
}

async function queryBinaryVersion(
  binPath: string,
  log?: vscode.LogOutputChannel,
): Promise<string | undefined> {
  try {
    const { stdout } = await execFileAsync(binPath, ["version"], {
      timeout: 5_000,
    });
    const match = /^gotest\s+(v\S+)/m.exec(stdout);
    if (match) {
      return match[1];
    }
    log?.debug(
      `[cli] unexpected version output from ${binPath}: ${stdout.trim()}`,
    );
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    log?.debug(`[cli] version check failed for ${binPath}: ${msg}`);
  }
  return undefined;
}

async function hasReplaceDirective(
  workspaceDir: string,
  modulePath: string,
): Promise<boolean> {
  try {
    const content = await readFile(path.join(workspaceDir, "go.mod"), "utf-8");
    let candidate = modulePath;
    while (candidate) {
      const escaped = escapeRegExp(candidate);
      if (
        new RegExp(`^\\s*replace\\s+${escaped}(?:\\s|$)`, "m").test(content)
      ) {
        return true;
      }
      const entryPattern = new RegExp(`^\\s*${escaped}(?:\\s|$)`, "m");
      for (const block of content.matchAll(/^\s*replace\s*\(([\s\S]*?)\)/gm)) {
        if (entryPattern.test(block[1])) {
          return true;
        }
      }
      const lastSlash = candidate.lastIndexOf("/");
      if (lastSlash <= 0) break;
      candidate = candidate.substring(0, lastSlash);
    }
  } catch {
    // go.mod not found
  }
  return false;
}

async function extractVersionFromGoMod(
  workspaceDir: string,
  modulePath: string,
): Promise<string> {
  try {
    const goModPath = path.join(workspaceDir, "go.mod");
    const content = await readFile(goModPath, "utf-8");

    let candidate = modulePath;
    while (candidate) {
      const escaped = escapeRegExp(candidate);
      const patterns = [
        new RegExp(`^\\s*${escaped}\\s+(v[^\\s]+)`, "m"),
        new RegExp(`^\\s*require\\s+${escaped}\\s+(v[^\\s]+)`, "m"),
      ];
      for (const pattern of patterns) {
        const match = pattern.exec(content);
        if (match) return match[1];
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

export function compareVersions(a: string, b: string): number {
  const parse = (v: string) => v.replace(/^v/, "").split(".").map(Number);
  const pa = parse(a);
  const pb = parse(b);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const diff = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (diff !== 0) return diff;
  }
  return 0;
}

export function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function formatCliCommand(cmd: CliCommand): string {
  return `${cmd.bin} ${cmd.args.join(" ")}`;
}

export function scopedConfig(
  workspaceDir?: string,
): vscode.WorkspaceConfiguration {
  const scope = workspaceDir ? vscode.Uri.file(workspaceDir) : undefined;
  return vscode.workspace.getConfiguration("gotest", scope);
}

export function clearBinaryCache(): void {
  clearGoBinaryCache();
  versionWarningShown = false;
}
