import * as vscode from "vscode";
import * as path from "node:path";
import * as os from "node:os";
import { readFile, readdir, access, constants } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const goBinaryCache = new Map<string, string>();
let cachedGotestBinary: string | undefined;

export function clearGoBinaryCache(): void {
  goBinaryCache.clear();
  cachedGotestBinary = undefined;
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
    log?.appendLine(
      `[go] resolved go ${goVersion} via shell: ${shellVersioned}`,
    );
    return shellVersioned;
  }

  const whichVersioned = await which(versionedName);
  if (whichVersioned) {
    log?.appendLine(
      `[go] resolved go ${goVersion} via PATH: ${whichVersioned}`,
    );
    return whichVersioned;
  }

  log?.appendLine(
    `[go] go ${goVersion} not found, falling back to generic detection`,
  );
  return undefined;
}

async function resolveGenericGoBinary(
  log?: vscode.OutputChannel,
): Promise<string> {
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

export async function findInstalledGotest(
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

export async function fileExists(p: string): Promise<boolean> {
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
    const goDirs = entries
      .filter((e) => e.startsWith("go"))
      .sort()
      .reverse();
    for (const dir of goDirs) {
      paths.push(path.join(sdkDir, dir, "bin", "go"));
    }
  } catch {
    // ~/sdk doesn't exist
  }

  return paths;
}
