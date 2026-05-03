import * as vscode from "vscode";
import { spawn, type ChildProcess } from "node:child_process";
import type { PrepareOutput } from "./types.js";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";

export class DebugLauncher implements vscode.Disposable {
  private readonly prepareProcesses = new Map<string, ChildProcess>();
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

    const workspaceFolder = vscode.workspace.getWorkspaceFolder(
      vscode.Uri.file(pkgDir),
    );
    if (!workspaceFolder) {
      return;
    }

    let prepare: PrepareOutput;
    let child: ChildProcess;
    try {
      const cmd = await buildCliCommand(
        ["prepare", pkgDir],
        workspaceFolder.uri.fsPath,
        this.outputChannel,
      );
      this.outputChannel.appendLine(`[debug] ${formatCliCommand(cmd)}`);

      const result = await this.spawnPrepare(
        cmd.bin,
        cmd.args,
        workspaceFolder.uri.fsPath,
      );
      prepare = result.output;
      child = result.child;
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Unknown error running prepare";
      vscode.window.showErrorMessage(`gotest prepare failed: ${message}`);
      return;
    }

    const sessionName = `Go Test Suite Debug`;
    this.prepareProcesses.set(sessionName, child);

    const runFilter = buildRunFilter(items);

    const extraBuildFlags = scopedConfig(workspaceFolder.uri.fsPath).get<
      string[]
    >("buildFlags", []);

    const debugConfig: vscode.DebugConfiguration = {
      type: "go",
      name: sessionName,
      request: "launch",
      mode: "test",
      program: pkgDir,
      buildFlags:
        `-overlay=${prepare.overlayFile} ${extraBuildFlags.join(" ")}`.trim(),
      args: runFilter ? ["-test.run", runFilter] : [],
    };

    if (prepare.stateFile) {
      debugConfig.env = { GOTEST_SHARED_STATE_FILE: prepare.stateFile };
    }

    this.outputChannel.appendLine(
      `[debug] launching: ${JSON.stringify(debugConfig)}`,
    );

    const started = await vscode.debug.startDebugging(
      workspaceFolder,
      debugConfig,
    );

    if (!started) {
      this.killPrepareProcess(sessionName);
    }
  }

  registerCleanupOnSessionEnd(context: vscode.ExtensionContext): void {
    this.sessionListener = vscode.debug.onDidTerminateDebugSession(
      (session) => {
        this.killPrepareProcess(session.name);
      },
    );
    context.subscriptions.push(this.sessionListener);
  }

  dispose(): void {
    this.sessionListener?.dispose();
    for (const [name] of this.prepareProcesses) {
      this.killPrepareProcess(name);
    }
  }

  private spawnPrepare(
    bin: string,
    args: string[],
    cwd: string,
  ): Promise<{ output: PrepareOutput; child: ChildProcess }> {
    return new Promise((resolve, reject) => {
      const child = spawn(bin, args, { cwd });
      let stdout = "";
      let settled = false;

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
        if (!settled && stdout.includes("\n")) {
          settled = true;
          try {
            const output = JSON.parse(stdout.split("\n")[0]) as PrepareOutput;
            resolve({ output, child });
          } catch {
            child.kill("SIGTERM");
            reject(
              new Error(`Failed to parse prepare output: ${stdout.trim()}`),
            );
          }
        }
      });

      child.stderr.on("data", (data: Buffer) => {
        this.outputChannel.appendLine(
          `[debug:prepare] ${data.toString().trimEnd()}`,
        );
      });

      child.on("error", (err: Error) => {
        if (!settled) {
          settled = true;
          reject(err);
        }
      });

      child.on("close", (code) => {
        if (!settled) {
          settled = true;
          reject(new Error(`prepare exited with code ${code} before ready`));
        }
      });
    });
  }

  private killPrepareProcess(sessionName: string): void {
    const child = this.prepareProcesses.get(sessionName);
    if (child) {
      this.prepareProcesses.delete(sessionName);
      child.kill("SIGTERM");
    }
  }
}
