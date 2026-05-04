import * as vscode from "vscode";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents, groupEventsByPackage } from "./outputParser.js";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";
import {
  collectItems,
  groupByPackage,
  applyResults,
  spawnTestProcess,
  buildRunFilter,
} from "./runnerUtils.js";
import {
  runGoToolCoverFunc,
  splitCoverByPackage,
  splitFuncCoverageByPackage,
} from "./coverage.js";
import type { CoverageStore } from "./coverageStore.js";

export class TestRunner {
  private _lastJsonOutput = "";
  private readonly _onDidComplete = new vscode.EventEmitter<string>();
  readonly onDidComplete: vscode.Event<string> = this._onDidComplete.event;
  private activeRun: vscode.CancellationTokenSource | undefined;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
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
      this.outputChannel.appendLine("[runner] cancelling previous run");
      this.activeRun.cancel();
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;
    const cancelSub = token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Run");
    this._lastJsonOutput = "";
    let anyCoverOnRun = false;

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

        const filter = buildRunFilter(groupItems, importPath, this.cache);
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

      if (anyCoverOnRun) {
        const allCoverages = this.coverageStore!.buildFileCoverages(this.cache);
        for (const fc of allCoverages) {
          run.addCoverage(fc);
        }
        await this.coverageStore!.save();
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
    const importPaths = pkgInfos.map((p) => p.importPath);
    let coverFile: string | undefined;

    try {
      if (coverOnRun) {
        const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
        coverFile = path.join(tmpDir, "cover.out");
      }

      const cliArgs: string[] = ["-json", "-count=1", ...importPaths];
      if (coverFile) {
        cliArgs.push(`-coverprofile=${coverFile}`);
      }
      if (filter) {
        cliArgs.push("-run", filter);
      }
      cliArgs.push(...testFlags);

      const cmd = await buildCliCommand(
        cliArgs,
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.appendLine(`[runner] ${formatCliCommand(cmd)}`);

      const result = await spawnTestProcess(
        cmd.bin,
        cmd.args,
        workspaceDir,
        token,
        this.outputChannel,
        "runner",
      );
      this._lastJsonOutput += result.stdout;

      if (token.isCancellationRequested) {
        for (const info of pkgInfos) {
          for (const item of info.items) {
            run.skipped(item);
          }
        }
        return;
      }

      if (result.stdout) {
        const events = parseTestEvents(result.stdout);
        const eventsByPkg = groupEventsByPackage(events);

        for (const info of pkgInfos) {
          const pkgEvents = eventsByPkg.get(info.importPath) ?? [];
          applyResults(
            this.controller,
            run,
            pkgEvents,
            info.importPath,
            info.dir,
          );
        }
      } else if (result.exitCode !== 0) {
        const message =
          result.stderr.trim() || `gotest exited with code ${result.exitCode}`;
        for (const info of pkgInfos) {
          for (const item of info.items) {
            run.errored(item, new vscode.TestMessage(message));
          }
        }
      }

      if (coverFile) {
        try {
          const coverContent = await readFile(coverFile, "utf-8");
          let funcOutput: string | undefined;
          try {
            funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
          } catch {
            this.outputChannel.appendLine(
              "[runner] go tool cover -func failed",
            );
          }

          const coverByPkg = splitCoverByPackage(coverContent, importPaths);
          const funcByPkg = funcOutput
            ? splitFuncCoverageByPackage(funcOutput, importPaths)
            : undefined;

          for (const info of pkgInfos) {
            const pkgCover = coverByPkg.get(info.importPath);
            if (pkgCover) {
              this.coverageStore!.update(
                info.importPath,
                pkgCover,
                funcByPkg?.get(info.importPath),
              );
            }
          }
        } catch {
          this.outputChannel.appendLine("[runner] no coverprofile generated");
        }
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[runner] error: ${message}`);
      for (const info of pkgInfos) {
        for (const item of info.items) {
          run.errored(item, new vscode.TestMessage(message));
        }
      }
    } finally {
      if (coverFile) {
        rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
          () => {},
        );
      }
    }
  }
}
