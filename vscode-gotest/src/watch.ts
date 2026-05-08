import * as vscode from "vscode";
import { spawn, type ChildProcess } from "node:child_process";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents, type TestEvent } from "./outputParser.js";
import { buildCliCommand, formatCliCommand, type CliCommand } from "./cli.js";
import { resolveTestItem, applyResults } from "./runnerUtils.js";

/**
 * Wraps a single `gotest watch -json <scope>` child process.
 * Handles line-buffered parsing of the JSON event stream,
 * cycle boundary detection, and auto-restart on crash.
 */
class WatchProcess implements vscode.Disposable {
  private child: ChildProcess | undefined;
  private buffer = "";
  private cycleBuffer = "";
  private disposed = false;
  private restartCount = 0;
  private lastCrashTime = 0;

  constructor(
    private readonly pkgScope: string,
    private readonly cwd: string,
    private readonly cmd: CliCommand,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onCycleStart: () => void,
    private readonly onEvents: (jsonLines: string) => void,
    private readonly onError: (msg: string) => void,
    private readonly onExit: () => void,
  ) {
    this.start();
  }

  private start(): void {
    this.outputChannel.appendLine(
      `[watch] spawning: ${formatCliCommand(this.cmd)} (cwd: ${this.cwd})`,
    );

    this.child = spawn(this.cmd.bin, this.cmd.args, { cwd: this.cwd });
    this.buffer = "";
    this.cycleBuffer = "";

    this.child.stdout?.on("data", (data: Buffer) => {
      this.buffer += data.toString();
      this.processBuffer();
    });

    this.child.stderr?.on("data", (data: Buffer) => {
      this.outputChannel.appendLine(`[watch] stderr: ${data.toString()}`);
    });

    this.child.on("close", () => {
      if (!this.disposed) {
        this.maybeRestart();
      }
    });

    this.child.on("error", (err: Error) => {
      this.outputChannel.appendLine(`[watch] process error: ${err.message}`);
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
          // Flush any accumulated cycle events before starting new cycle
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
        this.outputChannel.appendLine(`[watch] non-JSON line: ${trimmed}`);
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

    const now = Date.now();
    if (now - this.lastCrashTime < 10_000) {
      this.outputChannel.appendLine(
        `[watch] process crashed too quickly, not restarting (scope: ${this.pkgScope})`,
      );
      this.onError(
        `Watch process for "${this.pkgScope}" crashed repeatedly. Stopping.`,
      );
      this.onExit();
      return;
    }

    this.lastCrashTime = now;
    this.restartCount++;
    this.outputChannel.appendLine(
      `[watch] restarting in 2s (attempt ${this.restartCount}, scope: ${this.pkgScope})`,
    );

    setTimeout(() => {
      if (!this.disposed) {
        this.start();
      }
    }, 2000);
  }

  dispose(): void {
    this.disposed = true;

    if (this.child) {
      const child = this.child;
      this.child = undefined;

      child.kill("SIGTERM");

      // Force kill after 2s if still alive
      const forceKill = setTimeout(() => {
        if (!child.killed) {
          child.kill("SIGKILL");
        }
      }, 2000);

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
  private pendingSave: Promise<void> = Promise.resolve();
  private readonly _onDidChange = new vscode.EventEmitter<void>();
  readonly onDidChange: vscode.Event<void> = this._onDidChange.event;
  private readonly statusBar: vscode.StatusBarItem;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onCycleComplete: (jsonOutput: string) => void,
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

    const cmd = await buildCliCommand(["watch", "-json", pkgScope], cwd);

    let cycleJsonAccumulator = "";

    const watcher = new WatchProcess(
      pkgScope,
      cwd,
      cmd,
      this.outputChannel,
      // onCycleStart
      () => {
        // End previous TestRun if any
        const existingRun = this.activeRuns.get(pkgScope);
        if (existingRun) {
          existingRun.end();
        }

        if (cycleJsonAccumulator) {
          this.onCycleComplete(cycleJsonAccumulator);
        }
        this.pendingSave = this.controller.saveResults().catch((err) => {
          this.outputChannel.appendLine(`[watch] save failed: ${err}`);
        });
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
        this.outputChannel.appendLine(`[watch] error: ${msg}`);
        vscode.window.showWarningMessage(`gotest watch: ${msg}`);
      },
      // onExit
      () => {
        // End current TestRun
        const run = this.activeRuns.get(pkgScope);
        if (run) {
          run.end();
          this.activeRuns.delete(pkgScope);
        }
        this.pendingSave = this.controller.saveResults().catch((err) => {
          this.outputChannel.appendLine(`[watch] save failed: ${err}`);
        });

        // Fire cycle complete with accumulated JSON
        if (cycleJsonAccumulator) {
          this.onCycleComplete(cycleJsonAccumulator);
        }

        // Remove from map
        this.watchers.delete(pkgScope);
        this.updateStatusBar();
        this._onDidChange.fire();
      },
    );

    this.watchers.set(pkgScope, watcher);
    this.updateStatusBar();
    this._onDidChange.fire();
  }

  stop(pkgScope: string): void {
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
    for (const [scope, watcher] of this.watchers) {
      watcher.dispose();
      const run = this.activeRuns.get(scope);
      if (run) {
        run.end();
      }
    }
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
        const applied = applyResults(this.controller, run, pkgEvents, importPath, pkgDir);
        for (const r of applied) {
          this.controller.recordResult(r.itemId, r.status, r.duration);
        }
      }
    }
  }
}
