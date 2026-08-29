import * as vscode from "vscode";
import * as path from "node:path";
import { killProcessTree } from "./processTree.js";
import { ManagedChild } from "./managedChild.js";
import { readFile } from "node:fs/promises";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import {
  extractDiagnosticLocation,
  extractTestMessages,
  isPackageSummaryLine,
  parseExpectedActual,
  type TestEvent,
} from "./outputParser.js";

export { killProcessTree };

export function enqueueDescendants(
  run: vscode.TestRun,
  item: vscode.TestItem,
): void {
  item.children.forEach((child) => {
    run.enqueued(child);
    enqueueDescendants(run, child);
  });
}

export function skipUnresolved(
  run: vscode.TestRun,
  item: vscode.TestItem,
  controller: GoTestController,
): void {
  item.children.forEach((child) => {
    skipUnresolved(run, child, controller);
    if (!controller.getResult(child.id)) {
      run.skipped(child);
    }
  });
}

export function startAncestors(
  run: vscode.TestRun,
  items: vscode.TestItem[],
): void {
  const seen = new Set<string>();
  for (const item of items) {
    let ancestor = item.parent;
    while (ancestor) {
      if (seen.has(ancestor.id)) break;
      run.started(ancestor);
      seen.add(ancestor.id);
      ancestor = ancestor.parent;
    }
  }
}

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

// splitTestPath cuts a go test name into its subtest levels. A single slash
// separates levels, but a run of them does not: `t.When("https:// URI")` is one
// subtest whose name happens to contain slashes, and go test reports it that
// way. Splitting on every slash invented two levels that no run ever produced,
// so an observed result landed beside its declared behavior instead of on it.
// The CLI applies the identical rule on both sides of the boundary.
function splitTestPath(path: string): string[] {
  const segments: string[] = [];
  let cur = "";
  for (let i = 0; i < path.length; i++) {
    const isSeparator =
      path[i] === "/" &&
      (i + 1 >= path.length || path[i + 1] !== "/") &&
      (i === 0 || path[i - 1] !== "/");
    if (isSeparator) {
      segments.push(cur);
      cur = "";
    } else {
      cur += path[i];
    }
  }
  if (cur.length > 0) {
    segments.push(cur);
  }
  return segments;
}

export function resolveTestItem(
  controller: GoTestController,
  testPath: string,
  importPath: string,
): vscode.TestItem | undefined {
  const segments = splitTestPath(testPath);
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
    const segment = segments[i];
    // One segment per level: the id is the go test path, which is also what a
    // statically declared behavior uses. createDynamicSubtest returns the
    // declared item when there is one and only fabricates when there is not.
    parentItem = controller.createDynamicSubtest(parentItem, segment, segment);
  }

  return parentItem;
}

export interface AppliedResult {
  itemId: string;
  status: "pass" | "fail" | "skip";
  duration?: number;
}

export function applyEvent(
  controller: GoTestController,
  run: vscode.TestRun,
  event: TestEvent,
  outputMap: Map<string, string>,
  importPath: string,
  pkgDir: string,
): AppliedResult | undefined {
  if (event.Action === "output") {
    const key = event.Test ?? "";
    const output = event.Output ?? "";
    if (!(key === "" && isPackageSummaryLine(output))) {
      const existing = outputMap.get(key) ?? "";
      outputMap.set(key, existing + output);
    }
  }

  if (event.Action === "output" && event.Output) {
    if (!event.Test && /^exit status \d+\n?$/.test(event.Output)) {
      return undefined;
    }
    const line = event.Output.replace(/\n$/, "\r\n");
    const testItem = event.Test
      ? resolveTestItem(controller, event.Test, importPath)
      : undefined;
    run.appendOutput(line, undefined, testItem);
    return undefined;
  }

  if (!event.Test) {
    if (
      event.Action === "pass" ||
      event.Action === "fail" ||
      event.Action === "skip"
    ) {
      const duration =
        event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;
      controller.recordResult(importPath, event.Action, duration);

      const pkgItem = controller.findItem(importPath);
      if (pkgItem) {
        if (event.Action === "fail") {
          const output = outputMap.get("") ?? "";
          const message = new vscode.TestMessage(output || "Package failed");
          const loc = extractDiagnosticLocation(output, pkgDir);
          if (loc) {
            message.location = new vscode.Location(
              vscode.Uri.file(loc.file),
              new vscode.Position(loc.line - 1, 0),
            );
          }
          run.failed(pkgItem, [message], duration);
        } else if (event.Action === "pass") {
          run.passed(pkgItem, duration);
        } else {
          run.skipped(pkgItem);
        }
        resolveAncestorsOf(run, pkgItem, controller);
      }
      return { itemId: importPath, status: event.Action, duration };
    }
    return undefined;
  }

  const item = resolveTestItem(controller, event.Test, importPath);
  if (!item) {
    return undefined;
  }

  const duration =
    event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;
  // The event's own clock, not ours: it brackets the node against every other
  // node in the stream, including ones from suites running in other processes.
  const stamp = Date.parse(event.Time);
  const at = Number.isNaN(stamp) ? undefined : stamp;

  switch (event.Action) {
    case "pass":
      run.passed(item, duration);
      controller.recordResult(item.id, "pass", duration, at);
      return { itemId: item.id, status: "pass", duration };
    case "fail": {
      const output = outputMap.get(event.Test) ?? "";
      const testMessages = extractTestMessages(output, pkgDir);
      const vscodeMessages = testMessages.map((msg) => {
        const parsed = parseExpectedActual(msg.message);
        const message = new vscode.TestMessage(
          parsed
            ? `${msg.message.split("\n")[0].replace(/:\s*$/, "")}: expected ${parsed.expected}, actual ${parsed.actual}`
            : msg.message,
        );
        if (parsed) {
          message.expectedOutput = parsed.expected;
          message.actualOutput = parsed.actual;
        }
        message.location = new vscode.Location(
          vscode.Uri.file(msg.file),
          new vscode.Position(msg.line - 1, 0),
        );
        return message;
      });
      if (vscodeMessages.length === 0) {
        const fallback = new vscode.TestMessage(output || "Test failed");
        const loc = extractDiagnosticLocation(output, pkgDir);
        if (loc) {
          fallback.location = new vscode.Location(
            vscode.Uri.file(loc.file),
            new vscode.Position(loc.line - 1, 0),
          );
        }
        vscodeMessages.push(fallback);
      }
      run.failed(item, vscodeMessages, duration);
      controller.recordResult(item.id, "fail", duration, at);
      return { itemId: item.id, status: "fail", duration };
    }
    case "skip":
      run.skipped(item);
      controller.recordResult(item.id, "skip", undefined, at);
      return { itemId: item.id, status: "skip", duration: undefined };
    case "run":
      run.started(item);
      if (at !== undefined) {
        controller.noteStart(item.id, at);
      }
      return undefined;
    case "pause":
    case "cont":
      // The node called t.Parallel. That is the one fact its own timestamps
      // cannot survive, so it has to be carried alongside them.
      controller.notePaused(item.id);
      return undefined;
  }

  return undefined;
}

export function applyResults(
  controller: GoTestController,
  run: vscode.TestRun,
  events: TestEvent[],
  importPath: string,
  pkgDir: string,
): AppliedResult[] {
  const outputMap = new Map<string, string>();
  const applied: AppliedResult[] = [];
  for (const event of events) {
    const result = applyEvent(
      controller,
      run,
      event,
      outputMap,
      importPath,
      pkgDir,
    );
    if (result) applied.push(result);
  }
  return applied;
}

export interface SpawnResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

export async function spawnTestProcess(
  bin: string,
  args: string[],
  cwd: string,
  token: vscode.CancellationToken,
  outputChannel: vscode.LogOutputChannel,
  label: string,
  env?: Record<string, string>,
  onStdoutLine?: (line: string) => void,
): Promise<SpawnResult> {
  const mc = new ManagedChild(bin, args, { cwd, env });
  let stdout = "";
  let stderr = "";
  let lineBuffer = "";
  let spawnError: Error | undefined;

  // Line assembly is the caller's job: ManagedChild decodes the stream so a
  // character split across two reads survives, but what counts as an event is
  // specific to this surface.
  mc.child.stdout?.on("data", (chunk: string) => {
    stdout += chunk;

    if (onStdoutLine) {
      lineBuffer += chunk;
      const lines = lineBuffer.split("\n");
      lineBuffer = lines.pop() ?? "";
      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed) {
          onStdoutLine(trimmed);
        }
      }
    }
  });

  mc.child.stderr?.on("data", (chunk: string) => {
    stderr += chunk;
  });

  mc.child.on("error", (err: Error) => {
    spawnError = err;
  });

  // A run: SIGTERM is what starts fixture teardown, so it gets the teardown
  // grace rather than the prompt one.
  const cancelListener = token.onCancellationRequested(() => {
    outputChannel.info(
      `[${label}] cancellation requested, sending SIGTERM (pid ${mc.pid})`,
    );
    void mc.terminate("teardown");
  });

  try {
    const { code } = await mc.exited;
    if (spawnError) {
      outputChannel.error(`[${label}] ${spawnError.message}`);
      throw spawnError;
    }
    if (onStdoutLine) {
      const remaining = lineBuffer.trim();
      if (remaining) {
        onStdoutLine(remaining);
      }
    }
    if (stderr) {
      for (const line of stderr.split("\n")) {
        if (line.trim()) {
          outputChannel.warn(`[${label}] stderr: ${line}`);
        }
      }
    }
    return { stdout, stderr, exitCode: code ?? 1 };
  } finally {
    cancelListener.dispose();
    mc.dispose();
  }
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
        // The id segment, not the label: a behavior is labelled with the text
        // the developer wrote ("classifying a number") while go test knows it
        // by its rewritten name ("classifying_a_number"). Filtering on the
        // label would never match.
        subtestParts.unshift(subtestSegmentOf(current));
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
        `^Test${suiteName}$/^${methodName}$/^${subtestParts
          .map(escapeRunPattern)
          .join("/")}$`,
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

export function computeWildcard(
  importPaths: string[],
  modulePath?: string,
): string[] | undefined {
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
  if (!modulePath || prefix !== modulePath) return [prefix + "/..."];

  const groups = new Map<string, string[]>();
  const ungrouped: string[] = [];
  for (const p of importPaths) {
    if (p === modulePath) {
      ungrouped.push(p);
      continue;
    }
    const rest = p.slice(modulePath.length + 1);
    const seg = rest.split("/")[0];
    let group = groups.get(seg);
    if (!group) {
      group = [];
      groups.set(seg, group);
    }
    group.push(p);
  }

  const result: string[] = [...ungrouped];
  for (const [seg, paths] of groups) {
    if (paths.length === 1) {
      result.push(paths[0]);
    } else {
      result.push(modulePath + "/" + seg + "/...");
    }
  }

  return result.length < importPaths.length ? result : undefined;
}

/**
 * Determines the gotest package patterns for a batch of packages.
 * Uses workspace patterns (from go.work) when available and the common
 * prefix equals the module path. Otherwise falls back to computeWildcard.
 */
export function resolveRunPatterns(
  importPaths: string[],
  modulePath: string | undefined,
  workspacePatterns?: string[],
): string[] | undefined {
  if (importPaths.length <= 1) return undefined;

  // If workspace patterns are available (go.work) and the common prefix
  // equals the module path, use the workspace patterns directly.
  // This handles multi-module workspaces: ["./...", "./examples/..."]
  if (workspacePatterns && workspacePatterns.length > 0 && modulePath) {
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
    const prefix = first.slice(0, prefixLen).join("/");
    if (prefix === modulePath) {
      return workspacePatterns;
    }
  }

  return computeWildcard(importPaths, modulePath);
}

/**
 * Reads go.work use directives and returns per-module patterns.
 * Returns undefined if no go.work exists.
 */
export async function readWorkspacePatterns(
  dir: string,
): Promise<string[] | undefined> {
  try {
    const content = await readFile(path.join(dir, "go.work"), "utf-8");

    const blockMatch = /^\s*use\s*\(\s*([\s\S]*?)\s*\)/m.exec(content);
    if (blockMatch) {
      const dirs = blockMatch[1]
        .split("\n")
        .map((l) => l.trim())
        .filter((l) => l && !l.startsWith("//"));
      if (dirs.length > 0) {
        return dirs.map((d) => (d === "." ? "./..." : `${d}/...`));
      }
    }

    const singles: string[] = [];
    const singleUse = /^\s*use\s+(\S+)/gm;
    let m: RegExpExecArray | null;
    while ((m = singleUse.exec(content)) !== null) {
      singles.push(m[1]);
    }
    if (singles.length > 0) {
      return singles.map((d) => (d === "." ? "./..." : `${d}/...`));
    }
  } catch {}
  return undefined;
}

export function getPackageDir(
  item: vscode.TestItem,
  cache: DiscoveryCache,
): string | undefined {
  const pkg = getPackageItem(item);
  return cache.getPackage(pkg.id)?.dir;
}

export function resolveAncestorsOf(
  run: vscode.TestRun,
  item: vscode.TestItem,
  controller: GoTestController,
): void {
  let current = item.parent;
  while (current) {
    let anyFailed = false;
    let allResolved = true;
    current.children.forEach((child) => {
      const result = controller.getResult(child.id);
      if (result) {
        if (result.status === "fail") anyFailed = true;
        return;
      }
      const isStructural =
        child.id.startsWith("dir:") ||
        child.id.startsWith("wsFolder:") ||
        child.id.startsWith("module:");
      if (isStructural) {
        const r = resolveItemRecursive(run, child, controller);
        if (r.anyResolved) {
          if (r.anyFailed) anyFailed = true;
        } else {
          allResolved = false;
        }
      } else {
        allResolved = false;
      }
    });
    if (!allResolved) break;
    run.started(current);
    if (anyFailed) {
      run.failed(current, []);
    } else {
      run.passed(current);
    }
    controller.recordResult(current.id, anyFailed ? "fail" : "pass", undefined);
    current = current.parent;
  }
}

export function resolveAncestorItems(
  run: vscode.TestRun,
  controller: GoTestController,
): void {
  controller.testController.items.forEach((item) => {
    resolveItemRecursive(run, item, controller);
  });
}

function resolveItemRecursive(
  run: vscode.TestRun,
  item: vscode.TestItem,
  controller: GoTestController,
): { anyFailed: boolean; anyResolved: boolean } {
  const directResult = controller.getResult(item.id);
  const isPackage = item.tags.some((t) => t.id === "package");
  const isStructural =
    item.id.startsWith("dir:") ||
    item.id.startsWith("wsFolder:") ||
    item.id.startsWith("module:");

  if (directResult && !isPackage && !isStructural) {
    return { anyFailed: directResult.status === "fail", anyResolved: true };
  }

  let anyFailed = false;
  let anyResolved = false;
  item.children.forEach((child) => {
    const r = resolveItemRecursive(run, child, controller);
    if (r.anyResolved) anyResolved = true;
    if (r.anyFailed) anyFailed = true;
  });

  if (anyResolved) {
    run.started(item);
    if (anyFailed) {
      run.failed(item, []);
    } else {
      run.passed(item);
    }
    if (isStructural) {
      controller.recordResult(item.id, anyFailed ? "fail" : "pass", undefined);
    }
  }

  return { anyFailed, anyResolved };
}

// subtestSegmentOf returns the single go test path segment an item adds to its
// parent. Ids are the test path, so the segment is what follows the parent's id.
function subtestSegmentOf(item: vscode.TestItem): string {
  const parentId = item.parent?.id;
  if (parentId && item.id.startsWith(parentId + "/")) {
    return item.id.slice(parentId.length + 1);
  }
  return item.label;
}

// escapeRunPattern quotes regex metacharacters in a subtest name. Behavior
// descriptions are prose — "handles (nested) values" is an ordinary thing to
// write — and go test's -run is a regular expression, so the name has to be
// matched literally rather than interpreted.
function escapeRunPattern(segment: string): string {
  return segment.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
