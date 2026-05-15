import * as vscode from "vscode";
import * as path from "node:path";
import { access } from "node:fs/promises";
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
    private readonly outputChannel: vscode.OutputChannel,
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

  private async execute(
    workspaceDir: string,
    patterns?: string[],
  ): Promise<void> {
    if (!(await this.isGoWorkspace(workspaceDir))) {
      this.outputChannel.appendLine(
        `[discovery] skipping non-Go workspace: ${workspaceDir}`,
      );
      return;
    }

    try {
      const cmd = await buildCliCommand(
        ["discover", ...(patterns ?? ["./..."])],
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.appendLine(`[discovery] ${formatCliCommand(cmd)}`);

      const { stdout } = await execFileAsync(cmd.bin, cmd.args, {
        cwd: workspaceDir,
        timeout: 30_000,
      });

      const output: DiscoverOutput = JSON.parse(stdout);
      const effectivePatterns = patterns ?? ["./..."];
      const fullScan = effectivePatterns.some((p) => p.includes("..."));
      const warnings = output.warnings ?? [];
      const packages = output.packages ?? [];
      this.cache.update(packages, fullScan, workspaceDir, warnings);
      this.hasShownError = false;
      for (const w of warnings) {
        const loc = w.file ? ` (${w.file}:${w.line ?? 0})` : "";
        this.outputChannel.appendLine(
          `[discovery] warning: ${w.importPath}${loc}: ${w.message}`,
        );
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[discovery] error: ${message}`);
      if (!this.hasShownError) {
        this.hasShownError = true;
        vscode.window
          .showWarningMessage(
            `Go Test Suites: discovery failed. Ensure 'go' is installed and the gotest module is accessible.`,
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
