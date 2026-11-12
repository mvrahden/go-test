import * as vscode from "vscode";
import * as path from "node:path";
import { readFile } from "node:fs/promises";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { SpecViewPanel } from "./specView.js";
import type { TestResult } from "./testResultStore.js";
import { parseTestEvents, type TestEvent } from "./outputParser.js";
import { buildCliCommand } from "./cli.js";
import { specRenderTimeoutSeconds } from "./config.js";
import { ManagedChild } from "./managedChild.js";
import { OnceLog, StreamApplier, resolveAncestorItems } from "./runnerUtils.js";

const UNSTAMPED_IMPORT =
  "[import] the stream's subtests carry no placement and the CLI could not add it; their results are not shown on their items";

export interface ImportDeps {
  controller: GoTestController;
  cache: DiscoveryCache;
  specView: SpecViewPanel;
  outputChannel: vscode.LogOutputChannel;
}

export interface ImportOutcome {
  file: string;
  // Packages the stream names that this workspace does not hold.
  missing: string[];
}

// ImportedResults stands in for the controller while a saved stream is
// replayed: verdicts land on the Test Explorer's items, but the result store
// never sees them. They describe another machine's run, and a reload restores
// this workspace's own.
class ImportedResults {
  private readonly results = new Map<string, TestResult>();

  constructor(private readonly controller: GoTestController) {}

  get testController(): vscode.TestController {
    return this.controller.testController;
  }
  findItem(id: string): vscode.TestItem | undefined {
    return this.controller.findItem(id);
  }
  createDynamicSubtest(
    parent: vscode.TestItem,
    subtestPath: string,
    label: string,
  ): vscode.TestItem {
    return this.controller.createDynamicSubtest(parent, subtestPath, label);
  }
  recordResult(
    itemId: string,
    status: TestResult["status"],
    duration?: number,
  ): void {
    this.results.set(itemId, { status, duration });
  }
  getResult(itemId: string): TestResult | undefined {
    return this.results.get(itemId);
  }
  noteStart(): void {}
  notePaused(): void {}
}

// openEventsFile opens a saved go test -json stream, such as a CI run's
// --events-file: in the Spec View as a source of its own, and in the Test
// Explorer as a run named after the file.
export async function openEventsFile(
  deps: ImportDeps,
  target?: string | vscode.Uri,
): Promise<ImportOutcome | undefined> {
  const file = await pickFile(target);
  if (!file) return undefined;

  let text: string;
  try {
    text = await readFile(file, "utf-8");
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(`gotest: cannot read ${file}: ${message}`);
    return undefined;
  }
  let events = testEvents(text);
  if (events.length === 0) {
    vscode.window.showWarningMessage(
      `gotest: ${path.basename(file)} holds no go test events.`,
    );
    return undefined;
  }

  const label = path.basename(file);
  const folder =
    vscode.workspace.getWorkspaceFolder(vscode.Uri.file(file))?.uri.fsPath ??
    vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ??
    path.dirname(file);

  // A stream of an older gotest or of plain go test says nowhere where a
  // subtest sits; the CLI places it by the file's order, as a live run does.
  if (needsStamps(events)) {
    const stamped = await stampWithCli(file, folder, deps.outputChannel);
    if (stamped) events = testEvents(stamped);
  }

  const run = deps.controller.createTestRun(
    new vscode.TestRunRequest(),
    `Imported: ${label}`,
  );
  const results = new ImportedResults(deps.controller);
  const controller = results as unknown as GoTestController;
  const applier = new StreamApplier(
    controller,
    run,
    (importPath) => deps.cache.getPackage(importPath)?.dir,
    folder,
    new OnceLog((m) => deps.outputChannel.warn(m)),
    UNSTAMPED_IMPORT,
  );
  applier.apply(events);
  resolveAncestorItems(run, controller);

  // What the CLI booked for the run itself, marked RunLevel, has no item; it
  // is said in the run's output. A real package the workspace lacks is
  // listed, not dropped.
  const runLevel = new Set<string>();
  for (const ev of events) {
    if (ev.RunLevel !== true) continue;
    runLevel.add(ev.Package);
    if (ev.Action === "output" && ev.Output) {
      run.appendOutput(`[${ev.Package}] ${ev.Output.replace(/\n$/, "")}\r\n`);
    }
  }
  const missing = Array.from(applier.unknownPackages)
    .filter((p) => p && !runLevel.has(p))
    .sort();
  if (missing.length > 0) {
    run.appendOutput(
      `[import] not in this workspace, shown in the Spec View only:\r\n${missing
        .map((p) => `  ${p}`)
        .join("\r\n")}\r\n`,
    );
    const shown = missing.slice(0, 5).join(", ");
    const more = missing.length > 5 ? ` and ${missing.length - 5} more` : "";
    vscode.window.showInformationMessage(
      `gotest: ${label} names ${missing.length} package(s) this workspace does not hold: ${shown}${more}.`,
    );
  }
  run.end();
  deps.outputChannel.info(
    `[import] ${file}: ${events.length} event(s), ${missing.length} package(s) not in the workspace`,
  );

  await deps.specView.openStream(`file:${file}`, label, {
    stream: text,
    folder,
  });
  return { file, missing };
}

// testEvents are a stream's test events; a capture of gotest watch carries
// its cycle markers, which are not tests.
function testEvents(text: string): TestEvent[] {
  return parseTestEvents(text).filter(
    (e) => !String(e.Action).startsWith("watch-"),
  );
}

// needsStamps says a stream has subtests and no event says where one sits.
export function needsStamps(events: TestEvent[]): boolean {
  return (
    !events.some((e) => e.Gotest) &&
    events.some((e) => e.Test?.includes("/") === true)
  );
}

// stampWithCli is the file as `gotest spec --input --stamp` writes it back;
// undefined when the CLI cannot, such as one older than the flag.
async function stampWithCli(
  file: string,
  folder: string,
  log: vscode.LogOutputChannel,
): Promise<string | undefined> {
  let mc: ManagedChild | undefined;
  try {
    const cmd = await buildCliCommand(
      ["spec", `--input=${file}`, "--stamp"],
      folder,
      log,
    );
    mc = new ManagedChild(cmd.bin, cmd.args, { cwd: folder, kind: "spec" });
    let stdout = "";
    let stderr = "";
    mc.child.stdout?.on("data", (chunk: string) => (stdout += chunk));
    mc.child.stderr?.on("data", (chunk: string) => (stderr += chunk));
    mc.child.on("error", () => {});
    const child = mc;
    const budget = setTimeout(
      () => void child.terminate("prompt"),
      specRenderTimeoutSeconds(folder) * 1000,
    );
    const { code } = await mc.exited.finally(() => clearTimeout(budget));
    if (code === 0) return stdout;
    log.warn(`[import] gotest spec --stamp exited ${code}: ${stderr.trim()}`);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    log.warn(`[import] cannot stamp ${file}: ${message}`);
  } finally {
    mc?.dispose();
  }
  return undefined;
}

async function pickFile(
  target?: string | vscode.Uri,
): Promise<string | undefined> {
  if (typeof target === "string" && target) return target;
  if (target && typeof target === "object" && "fsPath" in target) {
    return target.fsPath;
  }
  const picked = await vscode.window.showOpenDialog({
    canSelectMany: false,
    openLabel: "Open Events File",
    filters: {
      "go test -json streams": ["jsonl", "json", "ndjson", "log", "txt"],
      "All files": ["*"],
    },
  });
  return picked?.[0]?.fsPath;
}
