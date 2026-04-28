import * as vscode from "vscode";
import { spawn, type ChildProcess } from "node:child_process";
import type { GoTestController } from "./testController.js";
import { parseTestEvents } from "./outputParser.js";
import { buildCliCommand, formatCliCommand, type CliCommand } from "./cli.js";

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
  private readonly _onDidChange = new vscode.EventEmitter<void>();
  readonly onDidChange: vscode.Event<void> = this._onDidChange.event;
  private readonly statusBar: vscode.StatusBarItem;

  constructor(
    private readonly controller: GoTestController,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onCycleComplete: (jsonOutput: string) => void,
  ) {
    this.statusBar = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      40,
    );
    this.statusBar.command = "gotest.stopWatch";
  }

  start(pkgScope: string, cwd: string): void {
    // Kill existing watcher for same scope
    if (this.watchers.has(pkgScope)) {
      this.stop(pkgScope);
    }

    const cmd = buildCliCommand(["watch", "-json", pkgScope]);

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
          this.applyWatchEvents(run, jsonLines, pkgScope);
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

  private applyWatchEvents(
    run: vscode.TestRun,
    jsonLines: string,
    importPath: string,
  ): void {
    const events = parseTestEvents(jsonLines);
    for (const event of events) {
      if (!event.Test) {
        continue;
      }

      const item = this.resolveTestItem(event.Test, importPath);
      if (!item) {
        continue;
      }

      switch (event.Action) {
        case "pass":
          run.passed(item, event.Elapsed ? event.Elapsed * 1000 : undefined);
          break;
        case "fail":
          run.failed(
            item,
            [new vscode.TestMessage("Test failed")],
            event.Elapsed ? event.Elapsed * 1000 : undefined,
          );
          break;
        case "skip":
          run.skipped(item);
          break;
        case "run":
          run.started(item);
          break;
      }
    }
  }

  /**
   * Resolve a test event's Test field to a TestItem.
   *
   * The Test field format is: TestSuiteName/MethodName/SubtestPath
   * The generated code wraps suites as `func TestSuiteName(t)`,
   * so we strip the "Test" prefix from the first segment to find the suite.
   */
  private resolveTestItem(
    testPath: string,
    importPath: string,
  ): vscode.TestItem | undefined {
    const segments = testPath.split("/");
    if (segments.length === 0) {
      return undefined;
    }

    // First segment is "TestSuiteName" — strip "Test" prefix to get suite name
    const firstSegment = segments[0];
    const suiteName = firstSegment.startsWith("Test")
      ? firstSegment.slice(4)
      : firstSegment;

    // Find suite item
    const suiteId = `${importPath}/${suiteName}`;
    const suiteItem = this.controller.findItem(suiteId);
    if (!suiteItem) {
      return undefined;
    }

    if (segments.length === 1) {
      return suiteItem;
    }

    // Second segment is the method name
    const methodName = segments[1];
    const methodId = `${suiteId}/${methodName}`;
    const methodItem = this.controller.findItem(methodId);
    if (!methodItem) {
      return undefined;
    }

    if (segments.length === 2) {
      return methodItem;
    }

    // Deeper segments are dynamic subtests
    let parentItem = methodItem;
    for (let i = 2; i < segments.length; i++) {
      const subtestLabel = segments[i];
      const subtestPath = segments.slice(2, i + 1).join("/");
      parentItem = this.controller.createDynamicSubtest(
        parentItem,
        subtestPath,
        subtestLabel,
      );
    }

    return parentItem;
  }
}
