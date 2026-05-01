import * as vscode from "vscode";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm } from "node:fs/promises";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents } from "./outputParser.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";
import {
  collectItems,
  groupByPackage,
  getItemDepth,
  applyResults,
  spawnTestProcess,
} from "./runnerUtils.js";

const execFileAsync = promisify(execFile);

export class TestRunner {
  private _lastJsonOutput = "";
  private readonly _onDidComplete = new vscode.EventEmitter<string>();
  readonly onDidComplete: vscode.Event<string> = this._onDidComplete.event;
  private activeRun: vscode.CancellationTokenSource | undefined;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
  ) {}

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
    this._onDidComplete.dispose();
  }

  async run(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
    if (this.activeRun) {
      this.outputChannel.appendLine("[runner] cancelling previous run");
      this.activeRun.cancel();
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;
    const cancelSub = token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Run");
    this._lastJsonOutput = "";

    try {
      const items = collectItems(this.controller, request);
      if (items.length === 0) {
        run.end();
        return;
      }

      for (const item of items) {
        run.started(item);
      }

      const groups = groupByPackage(items);

      for (const [importPath, groupItems] of groups) {
        if (effectiveToken.isCancellationRequested) {
          for (const item of groupItems) {
            run.skipped(item);
          }
          continue;
        }

        const pkg = this.cache.getPackage(importPath);
        if (!pkg) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(`Package not found: ${importPath}`),
            );
          }
          continue;
        }

        const workspaceDir = this.cache.getWorkspaceDir(importPath);
        if (!workspaceDir) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(`Workspace folder not found for: ${importPath}`),
            );
          }
          continue;
        }

        const filter = this.buildRunFilter(groupItems, importPath);
        const testFlags =
          vscode.workspace
            .getConfiguration("gotest")
            .get<string[]>("testFlags") ?? [];

        let overlayDir: string | undefined;
        try {
          const overlayCmd = await buildCliCommand(["overlay", importPath], workspaceDir);
          this.outputChannel.appendLine(`[runner] ${formatCliCommand(overlayCmd)}`);
          const { stdout: overlayStdout } = await execFileAsync(
            overlayCmd.bin,
            overlayCmd.args,
            { cwd: workspaceDir },
          );
          const overlay = JSON.parse(overlayStdout) as { overlayFile: string; dir: string };
          overlayDir = overlay.dir;

          const goTestArgs = [
            "test",
            `-overlay=${overlay.overlayFile}`,
            "-count=1",
            "-json",
            importPath,
          ];
          if (filter) {
            goTestArgs.push("-run", filter);
          }
          goTestArgs.push(...testFlags);

          this.outputChannel.appendLine(`[runner] go ${goTestArgs.join(" ")}`);
          const stdout = await spawnTestProcess("go", goTestArgs, workspaceDir, effectiveToken, this.outputChannel, "runner");
          this._lastJsonOutput += stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          const events = parseTestEvents(stdout);
          applyResults(this.controller, run, events, importPath, pkg.dir);
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : String(err);
          this.outputChannel.appendLine(`[runner] error: ${message}`);
          for (const item of groupItems) {
            run.errored(item, new vscode.TestMessage(message));
          }
        } finally {
          if (overlayDir) {
            rm(overlayDir, { recursive: true, force: true }).catch(() => {});
          }
        }
      }
    } finally {
      cancelSub.dispose();
      if (this.activeRun === cts) {
        this.activeRun = undefined;
      }
      cts.dispose();
      if (this._lastJsonOutput) {
        this._onDidComplete.fire(this._lastJsonOutput);
      }
      run.end();
    }
  }

  private suiteHasFixtures(suiteName: string, importPath: string): boolean {
    const pkg = this.cache.getPackage(importPath);
    if (!pkg) {
      return false;
    }
    const suite = pkg.suites.find((s) => s.name === suiteName);
    return suite !== undefined && suite.fixtures.length > 0;
  }

  private buildRunFilter(
    items: vscode.TestItem[],
    importPath: string,
  ): string | undefined {
    if (items.some((item) => getItemDepth(item) === 0)) {
      return undefined;
    }

    const suiteGroups = new Map<
      string,
      { wholeSuite: boolean; methods: string[]; subtests: string[] }
    >();

    for (const item of items) {
      const depth = getItemDepth(item);

      if (depth === 1) {
        const suiteName = item.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteGroups.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteGroups.set(suiteName, group);
        }
        group.wholeSuite = true;
      } else if (depth === 2) {
        const suiteName = item.parent!.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteGroups.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteGroups.set(suiteName, group);
        }
        group.methods.push(item.label);
      } else if (depth >= 3) {
        let current = item;
        const subtestParts: string[] = [];
        while (getItemDepth(current) > 2) {
          subtestParts.unshift(current.label);
          current = current.parent!;
        }
        const methodName = current.label;
        const suiteName = current.parent!.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteGroups.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteGroups.set(suiteName, group);
        }
        group.subtests.push(`^Test${suiteName}$/^${methodName}$/^${subtestParts.join("/")}$`);
      }
    }

    const filters: string[] = [];
    for (const [suiteName, group] of suiteGroups) {
      if (group.wholeSuite) {
        filters.push(`^Test${suiteName}$`);
      } else if (group.subtests.length > 0) {
        filters.push(...group.subtests);
      } else if (group.methods.length === 1) {
        filters.push(`^Test${suiteName}$/^${group.methods[0]}$`);
      } else if (group.methods.length > 1) {
        filters.push(`^Test${suiteName}$/^(${group.methods.join("|")})$`);
      }
    }

    return filters.length === 0 ? undefined : filters.length === 1 ? filters[0] : filters.join("|");
  }
}
