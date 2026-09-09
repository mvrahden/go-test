// The Test Explorer's Fuzz run profile: the opt-in counterpart of Bench. It
// is bound to the "fuzz" tag, so it is offered only when fuzz targets are in
// the selection, and it never runs under plain Run — a search burns CPU on
// request, not as a side effect of running tests.
import * as vscode from "vscode";
import { buildCliCommand, formatCliCommand, scopedConfig } from "./cli.js";
import { spawnTestProcess, isFuzzMethodLabel } from "./runnerUtils.js";
import {
  isValidGoDuration,
  offerCrasherActions,
  parseCrasherLine,
  type FuzzDeps,
} from "./fuzz.js";
import type { GoTestController } from "./testController.js";

export { parseCrasherLine } from "./fuzz.js";

export const FUZZ_TAG = "fuzz";

// collectFuzzItems resolves a run request to fuzz items: the whole tree when
// nothing is named, otherwise the selection descended to its fuzz items,
// minus anything excluded (a container excludes everything beneath it).
export function collectFuzzItems(
  include: readonly vscode.TestItem[] | undefined,
  roots: vscode.TestItemCollection,
  exclude: readonly vscode.TestItem[] | undefined,
): vscode.TestItem[] {
  const excluded = new Set((exclude ?? []).map((i) => i.id));
  const out: vscode.TestItem[] = [];
  const seen = new Set<string>();
  const visit = (item: vscode.TestItem): void => {
    if (excluded.has(item.id)) return;
    if (item.tags.some((t) => t.id === FUZZ_TAG)) {
      if (!seen.has(item.id)) {
        seen.add(item.id);
        out.push(item);
      }
      return;
    }
    item.children.forEach(visit);
  };
  if (include) include.forEach(visit);
  else roots.forEach(visit);
  return out;
}

export interface FuzzItemTarget {
  importPath: string;
  suiteName: string;
  methodName: string;
  wrapper: string;
}

// fuzzTargetFromItemId reads "<importPath>/<Suite>/<FuzzMethod>" from the
// right, since the import path carries slashes of its own.
export function fuzzTargetFromItemId(id: string): FuzzItemTarget | undefined {
  const parts = id.split("/");
  if (parts.length < 3) return undefined;
  const methodName = parts[parts.length - 1];
  const suiteName = parts[parts.length - 2];
  if (!isFuzzMethodLabel(methodName) || methodName.startsWith("X_")) {
    return undefined;
  }
  return {
    importPath: parts.slice(0, -2).join("/"),
    suiteName,
    methodName,
    wrapper: `Fuzz${suiteName}_${methodName}`,
  };
}

export interface FuzzSession {
  importPath: string;
  target?: string;
  wrappers: string[];
}

// planFuzzSessions groups selected targets into CLI sessions: one per
// package when every target of the package is selected, so the CLI shares
// the budget jobs-aware across them; otherwise one per target, since the
// CLI takes a single --target.
export function planFuzzSessions(
  ids: string[],
  targetsOf: (importPath: string) => string[],
): FuzzSession[] {
  const byPackage = new Map<string, string[]>();
  for (const id of ids) {
    const t = fuzzTargetFromItemId(id);
    if (!t) continue;
    const list = byPackage.get(t.importPath) ?? [];
    if (!list.includes(t.wrapper)) list.push(t.wrapper);
    byPackage.set(t.importPath, list);
  }
  const sessions: FuzzSession[] = [];
  for (const [importPath, selected] of byPackage) {
    const all = targetsOf(importPath);
    const whole = all.length > 0 && all.every((w) => selected.includes(w));
    if (whole) {
      sessions.push({ importPath, target: undefined, wrappers: all });
    } else {
      for (const wrapper of selected) {
        sessions.push({ importPath, target: wrapper, wrappers: [wrapper] });
      }
    }
  }
  return sessions;
}

export interface SessionOutcome {
  exitCode: number;
  cancelled: boolean;
  crashers: Map<string, string[]>;
  failed: Set<string>;
  sessionLine: string;
  stderr?: string;
}

export interface FuzzVerdict {
  status: "passed" | "failed" | "skipped" | "errored";
  message: string;
}

// fuzzVerdicts maps one session's outcome onto its targets. A crasher file
// is the finding signal; a FAIL without one is a seed or corpus entry that
// reproduces on a normal run; a cancelled search found nothing and is not a
// failure; exit 2 means the CLI could not run at all.
export function fuzzVerdicts(
  wrappers: string[],
  o: SessionOutcome,
): Map<string, FuzzVerdict> {
  const out = new Map<string, FuzzVerdict>();
  for (const w of wrappers) {
    const files = o.crashers.get(w);
    if (o.exitCode === 2) {
      out.set(w, {
        status: "errored",
        message: o.stderr?.trim() || "gotest fuzz failed (exit 2)",
      });
    } else if (files && files.length > 0) {
      out.set(w, {
        status: "failed",
        message:
          `${files.length} new crasher${files.length === 1 ? "" : "s"}:\n` +
          files.map((f) => `  ${f}`).join("\n") +
          "\nPromote it to a seed with the CodeLens or `gotest fuzz promote`.",
      });
    } else if (o.failed.has(w)) {
      out.set(w, {
        status: "failed",
        message:
          "failed without a new crasher file: a seed or existing corpus entry fails, and it reproduces on a normal run",
      });
    } else if (o.cancelled) {
      out.set(w, { status: "skipped", message: "cancelled" });
    } else {
      out.set(w, { status: "passed", message: o.sessionLine });
    }
  }
  return out;
}

// runFuzzProfile is the profile's run handler.
export async function runFuzzProfile(
  request: vscode.TestRunRequest,
  token: vscode.CancellationToken,
  deps: FuzzDeps & { controller: GoTestController },
): Promise<void> {
  const { cache, outputChannel, controller } = deps;
  const items = collectFuzzItems(
    request.include,
    controller.testController.items,
    request.exclude,
  );
  if (items.length === 0) return;

  const byWrapper = new Map<string, vscode.TestItem>();
  for (const item of items) {
    const t = fuzzTargetFromItemId(item.id);
    if (t) byWrapper.set(`${t.importPath} ${t.wrapper}`, item);
  }
  const targetsOf = (importPath: string): string[] => {
    const pkg = cache.getPackage(importPath);
    if (!pkg) return [];
    return pkg.suites.flatMap((s) =>
      (s.fuzzers ?? [])
        .filter((f) => !f.excluded)
        .map((f) => `Fuzz${s.name}_${f.name}`),
    );
  };
  const sessions = planFuzzSessions(
    items.map((i) => i.id),
    targetsOf,
  );

  const run = controller.createTestRun(request, "Go Fuzz Run");
  for (const item of items) run.started(item);
  const found: {
    importPath: string;
    target: FuzzItemTarget;
    files: string[];
  }[] = [];
  try {
    for (const session of sessions) {
      if (token.isCancellationRequested) break;
      const sessionItems = session.wrappers
        .map((w) => byWrapper.get(`${session.importPath} ${w}`))
        .filter((i): i is vscode.TestItem => !!i);
      const outcome = await runSession(session, token, deps, run);
      const verdicts = fuzzVerdicts(session.wrappers, outcome);
      for (const item of sessionItems) {
        const t = fuzzTargetFromItemId(item.id);
        const v = t && verdicts.get(t.wrapper);
        if (!t || !v) continue;
        switch (v.status) {
          case "passed":
            run.passed(item);
            break;
          case "skipped":
            run.skipped(item);
            break;
          case "failed":
            run.failed(item, new vscode.TestMessage(v.message));
            break;
          case "errored":
            run.errored(item, new vscode.TestMessage(v.message));
            break;
        }
        const files = outcome.crashers.get(t.wrapper);
        if (files && files.length > 0) {
          found.push({ importPath: session.importPath, target: t, files });
        }
      }
    }
  } finally {
    run.end();
  }

  // One notification per run, on the first finding: the tree already holds
  // every verdict, and promote acts on the whole package anyway.
  if (found.length > 0) {
    const f = found[0];
    await offerCrasherActions(
      f.importPath,
      f.target.suiteName,
      f.target.methodName,
      f.files,
      deps,
    );
  }
}

async function runSession(
  session: FuzzSession,
  token: vscode.CancellationToken,
  deps: FuzzDeps,
  run: vscode.TestRun,
): Promise<SessionOutcome> {
  const { cache, outputChannel } = deps;
  const outcome: SessionOutcome = {
    exitCode: 2,
    cancelled: false,
    crashers: new Map(),
    failed: new Set(),
    sessionLine: "",
  };
  const workspaceDir = cache.getWorkspaceDir(session.importPath);
  if (!workspaceDir) {
    outcome.stderr = `no workspace dir for ${session.importPath}`;
    return outcome;
  }
  const budget = (
    scopedConfig(workspaceDir).get<string>("fuzz.for", "") ?? ""
  ).trim();
  if (budget && !isValidGoDuration(budget)) {
    outcome.stderr = `gotest.fuzz.for is not a Go duration: ${JSON.stringify(budget)}`;
    return outcome;
  }
  const args = ["fuzz", session.importPath];
  if (session.target) args.push(`--target=${session.target}`);
  if (budget) args.push(`--for=${budget}`);

  let cmd;
  try {
    cmd = await buildCliCommand(args, workspaceDir, outputChannel);
  } catch (err: unknown) {
    outcome.stderr = err instanceof Error ? err.message : String(err);
    return outcome;
  }
  outputChannel.info(`[fuzz] ${formatCliCommand(cmd)}`);

  const onLine = (line: string): void => {
    run.appendOutput(line + "\r\n");
    const crasher = parseCrasherLine(line);
    if (crasher) {
      const list = outcome.crashers.get(crasher.wrapper) ?? [];
      list.push(crasher.path);
      outcome.crashers.set(crasher.wrapper, list);
    }
    const fail = /^\[(\S+)\] (?:--- )?FAIL\b/.exec(line);
    if (fail) outcome.failed.add(fail[1]);
    if (line.startsWith("fuzzed ")) outcome.sessionLine = line;
  };
  const result = await spawnTestProcess(
    cmd.bin,
    cmd.args,
    workspaceDir,
    token,
    outputChannel,
    "fuzz",
    undefined,
    onLine,
    onLine,
  );
  outcome.exitCode = result.exitCode;
  outcome.cancelled = token.isCancellationRequested;
  outcome.stderr = result.stderr;
  return outcome;
}
