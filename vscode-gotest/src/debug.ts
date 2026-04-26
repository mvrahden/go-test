import * as vscode from "vscode";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm } from "node:fs/promises";
import type { OverlayOutput } from "./types.js";

const execFileAsync = promisify(execFile);

export class DebugLauncher implements vscode.Disposable {
  private readonly overlayDirs = new Set<string>();
  private sessionListener: vscode.Disposable | undefined;

  constructor(private readonly outputChannel: vscode.OutputChannel) {}

  async debug(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
    buildRunFilter: (items: readonly vscode.TestItem[]) => string | undefined,
    getPackageDir: (item: vscode.TestItem) => string | undefined,
  ): Promise<void> {
    const items = request.include;
    if (!items || items.length === 0) {
      return;
    }

    const pkgDir = getPackageDir(items[0]);
    if (!pkgDir) {
      return;
    }

    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    if (!workspaceFolder) {
      return;
    }

    const cliPath = vscode.workspace
      .getConfiguration("gotest")
      .get<string>("cliPath", "gotest");

    // Generate overlay
    let overlay: OverlayOutput;
    try {
      this.outputChannel.appendLine(`[debug] ${cliPath} overlay ${pkgDir}`);
      const { stdout } = await execFileAsync(cliPath, ["overlay", pkgDir], {
        cwd: workspaceFolder.uri.fsPath,
      });
      overlay = JSON.parse(stdout) as OverlayOutput;
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Unknown error generating overlay";
      vscode.window.showErrorMessage(`gotest overlay failed: ${message}`);
      return;
    }

    // Track overlay dir for cleanup
    this.overlayDirs.add(overlay.dir);

    // Build run filter
    const runFilter = buildRunFilter(items);

    // Build extra build flags
    const extraBuildFlags = vscode.workspace
      .getConfiguration("gotest")
      .get<string[]>("buildFlags", []);

    // Construct debug configuration
    const debugConfig: vscode.DebugConfiguration = {
      type: "go",
      name: "Go Test Suite Debug",
      request: "launch",
      mode: "test",
      program: pkgDir,
      buildFlags: `-overlay=${overlay.overlayFile} ${extraBuildFlags.join(" ")}`.trim(),
      args: runFilter ? ["-test.run", runFilter] : [],
    };

    this.outputChannel.appendLine(
      `[debug] launching: ${JSON.stringify(debugConfig)}`,
    );

    // Launch debug session
    const started = await vscode.debug.startDebugging(
      workspaceFolder,
      debugConfig,
    );

    if (!started) {
      // Clean up overlay immediately on failure
      this.cleanupOverlayDir(overlay.dir);
    }
  }

  registerCleanupOnSessionEnd(context: vscode.ExtensionContext): void {
    this.sessionListener = vscode.debug.onDidTerminateDebugSession((session) => {
      if (session.name === "Go Test Suite Debug") {
        this.cleanupAllOverlays();
      }
    });
    context.subscriptions.push(this.sessionListener);
  }

  dispose(): void {
    this.sessionListener?.dispose();
    this.cleanupAllOverlays();
  }

  private cleanupAllOverlays(): void {
    for (const dir of this.overlayDirs) {
      this.cleanupOverlayDir(dir);
    }
  }

  private cleanupOverlayDir(dir: string): void {
    this.overlayDirs.delete(dir);
    rm(dir, { recursive: true, force: true }).catch((err: unknown) => {
      this.outputChannel.appendLine(
        `[debug] failed to clean up overlay dir ${dir}: ${err}`,
      );
    });
  }
}
