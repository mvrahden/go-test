import * as vscode from "vscode";
import * as path from "node:path";
import { readdir } from "node:fs/promises";
import type { DiscoveryCache } from "./discovery.js";
import { buildCliCommand, buildFuzzArgs, formatCliCommand } from "./cli.js";
import { spawnTestProcess } from "./runnerUtils.js";

// The fuzz surfaces shell out to `gotest fuzz` and its triage/promote
// subcommands rather than reimplementing any of their logic: the CLI owns
// target selection (--target), the budget schedule (--for), the exit
// contract (0 = nothing found, 1 = finding, 2 = cannot run), crasher
// detection (corpus-directory scan), and source splicing (promote). This
// module only turns those contracts into editor affordances — a budget
// picker, a cancellable progress notification, and finding-driven actions.

// --- pure parsing helpers (unit-tested) ---

export interface FuzzProgress {
  elapsed: string;
  execs: number;
  rate: number;
  interesting?: number;
}

// parseFuzzProgress reads go's fuzzing status lines as the orchestrator
// streams them (prefixed with "[<Func>] "), e.g.
//   [FuzzX] fuzz: elapsed: 3s, execs: 393973 (131314/sec), new interesting: 0 (total: 76)
export function parseFuzzProgress(line: string): FuzzProgress | undefined {
  const m =
    /fuzz: elapsed: ([^,]+), execs: (\d+) \((\d+)\/sec\)(?:, new interesting: (\d+))?/.exec(
      line,
    );
  if (!m) return undefined;
  return {
    elapsed: m[1],
    execs: Number(m[2]),
    rate: Number(m[3]),
    interesting: m[4] !== undefined ? Number(m[4]) : undefined,
  };
}

// parseNewCrasher extracts the corpus-entry path from the orchestrator's
// "[<Func>] new crasher: <path>" line.
export function parseNewCrasher(line: string): string | undefined {
  const m = / new crasher: (.+)$/.exec(line);
  return m?.[1];
}

// parsePromotedSeed reads promote's confirmation line:
//   promoted FuzzX/1a2b3c -> f.Add(...) in examples/fuzzing/suite_test.go:83
export function parsePromotedSeed(
  line: string,
): { file: string; line: number } | undefined {
  const m = /^promoted \S+ -> f\.Add\(.*\) in (.+):(\d+)$/.exec(line);
  if (!m) return undefined;
  return { file: m[1], line: Number(m[2]) };
}

// isValidGoDuration accepts what time.ParseDuration accepts for positive
// durations: one or more <number><unit> segments.
export function isValidGoDuration(s: string): boolean {
  return /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/.test(s.trim());
}

// countCrasherEntries returns the number of corpus entry files on disk for
// a generated wrapper, or 0 when the directory does not exist. Used by the
// CodeLens provider to surface pending crashers where the target lives.
export async function countCrasherEntries(
  pkgDir: string,
  wrapperName: string,
): Promise<number> {
  try {
    const entries = await readdir(
      path.join(pkgDir, "testdata", "fuzz", wrapperName),
      { withFileTypes: true },
    );
    return entries.filter((e) => e.isFile()).length;
  } catch {
    return 0;
  }
}

// --- budget selection ---

interface BudgetChoice {
  budget?: string; // undefined = until stopped
}

async function pickFuzzBudget(): Promise<BudgetChoice | undefined> {
  const picked = await vscode.window.showQuickPick(
    [
      { label: "30s", description: "quick shakeout" },
      { label: "5m", description: "standard session" },
      { label: "30m", description: "deep session" },
      { label: "Until stopped", description: "fuzz until you cancel" },
      { label: "Custom…", description: "any Go duration, e.g. 90s or 2m30s" },
    ],
    { placeHolder: "How long should this target fuzz?" },
  );
  if (!picked) return undefined;
  if (picked.label === "Until stopped") return {};
  if (picked.label !== "Custom…") return { budget: picked.label };

  const custom = await vscode.window.showInputBox({
    prompt: "Fuzz budget (Go duration)",
    placeHolder: "90s, 2m30s, 1h…",
    validateInput: (value) =>
      isValidGoDuration(value) ? undefined : "not a Go duration (e.g. 5m, 90s)",
  });
  if (custom === undefined) return undefined;
  return { budget: custom.trim() };
}

// --- session, triage, promote ---

export interface FuzzDeps {
  cache: DiscoveryCache;
  outputChannel: vscode.LogOutputChannel;
  // Called after promote lands source edits, so discovery/CodeLenses refresh.
  onSourceChanged?: (importPath: string) => void;
}

function resolveWorkspace(
  importPath: string,
  deps: FuzzDeps,
): { workspaceDir: string } | undefined {
  const workspaceDir = deps.cache.getWorkspaceDir(importPath);
  if (!workspaceDir) {
    vscode.window.showErrorMessage(
      `gotest: no workspace folder known for ${importPath} — re-run discovery`,
    );
    return undefined;
  }
  return { workspaceDir };
}

export async function runFuzzCommand(
  importPath: string,
  suiteName: string,
  methodName: string,
  deps: FuzzDeps,
): Promise<void> {
  const ws = resolveWorkspace(importPath, deps);
  if (!ws) return;
  const choice = await pickFuzzBudget();
  if (!choice) return;

  const wrapper = `Fuzz${suiteName}_${methodName}`;
  const cmd = await buildCliCommand(
    buildFuzzArgs(importPath, suiteName, methodName, choice.budget),
    ws.workspaceDir,
    deps.outputChannel,
  );
  deps.outputChannel.info(`[fuzz] ${formatCliCommand(cmd)}`);
  deps.outputChannel.show(true);

  const crashers: string[] = [];
  const title = choice.budget
    ? `Fuzzing ${wrapper} (${choice.budget})`
    : `Fuzzing ${wrapper} (until stopped)`;

  const result = await vscode.window.withProgress(
    {
      location: vscode.ProgressLocation.Notification,
      title,
      cancellable: true,
    },
    (progress, token) =>
      spawnTestProcess(
        cmd.bin,
        cmd.args,
        ws.workspaceDir,
        token,
        deps.outputChannel,
        "fuzz",
        undefined,
        (line) => {
          deps.outputChannel.info(`[fuzz] ${line}`);
          const p = parseFuzzProgress(line);
          if (p) {
            const interesting =
              p.interesting !== undefined
                ? `, ${p.interesting} new interesting`
                : "";
            progress.report({
              message: `${p.elapsed} — ${p.execs.toLocaleString()} execs (${p.rate}/sec)${interesting}`,
            });
          }
        },
        (line) => {
          deps.outputChannel.warn(`[fuzz] ${line}`);
          const crasher = parseNewCrasher(line);
          if (crasher) crashers.push(crasher);
        },
      ),
  );

  // The CLI's exit contract drives the ending: 0 = the search ran and
  // found nothing (including cancelled sessions — cancellation is the
  // normal end of "until stopped"), 1 = a finding, 2 = could not run.
  if (result.exitCode === 0) {
    vscode.window.showInformationMessage(
      `Fuzzing ${wrapper}: no failures found.`,
    );
    return;
  }

  if (result.exitCode === 1 && crashers.length > 0) {
    const entry = path.basename(crashers[0]);
    const plural = crashers.length === 1 ? "crasher" : "crashers";
    const action = await vscode.window.showWarningMessage(
      `Fuzzing ${wrapper}: ${crashers.length} new ${plural} found.`,
      "Show Decoded Input",
      "Promote to Seed",
      "Debug Crasher",
    );
    if (action === "Show Decoded Input") {
      await triageCrashers(importPath, deps);
    } else if (action === "Promote to Seed") {
      await promoteCrashers(importPath, deps);
    } else if (action === "Debug Crasher") {
      await vscode.commands.executeCommand(
        "gotest.debugFuzz",
        importPath,
        suiteName,
        methodName,
        entry,
      );
    }
    return;
  }

  if (result.exitCode === 1) {
    vscode.window.showWarningMessage(
      `Fuzzing ${wrapper}: failing without a new crasher file — a seed or existing corpus entry fails; it reproduces on a normal test run.`,
    );
    return;
  }

  const open = await vscode.window.showErrorMessage(
    `gotest fuzz failed (exit ${result.exitCode}).`,
    "Open Output",
  );
  if (open) deps.outputChannel.show();
}

export async function triageCrashers(
  importPath: string,
  deps: FuzzDeps,
): Promise<void> {
  const ws = resolveWorkspace(importPath, deps);
  if (!ws) return;

  const cmd = await buildCliCommand(
    ["fuzz", "triage", importPath],
    ws.workspaceDir,
    deps.outputChannel,
  );
  deps.outputChannel.info(`[fuzz] ${formatCliCommand(cmd)}`);

  const cts = new vscode.CancellationTokenSource();
  try {
    const result = await spawnTestProcess(
      cmd.bin,
      cmd.args,
      ws.workspaceDir,
      cts.token,
      deps.outputChannel,
      "fuzz",
      undefined,
      (line) => deps.outputChannel.info(`[triage] ${line}`),
    );
    deps.outputChannel.show(true);

    const inputs = result.stdout
      .split("\n")
      .map((l) => /^\s*input: (.+)$/.exec(l)?.[1])
      .filter((v): v is string => v !== undefined);
    if (inputs.length > 0) {
      const more = inputs.length > 1 ? ` (+${inputs.length - 1} more)` : "";
      vscode.window.showInformationMessage(
        `Decoded crasher input: ${inputs[0]}${more}`,
      );
    } else if (result.exitCode !== 0) {
      vscode.window.showWarningMessage(
        "gotest fuzz triage reported problems — see the output channel.",
      );
    } else {
      vscode.window.showInformationMessage("No crashers found to triage.");
    }
  } finally {
    cts.dispose();
  }
}

export async function promoteCrashers(
  importPath: string,
  deps: FuzzDeps,
): Promise<void> {
  const ws = resolveWorkspace(importPath, deps);
  if (!ws) return;

  const cmd = await buildCliCommand(
    ["fuzz", "promote", importPath],
    ws.workspaceDir,
    deps.outputChannel,
  );
  deps.outputChannel.info(`[fuzz] ${formatCliCommand(cmd)}`);

  const cts = new vscode.CancellationTokenSource();
  try {
    const result = await spawnTestProcess(
      cmd.bin,
      cmd.args,
      ws.workspaceDir,
      cts.token,
      deps.outputChannel,
      "fuzz",
      undefined,
      (line) => deps.outputChannel.info(`[promote] ${line}`),
    );

    const promoted = result.stdout
      .split("\n")
      .map(parsePromotedSeed)
      .filter((v): v is { file: string; line: number } => v !== undefined);

    if (result.exitCode !== 0) {
      deps.outputChannel.show(true);
      vscode.window.showErrorMessage(
        `gotest fuzz promote failed (exit ${result.exitCode}) — see the output channel.`,
      );
      // Promote leaves crasher files in place on failure, so no refresh.
      return;
    }

    if (promoted.length === 0) {
      vscode.window.showInformationMessage("No crashers found to promote.");
      return;
    }

    // Promote splices typed f.Add seeds into user source — reveal the
    // first edit so the change is seen, not just reported.
    const first = promoted[0];
    const absolute = path.isAbsolute(first.file)
      ? first.file
      : path.join(ws.workspaceDir, first.file);
    const doc = await vscode.workspace.openTextDocument(absolute);
    const position = new vscode.Position(Math.max(0, first.line - 1), 0);
    await vscode.window.showTextDocument(doc, {
      selection: new vscode.Range(position, position),
    });

    const plural = promoted.length === 1 ? "crasher" : "crashers";
    vscode.window.showInformationMessage(
      `Promoted ${promoted.length} ${plural} into typed f.Add seed${promoted.length === 1 ? "" : "s"}.`,
    );
    deps.onSourceChanged?.(importPath);
  } finally {
    cts.dispose();
  }
}
