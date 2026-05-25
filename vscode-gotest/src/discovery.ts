import * as vscode from "vscode";
import * as path from "node:path";
import { access, readFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type {
  DiscoverOutput,
  DiscoverPackage,
  DiscoverWarning,
} from "./types.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";

const execFileAsync = promisify(execFile);

export class DiscoveryCache implements vscode.Disposable {
  private cache = new Map<string, DiscoverPackage>();
  private workspaceDirs = new Map<string, string>();
  private dirIndex = new Map<string, string>();
  private _warnings: (DiscoverWarning & { _wsDir: string })[] = [];
  private _onDidUpdate = new vscode.EventEmitter<void>();

  readonly onDidUpdate: vscode.Event<void> = this._onDidUpdate.event;

  get packages(): DiscoverPackage[] {
    return Array.from(this.cache.values());
  }

  get warnings(): DiscoverWarning[] {
    return this._warnings;
  }

  getPackage(importPath: string): DiscoverPackage | undefined {
    return this.cache.get(importPath);
  }

  getWorkspaceDir(importPath: string): string | undefined {
    return this.workspaceDirs.get(importPath);
  }

  resolveImportPath(importPath: string): string | undefined {
    const pkg = this.getPackage(importPath);
    return pkg?.dir;
  }

  resolveFileToPackage(filePath: string): string | undefined {
    const lastSep = Math.max(
      filePath.lastIndexOf("/"),
      filePath.lastIndexOf("\\"),
    );
    const dir = lastSep >= 0 ? filePath.substring(0, lastSep) : filePath;
    return this.dirIndex.get(dir);
  }

  update(
    packages: DiscoverPackage[],
    fullScan: boolean,
    workspaceDir: string,
    warnings?: DiscoverWarning[],
  ): void {
    if (fullScan) {
      const resultPaths = new Set(packages.map((p) => p.importPath));
      for (const [key, wsDir] of this.workspaceDirs) {
        if (wsDir === workspaceDir && !resultPaths.has(key)) {
          const pkg = this.cache.get(key);
          if (pkg) this.dirIndex.delete(pkg.dir);
          this.cache.delete(key);
          this.workspaceDirs.delete(key);
        }
      }
    }
    for (const pkg of packages) {
      const prev = this.cache.get(pkg.importPath);
      if (prev && prev.dir !== pkg.dir) this.dirIndex.delete(prev.dir);
      this.cache.set(pkg.importPath, pkg);
      this.dirIndex.set(pkg.dir, pkg.importPath);
      this.workspaceDirs.set(pkg.importPath, workspaceDir);
    }
    if (warnings !== undefined) {
      this._warnings = this._warnings.filter((w) => w._wsDir !== workspaceDir);
      for (const w of warnings) {
        this._warnings.push({ ...w, _wsDir: workspaceDir });
      }
    }
    this._onDidUpdate.fire();
  }

  clear(): void {
    this.cache.clear();
    this.dirIndex.clear();
    this.workspaceDirs.clear();
    this._warnings = [];
    this._onDidUpdate.fire();
  }

  dispose(): void {
    this._onDidUpdate.dispose();
  }
}

export class DiscoveryService {
  private running = false;
  private pending: {
    workspaceDir: string;
    patterns?: string[];
    resolve: () => void;
  }[] = [];
  private hasShownError = false;

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.LogOutputChannel,
  ) {}

  async discover(workspaceDir: string, patterns?: string[]): Promise<void> {
    if (this.running) {
      const existing = this.pending.findIndex(
        (p) => p.workspaceDir === workspaceDir,
      );
      if (existing >= 0) {
        this.pending[existing].resolve();
        this.pending.splice(existing, 1);
      }
      return new Promise<void>((resolve) => {
        this.pending.push({ workspaceDir, patterns, resolve });
      });
    }

    this.running = true;
    try {
      await this.execute(workspaceDir, patterns);
    } finally {
      this.running = false;
      const next = this.pending.shift();
      if (next) {
        this.discover(next.workspaceDir, next.patterns).then(next.resolve);
      }
    }
  }

  async discoverPackage(
    workspaceDir: string,
    pkgPattern: string,
  ): Promise<void> {
    return this.discover(workspaceDir, [pkgPattern]);
  }

  private async isGoWorkspace(dir: string): Promise<boolean> {
    for (const name of ["go.mod", "go.work"]) {
      try {
        await access(path.join(dir, name));
        return true;
      } catch {}
    }
    return false;
  }

  private async resolveWorkspacePatterns(
    workspaceDir: string,
  ): Promise<string[]> {
    try {
      const content = await readFile(
        path.join(workspaceDir, "go.work"),
        "utf-8",
      );
      const usePattern = /^\s*use\s*\(\s*([\s\S]*?)\s*\)/m;
      const blockMatch = usePattern.exec(content);
      if (blockMatch) {
        const dirs = blockMatch[1]
          .split("\n")
          .map((l) => l.trim())
          .filter((l) => l && !l.startsWith("//"));
        if (dirs.length > 1) {
          return dirs.map((d) => `${d}/...`);
        }
      }
      const singleUse = /^\s*use\s+(\S+)/gm;
      const singles: string[] = [];
      let m: RegExpExecArray | null;
      while ((m = singleUse.exec(content)) !== null) {
        singles.push(m[1]);
      }
      if (singles.length > 1) {
        return singles.map((d) => `${d}/...`);
      }
    } catch {}
    return ["./..."];
  }

  private static readonly MAX_RETRIES = 3;
  private static readonly RETRY_DELAYS_MS = [2_000, 4_000];

  private async execute(
    workspaceDir: string,
    patterns?: string[],
  ): Promise<void> {
    if (!(await this.isGoWorkspace(workspaceDir))) {
      this.outputChannel.info(
        `[discovery] skipping non-Go workspace: ${workspaceDir}`,
      );
      return;
    }

    const effectivePatterns =
      patterns ?? (await this.resolveWorkspacePatterns(workspaceDir));

    for (let attempt = 1; attempt <= DiscoveryService.MAX_RETRIES; attempt++) {
      if (this.pending.some((p) => p.workspaceDir === workspaceDir)) {
        this.outputChannel.debug(
          `[discovery] superseded by queued request, aborting retry`,
        );
        return;
      }

      try {
        const cmd = await buildCliCommand(
          ["discover", ...effectivePatterns],
          workspaceDir,
          this.outputChannel,
        );
        this.outputChannel.info(`[discovery] ${formatCliCommand(cmd)}`);

        const { stdout } = await execFileAsync(cmd.bin, cmd.args, {
          cwd: workspaceDir,
          timeout: 30_000,
        });

        const jsonStart = stdout.indexOf("{");
        const json = jsonStart > 0 ? stdout.substring(jsonStart) : stdout;
        const output: DiscoverOutput = JSON.parse(json);
        const fullScan = effectivePatterns.some((p) => p.includes("..."));
        const warnings = output.warnings ?? [];
        const packages = output.packages ?? [];
        this.cache.update(packages, fullScan, workspaceDir, warnings);
        this.hasShownError = false;
        for (const w of warnings) {
          const loc = w.file ? ` (${w.file}:${w.line ?? 0})` : "";
          this.outputChannel.warn(
            `[discovery] ${w.importPath}${loc}: ${w.message}`,
          );
        }
        return;
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        if (attempt < DiscoveryService.MAX_RETRIES) {
          this.outputChannel.debug(
            `[discovery] attempt ${attempt}/${DiscoveryService.MAX_RETRIES} failed, retrying: ${message}`,
          );
          const delay = DiscoveryService.RETRY_DELAYS_MS[attempt - 1] ?? 4_000;
          await new Promise((r) => setTimeout(r, delay));
          continue;
        }
        this.outputChannel.error(
          `[discovery] failed after ${DiscoveryService.MAX_RETRIES} attempts: ${message}`,
        );
        if (!this.hasShownError) {
          this.hasShownError = true;
          vscode.window
            .showWarningMessage(
              `gotest: discovery failed. Ensure 'go' is installed and the gotest module is accessible.`,
              "Open Output",
            )
            .then((choice) => {
              if (choice === "Open Output") {
                this.outputChannel.show();
              }
            });
        }
      }
    }
  }
}
