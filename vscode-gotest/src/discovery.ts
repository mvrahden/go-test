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

  update(packages: DiscoverPackage[]): void {
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

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
  ) {}

  async discover(workspaceDir: string, patterns?: string[]): Promise<void> {
    if (this.running) {
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
      if (output.packages && output.packages.length > 0) {
        this.cache.update(output.packages);
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[discovery] error: ${message}`);
    } finally {
      this.running = false;
    }
  }

  async discoverPackage(
    workspaceDir: string,
    pkgPattern: string,
  ): Promise<void> {
    return this.discover(workspaceDir, [pkgPattern]);
  }
}
