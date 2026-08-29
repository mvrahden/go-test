import * as vscode from "vscode";
import { ManagedChild } from "./managedChild.js";
import type { PrepareOutput } from "./types.js";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";
import { killProcessTree } from "./runnerUtils.js";

export class DebugLauncher implements vscode.Disposable {
  private readonly prepareProcesses = new Map<string, ManagedChild>();
  private sessionListener: vscode.Disposable | undefined;

  constructor(private readonly outputChannel: vscode.LogOutputChannel) {}

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
    let managed: ManagedChild;
    try {
      const cmd = await buildCliCommand(
        ["prepare", pkgDir],
        workspaceFolder.uri.fsPath,
        this.outputChannel,
      );
      this.outputChannel.info(`[debug] ${formatCliCommand(cmd)}`);

      const timeout =
        scopedConfig(workspaceFolder.uri.fsPath).get<number>(
          "debug.prepareTimeout",
        ) ?? 60;
      const result = await this.spawnPrepare(
        cmd.bin,
        cmd.args,
        workspaceFolder.uri.fsPath,
        timeout,
        token,
      );
      prepare = result.output;
      managed = result.managed;
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Unknown error running prepare";
      vscode.window.showErrorMessage(`gotest prepare failed: ${message}`);
      return;
    }

    const prepareKey = `gotest-prepare:${pkgDir}`;
    this.killPrepareProcess(prepareKey);
    this.prepareProcesses.set(prepareKey, managed);

    const runFilter = buildRunFilter(items);

    const extraBuildFlags = scopedConfig(workspaceFolder.uri.fsPath).get<
      string[]
    >("buildFlags", []);

    const debugConfig: vscode.DebugConfiguration = {
      type: "go",
      name: "Go Test Suite Debug",
      request: "launch",
      mode: "test",
      program: pkgDir,
      buildFlags:
        `-overlay=${prepare.overlayFile} ${extraBuildFlags.join(" ")}`.trim(),
      args: runFilter ? ["-test.run", runFilter] : [],
      __prepareKey: prepareKey,
    };

    if (prepare.stateFile) {
      debugConfig.env = { GOTEST_SHARED_STATE_FILE: prepare.stateFile };
    }

    this.outputChannel.info(
      `[debug] launching: ${JSON.stringify(debugConfig)}`,
    );

    const started = await vscode.debug.startDebugging(
      workspaceFolder,
      debugConfig,
    );

    if (!started) {
      this.killPrepareProcess(prepareKey);
    }
  }

  registerCleanupOnSessionEnd(context: vscode.ExtensionContext): void {
    this.sessionListener = vscode.debug.onDidTerminateDebugSession(
      (session) => {
        const key = (session.configuration as Record<string, unknown>)
          ?.__prepareKey;
        if (typeof key === "string") {
          this.killPrepareProcess(key);
        }
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
    timeoutSeconds: number,
    token: vscode.CancellationToken,
  ): Promise<{ output: PrepareOutput; managed: ManagedChild }> {
    return new Promise((resolve, reject) => {
      // A daemon: `prepare` blocks until SIGTERM holding shared fixtures open,
      // so it gets the teardown grace when it is eventually ended.
      const managed = new ManagedChild(bin, args, { cwd });
      const child = managed.child;
      let stdout = "";
      let stderr = "";
      let settled = false;
      const settle = (fn: () => void) => {
        if (!settled) {
          settled = true;
          clearTimeout(timer);
          cancelListener.dispose();
          fn();
        }
      };

      const timer = setTimeout(() => {
        settle(() => {
          void managed.terminate("teardown");
          reject(new Error(`timed out after ${timeoutSeconds}s`));
        });
      }, timeoutSeconds * 1000);

      const cancelListener = token.onCancellationRequested(() => {
        settle(() => {
          void managed.terminate("teardown");
          reject(new Error("cancelled"));
        });
      });

      child.stdout?.on("data", (chunk: string) => {
        stdout += chunk;
        if (!settled && stdout.includes("\n")) {
          settle(() => {
            try {
              const output = JSON.parse(
                stdout.split("\n")[0].trim(),
              ) as PrepareOutput;
              resolve({ output, managed });
            } catch {
              void managed.terminate("teardown");
              reject(
                new Error(`Failed to parse prepare output: ${stdout.trim()}`),
              );
            }
          });
        }
      });

      child.stderr?.on("data", (chunk: string) => {
        stderr += chunk;
        this.outputChannel.debug(`[debug:prepare] ${chunk.trimEnd()}`);
      });

      child.on("error", (err: Error) => {
        settle(() => reject(err));
      });

      child.on("close", (code) => {
        // prepare fails fast on packages that do not build; its stderr carries
        // the compiler diagnostics, which are the only actionable part.
        settle(() => {
          const detail = stderr.trim();
          reject(
            new Error(
              `prepare exited with code ${code} before ready${detail ? `:\n${detail}` : ""}`,
            ),
          );
        });
      });
    });
  }

  private killPrepareProcess(sessionName: string): void {
    const managed = this.prepareProcesses.get(sessionName);
    if (managed) {
      this.prepareProcesses.delete(sessionName);
      void managed.terminate("teardown");
    }
  }
}
