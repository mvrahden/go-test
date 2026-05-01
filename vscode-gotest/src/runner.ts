import * as vscode from "vscode";
import { execFile, spawn } from "node:child_process";
import { promisify } from "node:util";
import { rm } from "node:fs/promises";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import {
  parseTestEvents,
  extractTestMessages,
  type TestEvent,
  type TestMessage,
} from "./outputParser.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";

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
    token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Run");
    this._lastJsonOutput = "";

    try {
      const items = this.collectItems(request);
      if (items.length === 0) {
        run.end();
        return;
      }

      // Mark all items as started
      for (const item of items) {
        run.started(item);
      }

      // Group items by package
      const groups = this.groupByPackage(items);

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

        const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
        if (!workspaceDir) {
          continue;
        }

        const filter = this.buildRunFilter(groupItems, importPath);
        const testFlags =
          vscode.workspace
            .getConfiguration("gotest")
            .get<string[]>("testFlags") ?? [];

        let overlayDir: string | undefined;
        try {
          const overlayCmd = await buildCliCommand(["overlay", importPath]);
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
          const stdout = await this.spawnProcess("go", goTestArgs, workspaceDir, effectiveToken);
          this._lastJsonOutput += stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          const events = parseTestEvents(stdout);
          this.applyResults(run, events, importPath, pkg.dir);
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

  private collectItems(request: vscode.TestRunRequest): vscode.TestItem[] {
    const items: vscode.TestItem[] = [];

    if (request.include && request.include.length > 0) {
      for (const item of request.include) {
        items.push(item);
      }
    } else {
      // Collect all items from the controller
      this.controller.testController.items.forEach((item) => {
        items.push(item);
      });
    }

    return items;
  }

  private groupByPackage(
    items: vscode.TestItem[],
  ): Map<string, vscode.TestItem[]> {
    const groups = new Map<string, vscode.TestItem[]>();

    for (const item of items) {
      const root = this.getRootItem(item);
      const importPath = root.id;

      let group = groups.get(importPath);
      if (!group) {
        group = [];
        groups.set(importPath, group);
      }
      group.push(item);
    }

    return groups;
  }

  private getRootItem(item: vscode.TestItem): vscode.TestItem {
    let current = item;
    while (current.parent) {
      current = current.parent;
    }
    return current;
  }

  private getItemDepth(item: vscode.TestItem): number {
    let depth = 0;
    let current = item;
    while (current.parent) {
      current = current.parent;
      depth++;
    }
    return depth;
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
    if (items.some((item) => this.getItemDepth(item) === 0)) {
      return undefined;
    }

    const suiteGroups = new Map<
      string,
      { wholeSuite: boolean; methods: string[]; subtests: string[] }
    >();

    for (const item of items) {
      const depth = this.getItemDepth(item);

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
        while (this.getItemDepth(current) > 2) {
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

  private spawnProcess(
    cliPath: string,
    args: string[],
    cwd: string,
    token: vscode.CancellationToken,
  ): Promise<string> {
    return new Promise<string>((resolve) => {
      const child = spawn(cliPath, args, { cwd });
      let stdout = "";
      let stderr = "";

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
      });

      child.stderr.on("data", (data: Buffer) => {
        stderr += data.toString();
      });

      const cancelListener = token.onCancellationRequested(() => {
        child.kill("SIGTERM");
      });

      child.on("close", (code) => {
        cancelListener.dispose();
        this.outputChannel.appendLine(`[runner] exited code=${code} stdout=${stdout.length}b stderr=${stderr.length}b`);
        if (stdout.length < 1000) {
          this.outputChannel.appendLine(`[runner] stdout: ${stdout}`);
        }
        if (stderr) {
          this.outputChannel.appendLine(`[runner] stderr: ${stderr}`);
        }
        resolve(stdout);
      });

      child.on("error", (err: Error) => {
        cancelListener.dispose();
        this.outputChannel.appendLine(`[runner] error: ${err.message}`);
        resolve(stdout);
      });
    });
  }

  private applyResults(
    run: vscode.TestRun,
    events: TestEvent[],
    importPath: string,
    pkgDir: string,
  ): void {
    // Collect output per test for extracting failure messages
    const outputMap = new Map<string, string>();

    for (const event of events) {
      if (event.Action === "output" && event.Test) {
        const existing = outputMap.get(event.Test) ?? "";
        outputMap.set(event.Test, existing + (event.Output ?? ""));
      }
    }

    for (const event of events) {
      if (!event.Test) {
        continue;
      }

      const item = this.resolveTestItem(event.Test, importPath);
      if (!item) {
        continue;
      }

      const duration =
        event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;

      switch (event.Action) {
        case "pass":
          run.passed(item, duration);
          break;
        case "fail": {
          const output = outputMap.get(event.Test) ?? "";
          const testMessages = extractTestMessages(output, pkgDir);
          const vscodeMessages = testMessages.map((msg) => {
            const message = new vscode.TestMessage(msg.message);
            message.location = new vscode.Location(
              vscode.Uri.file(msg.file),
              new vscode.Position(msg.line - 1, 0),
            );
            return message;
          });
          if (vscodeMessages.length === 0) {
            vscodeMessages.push(
              new vscode.TestMessage(output || "Test failed"),
            );
          }
          run.failed(item, vscodeMessages, duration);
          break;
        }
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
