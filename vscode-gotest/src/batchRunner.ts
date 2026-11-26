import * as vscode from "vscode";
import * as path from "node:path";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { CoverageStore } from "./coverageStore.js";
import { type TestEvent } from "./outputParser.js";
import { buildCliCommand, clearBinaryCache, formatCliCommand } from "./cli.js";
import { readModulePath } from "./gomod.js";
import {
  OnceLog,
  StreamApplier,
  enqueueDescendants,
  startAncestors,
  getPackageItem,
  skipUnresolved,
  spawnTestProcess,
  resolveRunPatterns,
  readWorkspacePatterns,
  type AppliedResult,
  erroredItem,
  itemTestPath,
  markFailedRun,
} from "./runnerUtils.js";
import { coverOf, type RunPiece } from "./specStore.js";
import { nextEventsFile, pruneEventsFiles } from "./eventsFiles.js";
import { liveResults } from "./config.js";
import type { RunProgress } from "./liveProgress.js";
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
  // Run with --changed: packages the CLI leaves out are neither skipped
  // nor errored; they keep what they have.
  changed?: boolean;
  coverage?: {
    store: CoverageStore;
    testOnly?: boolean;
  };
  onResults?: (results: AppliedResult[]) => void;
  // Every discovered package: a widened pattern reaching one outside the
  // selection would let its focus skip the selected suites. Without it the
  // patterns widen unchecked.
  knownPackages?: { importPath: string; dir: string }[];
  // The run this invocation belongs to, told of each event as it applies.
  progress?: RunProgress;
}

// liveEnv is the environment of a run child: GOTEST_LIVE=1 unless the
// folder turned live results off. The CLI reads it only for a -json run.
export function liveEnv(
  workspaceDir: string,
  env: Record<string, string> | undefined,
): Record<string, string> | undefined {
  if (!liveResults(workspaceDir)) return env;
  return { ...env, GOTEST_LIVE: "1" };
}

export interface BatchResult {
  stdout: string;
  // The run as the Spec View keeps it.
  piece: RunPiece;
}

// runPiece says which packages a filtered batch ran in part, and what of
// them the filter asked for.
export function runPiece(
  stdout: string,
  config: Pick<BatchConfig, "pkgInfos" | "filter" | "workspaceDir">,
  at = Date.now(),
): RunPiece {
  const piece: RunPiece = { stream: stdout, folder: config.workspaceDir, at };
  if (!config.filter) return piece;
  piece.partial = new Map();
  for (const info of config.pkgInfos) {
    const paths = info.items
      .map(itemTestPath)
      .filter((p): p is string => p !== undefined);
    if (paths.length > 0) piece.partial.set(info.importPath, coverOf(paths));
  }
  return piece;
}

// How a --changed run narrowed itself, as its manifest says.
export interface Narrowed {
  ran: number;
  unchanged: number;
  noRecord: boolean;
}

// changedOf reads the run's manifest from its -json stream.
export function changedOf(stdout: string): Narrowed | undefined {
  for (const line of stdout.split("\n")) {
    if (!line.includes('"Manifest"')) continue;
    try {
      const ev = JSON.parse(line) as {
        RunLevel?: boolean;
        Manifest?: {
          Changed?: { Ran?: number; Unchanged?: number; NoRecord?: boolean };
        };
      };
      const c = ev.RunLevel ? ev.Manifest?.Changed : undefined;
      if (!c) continue;
      return {
        ran: c.Ran ?? 0,
        unchanged: c.Unchanged ?? 0,
        noRecord: !!c.NoRecord,
      };
    } catch {
      // not an event
    }
  }
  return undefined;
}

// narrowedLine says, in the run's output, that it was not a full run.
export function narrowedLine(n: Narrowed): string {
  const pkgs = (k: number) => `${k} package${k === 1 ? "" : "s"}`;
  if (n.noRecord)
    return `[gotest] --changed found no run record, so it ran everything: ${pkgs(n.ran)}.`;
  return `[gotest] narrowed by --changed: ran ${pkgs(n.ran)}, left out ${pkgs(n.unchanged)} whose inputs did not change; their verdicts are the last record's, not this run's.`;
}

export async function executeBatch(config: BatchConfig): Promise<BatchResult> {
  const result = await runBatch(config);
  config.progress?.finished(result.piece);
  return result;
}

async function runBatch(config: BatchConfig): Promise<BatchResult> {
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
    coverage,
    onResults,
    knownPackages,
    changed,
    progress,
  } = config;

  const importPaths = pkgInfos.map((p) => p.importPath);
  let cliPkgArgs: string[];
  if (filter) {
    cliPkgArgs = importPaths;
  } else {
    const modulePath = await readModulePath(workspaceDir);
    const wsPatterns = await readWorkspacePatterns(workspaceDir);
    const wildcards = resolveRunPatterns(
      importPaths,
      modulePath,
      wsPatterns,
      knownPackages && { workspaceDir, packages: knownPackages },
    );
    cliPkgArgs = wildcards ?? importPaths;
  }
  let coverFile: string | undefined;

  try {
    if (coverage) {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");
    }

    const gotestArgs: string[] = [];
    // The stream on disk too, so the run's record names a file to open.
    const eventsFile = nextEventsFile();
    if (eventsFile) gotestArgs.push(`--events-file=${eventsFile}`);
    if (changed) gotestArgs.push("--changed");
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
    const cmd = await buildCliCommand(cliArgs, workspaceDir, outputChannel);
    outputChannel.info(`[${label}] ${formatCliCommand(cmd)}`);

    const pkgDirMap = new Map(pkgInfos.map((p) => [p.importPath, p.dir]));
    // A package's output is kept for the whole run, not dropped at its
    // verdict: a --timeout writes the hung test's verdict after the
    // package's, and under GOTEST_LIVE suites interleave.
    const applier = new StreamApplier(
      controller,
      run,
      (importPath) => pkgDirMap.get(importPath),
      workspaceDir,
      new OnceLog((m) => outputChannel.warn(m)),
    );
    let streamed = "";
    progress?.streaming(() => runPiece(streamed, config));

    // A --changed run starts nothing up front; a package is marked running
    // once the CLI says it runs it: in the run's manifest, or, from a CLI
    // whose manifest lists none, at the package's own start event.
    const infoByPath = new Map(pkgInfos.map((p) => [p.importPath, p]));
    const marked = new Set<string>();
    const markRunning = (paths: readonly string[]) => {
      const started: vscode.TestItem[] = [];
      for (const importPath of paths) {
        const info = infoByPath.get(importPath);
        if (!info || marked.has(importPath)) continue;
        marked.add(importPath);
        for (const item of info.items) {
          controller.clearResults(item);
          run.started(item);
          enqueueDescendants(run, item);
          started.push(item);
        }
      }
      if (started.length > 0) startAncestors(run, started);
    };

    const handleStdoutLine = (line: string) => {
      streamed += line + "\n";
      let event: TestEvent;
      try {
        event = JSON.parse(line) as TestEvent;
        if (!event.Action) return;
      } catch {
        return;
      }
      if (changed && event.Action === "start") {
        const listed = event.Manifest?.Changed?.Packages;
        if (event.RunLevel) {
          if (Array.isArray(listed)) markRunning(listed);
        } else if (!event.Test) {
          markRunning([event.Package]);
        }
      }
      const applied = applier.apply([event]);
      if (applied.length > 0) onResults?.(applied);
      if (pkgDirMap.has(event.Package)) progress?.note(event);
    };

    // What the CLI says on stderr — a note that a package was not run, a
    // run's failure — belongs with the run, as it arrives.
    const handleStderrLine = (line: string) => {
      if (!/^exit status \d+$/.test(line)) run.appendOutput(line + "\r\n");
    };

    const result = await spawnTestProcess(
      cmd.bin,
      cmd.args,
      workspaceDir,
      token,
      outputChannel,
      label,
      liveEnv(workspaceDir, env),
      handleStdoutLine,
      handleStderrLine,
    );

    const narrowed = changed ? changedOf(result.stdout) : undefined;
    if (narrowed) run.appendOutput(narrowedLine(narrowed) + "\r\n");

    if (token.isCancellationRequested) {
      // A narrowed run skips only what it said it would run.
      const skippedPkgs = pkgInfos.filter(
        (i) =>
          !applier.reported.has(i.importPath) &&
          (!changed || marked.has(i.importPath)),
      );
      if (skippedPkgs.length > 0) {
        outputChannel.info(
          `[${label}] cancelled, skipping ${skippedPkgs.length} remaining package(s)`,
        );
      }
      for (const info of skippedPkgs) {
        for (const item of info.items) {
          run.skipped(item);
          skipUnresolved(run, item, controller);
        }
      }
      return { stdout: result.stdout, piece: runPiece(result.stdout, config) };
    }

    if (result.exitCode !== 0) {
      const stderrFiltered = result.stderr
        .split("\n")
        .filter((line) => !/^exit status \d+$/.test(line.trim()))
        .join("\n")
        .trim();

      let attributed = false;
      for (const info of pkgInfos) {
        // A narrowed run left the package out; it did not fail.
        if (
          applier.reported.has(info.importPath) ||
          (changed && !marked.has(info.importPath))
        ) {
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
          erroredItem(run, controller, item, diagnostic);
          skipUnresolved(run, item, controller);
        }
        attributed = true;
      }

      // Every package reported, the stream carried no failing verdict, and the
      // run still failed: the reason is not a test's — a shared fixture that
      // would not release what it holds, a census, a deadline. Nothing above
      // carries it, so the tree would read green for a run the CLI failed.
      // Mark the packages instead; their tests did pass, and the failure is
      // the package's, not theirs. A stream that did carry a failure is left
      // alone: the tree already shows red, and overwriting that verdict would
      // lose which test it was. The -json stream is not part of the message
      // here — it is the whole run's output, not something to read on an item.
      markFailedRun(
        run,
        pkgInfos
          .filter((info) => !changed || applier.reported.has(info.importPath))
          .flatMap((info) => info.items.map(getPackageItem)),
        {
          exitCode: result.exitCode,
          cancelled: false,
          failed: attributed || applier.failed,
          reason: stderrFiltered,
        },
        controller,
      );
    }

    // What a narrowed run marked and no event resolved, as a full run does.
    for (const importPath of marked) {
      for (const item of infoByPath.get(importPath)!.items) {
        skipUnresolved(run, item, controller);
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

    return { stdout: result.stdout, piece: runPiece(result.stdout, config) };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    if (err instanceof Error && "code" in err && err.code === "ENOENT") {
      clearBinaryCache();
    }
    outputChannel.error(`[${label}] ${message}`);
    for (const info of pkgInfos) {
      for (const item of info.items) {
        erroredItem(run, controller, item, message);
        skipUnresolved(run, item, controller);
      }
    }
    return { stdout: "", piece: runPiece("", config) };
  } finally {
    pruneEventsFiles();
    if (coverFile) {
      rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
        () => {},
      );
    }
  }
}
