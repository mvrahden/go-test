import * as vscode from "vscode";
import * as path from "node:path";
import * as os from "node:os";
import {
  readFile,
  access,
  mkdir,
  constants,
} from "node:fs/promises";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import {
  resolveGoBinary,
  findInstalledGotest,
  fileExists,
  clearGoBinaryCache,
} from "./goBinary.js";

export { resolveGoBinary } from "./goBinary.js";

const execFileAsync = promisify(execFile);
const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";
const MIN_CLI_VERSION = "v1.3.0";

export interface CliCommand {
  bin: string;
  args: string[];
}

const replaceBinaryCache = new Map<string, { hash: string; binPath: string }>();
let versionWarningShown = false;

export async function validateGoBinary(
  log?: vscode.OutputChannel,
  workspaceDir?: string,
): Promise<string | undefined> {
  const goBin = await resolveGoBinary(log, workspaceDir);
  try {
    const { stdout } = await execFileAsync(goBin, ["version"], {
      timeout: 5_000,
      cwd: workspaceDir,
    });
    log?.appendLine(`[go] binary validated: ${stdout.trim()}`);
    return goBin;
  } catch {
    log?.appendLine(`[go] binary "${goBin}" failed validation`);
    clearGoBinaryCache();
    return undefined;
  }
}

export async function buildCliCommand(
  subcommandArgs: string[],
  workspaceDir?: string,
  log?: vscode.OutputChannel,
): Promise<CliCommand> {
  const config = scopedConfig(workspaceDir);

  const buildTags = config.get<string>("buildTags", "").trim();
  if (buildTags) {
    subcommandArgs = [...subcommandArgs, `-tags=${buildTags}`];
  }

  // 1. Explicit cliPath override (highest priority)
  const cliPath = config.get<string>("cliPath", "").trim();
  if (cliPath) {
    const resolved = resolveCliPath(cliPath, workspaceDir);
    if (await fileExists(resolved)) {
      log?.appendLine(`[cli] cliPath override: ${resolved}`);
      return { bin: resolved, args: subcommandArgs };
    }
    log?.appendLine(
      `[cli] cliPath "${resolved}" not found, probing alternatives`,
    );
  }

  // 2. Project-pinned version from go.mod
  const modulePath = config.get<string>("modulePath") ?? DEFAULT_MODULE_PATH;
  const effectiveDir =
    workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (effectiveDir && !modulePath.includes("@")) {
    const version = await extractVersionFromGoMod(effectiveDir, modulePath);
    if (version !== "latest") {
      if (compareVersions(version, MIN_CLI_VERSION) >= 0) {
        const goBin = await resolveGoBinary(log, workspaceDir);

        if (await hasReplaceDirective(effectiveDir, modulePath)) {
          const bin = await buildCachedBinary(
            goBin,
            effectiveDir,
            modulePath,
            log,
          );
          if (bin) {
            log?.appendLine(`[cli] using go.mod (replace): ${bin}`);
            return { bin, args: subcommandArgs };
          }
          log?.appendLine(`[cli] replace build failed, falling back to go run`);
        }

        const qualified = `${modulePath}@${version}`;
        log?.appendLine(`[cli] using go.mod: ${goBin} run ${qualified}`);
        return {
          bin: goBin,
          args: ["run", qualified, "--", ...subcommandArgs],
        };
      }
      log?.appendLine(
        `[cli] go.mod pins ${version}, requires >= ${MIN_CLI_VERSION}`,
      );
      showVersionWarning(version, effectiveDir, log);
    }
  }

  // 2b. Workspace IS the gotest module (development / go.work overlap)
  if (effectiveDir) {
    const declaredModule = await readModuleDeclaration(effectiveDir);
    if (
      declaredModule &&
      (modulePath === declaredModule ||
        modulePath.startsWith(declaredModule + "/"))
    ) {
      const goBin = await resolveGoBinary(log, workspaceDir);
      const bin = await buildCachedBinary(goBin, effectiveDir, modulePath, log);
      if (bin) {
        log?.appendLine(`[cli] using local module: ${bin}`);
        return { bin, args: subcommandArgs };
      }
      log?.appendLine(`[cli] local module build failed, continuing`);
    }
  }

  // 3. Globally installed binary
  const gotest = await findInstalledGotest(workspaceDir, log);
  if (gotest) {
    log?.appendLine(`[cli] using installed binary: ${gotest}`);
    return { bin: gotest, args: subcommandArgs };
  }

  // 4. Fallback: go run @latest
  const goBin = await resolveGoBinary(log, workspaceDir);
  const qualified = modulePath.includes("@")
    ? modulePath
    : `${modulePath}@latest`;
  log?.appendLine(`[cli] using fallback: ${goBin} run ${qualified}`);
  return { bin: goBin, args: ["run", qualified, "--", ...subcommandArgs] };
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
  log?: vscode.OutputChannel,
): void {
  if (versionWarningShown) {
    return;
  }
  versionWarningShown = true;
  vscode.window
    .showWarningMessage(
      `Go Test Suites: go.mod pins gotest ${version}, but >= ${MIN_CLI_VERSION} is required.`,
      "Upgrade",
    )
    .then(async (choice) => {
      if (choice !== "Upgrade") return;
      try {
        const goBin = await resolveGoBinary(log, effectiveDir);
        const args = ["get", `${DEFAULT_MODULE_PATH}@latest`];
        log?.appendLine(`[cli] upgrading: ${goBin} ${args.join(" ")}`);
        await execFileAsync(goBin, args, {
          cwd: effectiveDir,
          timeout: 30_000,
        });
        log?.appendLine("[cli] upgrade complete");
        vscode.window.showInformationMessage(
          "Go Test Suites: gotest dependency upgraded to latest.",
        );
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        log?.appendLine(`[cli] upgrade failed: ${msg}`);
        vscode.window.showErrorMessage(
          `Go Test Suites: upgrade failed. ${msg}`,
        );
      }
    });
}

async function hasReplaceDirective(
  workspaceDir: string,
  modulePath: string,
): Promise<boolean> {
  try {
    const content = await readFile(path.join(workspaceDir, "go.mod"), "utf-8");
    let candidate = modulePath;
    while (candidate) {
      const pattern = new RegExp(
        `^\\s*replace\\s+${escapeRegExp(candidate)}\\b`,
        "m",
      );
      if (pattern.test(content)) {
        return true;
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

async function buildCachedBinary(
  goBin: string,
  workspaceDir: string,
  modulePath: string,
  log?: vscode.OutputChannel,
): Promise<string | undefined> {
  const goModPath = path.join(workspaceDir, "go.mod");
  let goModContent: string;
  try {
    goModContent = await readFile(goModPath, "utf-8");
  } catch {
    return undefined;
  }

  const h = createHash("sha256").update(goModContent).update(modulePath);
  try {
    const goSumContent = await readFile(
      path.join(workspaceDir, "go.sum"),
      "utf-8",
    );
    h.update(goSumContent);
  } catch {
    // go.sum may not exist
  }
  const hash = h.digest("hex").substring(0, 16);
  const cached = replaceBinaryCache.get(workspaceDir);
  if (cached?.hash === hash) {
    try {
      await access(cached.binPath, constants.X_OK);
      return cached.binPath;
    } catch {
      // binary removed, rebuild
    }
  }

  const cacheDir = path.join(os.tmpdir(), "vscode-gotest");
  try {
    await mkdir(cacheDir, { recursive: true });
  } catch {
    return undefined;
  }

  const dirHash = createHash("sha256")
    .update(workspaceDir)
    .digest("hex")
    .substring(0, 12);
  const binPath = path.join(cacheDir, `gotest-${dirHash}`);

  try {
    log?.appendLine(`[cli] building gotest from replace directive...`);
    await execFileAsync(goBin, ["build", "-o", binPath, modulePath], {
      cwd: workspaceDir,
      timeout: 60_000,
    });
    replaceBinaryCache.set(workspaceDir, { hash, binPath });
    return binPath;
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    log?.appendLine(`[cli] replace build error: ${msg}`);
    return undefined;
  }
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
  replaceBinaryCache.clear();
  versionWarningShown = false;
}
