import * as vscode from "vscode";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { DiscoverOutput, DiscoverPackage } from "./types.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";

const execFileAsync = promisify(execFile);

export class DiscoveryCache implements vscode.Disposable {
  private cache = new Map<string, DiscoverPackage>();
  private _onDidUpdate = new vscode.EventEmitter<void>();

  readonly onDidUpdate: vscode.Event<void> = this._onDidUpdate.event;

  get packages(): DiscoverPackage[] {
    return Array.from(this.cache.values());
  }

  getPackage(importPath: string): DiscoverPackage | undefined {
    return this.cache.get(importPath);
  }

  resolveImportPath(importPath: string): string | undefined {
    const pkg = this.getPackage(importPath);
    return pkg?.dir;
  }

  resolveFileToPackage(filePath: string): string | undefined {
    const dir = filePath.substring(0, filePath.lastIndexOf("/"));
    for (const pkg of this.cache.values()) {
      if (pkg.dir === dir) {
        return pkg.importPath;
      }
    }
    return undefined;
  }

  update(packages: DiscoverPackage[], fullScan: boolean): void {
    if (fullScan) {
      const resultPaths = new Set(packages.map((p) => p.importPath));
      for (const key of this.cache.keys()) {
        if (!resultPaths.has(key)) {
          this.cache.delete(key);
        }
      }
    }
    for (const pkg of packages) {
      this.cache.set(pkg.importPath, pkg);
    }
    this._onDidUpdate.fire();
  }

  clear(): void {
    this.cache.clear();
    this._onDidUpdate.fire();
  }

  dispose(): void {
    this._onDidUpdate.dispose();
  }
}

export class DiscoveryService {
  private running = false;
  private pending: { workspaceDir: string; patterns?: string[] } | undefined;
  private hasShownError = false;

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
  ) {}

  async discover(workspaceDir: string, patterns?: string[]): Promise<void> {
    if (this.running) {
      this.pending = { workspaceDir, patterns };
      return;
    }

    this.running = true;
    try {
      const cmd = await buildCliCommand(["discover", ...(patterns ?? ["./..."])]);
      this.outputChannel.appendLine(`[discovery] ${formatCliCommand(cmd)}`);

      const { stdout } = await execFileAsync(cmd.bin, cmd.args, {
        cwd: workspaceDir,
        timeout: 30_000,
      });

      const output: DiscoverOutput = JSON.parse(stdout);
      const effectivePatterns = patterns ?? ["./..."];
      const fullScan = effectivePatterns.some((p) => p.includes("..."));
      if (output.packages && output.packages.length > 0) {
        this.cache.update(output.packages, fullScan);
      } else if (fullScan) {
        this.cache.clear();
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[discovery] error: ${message}`);
      if (!this.hasShownError) {
        this.hasShownError = true;
        vscode.window.showWarningMessage(
          `Go Test Suites: discovery failed. Ensure 'go' is installed and the gotest module is accessible.`,
          "Open Output",
        ).then((choice) => {
          if (choice === "Open Output") {
            this.outputChannel.show();
          }
        });
      }
    } finally {
      this.running = false;
      if (this.pending) {
        const next = this.pending;
        this.pending = undefined;
        this.discover(next.workspaceDir, next.patterns);
      }
    }
  }

  async discoverPackage(
    workspaceDir: string,
    pkgPattern: string,
  ): Promise<void> {
    return this.discover(workspaceDir, [pkgPattern]);
  }
}
