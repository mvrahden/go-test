import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";

export interface PathNode {
  segment: string;
  children: Map<string, PathNode>;
  importPath?: string;
}

export function buildPathTrie(
  entries: { relativePath: string; importPath: string }[],
): PathNode {
  const root: PathNode = { segment: "", children: new Map() };

  for (const entry of entries) {
    if (entry.relativePath === ".") {
      root.importPath = entry.importPath;
      continue;
    }
    const parts = entry.relativePath.split("/");
    let current = root;
    for (const part of parts) {
      let child = current.children.get(part);
      if (!child) {
        child = { segment: part, children: new Map() };
        current.children.set(part, child);
      }
      current = child;
    }
    current.importPath = entry.importPath;
  }

  return root;
}

export function collapsePathTrie(node: PathNode): void {
  const collapsed = new Map<string, PathNode>();
  for (const [, child] of node.children) {
    collapseNode(child);
    collapsed.set(child.segment, child);
  }
  node.children = collapsed;
}

function collapseNode(node: PathNode): void {
  while (node.children.size === 1 && !node.importPath) {
    const [, child] = [...node.children.entries()][0];
    node.segment = node.segment
      ? `${node.segment}/${child.segment}`
      : child.segment;
    node.importPath = child.importPath;
    node.children = child.children;
  }
  const collapsed = new Map<string, PathNode>();
  for (const [, child] of node.children) {
    collapseNode(child);
    collapsed.set(child.segment, child);
  }
  node.children = collapsed;
}

export interface TestResult {
  status: "pass" | "fail" | "skip";
  duration?: number;
}

export class GoTestController implements vscode.Disposable {
  private controller: vscode.TestController;
  private disposables: vscode.Disposable[] = [];
  private results = new Map<string, TestResult>();

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    runHandler: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
    debugHandler: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
    coverageHandler: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
  ) {
    this.controller = vscode.tests.createTestController(
      "gotest",
      "Go Test Suites",
    );

    this.controller.createRunProfile(
      "Run",
      vscode.TestRunProfileKind.Run,
      (request, token) => runHandler(request, token),
      true,
    );

    this.controller.createRunProfile(
      "Debug",
      vscode.TestRunProfileKind.Debug,
      (request, token) => debugHandler(request, token),
      true,
    );

    this.controller.createRunProfile(
      "Coverage",
      vscode.TestRunProfileKind.Coverage,
      (request, token) => coverageHandler(request, token),
      false,
    );

    let rebuildTimer: ReturnType<typeof setTimeout> | undefined;
    this.disposables.push(
      this.cache.onDidUpdate(() => {
        if (rebuildTimer) clearTimeout(rebuildTimer);
        rebuildTimer = setTimeout(() => {
          rebuildTimer = undefined;
          this.rebuild();
        }, 50);
      }),
    );
  }

  get testController(): vscode.TestController {
    return this.controller;
  }

  rebuild(): void {
    const packages = this.cache.packages;
    const wsGroups = new Map<
      string,
      { relativePath: string; importPath: string }[]
    >();

    for (const pkg of packages) {
      const wsDir = this.cache.getWorkspaceDir(pkg.importPath);
      if (!wsDir) continue;

      let relativePath = pkg.dir.startsWith(wsDir)
        ? pkg.dir.slice(wsDir.length).replace(/^[/\\]+/, "")
        : pkg.dir;
      if (!relativePath) relativePath = ".";

      let group = wsGroups.get(wsDir);
      if (!group) {
        group = [];
        wsGroups.set(wsDir, group);
      }
      group.push({ relativePath, importPath: pkg.importPath });
    }

    const seenIds = new Set<string>();
    const rootIds = new Set<string>();

    for (const [_wsDir, entries] of wsGroups) {
      const trie = buildPathTrie(entries);
      collapsePathTrie(trie);

      if (trie.importPath && trie.children.size === 0) {
        this.addPackageItem(trie.importPath, this.controller.items, seenIds);
        rootIds.add(trie.importPath);
      } else if (trie.importPath) {
        this.addPackageItem(trie.importPath, this.controller.items, seenIds);
        rootIds.add(trie.importPath);
        for (const child of trie.children.values()) {
          this.addTrieNode(child, this.controller.items, seenIds);
          rootIds.add(
            child.importPath && child.children.size === 0
              ? child.importPath
              : `dir:${child.segment}`,
          );
        }
      } else {
        for (const child of trie.children.values()) {
          this.addTrieNode(child, this.controller.items, seenIds);
          rootIds.add(
            child.importPath && child.children.size === 0
              ? child.importPath
              : `dir:${child.segment}`,
          );
        }
      }
    }

    this.controller.items.forEach((child) => {
      if (!rootIds.has(child.id) && !child.id.includes("/dynamic/")) {
        this.controller.items.delete(child.id);
      }
    });
  }

  private addTrieNode(
    node: PathNode,
    parent: vscode.TestItemCollection,
    seenIds: Set<string>,
  ): void {
    if (node.importPath && node.children.size === 0) {
      this.addPackageItem(node.importPath, parent, seenIds);
      return;
    }

    const dirId = `dir:${node.segment}`;
    seenIds.add(dirId);

    let dirItem = parent.get(dirId);
    if (!dirItem) {
      dirItem = this.controller.createTestItem(dirId, node.segment);
    }
    parent.add(dirItem);

    if (node.importPath) {
      this.addPackageItem(node.importPath, dirItem.children, seenIds);
    }

    const seenChildIds = new Set<string>();
    for (const child of node.children.values()) {
      this.addTrieNode(child, dirItem.children, seenIds);
      const childId =
        child.importPath && child.children.size === 0
          ? child.importPath
          : `dir:${child.segment}`;
      seenChildIds.add(childId);
    }

    dirItem.children.forEach((child) => {
      if (!seenChildIds.has(child.id) && !child.id.includes("/dynamic/")) {
        dirItem.children.delete(child.id);
      }
    });
  }

  private addPackageItem(
    importPath: string,
    parent: vscode.TestItemCollection,
    seenIds: Set<string>,
  ): void {
    const pkg = this.cache.getPackage(importPath);
    if (!pkg) return;

    seenIds.add(importPath);

    let pkgItem = parent.get(importPath);
    if (!pkgItem) {
      const label = pkg.dir.split("/").pop() || importPath;
      pkgItem = this.controller.createTestItem(importPath, label);
    }
    pkgItem.tags = [
      new vscode.TestTag("package"),
      ...this.buildTags(false, false, false),
    ];
    pkgItem.description = importPath;
    parent.add(pkgItem);

    const seenSuiteIds = new Set<string>();

    for (const suite of pkg.suites) {
      const suiteId = `${importPath}/${suite.name}`;
      seenSuiteIds.add(suiteId);

      const suiteUri = vscode.Uri.file(path.join(pkg.dir, suite.file));
      let suiteItem = pkgItem.children.get(suiteId);
      if (!suiteItem) {
        suiteItem = this.controller.createTestItem(
          suiteId,
          suite.name,
          suiteUri,
        );
      }
      suiteItem.range = new vscode.Range(
        new vscode.Position(suite.line - 1, suite.col - 1),
        new vscode.Position(suite.line - 1, suite.col - 1),
      );
      suiteItem.tags = this.buildTags(
        suite.focused,
        suite.excluded,
        suite.parallel,
        suite.guarded,
      );
      suiteItem.description = suite.guarded ? "guarded" : undefined;
      pkgItem.children.add(suiteItem);

      const seenMethodIds = new Set<string>();

      for (const method of suite.methods) {
        const methodId = `${suiteId}/${method.name}`;
        seenMethodIds.add(methodId);

        const methodUri = vscode.Uri.file(path.join(pkg.dir, method.file));
        let methodItem = suiteItem.children.get(methodId);
        if (!methodItem) {
          methodItem = this.controller.createTestItem(
            methodId,
            method.name,
            methodUri,
          );
        }
        methodItem.range = new vscode.Range(
          new vscode.Position(method.line - 1, method.col - 1),
          new vscode.Position(method.line - 1, method.col - 1),
        );
        methodItem.tags = this.buildTags(
          method.focused,
          method.excluded,
          method.parallel,
        );
        suiteItem.children.add(methodItem);
      }

      suiteItem.children.forEach((child) => {
        if (!seenMethodIds.has(child.id) && !child.id.includes("/dynamic/")) {
          suiteItem.children.delete(child.id);
        }
      });
    }

    pkgItem.children.forEach((child) => {
      if (!seenSuiteIds.has(child.id) && !child.id.includes("/dynamic/")) {
        pkgItem.children.delete(child.id);
      }
    });
  }

  clearDynamicChildren(item: vscode.TestItem): void {
    const toDelete: string[] = [];
    item.children.forEach((child) => {
      if (child.id.includes("/dynamic/")) {
        toDelete.push(child.id);
      }
    });
    for (const id of toDelete) {
      item.children.delete(id);
    }
  }

  createDynamicSubtest(
    parentItem: vscode.TestItem,
    subtestPath: string,
    label: string,
  ): vscode.TestItem {
    const id = `${parentItem.id}/dynamic/${subtestPath}`;
    const existing = parentItem.children.get(id);
    if (existing) {
      return existing;
    }

    const item = this.controller.createTestItem(id, label, parentItem.uri);
    parentItem.children.add(item);
    return item;
  }

  createTestRun(request: vscode.TestRunRequest, name: string): vscode.TestRun {
    return this.controller.createTestRun(request, name);
  }

  findItem(id: string): vscode.TestItem | undefined {
    return this.findItemRecursive(this.controller.items, id);
  }

  recordResult(
    itemId: string,
    status: TestResult["status"],
    duration?: number,
  ): void {
    this.results.set(itemId, { status, duration });
  }

  async copyTestResults(rootItem?: vscode.TestItem): Promise<void> {
    type Agg = {
      passed: number;
      failed: number;
      skipped: number;
      duration: number;
    };
    type Row = {
      label: string;
      duration?: number;
      status?: string;
      agg?: Agg;
    };
    const rows: Row[] = [];

    const walkItem = (item: vscode.TestItem, indent: number): Agg => {
      const structural =
        item.id.startsWith("dir:") || item.tags.some((t) => t.id === "package");
      const result = structural ? undefined : this.results.get(item.id);

      const rowIdx = rows.length;
      rows.push({
        label: "  ".repeat(indent) + item.label,
        duration: result?.duration,
        status: result?.status,
      });

      const childAgg: Agg = { passed: 0, failed: 0, skipped: 0, duration: 0 };
      item.children.forEach((child) => {
        const ca = walkItem(child, indent + 1);
        childAgg.passed += ca.passed;
        childAgg.failed += ca.failed;
        childAgg.skipped += ca.skipped;
        childAgg.duration += ca.duration;
      });

      if (item.children.size > 0) {
        rows[rowIdx].agg = childAgg;
        return childAgg;
      }

      const leafAgg: Agg = { passed: 0, failed: 0, skipped: 0, duration: 0 };
      if (result?.status === "pass") leafAgg.passed = 1;
      else if (result?.status === "fail") leafAgg.failed = 1;
      else if (result?.status === "skip") leafAgg.skipped = 1;
      if (result?.duration) leafAgg.duration = result.duration;
      return leafAgg;
    };

    const resolved = rootItem ? this.findItem(rootItem.id) : undefined;
    if (resolved) {
      walkItem(resolved, 0);
    } else {
      this.controller.items.forEach((item) => walkItem(item, 0));
    }

    if (rows.length === 0) {
      vscode.window.showInformationMessage(
        "No test items available. Run discovery first.",
      );
      return;
    }

    const maxLabelLen = Math.max(4, ...rows.map((r) => r.label.length));
    const header = `${"Test".padEnd(maxLabelLen)}  Time       Result`;
    const separator = "-".repeat(header.length);

    const lines = [header, separator];
    let totalPassed = 0;
    let totalFailed = 0;
    let totalSkipped = 0;
    let totalDuration = 0;

    for (const row of rows) {
      if (row.agg) {
        const a = row.agg;
        const aggTime =
          a.duration > 0 ? (a.duration / 1000).toFixed(3) + "s" : "-";
        const parts: string[] = [];
        if (a.passed > 0) parts.push(`${a.passed} passed`);
        if (a.failed > 0) parts.push(`${a.failed} failed`);
        if (a.skipped > 0) parts.push(`${a.skipped} skipped`);
        const aggSummary = parts.length > 0 ? parts.join(", ") : "-";
        lines.push(
          `${row.label.padEnd(maxLabelLen)}  ${aggTime.padEnd(9)}  ${aggSummary}`,
        );
        continue;
      }

      const time =
        row.duration !== undefined
          ? (row.duration / 1000).toFixed(3) + "s"
          : "-";
      const status = row.status ?? "-";
      lines.push(
        `${row.label.padEnd(maxLabelLen)}  ${time.padEnd(9)}  ${status}`,
      );

      if (row.status === "pass") totalPassed++;
      else if (row.status === "fail") totalFailed++;
      else if (row.status === "skip") totalSkipped++;
      if (row.duration) totalDuration += row.duration;
    }

    lines.push(separator);
    const hasResults = totalPassed + totalFailed + totalSkipped > 0;
    if (hasResults) {
      const parts: string[] = [];
      if (totalPassed > 0) parts.push(`${totalPassed} passed`);
      if (totalFailed > 0) parts.push(`${totalFailed} failed`);
      if (totalSkipped > 0) parts.push(`${totalSkipped} skipped`);
      lines.push(
        `Total: ${parts.join(", ")} (${(totalDuration / 1000).toFixed(3)}s)`,
      );
    } else {
      lines.push("Total: no results");
    }

    const text = lines.join("\n");
    await vscode.env.clipboard.writeText(text);
    vscode.window.showInformationMessage("Test results copied to clipboard.");
  }

  dispose(): void {
    for (const d of this.disposables) {
      d.dispose();
    }
    this.disposables = [];
    this.controller.dispose();
  }

  private findItemRecursive(
    collection: vscode.TestItemCollection,
    id: string,
  ): vscode.TestItem | undefined {
    const direct = collection.get(id);
    if (direct) {
      return direct;
    }
    let found: vscode.TestItem | undefined;
    collection.forEach((item) => {
      if (!found) {
        found = this.findItemRecursive(item.children, id);
      }
    });
    return found;
  }

  private buildTags(
    focused: boolean,
    excluded: boolean,
    parallel: boolean,
    guarded?: boolean,
  ): readonly vscode.TestTag[] {
    const tags: vscode.TestTag[] = [];
    if (focused) {
      tags.push(new vscode.TestTag("focused"));
    }
    if (excluded) {
      tags.push(new vscode.TestTag("excluded"));
    }
    if (parallel) {
      tags.push(new vscode.TestTag("parallel"));
    }
    if (guarded) {
      tags.push(new vscode.TestTag("guarded"));
    }
    return tags;
  }
}
