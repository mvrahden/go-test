import * as vscode from "vscode";
import { spawn, type ChildProcess } from "node:child_process";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents, type TestEvent } from "./outputParser.js";
import {
  buildCliCommand,
  clearBinaryCache,
  formatCliCommand,
  type CliCommand,
} from "./cli.js";
import {
  resolveTestItem,
  applyResults,
  killProcessTree,
  resolveAncestorItems,
  skipUnresolved,
} from "./runnerUtils.js";
import type { RunRegistry } from "./runRegistry.js";

/**
 * Wraps a single `gotest watch -json <scope>` child process.
 * Handles line-buffered parsing of the JSON event stream,
 * cycle boundary detection, and auto-restart on crash.
 */
class WatchProcess implements vscode.Disposable {
  private static readonly MAX_CONSECUTIVE_CRASHES = 5;
  private static readonly BASE_RESTART_DELAY_MS = 2000;
  private static readonly MAX_RESTART_DELAY_MS = 30_000;

  private child: ChildProcess | undefined;
  private buffer = "";
  private cycleBuffer = "";
  private disposed = false;
  private consecutiveCrashes = 0;
  private needsResolve = false;

  private cmd: CliCommand;

  constructor(
    private readonly pkgScope: string,
    private readonly cwd: string,
    cmd: CliCommand,
    private readonly buildCmd: () => Promise<CliCommand>,
    private readonly outputChannel: vscode.LogOutputChannel,
    private readonly onCycleStart: () => void,
    private readonly onEvents: (jsonLines: string) => void,
    private readonly onError: (msg: string) => void,
    private readonly onExit: () => void,
  ) {
    this.cmd = cmd;
    this.start();
  }

  private start(): void {
    this.outputChannel.info(
      `[watch] spawning: ${formatCliCommand(this.cmd)} (cwd: ${this.cwd})`,
    );

    this.child = spawn(this.cmd.bin, this.cmd.args, {
      cwd: this.cwd,
      detached: true,
    });
    this.buffer = "";
    this.cycleBuffer = "";

    this.child.stdout?.on("data", (data: Buffer) => {
      this.buffer += data.toString();
      this.processBuffer();
    });

    this.child.stderr?.on("data", (data: Buffer) => {
      this.outputChannel.warn(`[watch] stderr: ${data.toString().trimEnd()}`);
    });

    this.child.on("close", () => {
      if (!this.disposed) {
        this.maybeRestart();
      }
    });

    this.child.on("error", (err: Error) => {
      this.outputChannel.error(`[watch] process error: ${err.message}`);
      if ("code" in err && err.code === "ENOENT") {
        clearBinaryCache();
        this.needsResolve = true;
      }
      if (!this.disposed) {
        this.maybeRestart();
      }
    });
  }

  private processBuffer(): void {
    const lines = this.buffer.split("\n");
    // Keep incomplete last line in buffer
    this.buffer = lines.pop() ?? "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) {
        continue;
      }

      try {
        const event = JSON.parse(trimmed);

        if (event.Action === "watch-start") {
          this.consecutiveCrashes = 0;
          if (this.cycleBuffer) {
            this.onEvents(this.cycleBuffer);
            this.cycleBuffer = "";
          }
          this.onCycleStart();
          continue;
        }

        if (event.Action === "watch-error") {
          this.onError(event.Output ?? "Unknown watch error");
          continue;
        }

        // Accumulate event line in cycle buffer
        this.cycleBuffer += trimmed + "\n";
      } catch {
        // Non-JSON line, skip
        this.outputChannel.debug(`[watch] non-JSON line: ${trimmed}`);
      }
    }

    // Flush accumulated events at end of data chunk
    if (this.cycleBuffer) {
      this.onEvents(this.cycleBuffer);
      this.cycleBuffer = "";
    }
  }

  private maybeRestart(): void {
    if (this.disposed) {
      return;
    }

    const autoRestart =
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("watch.autoRestart") ?? true;

    if (!autoRestart) {
      this.onExit();
      return;
    }

    this.consecutiveCrashes++;

    if (this.consecutiveCrashes >= WatchProcess.MAX_CONSECUTIVE_CRASHES) {
      this.outputChannel.error(
        `[watch] ${this.consecutiveCrashes} consecutive crashes, stopping (scope: ${this.pkgScope})`,
      );
      this.onError(
        `Watch process for "${this.pkgScope}" crashed ${this.consecutiveCrashes} times. Stopping.`,
      );
      this.onExit();
      return;
    }

    const delay = Math.min(
      WatchProcess.BASE_RESTART_DELAY_MS *
        Math.pow(2, this.consecutiveCrashes - 1),
      WatchProcess.MAX_RESTART_DELAY_MS,
    );

    this.outputChannel.warn(
      `[watch] restarting in ${delay / 1000}s (crash ${this.consecutiveCrashes}/${WatchProcess.MAX_CONSECUTIVE_CRASHES}, scope: ${this.pkgScope})`,
    );

    setTimeout(async () => {
      if (this.disposed) return;
      if (this.needsResolve) {
        this.needsResolve = false;
        try {
          this.cmd = await this.buildCmd();
        } catch {
          this.onExit();
          return;
        }
      }
      this.start();
    }, delay);
  }

  dispose(): void {
    this.disposed = true;

    if (this.child) {
      const child = this.child;
      this.child = undefined;

      this.outputChannel.info(`[watch] sending SIGTERM (pid ${child.pid})`);
      killProcessTree(child, "SIGTERM");

      const killTimeout =
        vscode.workspace
          .getConfiguration("gotest")
          .get<number>("forceKillTimeout", 600) * 1000;
      const forceKill = setTimeout(() => {
        if (!child.killed) {
          killProcessTree(child, "SIGKILL");
        }
      }, killTimeout);

      child.on("close", () => {
        clearTimeout(forceKill);
      });
    }
  }
}

/**
 * Manages the lifecycle of watch processes.
 * Spawns `gotest watch -json <scope>` processes, parses streaming JSON events,
 * and updates Test Explorer results in real-time.
 */
export class WatchManager implements vscode.Disposable {
  private watchers = new Map<string, WatchProcess>();
  private activeRuns = new Map<string, vscode.TestRun>();
  private watchRecordIds = new Map<string, string>();
  private readonly _onDidChange = new vscode.EventEmitter<void>();
  readonly onDidChange: vscode.Event<void> = this._onDidChange.event;
  private readonly statusBar: vscode.StatusBarItem;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.LogOutputChannel,
    private readonly onCycleComplete: (
      jsonOutput: string,
      scope: string,
      cwd: string,
    ) => void,
    private readonly registry: RunRegistry,
  ) {
    this.statusBar = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      40,
    );
    this.statusBar.command = "gotest.stopWatch";
  }

  async start(pkgScope: string, cwd: string): Promise<void> {
    // Kill existing watcher for same scope
    if (this.watchers.has(pkgScope)) {
      this.stop(pkgScope);
    }

    const buildCmd = () =>
      buildCliCommand(["watch", "--", "-json", pkgScope], cwd);
    const cmd = await buildCmd();

    let cycleJsonAccumulator = "";

    const watcher = new WatchProcess(
      pkgScope,
      cwd,
      cmd,
      buildCmd,
      this.outputChannel,
      // onCycleStart
      () => {
        // End previous TestRun if any
        const existingRun = this.activeRuns.get(pkgScope);
        if (existingRun) {
          this.controller.testController.items.forEach((root) => {
            skipUnresolved(existingRun, root, this.controller);
          });
          resolveAncestorItems(existingRun, this.controller);
          existingRun.end();
        }

        if (cycleJsonAccumulator) {
          this.onCycleComplete(cycleJsonAccumulator, pkgScope, cwd);
        }
        this.controller.saveResults();
        cycleJsonAccumulator = "";

        // Create new TestRun
        const run = this.controller.createTestRun(
          new vscode.TestRunRequest(),
          `Watch: ${pkgScope}`,
        );
        this.activeRuns.set(pkgScope, run);
      },
      // onEvents
      (jsonLines: string) => {
        cycleJsonAccumulator += jsonLines;
        const run = this.activeRuns.get(pkgScope);
        if (run) {
          this.applyWatchEvents(run, jsonLines);
        }
      },
      // onError
      (msg: string) => {
        this.outputChannel.error(`[watch] ${msg}`);
        vscode.window.showWarningMessage(`gotest watch: ${msg}`);
      },
      // onExit
      () => {
        const recordId = this.watchRecordIds.get(pkgScope);
        if (recordId) {
          this.registry.crash(recordId);
          this.watchRecordIds.delete(pkgScope);
        }
        const run = this.activeRuns.get(pkgScope);
        if (run) {
          run.appendOutput(
            "\r\n[watch] Process exited unexpectedly — results may be incomplete\r\n",
          );
          run.end();
          this.activeRuns.delete(pkgScope);
        }
        this.controller.saveResults();

        // Fire cycle complete with accumulated JSON
        if (cycleJsonAccumulator) {
          this.onCycleComplete(cycleJsonAccumulator, pkgScope, cwd);
        }

        // Remove from map
        this.watchers.delete(pkgScope);
        this.updateStatusBar();
        this._onDidChange.fire();
      },
    );

    this.watchers.set(pkgScope, watcher);
    const recordId = this.registry.register({
      kind: "watch",
      packages: [pkgScope],
    }).id;
    this.watchRecordIds.set(pkgScope, recordId);
    this.updateStatusBar();
    this._onDidChange.fire();
  }

  stop(pkgScope: string): void {
    this.outputChannel.info(`[watch] stop requested for ${pkgScope}`);
    const recordId = this.watchRecordIds.get(pkgScope);
    if (recordId) {
      this.registry.cancel(recordId);
      this.watchRecordIds.delete(pkgScope);
    }
    const watcher = this.watchers.get(pkgScope);
    if (watcher) {
      watcher.dispose();
      this.watchers.delete(pkgScope);
    }

    const run = this.activeRuns.get(pkgScope);
    if (run) {
      run.end();
      this.activeRuns.delete(pkgScope);
    }

    this.updateStatusBar();
    this._onDidChange.fire();
  }

  stopAll(): void {
    this.outputChannel.info(
      `[watch] stopping all (${this.watchers.size} active)`,
    );
    for (const [scope, watcher] of this.watchers) {
      const recordId = this.watchRecordIds.get(scope);
      if (recordId) {
        this.registry.cancel(recordId);
      }
      watcher.dispose();
      const run = this.activeRuns.get(scope);
      if (run) {
        run.end();
      }
    }
    this.watchRecordIds.clear();
    this.watchers.clear();
    this.activeRuns.clear();
    this.updateStatusBar();
    this._onDidChange.fire();
  }

  get activeCount(): number {
    return this.watchers.size;
  }

  isWatching(pkgScope: string): boolean {
    return this.watchers.has(pkgScope);
  }

  dispose(): void {
    this.stopAll();
    this.statusBar.dispose();
    this._onDidChange.dispose();
  }

  private updateStatusBar(): void {
    const count = this.watchers.size;
    if (count === 0) {
      this.statusBar.hide();
    } else {
      this.statusBar.text = `$(eye) gotest watch (${count})`;
      this.statusBar.tooltip = `${count} active watch process${count > 1 ? "es" : ""} — click to stop all`;
      this.statusBar.show();
    }
  }

  private applyWatchEvents(run: vscode.TestRun, jsonLines: string): void {
    const events = parseTestEvents(jsonLines);

    const byPackage = new Map<string, TestEvent[]>();
    for (const event of events) {
      let group = byPackage.get(event.Package);
      if (!group) {
        group = [];
        byPackage.set(event.Package, group);
      }
      group.push(event);
    }

    for (const [importPath, pkgEvents] of byPackage) {
      const pkgDir = this.cache.resolveImportPath(importPath);
      if (pkgDir) {
        applyResults(this.controller, run, pkgEvents, importPath, pkgDir);
      }
    }

    resolveAncestorItems(run, this.controller);
  }
}
