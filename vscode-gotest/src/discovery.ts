import * as vscode from "vscode";
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
    for (const pkg of this.cache.values()) {
      if (pkg.dir === dir) {
        return pkg.importPath;
      }
    }
    return undefined;
  }

  update(
    packages: DiscoverPackage[],
    fullScan: boolean,
    workspaceDir: string,
    warnings?: DiscoverWarning[],
  ): void {
    if (fullScan) {
      const resultPaths = new Set(packages.map((p) => p.importPath));
      for (const [key, dir] of this.workspaceDirs) {
        if (dir === workspaceDir && !resultPaths.has(key)) {
          this.cache.delete(key);
          this.workspaceDirs.delete(key);
        }
      }
    }
    for (const pkg of packages) {
      this.cache.set(pkg.importPath, pkg);
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

  private async execute(
    workspaceDir: string,
    patterns?: string[],
  ): Promise<void> {
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
