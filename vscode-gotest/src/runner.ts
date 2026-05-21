import * as vscode from "vscode";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { scopedConfig } from "./cli.js";
import {
  collectItems,
  enqueueDescendants,
  groupByPackage,
  buildRunFilter,
  resolvePackageItems,
} from "./runnerUtils.js";
import type { CoverageStore } from "./coverageStore.js";
import { executeBatch } from "./batchRunner.js";
import type { RunRegistry } from "./runRegistry.js";

export class TestRunner {
  private _lastJsonOutput = "";
  private readonly _onDidComplete = new vscode.EventEmitter<string>();
  readonly onDidComplete: vscode.Event<string> = this._onDidComplete.event;
  private activeRun: vscode.CancellationTokenSource | undefined;
  private activeRecordId: string | undefined;
  private previousRunPromise: Promise<void> = Promise.resolve();

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.LogOutputChannel,
    private readonly registry: RunRegistry,
    private readonly coverageStore?: CoverageStore,
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
      this.outputChannel.info("[runner] cancelling previous run");
      this.activeRun.cancel();
      if (this.activeRecordId) {
        this.registry.cancel(this.activeRecordId);
        this.activeRecordId = undefined;
      }
      await this.previousRunPromise;
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;
    const cancelSub = token.onCancellationRequested(() => {
      this.outputChannel.info("[runner] stop requested");
      cts.cancel();
    });
    const effectiveToken = cts.token;

    this.outputChannel.info("[runner] run started");
    const run = this.controller.createTestRun(request, "Go Test Run");
    this._lastJsonOutput = "";
    let anyCoverOnRun = false;

    let resolveRun!: () => void;
    this.previousRunPromise = new Promise<void>((r) => {
      resolveRun = r;
    });

    let recordId: string | undefined;

    try {
      const items = collectItems(this.controller, request);
      if (items.length === 0) {
        return;
      }

      recordId = this.registry.register({
        kind: "test",
        packages: items.map((i) => i.id),
      }).id;
      this.activeRecordId = recordId;

      for (const item of items) {
        this.controller.clearResults(item);
        run.started(item);
        enqueueDescendants(run, item);
      }

      const groups = groupByPackage(items);

      interface PkgInfo {
        importPath: string;
        items: vscode.TestItem[];
        dir: string;
        workspaceDir: string;
        filter: string | undefined;
      }

      const validPkgs: PkgInfo[] = [];

      for (const [importPath, groupItems] of groups) {
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
              new vscode.TestMessage(
                `Workspace folder not found for: ${importPath}`,
              ),
            );
          }
          continue;
        }

        const filter = buildRunFilter(groupItems);
        validPkgs.push({
          importPath,
          items: groupItems,
          dir: pkg.dir,
          workspaceDir,
          filter,
        });
      }

      const byWorkspace = new Map<string, PkgInfo[]>();
      for (const info of validPkgs) {
        let list = byWorkspace.get(info.workspaceDir);
        if (!list) {
          list = [];
          byWorkspace.set(info.workspaceDir, list);
        }
        list.push(info);
      }

      for (const [workspaceDir, pkgInfos] of byWorkspace) {
        if (effectiveToken.isCancellationRequested) {
          for (const info of pkgInfos) {
            for (const item of info.items) {
              run.skipped(item);
            }
          }
          continue;
        }

        const config = scopedConfig(workspaceDir);
        const testFlags = config.get<string[]>("testFlags") ?? [];
        const coverOnRun =
          this.coverageStore !== undefined &&
          (config.get<boolean>("coverOnRun") ?? true);
        if (coverOnRun) anyCoverOnRun = true;

        const unfiltered = pkgInfos.filter((p) => !p.filter);
        const filtered = pkgInfos.filter((p) => p.filter);

        if (unfiltered.length > 0) {
          await this.runBatch(
            unfiltered,
            undefined,
            workspaceDir,
            testFlags,
            coverOnRun,
            run,
            effectiveToken,
          );
        }

        for (const info of filtered) {
          if (effectiveToken.isCancellationRequested) {
            for (const item of info.items) {
              run.skipped(item);
            }
            continue;
          }
          await this.runBatch(
            [info],
            info.filter,
            workspaceDir,
            testFlags,
            coverOnRun,
            run,
            effectiveToken,
          );
        }
      }

      resolvePackageItems(run, items, this.controller);

      if (anyCoverOnRun) {
        const { coverages: allCoverages } =
          this.coverageStore!.buildFileCoverages(this.cache);
        for (const fc of allCoverages) {
          run.addCoverage(fc);
        }
        await this.coverageStore!.save();
      }
      this.controller.saveResults();
    } finally {
      const wasCancelled = effectiveToken.isCancellationRequested;
      if (recordId !== undefined && this.activeRecordId === recordId) {
        if (wasCancelled) {
          this.registry.cancel(recordId);
        } else {
          this.registry.complete(recordId);
        }
        this.activeRecordId = undefined;
      }
      resolveRun();
      cancelSub.dispose();
      if (this.activeRun === cts) {
        this.activeRun = undefined;
      }
      cts.dispose();
      if (this._lastJsonOutput) {
        this._onDidComplete.fire(this._lastJsonOutput);
      }
      run.end();
      this.outputChannel.info(`[runner] run ${wasCancelled ? "cancelled" : "completed"}`);
    }
  }

  private async runBatch(
    pkgInfos: {
      importPath: string;
      items: vscode.TestItem[];
      dir: string;
    }[],
    filter: string | undefined,
    workspaceDir: string,
    testFlags: string[],
    coverOnRun: boolean,
    run: vscode.TestRun,
    token: vscode.CancellationToken,
  ): Promise<void> {
    const result = await executeBatch({
      pkgInfos,
      filter,
      workspaceDir,
      testFlags,
      run,
      token,
      controller: this.controller,
      outputChannel: this.outputChannel,
      label: "runner",
      coverage: coverOnRun ? { store: this.coverageStore! } : undefined,
      onResults: (applied) => {
        for (const r of applied) {
          this.controller.recordResult(r.itemId, r.status, r.duration);
        }
      },
    });
    this._lastJsonOutput += result.stdout;
  }
}
