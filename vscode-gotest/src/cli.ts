import * as vscode from "vscode";
import * as path from "node:path";
import { readFile, access, constants } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const DEFAULT_MODULE_PATH = "github.com/mvrahden/go-test/cmd/gotest";
const MIN_CLI_VERSION = "v1.3.0";

export interface CliCommand {
  bin: string;
  args: string[];
}

let cachedGoBinary: string | undefined;
let cachedGotestBinary: string | undefined;

export async function buildCliCommand(
  subcommandArgs: string[],
  workspaceDir?: string,
  log?: vscode.OutputChannel,
): Promise<CliCommand> {
  const cliPath = vscode.workspace
    .getConfiguration("gotest")
    .get<string>("cliPath", "")
    .trim();

  if (cliPath) {
    const resolved = resolveCliPath(cliPath, workspaceDir);
    const exists = await fileExists(resolved);
    if (exists) {
      log?.appendLine(`[cli] using cliPath setting: ${resolved}`);
      return { bin: resolved, args: subcommandArgs };
    }
    log?.appendLine(`[cli] cliPath setting "${resolved}" not found, probing alternatives`);
  }

  const gotest = await findInstalledGotest(workspaceDir, log);
  if (gotest) {
    log?.appendLine(`[cli] using installed gotest: ${gotest}`);
    return { bin: gotest, args: subcommandArgs };
  }

  const goBin = await resolveGoBinary(log);
  const modulePath = vscode.workspace
    .getConfiguration("gotest")
    .get<string>("modulePath") ?? DEFAULT_MODULE_PATH;
  const qualified = await qualifyModulePath(modulePath, workspaceDir, log);
  log?.appendLine(`[cli] falling back to: ${goBin} run ${qualified}`);
  return { bin: goBin, args: ["run", qualified, "--", ...subcommandArgs] };
}

export async function resolveGoBinary(log?: vscode.OutputChannel): Promise<string> {
  if (cachedGoBinary) {
    return cachedGoBinary;
  }

  const goroot = process.env.GOROOT;
  if (goroot) {
    const goBin = path.join(goroot, "bin", "go");
    if (await fileExists(goBin)) {
      log?.appendLine(`[cli] resolved go via GOROOT: ${goBin}`);
      cachedGoBinary = goBin;
      return goBin;
    }
  }

  const whichGo = await which("go");
  if (whichGo) {
    log?.appendLine(`[cli] resolved go via PATH: ${whichGo}`);
    cachedGoBinary = whichGo;
    return whichGo;
  }

  for (const candidate of commonGoPaths()) {
    if (await fileExists(candidate)) {
      log?.appendLine(`[cli] resolved go at common path: ${candidate}`);
      cachedGoBinary = candidate;
      return candidate;
    }
  }

  log?.appendLine("[cli] could not resolve go binary, using bare 'go'");
  return "go";
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
    const goBin = await resolveGoBinary();
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

async function qualifyModulePath(modulePath: string, workspaceDir?: string, log?: vscode.OutputChannel): Promise<string> {
  if (modulePath.includes("@")) {
    return modulePath;
  }

  const effectiveDir = workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!effectiveDir) {
    return `${modulePath}@latest`;
  }

  const version = await extractVersionFromGoMod(effectiveDir, modulePath);
  if (version !== "latest" && compareVersions(version, MIN_CLI_VERSION) < 0) {
    log?.appendLine(`[cli] go.mod pins ${version}, but extension requires >= ${MIN_CLI_VERSION}; using @latest`);
    vscode.window.showWarningMessage(
      `Go Test Suites: go.mod pins gotest ${version}, but this extension requires >= ${MIN_CLI_VERSION}. Using @latest instead.`,
      "Upgrade",
    ).then(async (choice) => {
      if (choice !== "Upgrade") return;
      try {
        const goBin = await resolveGoBinary(log);
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
    return `${modulePath}@latest`;
  }
  return `${modulePath}@${version}`;
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

function commonGoPaths(): string[] {
  const home = process.env.HOME ?? "";
  return [
    "/usr/local/go/bin/go",
    path.join(home, "go", "bin", "go"),
    path.join(home, "sdk", "go", "bin", "go"),
    "/usr/bin/go",
    "/snap/bin/go",
  ];
}

export function clearBinaryCache(): void {
  cachedGoBinary = undefined;
  cachedGotestBinary = undefined;
}
