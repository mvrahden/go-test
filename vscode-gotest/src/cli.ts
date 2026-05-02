import * as vscode from "vscode";
import * as path from "node:path";
import { readFile, readdir, access, constants } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";
const MIN_CLI_VERSION = "v1.3.0";

export interface CliCommand {
  bin: string;
  args: string[];
}

const goBinaryCache = new Map<string, string>();
let cachedGotestBinary: string | undefined;
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
    goBinaryCache.delete(workspaceDir ?? "");
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
    log?.appendLine(`[cli] cliPath "${resolved}" not found, probing alternatives`);
  }

  // 2. Project-pinned version from go.mod
  const modulePath = config.get<string>("modulePath") ?? DEFAULT_MODULE_PATH;
  const effectiveDir = workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (effectiveDir && !modulePath.includes("@")) {
    const version = await extractVersionFromGoMod(effectiveDir, modulePath);
    if (version !== "latest") {
      if (compareVersions(version, MIN_CLI_VERSION) >= 0) {
        const goBin = await resolveGoBinary(log, workspaceDir);
        const qualified = `${modulePath}@${version}`;
        log?.appendLine(`[cli] using go.mod: ${goBin} run ${qualified}`);
        return { bin: goBin, args: ["run", qualified, "--", ...subcommandArgs] };
      }
      log?.appendLine(`[cli] go.mod pins ${version}, requires >= ${MIN_CLI_VERSION}`);
      showVersionWarning(version, effectiveDir, log);
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
  const qualified = modulePath.includes("@") ? modulePath : `${modulePath}@latest`;
  log?.appendLine(`[cli] using fallback: ${goBin} run ${qualified}`);
  return { bin: goBin, args: ["run", qualified, "--", ...subcommandArgs] };
}

export async function resolveGoBinary(
  log?: vscode.OutputChannel,
  workspaceDir?: string,
): Promise<string> {
  const cacheKey = workspaceDir ?? "";
  const cached = goBinaryCache.get(cacheKey);
  if (cached) {
    return cached;
  }

  // 1. Try project-specific Go version from go.mod
  if (workspaceDir) {
    const projectGo = await resolveProjectGoBinary(workspaceDir, log);
    if (projectGo) {
      goBinaryCache.set(cacheKey, projectGo);
      return projectGo;
    }
  }

  // 2. Generic detection
  const generic = await resolveGenericGoBinary(log);
  goBinaryCache.set(cacheKey, generic);
  return generic;
}

async function resolveProjectGoBinary(
  workspaceDir: string,
  log?: vscode.OutputChannel,
): Promise<string | undefined> {
  const goVersion = await readGoVersionFromMod(workspaceDir);
  if (!goVersion) {
    return undefined;
  }

  log?.appendLine(`[go] go.mod declares go ${goVersion}`);

  // ~/sdk/go1.26.2/bin/go
  const home = process.env.HOME ?? "";
  const sdkBin = path.join(home, "sdk", `go${goVersion}`, "bin", "go");
  if (await fileExists(sdkBin)) {
    log?.appendLine(`[go] resolved go ${goVersion} via SDK: ${sdkBin}`);
    return sdkBin;
  }

  // go1.26.2 on PATH (installed via `go install golang.org/dl/go1.26.2`)
  const versionedName = `go${goVersion}`;
  const shellVersioned = await whichFromShell(versionedName);
  if (shellVersioned) {
    log?.appendLine(`[go] resolved go ${goVersion} via shell: ${shellVersioned}`);
    return shellVersioned;
  }

  const whichVersioned = await which(versionedName);
  if (whichVersioned) {
    log?.appendLine(`[go] resolved go ${goVersion} via PATH: ${whichVersioned}`);
    return whichVersioned;
  }

  log?.appendLine(`[go] go ${goVersion} not found, falling back to generic detection`);
  return undefined;
}

async function resolveGenericGoBinary(log?: vscode.OutputChannel): Promise<string> {
  const goroot = process.env.GOROOT;
  if (goroot) {
    const goBin = path.join(goroot, "bin", "go");
    if (await fileExists(goBin)) {
      log?.appendLine(`[go] resolved via GOROOT: ${goBin}`);
      return goBin;
    }
  }

  const shellGo = await whichFromShell("go");
  if (shellGo) {
    log?.appendLine(`[go] resolved via shell: ${shellGo}`);
    return shellGo;
  }

  const whichGo = await which("go");
  if (whichGo) {
    log?.appendLine(`[go] resolved via PATH: ${whichGo}`);
    return whichGo;
  }

  for (const candidate of await commonGoPaths()) {
    if (await fileExists(candidate)) {
      log?.appendLine(`[go] resolved at common path: ${candidate}`);
      return candidate;
    }
  }

  log?.appendLine("[go] could not resolve binary, using bare 'go'");
  return "go";
}

async function readGoVersionFromMod(workspaceDir: string): Promise<string | undefined> {
  try {
    const goModPath = path.join(workspaceDir, "go.mod");
    const content = await readFile(goModPath, "utf-8");
    const match = /^\s*go\s+(\d+\.\d+(?:\.\d+)?)\s*$/m.exec(content);
    return match?.[1];
  } catch {
    return undefined;
  }
}

async function findInstalledGotest(
  workspaceDir?: string,
  log?: vscode.OutputChannel,
): Promise<string | undefined> {
  if (cachedGotestBinary) {
    if (await fileExists(cachedGotestBinary)) {
      return cachedGotestBinary;
    }
    cachedGotestBinary = undefined;
  }

  const gobin = process.env.GOBIN;
  if (gobin) {
    const p = path.join(gobin, "gotest");
    if (await fileExists(p)) {
      cachedGotestBinary = p;
      return p;
    }
  }

  const gopath = process.env.GOPATH ?? path.join(process.env.HOME ?? "", "go");
  const gopathBin = path.join(gopath, "bin", "gotest");
  if (await fileExists(gopathBin)) {
    cachedGotestBinary = gopathBin;
    return gopathBin;
  }

  try {
    const goBin = await resolveGoBinary(undefined, workspaceDir);
    const { stdout } = await execFileAsync(goBin, ["env", "GOBIN"], {
      cwd: workspaceDir,
      timeout: 5_000,
    });
    const envGobin = stdout.trim();
    if (envGobin) {
      const p = path.join(envGobin, "gotest");
      if (await fileExists(p)) {
        cachedGotestBinary = p;
        return p;
      }
    }

    const { stdout: gpOut } = await execFileAsync(goBin, ["env", "GOPATH"], {
      cwd: workspaceDir,
      timeout: 5_000,
    });
    const envGopath = gpOut.trim();
    if (envGopath) {
      const p = path.join(envGopath, "bin", "gotest");
      if (await fileExists(p)) {
        cachedGotestBinary = p;
        return p;
      }
    }
  } catch {
    log?.appendLine("[cli] failed to query go env for gotest location");
  }

  const whichGotest = await which("gotest");
  if (whichGotest) {
    cachedGotestBinary = whichGotest;
    return whichGotest;
  }

  return undefined;
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
  vscode.window.showWarningMessage(
    `Go Test Suites: go.mod pins gotest ${version}, but >= ${MIN_CLI_VERSION} is required.`,
    "Upgrade",
  ).then(async (choice) => {
    if (choice !== "Upgrade") return;
    try {
      const goBin = await resolveGoBinary(log, effectiveDir);
      const args = ["get", `${DEFAULT_MODULE_PATH}@latest`];
      log?.appendLine(`[cli] upgrading: ${goBin} ${args.join(" ")}`);
      await execFileAsync(goBin, args, { cwd: effectiveDir, timeout: 30_000 });
      log?.appendLine("[cli] upgrade complete");
      vscode.window.showInformationMessage("Go Test Suites: gotest dependency upgraded to latest.");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      log?.appendLine(`[cli] upgrade failed: ${msg}`);
      vscode.window.showErrorMessage(`Go Test Suites: upgrade failed. ${msg}`);
    }
  });
}

async function extractVersionFromGoMod(workspaceDir: string, modulePath: string): Promise<string> {
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

function compareVersions(a: string, b: string): number {
  const parse = (v: string) => v.replace(/^v/, "").split(".").map(Number);
  const pa = parse(a);
  const pb = parse(b);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const diff = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (diff !== 0) return diff;
  }
  return 0;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function formatCliCommand(cmd: CliCommand): string {
  return `${cmd.bin} ${cmd.args.join(" ")}`;
}

export function scopedConfig(workspaceDir?: string): vscode.WorkspaceConfiguration {
  const scope = workspaceDir ? vscode.Uri.file(workspaceDir) : undefined;
  return vscode.workspace.getConfiguration("gotest", scope);
}

async function fileExists(p: string): Promise<boolean> {
  try {
    await access(p, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

async function which(name: string): Promise<string | undefined> {
  try {
    const { stdout } = await execFileAsync("which", [name], { timeout: 3_000 });
    const resolved = stdout.trim();
    return resolved || undefined;
  } catch {
    return undefined;
  }
}

async function whichFromShell(name: string): Promise<string | undefined> {
  const shell = process.env.SHELL ?? "/bin/bash";
  try {
    const { stdout } = await execFileAsync(shell, ["-lc", `command -v ${name}`], { timeout: 5_000 });
    const resolved = stdout.trim();
    return resolved || undefined;
  } catch {
    return undefined;
  }
}

async function commonGoPaths(): Promise<string[]> {
  const home = process.env.HOME ?? "";
  const paths = [
    "/usr/local/go/bin/go",
    path.join(home, "go", "bin", "go"),
    "/usr/bin/go",
    "/snap/bin/go",
  ];

  const sdkDir = path.join(home, "sdk");
  try {
    const entries = await readdir(sdkDir);
    const goDirs = entries.filter((e) => e.startsWith("go")).sort().reverse();
    for (const dir of goDirs) {
      paths.push(path.join(sdkDir, dir, "bin", "go"));
    }
  } catch {
    // ~/sdk doesn't exist
  }

  return paths;
}

export function clearBinaryCache(): void {
  goBinaryCache.clear();
  cachedGotestBinary = undefined;
  versionWarningShown = false;
}
