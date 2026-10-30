import * as vscode from "vscode";
import * as path from "node:path";
import { readFile } from "node:fs/promises";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { SpecViewPanel } from "./specView.js";
import type { TestResult } from "./testResultStore.js";
import { parseTestEvents } from "./outputParser.js";
import { StreamApplier, resolveAncestorItems } from "./runnerUtils.js";
import { isRunLevelPackage } from "./specStore.js";

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
  const events = parseTestEvents(text);
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
  );
  applier.apply(events);
  resolveAncestorItems(run, controller);

  // What the CLI booked for the run itself has no item; it is said in the
  // run's output. A real package the workspace lacks is listed, not dropped.
  for (const ev of events) {
    if (isRunLevelPackage(ev.Package) && ev.Action === "output" && ev.Output) {
      run.appendOutput(`[${ev.Package}] ${ev.Output.replace(/\n$/, "")}\r\n`);
    }
  }
  const missing = Array.from(applier.unknownPackages)
    .filter((p) => p && !isRunLevelPackage(p))
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
