import * as vscode from "vscode";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { parseTestEvents } from "./outputParser.js";
import {
  buildCliCommand,
  formatCliCommand,
  resolveGoBinary,
  scopedConfig,
} from "./cli.js";
import { startSharedSetup, type SharedSetupProcess } from "./sharedFixtures.js";
import type { OverlayOutput } from "./types.js";
import {
  collectItems,
  groupByPackage,
  applyResults,
  spawnTestProcess,
  buildRunFilter,
} from "./runnerUtils.js";
import { runGoToolCoverFunc } from "./coverage.js";
import type { CoverageStore } from "./coverageStore.js";

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
              new vscode.TestMessage(
                `Workspace folder not found for: ${importPath}`,
              ),
            );
          }
          continue;
        }

        const filter = buildRunFilter(groupItems, importPath, this.cache);
        const config = scopedConfig(workspaceDir);
        const testFlags = config.get<string[]>("testFlags") ?? [];
        const coverOnRun =
          this.coverageStore !== undefined &&
          (config.get<boolean>("coverOnRun") ?? true);
        if (coverOnRun) anyCoverOnRun = true;

        let overlayDir: string | undefined;
        let coverFile: string | undefined;
        let sharedSetup: SharedSetupProcess | undefined;
        try {
          const overlayCmd = await buildCliCommand(
            ["overlay", importPath],
            workspaceDir,
            this.outputChannel,
          );
          this.outputChannel.appendLine(
            `[runner] ${formatCliCommand(overlayCmd)}`,
          );
          const { stdout: overlayStdout } = await execFileAsync(
            overlayCmd.bin,
            overlayCmd.args,
            { cwd: workspaceDir },
          );
          const overlay = JSON.parse(overlayStdout) as OverlayOutput;
          overlayDir = overlay.dir;
          if (overlay.sharedFixtures && overlay.sharedFixtures.length > 0) {
            const setupCmd = await buildCliCommand(
              ["shared-setup", `--dir=${overlay.dir}`],
              workspaceDir,
              this.outputChannel,
            );
            this.outputChannel.appendLine(
              `[runner] ${formatCliCommand(setupCmd)}`,
            );
            sharedSetup = await startSharedSetup(
              setupCmd,
              workspaceDir,
              overlay.sharedFixtures,
              this.outputChannel,
            );
          }

          if (coverOnRun) {
            const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
            coverFile = path.join(tmpDir, "cover.out");
          }

          const buildTags = config.get<string>("buildTags", "").trim();
          const goTestArgs = [
            "test",
            `-overlay=${overlay.overlayFile}`,
            "-count=1",
            "-json",
            importPath,
          ];
          if (coverFile) {
            goTestArgs.push(`-coverprofile=${coverFile}`);
          }
          if (buildTags) {
            goTestArgs.push(`-tags=${buildTags}`);
          }
          if (filter) {
            goTestArgs.push("-run", filter);
          }
          goTestArgs.push(...testFlags);

          const testEnv: Record<string, string> | undefined = sharedSetup
            ? { GOTEST_SHARED_STATE_FILE: sharedSetup.stateFile }
            : undefined;

          const goBin = await resolveGoBinary(this.outputChannel, workspaceDir);
          this.outputChannel.appendLine(
            `[runner] ${goBin} ${goTestArgs.join(" ")}`,
          );
          const stdout = await spawnTestProcess(
            goBin,
            goTestArgs,
            workspaceDir,
            effectiveToken,
            this.outputChannel,
            "runner",
            testEnv,
          );
          this._lastJsonOutput += stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          const events = parseTestEvents(stdout);
          applyResults(this.controller, run, events, importPath, pkg.dir);

          if (coverFile) {
            try {
              const coverContent = await readFile(coverFile, "utf-8");
              let funcOutput: string | undefined;
              try {
                funcOutput = await runGoToolCoverFunc(
                  goBin,
                  coverFile,
                  workspaceDir,
                );
              } catch {
                this.outputChannel.appendLine(
                  "[runner] go tool cover -func failed",
                );
              }
              this.coverageStore!.update(importPath, coverContent, funcOutput);
            } catch {
              this.outputChannel.appendLine(
                "[runner] no coverprofile generated",
              );
            }
          }
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : String(err);
          this.outputChannel.appendLine(`[runner] error: ${message}`);
          for (const item of groupItems) {
            run.errored(item, new vscode.TestMessage(message));
          }
        } finally {
          sharedSetup?.dispose();
          if (overlayDir) {
            rm(overlayDir, { recursive: true, force: true }).catch(() => {});
          }
          if (coverFile) {
            rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
              () => {},
            );
          }
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
}
