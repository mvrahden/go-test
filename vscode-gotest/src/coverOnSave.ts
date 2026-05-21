import * as vscode from "vscode";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { CoverageStore } from "./coverageStore.js";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";
import { spawnTestProcess } from "./runnerUtils.js";
import { runGoToolCoverFunc } from "./coverageUtils.js";
import type { RunRegistry } from "./runRegistry.js";

export class CoverOnSave implements vscode.Disposable {
  private activeRun: vscode.CancellationTokenSource | undefined;
  private activeRecordId: string | undefined;
  private previousRunPromise: Promise<void> = Promise.resolve();
  private runQueue = Promise.resolve();
  private pendingPackages = new Set<string>();

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly store: CoverageStore,
    private readonly outputChannel: vscode.LogOutputChannel,
    private readonly registry: RunRegistry,
  ) {}

  run(importPath: string): Promise<void> {
    if (this.pendingPackages.has(importPath)) return this.runQueue;
    this.pendingPackages.add(importPath);
    this.runQueue = this.runQueue.then(async () => {
      this.pendingPackages.delete(importPath);
      await this.execute(importPath);
    });
    return this.runQueue;
  }

  private async execute(importPath: string): Promise<void> {
    if (this.activeRun) {
      this.activeRun.cancel();
      if (this.activeRecordId) {
        this.registry.cancel(this.activeRecordId);
        this.activeRecordId = undefined;
      }
      await this.previousRunPromise;
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;

    const pkg = this.cache.getPackage(importPath);
    if (!pkg) return;
    const workspaceDir = this.cache.getWorkspaceDir(importPath);
    if (!workspaceDir) return;

    const recordId = this.registry.register({
      kind: "coverage",
      packages: [importPath],
    }).id;
    this.activeRecordId = recordId;

    let resolveRun!: () => void;
    this.previousRunPromise = new Promise<void>((r) => {
      resolveRun = r;
    });

    const config = scopedConfig(workspaceDir);
    const testFlags = config.get<string[]>("testFlags") ?? [];
    let coverFile: string | undefined;

    try {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");

      const gotestArgs: string[] = [];
      const goTestArgs: string[] = [
        "-count=1",
        "-covermode=atomic",
        `-coverprofile=${coverFile}`,
        importPath,
      ];
      for (const flag of testFlags) {
        if (flag.startsWith("--")) {
          gotestArgs.push(flag);
        } else {
          goTestArgs.push(flag);
        }
      }
      const cliArgs = [...gotestArgs, "--", ...goTestArgs];

      const cmd = await buildCliCommand(
        cliArgs,
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.info(`[coverage:save] ${formatCliCommand(cmd)}`);

      await spawnTestProcess(
        cmd.bin,
        cmd.args,
        workspaceDir,
        cts.token,
        this.outputChannel,
        "coverage",
      );

      if (cts.token.isCancellationRequested) return;

      const coverContent = await readFile(coverFile, "utf-8");
      let funcOutput: string | undefined;
      try {
        funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
      } catch {
        this.outputChannel.warn("[coverage:save] go tool cover -func failed");
      }
      this.store.update(importPath, coverContent, funcOutput);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[coverage:save] ${message}`);
      return;
    } finally {
      if (this.activeRecordId === recordId) {
        this.registry.complete(recordId);
        this.activeRecordId = undefined;
      }
      resolveRun();
      if (this.activeRun === cts) this.activeRun = undefined;
      cts.dispose();
      if (coverFile)
        rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
          () => {},
        );
    }

    const request = new vscode.TestRunRequest();
    const run = this.controller.createTestRun(request, "Cover on Save");
    const { coverages: allCoverages } = this.store.buildFileCoverages(
      this.cache,
    );
    for (const fc of allCoverages) {
      run.addCoverage(fc);
    }
    run.end();
    await this.store.save();
  }

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
  }
}
