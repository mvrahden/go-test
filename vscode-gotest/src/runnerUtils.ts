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
  return items;
}

export function groupByPackage(
  items: vscode.TestItem[],
): Map<string, vscode.TestItem[]> {
  const groups = new Map<string, vscode.TestItem[]>();
  for (const item of items) {
    const root = getRootItem(item);
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

export function applyResults(
  controller: GoTestController,
  run: vscode.TestRun,
  events: TestEvent[],
  importPath: string,
  pkgDir: string,
): void {
  const outputMap = new Map<string, string>();

  for (const event of events) {
    if (event.Action === "output" && event.Test) {
      const existing = outputMap.get(event.Test) ?? "";
      outputMap.set(event.Test, existing + (event.Output ?? ""));
    }
  }

  for (const event of events) {
    if (event.Action === "output" && event.Output) {
      const line = event.Output.replace(/\n$/, "\r\n");
      const testItem = event.Test
        ? resolveTestItem(controller, event.Test, importPath)
        : undefined;
      run.appendOutput(line, undefined, testItem);
    }

    if (!event.Test) {
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
        break;
      }
      case "skip":
        run.skipped(item);
        break;
      case "run":
        run.started(item);
        break;
    }
  }
}

export function spawnTestProcess(
  bin: string,
  args: string[],
  cwd: string,
  token: vscode.CancellationToken,
  outputChannel: vscode.OutputChannel,
  label: string,
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const child = spawn(bin, args, { cwd });
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
        outputChannel.appendLine(`[${label}] stderr: ${stderr}`);
      }
      resolve(stdout);
    });

    child.on("error", (err: Error) => {
      cancelListener.dispose();
      outputChannel.appendLine(`[${label}] error: ${err.message}`);
      reject(err);
    });
  });
}

export function suiteHasFixtures(
  suiteName: string,
  importPath: string,
  cache: DiscoveryCache,
): boolean {
  const pkg = cache.getPackage(importPath);
  if (!pkg) {
    return false;
  }
  const suite = pkg.suites.find((s) => s.name === suiteName);
  return suite !== undefined && suite.fixtures.length > 0;
}

export function buildRunFilter(
  items: vscode.TestItem[],
  importPath: string,
  cache: DiscoveryCache,
): string | undefined {
  if (items.some((item) => getItemDepth(item) === 0)) {
    return undefined;
  }

  const suiteGroups = new Map<
    string,
    { wholeSuite: boolean; methods: string[]; subtests: string[] }
  >();

  for (const item of items) {
    const depth = getItemDepth(item);

    if (depth === 1) {
      const suiteName = item.label;
      if (suiteHasFixtures(suiteName, importPath, cache)) {
        return undefined;
      }
      let group = suiteGroups.get(suiteName);
      if (!group) {
        group = { wholeSuite: false, methods: [], subtests: [] };
        suiteGroups.set(suiteName, group);
      }
      group.wholeSuite = true;
    } else if (depth === 2) {
      const suiteName = item.parent!.label;
      if (suiteHasFixtures(suiteName, importPath, cache)) {
        return undefined;
      }
      let group = suiteGroups.get(suiteName);
      if (!group) {
        group = { wholeSuite: false, methods: [], subtests: [] };
        suiteGroups.set(suiteName, group);
      }
      group.methods.push(item.label);
    } else if (depth >= 3) {
      let current = item;
      const subtestParts: string[] = [];
      while (getItemDepth(current) > 2) {
        subtestParts.unshift(current.label);
        current = current.parent!;
      }
      const methodName = current.label;
      const suiteName = current.parent!.label;
      if (suiteHasFixtures(suiteName, importPath, cache)) {
        return undefined;
      }
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

export function getPackageDir(
  item: vscode.TestItem,
  cache: DiscoveryCache,
): string | undefined {
  let current: vscode.TestItem | undefined = item;
  while (current?.parent) {
    current = current.parent;
  }
  return cache.getPackage(current?.id || "")?.dir;
}
