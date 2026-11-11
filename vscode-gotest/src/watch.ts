import * as vscode from "vscode";
import type { ChildProcess } from "node:child_process";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents } from "./outputParser.js";
import {
  buildCliCommand,
  clearBinaryCache,
  formatCliCommand,
  offerCliUpgrade,
  type CliCommand,
} from "./cli.js";
import {
  resolveAncestorItems,
  skipUnresolved,
  OnceLog,
  StreamApplier,
} from "./runnerUtils.js";
import { ManagedChild } from "./managedChild.js";
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
  private managed: ManagedChild | undefined;
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
    private readonly onCycleEnd: (exitCode: number) => void,
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

    // Decoded by ManagedChild: the watcher runs for hours and its events arrive
    // in whatever pieces the pipe delivers, boundaries falling mid-character.
    this.managed = new ManagedChild(this.cmd.bin, this.cmd.args, {
      cwd: this.cwd,
      kind: "watch",
      control: true,
    });
    this.child = this.managed.child;
    this.buffer = "";
    this.cycleBuffer = "";

    this.child.stdout?.on("data", (chunk: string) => {
      this.buffer += chunk;
      this.processBuffer();
    });

    this.child.stderr?.on("data", (chunk: string) => {
      this.outputChannel.warn(`[watch] stderr: ${chunk.trimEnd()}`);
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

        // The cycle's verdict is complete: hand on what it streamed, then
        // close it, rather than waiting for the next save to start one.
        if (event.Action === "watch-end") {
          if (this.cycleBuffer) {
            this.onEvents(this.cycleBuffer);
            this.cycleBuffer = "";
          }
          this.onCycleEnd(typeof event.Exit === "number" ? event.Exit : 0);
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

  // forceKill=false is for shutdown. VS Code's deactivate window is seconds,
  // so any kill timer long enough to respect a fixture teardown would never
  // fire, and any timer short enough to fire would cut that teardown off. The
  // CLI has its own force-kill backstop, so a stop alone is both the most and
  // the least we can honestly do; the extension host's exit closing the CLI's
  // stdin is a stop too.
  dispose(forceKill = true): void {
    this.disposed = true;

    const managed = this.managed;
    if (!managed) return;
    this.managed = undefined;
    this.child = undefined;

    this.outputChannel.info(`[watch] stopping (pid ${managed.pid})`);

    if (!forceKill) {
      // Shutdown: the stop alone, as above.
      managed.requestStop();
      managed.dispose();
      return;
    }

    // A daemon holding fixtures open by design: it gets the teardown grace.
    void managed.terminate("teardown");
  }
}

/**
 * One watch cycle: the TestRun it drives and the stream it produced. The
 * cycle's events arrive in whatever pieces the pipe delivers, so output and
 * build diagnostics are kept for the whole cycle, not per piece.
 */
export class WatchCycle {
  private stream = "";
  private readonly applier: StreamApplier;

  constructor(
    readonly run: vscode.TestRun,
    private readonly controller: GoTestController,
    cache: DiscoveryCache,
    cwd: string,
    log?: (message: string) => void,
  ) {
    this.applier = new StreamApplier(
      controller,
      run,
      (importPath) => cache.resolveImportPath(importPath),
      cwd,
      new OnceLog(log),
    );
  }

  apply(jsonLines: string): void {
    this.stream += jsonLines;
    this.applier.apply(parseTestEvents(jsonLines));
    resolveAncestorItems(this.run, this.controller);
  }

  // end closes the run and returns the cycle's stream.
  end(exitCode?: number): string {
    this.controller.testController.items.forEach((root) => {
      skipUnresolved(this.run, root, this.controller);
    });
    resolveAncestorItems(this.run, this.controller);
    if (exitCode !== undefined && exitCode !== 0) {
      this.run.appendOutput(
        `\r\n[watch] cycle ended with exit code ${exitCode}\r\n`,
      );
    }
    this.run.end();
    return this.stream;
  }
}

/**
 * Manages the lifecycle of watch processes.
 * Spawns `gotest watch -json <scope>` processes, parses streaming JSON events,
 * and updates Test Explorer results in real-time.
 */
export class WatchManager implements vscode.Disposable {
  private watchers = new Map<string, WatchProcess>();
  private activeCycles = new Map<string, WatchCycle>();
  private watchRecordIds = new Map<string, string>();
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
    let cmd: CliCommand;
    try {
      cmd = await buildCmd();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[watch] ${message}`);
      if (!offerCliUpgrade(err)) {
        vscode.window.showErrorMessage(`gotest watch: ${message}`);
      }
      return;
    }

    // finishCycle ends the open cycle, if any, and hands its stream on.
    const finishCycle = (exitCode?: number) => {
      const cycle = this.activeCycles.get(pkgScope);
      if (!cycle) return;
      this.activeCycles.delete(pkgScope);
      const stream = cycle.end(exitCode);
      this.controller.saveResults();
      if (stream) this.onCycleComplete(stream, pkgScope, cwd);
    };

    const watcher = new WatchProcess(
      pkgScope,
      cwd,
      cmd,
      buildCmd,
      this.outputChannel,
      // onCycleStart
      () => {
        // A CLI without watch-end closes a cycle only by starting the next.
        finishCycle();
        const run = this.controller.createTestRun(
          new vscode.TestRunRequest(),
          `Watch: ${pkgScope}`,
        );
        this.activeCycles.set(
          pkgScope,
          new WatchCycle(run, this.controller, this.cache, cwd, (m) =>
            this.outputChannel.warn(m),
          ),
        );
      },
      // onEvents
      (jsonLines: string) => {
        this.activeCycles.get(pkgScope)?.apply(jsonLines);
      },
      // onCycleEnd
      (exitCode: number) => finishCycle(exitCode),
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
        this.activeCycles
          .get(pkgScope)
          ?.run.appendOutput(
            "\r\n[watch] Process exited unexpectedly — results may be incomplete\r\n",
          );
        finishCycle();

        // Remove from map
        this.watchers.delete(pkgScope);
        this.updateStatusBar();
      },
    );

    this.watchers.set(pkgScope, watcher);
    const recordId = this.registry.register({
      kind: "watch",
      packages: [pkgScope],
    }).id;
    this.watchRecordIds.set(pkgScope, recordId);
    this.updateStatusBar();
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

    const cycle = this.activeCycles.get(pkgScope);
    if (cycle) {
      cycle.run.end();
      this.activeCycles.delete(pkgScope);
    }

    this.updateStatusBar();
  }

  stopAll(forceKill = true): void {
    this.outputChannel.info(
      `[watch] stopping all (${this.watchers.size} active)`,
    );
    for (const [scope, watcher] of this.watchers) {
      const recordId = this.watchRecordIds.get(scope);
      if (recordId) {
        this.registry.cancel(recordId);
      }
      watcher.dispose(forceKill);
      this.activeCycles.get(scope)?.run.end();
    }
    this.watchRecordIds.clear();
    this.watchers.clear();
    this.activeCycles.clear();
    this.updateStatusBar();
  }

  get activeCount(): number {
    return this.watchers.size;
  }

  dispose(): void {
    this.stopAll(false);
    this.statusBar.dispose();
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
}
