import * as vscode from "vscode";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { CoverageStore } from "./coverageStore.js";
import { parseTestEvents, groupEventsByPackage } from "./outputParser.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";
import {
  applyResults,
  spawnTestProcess,
  computeWildcard,
  type AppliedResult,
} from "./runnerUtils.js";
import {
  runGoToolCoverFunc,
  splitCoverByPackage,
  splitFuncCoverageByPackage,
} from "./coverageUtils.js";

export interface BatchConfig {
  pkgInfos: {
    importPath: string;
    items: vscode.TestItem[];
    dir: string;
  }[];
  filter: string | undefined;
  workspaceDir: string;
  testFlags: string[];
  run: vscode.TestRun;
  token: vscode.CancellationToken;
  controller: GoTestController;
  outputChannel: vscode.OutputChannel;
  label: string;
  coverage?: {
    store: CoverageStore;
    testOnly?: boolean;
  };
  onResults?: (results: AppliedResult[]) => void;
}

export interface BatchResult {
  stdout: string;
}

export async function executeBatch(config: BatchConfig): Promise<BatchResult> {
  const {
    pkgInfos,
    filter,
    workspaceDir,
    testFlags,
    run,
    token,
    controller,
    outputChannel,
    label,
    coverage,
    onResults,
  } = config;

  const importPaths = pkgInfos.map((p) => p.importPath);
  const wildcard = filter ? undefined : computeWildcard(importPaths);
  const cliPkgArgs = wildcard ? [wildcard] : importPaths;
  let coverFile: string | undefined;

  try {
    if (coverage) {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");
    }

    const cliArgs: string[] = ["-json", "-count=1", ...cliPkgArgs];
    if (coverFile) {
      cliArgs.push("-covermode=atomic", `-coverprofile=${coverFile}`);
    }
    if (coverage?.testOnly) {
      cliArgs.push("-coverpkg=./...");
    }
    if (filter) {
      cliArgs.push("-run", filter);
    }
    cliArgs.push(...testFlags);

    const cmd = await buildCliCommand(cliArgs, workspaceDir, outputChannel);
    outputChannel.appendLine(`[${label}] ${formatCliCommand(cmd)}`);

    const result = await spawnTestProcess(
      cmd.bin,
      cmd.args,
      workspaceDir,
      token,
      outputChannel,
      label,
    );

    if (token.isCancellationRequested) {
      for (const info of pkgInfos) {
        for (const item of info.items) {
          run.skipped(item);
        }
      }
      return { stdout: result.stdout };
    }

    const events = result.stdout ? parseTestEvents(result.stdout) : [];
    const eventsByPkg = groupEventsByPackage(events);

    for (const info of pkgInfos) {
      const pkgEvents = eventsByPkg.get(info.importPath) ?? [];
      if (pkgEvents.length > 0) {
        const applied = applyResults(
          controller,
          run,
          pkgEvents,
          info.importPath,
          info.dir,
        );
        onResults?.(applied);
      }
    }

    if (result.exitCode !== 0) {
      const diagnostic = [
        result.stderr.trim(),
        result.stdout.trim(),
        `exit code ${result.exitCode}`,
      ]
        .filter(Boolean)
        .join("\n\n");

      for (const info of pkgInfos) {
        if ((eventsByPkg.get(info.importPath) ?? []).length === 0) {
          for (const item of info.items) {
            run.errored(item, new vscode.TestMessage(diagnostic));
          }
        }
      }
    }

    if (coverFile && coverage) {
      try {
        const coverContent = await readFile(coverFile, "utf-8");
        let funcOutput: string | undefined;
        try {
          funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
        } catch {
          outputChannel.appendLine(
            `[${label}] go tool cover -func failed`,
          );
        }

        if (coverage.testOnly) {
          for (const info of pkgInfos) {
            coverage.store.update(
              info.importPath,
              coverContent,
              funcOutput,
              true,
            );
          }
        } else {
          const coverByPkg = splitCoverByPackage(coverContent, importPaths);
          const funcByPkg = funcOutput
            ? splitFuncCoverageByPackage(funcOutput, importPaths)
            : undefined;

          for (const info of pkgInfos) {
            const pkgCover = coverByPkg.get(info.importPath);
            if (pkgCover) {
              coverage.store.update(
                info.importPath,
                pkgCover,
                funcByPkg?.get(info.importPath),
              );
            }
          }
        }
      } catch {
        outputChannel.appendLine(`[${label}] no coverprofile generated`);
      }
    }

    return { stdout: result.stdout };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    outputChannel.appendLine(`[${label}] error: ${message}`);
    for (const info of pkgInfos) {
      for (const item of info.items) {
        run.errored(item, new vscode.TestMessage(message));
      }
    }
    return { stdout: "" };
  } finally {
    if (coverFile) {
      rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
        () => {},
      );
    }
  }
}
