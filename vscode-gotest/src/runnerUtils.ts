import * as vscode from "vscode";
import { spawn } from "node:child_process";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import {
  parseTestEvents,
  extractTestMessages,
  type TestEvent,
} from "./outputParser.js";

export function collectItems(
  controller: GoTestController,
  request: vscode.TestRunRequest,
): vscode.TestItem[] {
  const items: vscode.TestItem[] = [];
  if (request.include && request.include.length > 0) {
    for (const item of request.include) {
      items.push(item);
    }
  } else {
    controller.testController.items.forEach((item) => {
      items.push(item);
    });
  }
  return expandToPackages(items);
}

export function groupByPackage(
  items: vscode.TestItem[],
): Map<string, vscode.TestItem[]> {
  const groups = new Map<string, vscode.TestItem[]>();
  for (const item of items) {
    const root = getPackageItem(item);
    let group = groups.get(root.id);
    if (!group) {
      group = [];
      groups.set(root.id, group);
    }
    group.push(item);
  }
  return groups;
}

export function getRootItem(item: vscode.TestItem): vscode.TestItem {
  let current = item;
  while (current.parent) {
    current = current.parent;
  }
  return current;
}

export function getItemDepth(item: vscode.TestItem): number {
  let depth = 0;
  let current = item;
  while (current.parent) {
    current = current.parent;
    depth++;
  }
  return depth;
}

const PACKAGE_TAG = "package";

export function getPackageItem(item: vscode.TestItem): vscode.TestItem {
  let current: vscode.TestItem = item;
  if (current.tags.some((t) => t.id === PACKAGE_TAG)) {
    return current;
  }
  while (current.parent) {
    current = current.parent;
    if (current.tags.some((t) => t.id === PACKAGE_TAG)) {
      return current;
    }
  }
  return current;
}

export function getPackageDepth(item: vscode.TestItem): number {
  let depth = 0;
  let current: vscode.TestItem | undefined = item;
  while (current) {
    if (current.tags.some((t) => t.id === PACKAGE_TAG)) {
      return depth;
    }
    depth++;
    current = current.parent;
  }
  return -1;
}

export function expandToPackages(items: vscode.TestItem[]): vscode.TestItem[] {
  const result: vscode.TestItem[] = [];
  const visit = (item: vscode.TestItem) => {
    if (item.tags.some((t) => t.id === PACKAGE_TAG)) {
      result.push(item);
      return;
    }
    item.children.forEach((child) => visit(child));
  };
  for (const item of items) {
    visit(item);
  }
  return result.length > 0 ? result : items;
}

export function resolveTestItem(
  controller: GoTestController,
  testPath: string,
  importPath: string,
): vscode.TestItem | undefined {
  const segments = testPath.split("/");
  if (segments.length === 0) {
    return undefined;
  }

  const firstSegment = segments[0];
  const suiteName = firstSegment.startsWith("Test")
    ? firstSegment.slice(4)
    : firstSegment;

  const suiteId = `${importPath}/${suiteName}`;
  const suiteItem = controller.findItem(suiteId);
  if (!suiteItem) {
    return undefined;
  }

  if (segments.length === 1) {
    return suiteItem;
  }

  const methodName = segments[1];
  const methodId = `${suiteId}/${methodName}`;
  const methodItem = controller.findItem(methodId);
  if (!methodItem) {
    return undefined;
  }

  if (segments.length === 2) {
    return methodItem;
  }

  let parentItem = methodItem;
  for (let i = 2; i < segments.length; i++) {
    const subtestLabel = segments[i];
    const subtestPath = segments.slice(2, i + 1).join("/");
    parentItem = controller.createDynamicSubtest(
      parentItem,
      subtestPath,
      subtestLabel,
    );
  }

  return parentItem;
}

export interface AppliedResult {
  itemId: string;
  status: "pass" | "fail" | "skip";
  duration?: number;
}

export function applyResults(
  controller: GoTestController,
  run: vscode.TestRun,
  events: TestEvent[],
  importPath: string,
  pkgDir: string,
): AppliedResult[] {
  const outputMap = new Map<string, string>();

  for (const event of events) {
    if (event.Action === "output" && event.Test) {
      const existing = outputMap.get(event.Test) ?? "";
      outputMap.set(event.Test, existing + (event.Output ?? ""));
    }
  }

  const applied: AppliedResult[] = [];

  for (const event of events) {
    if (event.Action === "output" && event.Output) {
      const line = event.Output.replace(/\n$/, "\r\n");
      const testItem = event.Test
        ? resolveTestItem(controller, event.Test, importPath)
        : undefined;
      run.appendOutput(line, undefined, testItem);
    }

    if (!event.Test) {
      if (
        event.Action === "pass" ||
        event.Action === "fail" ||
        event.Action === "skip"
      ) {
        const duration =
          event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;
        applied.push({ itemId: importPath, status: event.Action, duration });
      }
      continue;
    }

    const item = resolveTestItem(controller, event.Test, importPath);
    if (!item) {
      continue;
    }

    const duration =
      event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;

    switch (event.Action) {
      case "pass":
        run.passed(item, duration);
        applied.push({ itemId: item.id, status: "pass", duration });
        break;
      case "fail": {
        const output = outputMap.get(event.Test) ?? "";
        const testMessages = extractTestMessages(output, pkgDir);
        const vscodeMessages = testMessages.map((msg) => {
          const message = new vscode.TestMessage(msg.message);
          message.location = new vscode.Location(
            vscode.Uri.file(msg.file),
            new vscode.Position(msg.line - 1, 0),
          );
          return message;
        });
        if (vscodeMessages.length === 0) {
          vscodeMessages.push(new vscode.TestMessage(output || "Test failed"));
        }
        run.failed(item, vscodeMessages, duration);
        applied.push({ itemId: item.id, status: "fail", duration });
        break;
      }
      case "skip":
        run.skipped(item);
        applied.push({ itemId: item.id, status: "skip", duration: undefined });
        break;
      case "run":
        run.started(item);
        break;
    }
  }

  return applied;
}

export interface SpawnResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

export function spawnTestProcess(
  bin: string,
  args: string[],
  cwd: string,
  token: vscode.CancellationToken,
  outputChannel: vscode.OutputChannel,
  label: string,
  env?: Record<string, string>,
): Promise<SpawnResult> {
  return new Promise<SpawnResult>((resolve, reject) => {
    const child = spawn(bin, args, {
      cwd,
      env: env ? { ...process.env, ...env } : undefined,
    });
    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
    });

    child.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
    });

    const cancelListener = token.onCancellationRequested(() => {
      child.kill("SIGTERM");
    });

    child.on("close", (code) => {
      cancelListener.dispose();
      if (stderr) {
        for (const line of stderr.split("\n")) {
          if (line.trim()) {
            outputChannel.appendLine(`[${label}] stderr: ${line}`);
          }
        }
      }
      resolve({ stdout, stderr, exitCode: code ?? 1 });
    });

    child.on("error", (err: Error) => {
      cancelListener.dispose();
      outputChannel.appendLine(`[${label}] error: ${err.message}`);
      reject(err);
    });
  });
}

export function buildRunFilter(items: vscode.TestItem[]): string | undefined {
  if (items.some((item) => getPackageDepth(item) === 0)) {
    return undefined;
  }

  const suiteGroups = new Map<
    string,
    { wholeSuite: boolean; methods: string[]; subtests: string[] }
  >();

  for (const item of items) {
    const depth = getPackageDepth(item);

    if (depth === 1) {
      const suiteName = item.label;
      let group = suiteGroups.get(suiteName);
      if (!group) {
        group = { wholeSuite: false, methods: [], subtests: [] };
        suiteGroups.set(suiteName, group);
      }
      group.wholeSuite = true;
    } else if (depth === 2) {
      const suiteName = item.parent!.label;
      let group = suiteGroups.get(suiteName);
      if (!group) {
        group = { wholeSuite: false, methods: [], subtests: [] };
        suiteGroups.set(suiteName, group);
      }
      group.methods.push(item.label);
    } else if (depth >= 3) {
      let current = item;
      const subtestParts: string[] = [];
      while (getPackageDepth(current) > 2) {
        subtestParts.unshift(current.label);
        current = current.parent!;
      }
      const methodName = current.label;
      const suiteName = current.parent!.label;
      let group = suiteGroups.get(suiteName);
      if (!group) {
        group = { wholeSuite: false, methods: [], subtests: [] };
        suiteGroups.set(suiteName, group);
      }
      group.subtests.push(
        `^Test${suiteName}$/^${methodName}$/^${subtestParts.join("/")}$`,
      );
    }
  }

  const filters: string[] = [];
  for (const [suiteName, group] of suiteGroups) {
    if (group.wholeSuite) {
      filters.push(`^Test${suiteName}$`);
    } else if (group.subtests.length > 0) {
      filters.push(...group.subtests);
    } else if (group.methods.length === 1) {
      filters.push(`^Test${suiteName}$/^${group.methods[0]}$`);
    } else if (group.methods.length > 1) {
      filters.push(`^Test${suiteName}$/^(${group.methods.join("|")})$`);
    }
  }

  return filters.length === 0
    ? undefined
    : filters.length === 1
      ? filters[0]
      : filters.join("|");
}

export function computeWildcard(importPaths: string[]): string | undefined {
  if (importPaths.length <= 1) return undefined;

  const split = importPaths.map((p) => p.split("/"));
  const first = split[0];
  let prefixLen = 0;
  for (let i = 0; i < first.length; i++) {
    if (split.every((s) => s[i] === first[i])) {
      prefixLen = i + 1;
    } else {
      break;
    }
  }
  if (prefixLen === 0) return undefined;

  const prefix = first.slice(0, prefixLen).join("/");
  if (importPaths.every((p) => p === prefix)) return undefined;

  return prefix + "/...";
}

export function getPackageDir(
  item: vscode.TestItem,
  cache: DiscoveryCache,
): string | undefined {
  const pkg = getPackageItem(item);
  return cache.getPackage(pkg.id)?.dir;
}

export function resolvePackageItems(
  run: vscode.TestRun,
  items: vscode.TestItem[],
  controller: GoTestController,
): void {
  for (const item of items) {
    const pkgResult = controller.getResult(item.id);
    if (pkgResult) {
      if (pkgResult.status === "fail") {
        run.failed(item, [], pkgResult.duration);
      } else {
        run.passed(item, pkgResult.duration);
      }
      continue;
    }

    let anyFailed = false;
    let anyResolved = false;
    const visit = (child: vscode.TestItem) => {
      const result = controller.getResult(child.id);
      if (result) {
        anyResolved = true;
        if (result.status === "fail") anyFailed = true;
      }
      child.children.forEach(visit);
    };
    item.children.forEach(visit);

    if (!anyResolved) continue;

    if (anyFailed) {
      run.failed(item, []);
    } else {
      run.passed(item);
    }
  }
}
