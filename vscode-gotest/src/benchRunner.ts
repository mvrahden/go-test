// benchRunner drives `gotest bench --json` for the Bench run profile and the
// bench CodeLenses. Benchmarks are deliberate acts: nothing here hooks into
// save events or watch mode, and invocations run strictly serially — timing
// numbers taken in parallel are noise.

import * as vscode from "vscode";
import { spawn } from "node:child_process";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import {
  buildCliCommand,
  buildBenchArgs,
  formatCliCommand,
  type CliCommand,
} from "./cli.js";
import {
  parseBenchReport,
  formatBenchAnnotation,
  type BenchReport,
} from "./benchReport.js";
import type { BenchResultStore } from "./benchResultStore.js";

export interface BenchTarget {
  importPath: string;
  suiteName: string;
  /** Absent = every benchmark method in the suite. */
  methodName?: string;
}

/**
 * planBenchInvocations dedups a selection into the runs actually needed: a
 * suite-level target subsumes its method-level targets, and duplicates
 * collapse. Order is preserved (first occurrence wins) because benchmarks
 * run serially and the user watches them land one by one.
 */
export function planBenchInvocations(targets: BenchTarget[]): BenchTarget[] {
  const suiteLevel = new Set<string>();
  for (const t of targets) {
    if (!t.methodName) suiteLevel.add(`${t.importPath}/${t.suiteName}`);
  }
  const seen = new Set<string>();
  const plan: BenchTarget[] = [];
  for (const t of targets) {
    const suiteKey = `${t.importPath}/${t.suiteName}`;
    if (t.methodName && suiteLevel.has(suiteKey)) continue;
    const key = t.methodName ? `${suiteKey}/${t.methodName}` : suiteKey;
    if (seen.has(key)) continue;
    seen.add(key);
    plan.push(t);
  }
  return plan;
}

/**
 * benchTargetFromItemId decodes a benchmark TestItem id
 * ("<importPath>/<Suite>/<BenchmarkMethod>"). Import paths contain slashes,
 * suite and method names never do, so splitting from the right is exact.
 */
export function benchTargetFromItemId(id: string): BenchTarget | undefined {
  const parts = id.split("/");
  if (parts.length < 3) return undefined;
  const methodName = parts[parts.length - 1];
  const suiteName = parts[parts.length - 2];
  const importPath = parts.slice(0, -2).join("/");
  if (!methodName.startsWith("Benchmark")) return undefined;
  return { importPath, suiteName, methodName };
}

export class BenchRunner {
  private active: vscode.CancellationTokenSource | undefined;

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly store: BenchResultStore,
    private readonly outputChannel: vscode.LogOutputChannel,
    /** Called with every parsed report — gate diagnostics hang off this. */
    private readonly onReport?: (report: BenchReport) => void,
  ) {}

  dispose(): void {
    this.active?.cancel();
    this.active = undefined;
  }

  /** Handler for the tag-scoped "Bench" run profile. */
  async runProfile(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
    const items = (request.include ?? []).filter((i) =>
      i.tags.some((t) => t.id === "benchmark"),
    );
    if (items.length === 0) return;

    const targets: BenchTarget[] = [];
    const byKey = new Map<string, vscode.TestItem>();
    for (const item of items) {
      const target = benchTargetFromItemId(item.id);
      if (!target) continue;
      targets.push(target);
      byKey.set(item.id, item);
    }

    const run = this.controller.createTestRun(request, "Go Bench Run");
    for (const item of items) run.started(item);
    try {
      await this.execute(planBenchInvocations(targets), token, {
        onResult: (importPath, suiteName, methodName, ok, message) => {
          const item = byKey.get(`${importPath}/${suiteName}/${methodName}`);
          if (!item) return;
          if (ok) {
            run.passed(item);
          } else {
            run.failed(item, new vscode.TestMessage(message ?? "bench failed"));
          }
        },
        onError: (target, message) => {
          for (const item of items) {
            const t = benchTargetFromItemId(item.id);
            if (
              t &&
              t.importPath === target.importPath &&
              t.suiteName === target.suiteName &&
              (!target.methodName || t.methodName === target.methodName)
            ) {
              run.errored(item, new vscode.TestMessage(message));
            }
          }
        },
      });
    } finally {
      run.end();
    }
  }

  /** Entry point for the bench CodeLenses and commands. */
  async runTarget(target: BenchTarget): Promise<void> {
    const cts = new vscode.CancellationTokenSource();
    this.active?.cancel();
    this.active = cts;
    try {
      await this.execute([target], cts.token, {});
    } finally {
      if (this.active === cts) this.active = undefined;
      cts.dispose();
    }
  }

  /**
   * saveBaseline runs every benchmark in the workspace and saves a baseline.
   * The path comes from bench.baseline in .gotest.yml — resolved by the CLI,
   * never parsed here — with a save dialog as the fallback when the project
   * has no configured baseline.
   */
  async saveBaseline(workspaceDir: string): Promise<void> {
    const first = await this.runWorkspace(workspaceDir, ["--save="]);
    if (first.ok) {
      vscode.window.showInformationMessage("Bench baseline saved.");
      return;
    }
    if (!/--save needs a path/.test(first.error)) {
      vscode.window.showErrorMessage(`gotest bench failed: ${first.error}`);
      return;
    }
    const picked = await vscode.window.showSaveDialog({
      title: "Save Bench Baseline",
      filters: { "Bench baseline": ["json"] },
    });
    if (!picked) return;
    const second = await this.runWorkspace(workspaceDir, [
      `--save=${picked.fsPath}`,
    ]);
    if (second.ok) {
      vscode.window.showInformationMessage(
        `Bench baseline saved to ${picked.fsPath}.`,
      );
    } else {
      vscode.window.showErrorMessage(`gotest bench failed: ${second.error}`);
    }
  }

  /**
   * compareBaseline runs every benchmark in the workspace and compares. The
   * CLI compares against bench.baseline automatically when configured; when
   * the run comes back without deltas, the user picks a baseline file and
   * the comparison reruns explicitly.
   */
  async compareBaseline(workspaceDir: string): Promise<void> {
    let outcome = await this.runWorkspace(workspaceDir, []);
    if (outcome.ok && !outcome.report.deltas) {
      const picked = await vscode.window.showOpenDialog({
        title: "Compare vs Bench Baseline",
        canSelectMany: false,
        filters: { "Bench baseline": ["json"] },
      });
      if (!picked || picked.length === 0) return;
      outcome = await this.runWorkspace(workspaceDir, [
        `--against=${picked[0].fsPath}`,
      ]);
    }
    if (!outcome.ok) {
      vscode.window.showErrorMessage(`gotest bench failed: ${outcome.error}`);
      return;
    }

    const deltas = outcome.report.deltas ?? [];
    const significant = deltas.filter((d) => d.significant).length;
    const gate = outcome.report.gate;
    if (gate?.breached) {
      vscode.window.showWarningMessage(
        `Bench gate breached: ${gate.worstKey} +${gate.worstPct.toFixed(1)}% exceeds ${gate.thresholdPct}%.`,
      );
    } else {
      vscode.window.showInformationMessage(
        `Compared ${deltas.length} benchmark${deltas.length === 1 ? "" : "s"}: ${
          significant === 0
            ? "no significant change"
            : `${significant} significant change${significant === 1 ? "" : "s"}`
        }.`,
      );
    }
  }

  /**
   * runWorkspace runs `gotest bench ./... --json` with extra flags in one
   * workspace, records the report, and surfaces the outcome to the caller
   * instead of the UI — the baseline commands own their own messaging.
   */
  private async runWorkspace(
    workspaceDir: string,
    extra: string[],
  ): Promise<{ ok: true; report: BenchReport } | { ok: false; error: string }> {
    const cmd = await buildCliCommand(
      ["bench", "./...", ...extra, "--json"],
      workspaceDir,
      this.outputChannel,
    );
    this.outputChannel.info(`[bench] ${formatCliCommand(cmd)}`);

    const cts = new vscode.CancellationTokenSource();
    this.active?.cancel();
    this.active = cts;
    try {
      const { stdout, stderr, code } = await this.spawnBench(
        cmd,
        workspaceDir,
        cts.token,
      );
      if (stderr.trim()) this.outputChannel.warn(stderr.trimEnd());
      if (!stdout.trim()) {
        return {
          ok: false,
          error: stderr.trim() || `gotest bench exited with code ${code}`,
        };
      }
      const report = parseBenchReport(stdout);
      this.recordAndLog(report);
      return { ok: true, report };
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[bench] failed: ${message}`);
      return { ok: false, error: message };
    } finally {
      if (this.active === cts) this.active = undefined;
      cts.dispose();
    }
  }

  private async execute(
    plan: BenchTarget[],
    token: vscode.CancellationToken,
    hooks: {
      onResult?: (
        importPath: string,
        suiteName: string,
        methodName: string,
        ok: boolean,
        message?: string,
      ) => void;
      onError?: (target: BenchTarget, message: string) => void;
    },
  ): Promise<void> {
    for (const target of plan) {
      if (token.isCancellationRequested) return;

      const workspaceDir = this.cache.getWorkspaceDir(target.importPath);
      if (!workspaceDir) {
        const msg = `no workspace dir for ${target.importPath}`;
        this.outputChannel.error(`[bench] ${msg}`);
        hooks.onError?.(target, msg);
        continue;
      }

      const cmd = await buildCliCommand(
        [
          ...buildBenchArgs(
            target.importPath,
            target.suiteName,
            target.methodName,
          ),
          "--json",
        ],
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.info(`[bench] ${formatCliCommand(cmd)}`);

      let report: BenchReport;
      try {
        const { stdout, stderr, code } = await this.spawnBench(
          cmd,
          workspaceDir,
          token,
        );
        if (stderr.trim()) this.outputChannel.warn(stderr.trimEnd());
        if (code !== 0 && !stdout.trim()) {
          throw new Error(`gotest bench exited with code ${code}`);
        }
        report = parseBenchReport(stdout);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        this.outputChannel.error(`[bench] failed: ${message}`);
        hooks.onError?.(target, message);
        if (!hooks.onError) {
          vscode.window.showErrorMessage(`gotest bench failed: ${message}`);
        }
        continue;
      }

      this.recordAndLog(report);
      for (const result of report.baseline.results) {
        hooks.onResult?.(result.package, result.suite, result.name, true);
      }
    }
  }

  /**
   * recordAndLog persists the report and writes the supplementary log: one
   * line per method with the same numbers the annotation shows, so the
   * channel remains a readable record of every run.
   */
  private recordAndLog(report: BenchReport): void {
    const now = Date.now();
    this.store.recordReport(report, now);
    this.onReport?.(report);

    for (const result of report.baseline.results) {
      const n = result.samples.length;
      const mean =
        result.samples.reduce((sum, s) => sum + s.nsPerOp, 0) / Math.max(1, n);
      const last = result.samples[n - 1];
      this.outputChannel.info(
        `[bench] ${result.suite}/${result.name}: ${formatBenchAnnotation(
          {
            nsPerOp: mean,
            bytesPerOp: last?.bytesPerOp ?? 0,
            allocsPerOp: last?.allocsPerOp ?? 0,
            iterations: last?.iterations ?? 0,
          },
          now,
          now,
        )}`,
      );
    }
  }

  private spawnBench(
    cmd: CliCommand,
    cwd: string,
    token: vscode.CancellationToken,
  ): Promise<{ stdout: string; stderr: string; code: number | null }> {
    return new Promise((resolve, reject) => {
      const child = spawn(cmd.bin, cmd.args, { cwd });
      const sub = token.onCancellationRequested(() => child.kill("SIGTERM"));
      let stdout = "";
      let stderr = "";

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
      });
      child.stderr.on("data", (data: Buffer) => {
        stderr += data.toString();
      });
      child.on("close", (code) => {
        sub.dispose();
        resolve({ stdout, stderr, code });
      });
      child.on("error", (err: Error) => {
        sub.dispose();
        reject(err);
      });
    });
  }
}
