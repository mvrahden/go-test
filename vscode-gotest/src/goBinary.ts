import * as vscode from "vscode";
import * as path from "node:path";
import { readFile, readdir, access, constants } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const goBinaryCache = new Map<string, string>();

export function clearGoBinaryCache(): void {
  goBinaryCache.clear();
}

export async function resolveGoBinary(
  log?: vscode.LogOutputChannel,
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
  log?: vscode.LogOutputChannel,
): Promise<string | undefined> {
  const goVersion = await readGoVersionFromMod(workspaceDir);
  if (!goVersion) {
    return undefined;
  }

  log?.debug(`[go] go.mod declares go ${goVersion}`);

  const home = process.env.HOME ?? process.env.USERPROFILE ?? "";
  const goBin = process.platform === "win32" ? "go.exe" : "go";
  const sdkBin = path.join(home, "sdk", `go${goVersion}`, "bin", goBin);
  if (await fileExists(sdkBin)) {
    log?.debug(`[go] resolved go ${goVersion} via SDK: ${sdkBin}`);
    return sdkBin;
  }

  // go1.26.2 on PATH (installed via `go install golang.org/dl/go1.26.2`)
  // These can be wrapper stubs that fail when the SDK isn't downloaded,
  // so validate before accepting.
  const versionedName = `go${goVersion}`;
  const shellVersioned = await whichFromShell(versionedName);
  if (shellVersioned && (await validateBinary(shellVersioned))) {
    log?.debug(`[go] resolved go ${goVersion} via shell: ${shellVersioned}`);
    return shellVersioned;
  }

  const whichVersioned = await which(versionedName);
  if (whichVersioned && (await validateBinary(whichVersioned))) {
    log?.debug(`[go] resolved go ${goVersion} via PATH: ${whichVersioned}`);
    return whichVersioned;
  }

  log?.debug(
    `[go] go ${goVersion} not found, falling back to generic detection`,
  );
  return undefined;
}

async function resolveGenericGoBinary(
  log?: vscode.LogOutputChannel,
): Promise<string> {
  const goroot = process.env.GOROOT;
  if (goroot) {
    const goExe = process.platform === "win32" ? "go.exe" : "go";
    const goBin = path.join(goroot, "bin", goExe);
    if (await fileExists(goBin)) {
      log?.debug(`[go] resolved via GOROOT: ${goBin}`);
      return goBin;
    }
  }

  const shellGo = await whichFromShell("go");
  if (shellGo) {
    log?.debug(`[go] resolved via shell: ${shellGo}`);
    return shellGo;
  }

  const whichGo = await which("go");
  if (whichGo) {
    log?.debug(`[go] resolved via PATH: ${whichGo}`);
    return whichGo;
  }

  for (const candidate of await commonGoPaths()) {
    if (await fileExists(candidate)) {
      log?.debug(`[go] resolved at common path: ${candidate}`);
      return candidate;
    }
  }

  log?.warn("[go] could not resolve binary, using bare 'go'");
  return "go";
}

async function readGoVersionFromMod(
  workspaceDir: string,
): Promise<string | undefined> {
  try {
    const goModPath = path.join(workspaceDir, "go.mod");
    const content = await readFile(goModPath, "utf-8");
    const match = /^\s*go\s+(\d+\.\d+(?:\.\d+)?)\s*$/m.exec(content);
    return match?.[1];
  } catch {
    return undefined;
  }
}

async function validateBinary(bin: string): Promise<boolean> {
  try {
    await execFileAsync(bin, ["version"], { timeout: 5_000 });
    return true;
  } catch {
    return false;
  }
}

export async function fileExists(p: string): Promise<boolean> {
  try {
    await access(
      p,
      process.platform === "win32" ? constants.F_OK : constants.X_OK,
    );
    return true;
  } catch {
    return false;
  }
}

async function which(name: string): Promise<string | undefined> {
  const cmd = process.platform === "win32" ? "where" : "which";
  try {
    const { stdout } = await execFileAsync(cmd, [name], { timeout: 3_000 });
    const resolved = stdout.trim().split(/\r?\n/)[0];
    return resolved || undefined;
  } catch {
    return undefined;
  }
}

async function whichFromShell(name: string): Promise<string | undefined> {
  if (process.platform === "win32") {
    return undefined;
  }
  const shell = process.env.SHELL ?? "/bin/bash";
  try {
    const { stdout } = await execFileAsync(
      shell,
      ["-lc", `command -v ${name}`],
      { timeout: 5_000 },
    );
    const resolved = stdout.trim();
    return resolved || undefined;
  } catch {
    return undefined;
  }
}

async function commonGoPaths(): Promise<string[]> {
  const home = process.env.HOME ?? process.env.USERPROFILE ?? "";
  const goBin = process.platform === "win32" ? "go.exe" : "go";
  const paths: string[] = [];

  if (process.platform === "win32") {
    paths.push(path.join("C:\\Program Files\\Go\\bin", goBin));
    paths.push(path.join(home, "go", "bin", goBin));
    paths.push(path.join(home, "scoop", "apps", "go", "current", "bin", goBin));
  } else {
    paths.push("/usr/local/go/bin/go");
    paths.push(path.join(home, "go", "bin", "go"));
    paths.push("/usr/bin/go");
    paths.push("/snap/bin/go");
  }

  const sdkDir = path.join(home, "sdk");
  try {
    const entries = await readdir(sdkDir);
    const goDirs = entries
      .filter((e) => e.startsWith("go"))
      .sort()
      .reverse();
    for (const dir of goDirs) {
      paths.push(path.join(sdkDir, dir, "bin", goBin));
    }
  } catch {
    // ~/sdk doesn't exist
  }

  return paths;
}
