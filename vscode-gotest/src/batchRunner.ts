import * as vscode from "vscode";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { CoverageStore } from "./coverageStore.js";
import { type TestEvent } from "./outputParser.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";
import {
  applyResults,
  spawnTestProcess,
  resolveRunPatterns,
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
  outputChannel: vscode.LogOutputChannel;
  label: string;
  env?: Record<string, string>;
  moduleDir?: string;
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
    env,
    moduleDir,
    coverage,
    onResults,
  } = config;

  const importPaths = pkgInfos.map((p) => p.importPath);
  const modulePath = await readModulePath(moduleDir ?? workspaceDir);
  let cliPkgArgs: string[];
  let effectiveCwd = workspaceDir;
  if (filter) {
    cliPkgArgs = importPaths;
  } else {
    const resolved = resolveRunPatterns(
      importPaths,
      modulePath,
      moduleDir,
      workspaceDir,
    );
    cliPkgArgs = resolved.patterns;
    effectiveCwd = resolved.cwd;
  }
  let coverFile: string | undefined;

  try {
    if (coverage) {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");
    }

    const gotestArgs: string[] = [];
    const goTestArgs: string[] = ["-json", "-count=1", ...cliPkgArgs];
    if (coverFile) {
      goTestArgs.push("-covermode=atomic", `-coverprofile=${coverFile}`);
    }
    if (coverage?.testOnly) {
      goTestArgs.push("-coverpkg=./...");
    }
    if (filter) {
      goTestArgs.push("-run", filter);
    }
    for (const flag of testFlags) {
      if (flag.startsWith("--")) {
        gotestArgs.push(flag);
      } else {
        goTestArgs.push(flag);
      }
    }

    const cliArgs = [...gotestArgs, "--", ...goTestArgs];
    const cmd = await buildCliCommand(cliArgs, effectiveCwd, outputChannel);
    outputChannel.info(`[${label}] ${formatCliCommand(cmd)}`);

    const streamedPkgs = new Set<string>();
    const pkgEventBuffers = new Map<string, TestEvent[]>();
    const pkgDirMap = new Map(pkgInfos.map((p) => [p.importPath, p.dir]));

    const handleStdoutLine = (line: string) => {
      let event: TestEvent;
      try {
        event = JSON.parse(line) as TestEvent;
        if (!event.Action) return;
      } catch {
        return;
      }

      let buffer = pkgEventBuffers.get(event.Package);
      if (!buffer) {
        buffer = [];
        pkgEventBuffers.set(event.Package, buffer);
      }
      buffer.push(event);

      const isTerminal =
        !event.Test &&
        (event.Action === "pass" ||
          event.Action === "fail" ||
          event.Action === "skip");
      if (isTerminal) {
        const dir = pkgDirMap.get(event.Package);
        if (dir) {
          const applied = applyResults(
            controller,
            run,
            buffer,
            event.Package,
            dir,
          );
          onResults?.(applied);
          streamedPkgs.add(event.Package);
        }
        pkgEventBuffers.set(event.Package, []);
      }
    };

    const result = await spawnTestProcess(
      cmd.bin,
      cmd.args,
      effectiveCwd,
      token,
      outputChannel,
      label,
      env,
      handleStdoutLine,
    );

    for (const [pkg, buffer] of pkgEventBuffers) {
      if (buffer.length > 0) {
        const dir = pkgDirMap.get(pkg);
        if (dir) {
          const applied = applyResults(controller, run, buffer, pkg, dir);
          onResults?.(applied);
          const hasTerminal = buffer.some(
            (e) =>
              !e.Test &&
              (e.Action === "pass" ||
                e.Action === "fail" ||
                e.Action === "skip"),
          );
          if (hasTerminal) {
            streamedPkgs.add(pkg);
          }
        }
      }
    }

    if (token.isCancellationRequested) {
      const skippedPkgs = pkgInfos.filter(
        (i) => !streamedPkgs.has(i.importPath),
      );
      if (skippedPkgs.length > 0) {
        outputChannel.info(
          `[${label}] cancelled, skipping ${skippedPkgs.length} remaining package(s)`,
        );
      }
      for (const info of skippedPkgs) {
        for (const item of info.items) {
          run.skipped(item);
        }
      }
      return { stdout: result.stdout };
    }

    if (result.exitCode !== 0) {
      const stderrFiltered = result.stderr
        .split("\n")
        .filter((line) => !/^exit status \d+$/.test(line.trim()))
        .join("\n")
        .trim();

      if (stderrFiltered) {
        run.appendOutput(stderrFiltered.replace(/\n/g, "\r\n") + "\r\n");
      }

      for (const info of pkgInfos) {
        if (streamedPkgs.has(info.importPath)) {
          continue;
        }

        const diagnostic = [
          stderrFiltered,
          result.stdout.trim(),
          `exit code ${result.exitCode}`,
        ]
          .filter(Boolean)
          .join("\n\n");
        for (const item of info.items) {
          run.errored(item, new vscode.TestMessage(diagnostic));
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
          outputChannel.warn(`[${label}] go tool cover -func failed`);
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
        outputChannel.warn(`[${label}] no coverprofile generated`);
      }
    }

    return { stdout: result.stdout };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    outputChannel.error(`[${label}] ${message}`);
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

async function readModulePath(dir: string): Promise<string | undefined> {
  try {
    const content = await readFile(path.join(dir, "go.mod"), "utf-8");
    const match = /^\s*module\s+(\S+)/m.exec(content);
    return match?.[1];
  } catch {
    return undefined;
  }
}
