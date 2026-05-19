import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import { TestResultStore, type TestResult } from "./testResultStore.js";
export type { TestResult } from "./testResultStore.js";
import { type PathNode, buildPathTrie, collapsePathTrie } from "./pathTrie.js";

export class GoTestController implements vscode.Disposable {
  private static readonly MAX_DYNAMIC_SUBTESTS = 100;

  private controller: vscode.TestController;
  private disposables: vscode.Disposable[] = [];
  private coverageProfile: vscode.TestRunProfile | undefined;
  private dynamicOverflow = new Map<string, number>();

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly resultStore: TestResultStore,
    private readonly outputChannel: vscode.LogOutputChannel,
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
      "gotest",
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

    this.coverageProfile = this.controller.createRunProfile(
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
    const isMultiFolder = wsGroups.size > 1;

    for (const [wsDir, entries] of wsGroups) {
      const trie = buildPathTrie(entries);
      collapsePathTrie(trie);

      const idPrefix = isMultiFolder ? `${this.folderName(wsDir)}/` : "";
      const target = isMultiFolder
        ? this.getOrCreateFolderItem(wsDir, seenIds, rootIds)
        : this.controller.items;

      if (trie.importPath && trie.children.size === 0) {
        this.addPackageItem(trie.importPath, target, seenIds);
        if (!isMultiFolder) rootIds.add(trie.importPath);
      } else if (trie.importPath) {
        this.addPackageItem(trie.importPath, target, seenIds);
        if (!isMultiFolder) rootIds.add(trie.importPath);
        for (const child of trie.children.values()) {
          this.addTrieNode(child, target, seenIds, idPrefix);
          if (!isMultiFolder) {
            rootIds.add(
              child.importPath && child.children.size === 0
                ? child.importPath
                : `dir:${child.segment}`,
            );
          }
        }
      } else {
        for (const child of trie.children.values()) {
          this.addTrieNode(child, target, seenIds, idPrefix);
          if (!isMultiFolder) {
            rootIds.add(
              child.importPath && child.children.size === 0
                ? child.importPath
                : `dir:${child.segment}`,
            );
          }
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
    idPrefix = "",
  ): void {
    if (node.importPath && node.children.size === 0) {
      this.addPackageItem(node.importPath, parent, seenIds);
      return;
    }

    const dirId = `dir:${idPrefix}${node.segment}`;
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
      this.addTrieNode(child, dirItem.children, seenIds, idPrefix);
      const childId =
        child.importPath && child.children.size === 0
          ? child.importPath
          : `dir:${idPrefix}${child.segment}`;
      seenChildIds.add(childId);
    }

    dirItem.children.forEach((child) => {
      if (!seenChildIds.has(child.id) && !child.id.includes("/dynamic/")) {
        dirItem.children.delete(child.id);
      }
    });
  }

  private folderName(wsDir: string): string {
    return (
      vscode.workspace.getWorkspaceFolder(vscode.Uri.file(wsDir))?.name ??
      path.basename(wsDir)
    );
  }

  private getOrCreateFolderItem(
    wsDir: string,
    seenIds: Set<string>,
    rootIds: Set<string>,
  ): vscode.TestItemCollection {
    const name = this.folderName(wsDir);
    const id = `wsFolder:${name}`;
    seenIds.add(id);
    rootIds.add(id);

    let item = this.controller.items.get(id);
    if (!item) {
      item = this.controller.createTestItem(id, name);
    }
    this.controller.items.add(item);
    return item.children;
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
    if (this.dynamicOverflow.delete(item.id)) {
      item.description = undefined;
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

    if (parentItem.children.size >= GoTestController.MAX_DYNAMIC_SUBTESTS) {
      const overflow = (this.dynamicOverflow.get(parentItem.id) ?? 0) + 1;
      this.dynamicOverflow.set(parentItem.id, overflow);
      parentItem.description = `${parentItem.children.size + overflow} subtests (${parentItem.children.size} shown)`;
      return parentItem;
    }

    const item = this.controller.createTestItem(id, label, parentItem.uri);
    parentItem.children.add(item);
    return item;
  }

  setCoverageDetailProvider(
    provider: (uri: vscode.Uri) => vscode.FileCoverageDetail[],
  ): void {
    if (this.coverageProfile) {
      this.coverageProfile.loadDetailedCoverage = async (
        _testRun,
        fileCoverage,
        _token,
      ) => provider(fileCoverage.uri);
    }
  }

  createTestRun(request: vscode.TestRunRequest, name: string): vscode.TestRun {
    return this.controller.createTestRun(request, name);
  }

  findItem(id: string): vscode.TestItem | undefined {
    return this.findItemRecursive(this.controller.items, id);
  }

  recordResult(
    itemId: string,
    status: "pass" | "fail" | "skip",
    duration?: number,
  ): void {
    this.resultStore.record(itemId, status, duration);
  }

  getResult(itemId: string): TestResult | undefined {
    return this.resultStore.get(itemId);
  }

  clearResults(item: vscode.TestItem): void {
    this.resultStore.delete(item.id);
    item.children.forEach((child) => this.clearResults(child));
    this.clearDynamicChildren(item);
  }

  saveResults(): void {
    this.resultStore.save();
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
