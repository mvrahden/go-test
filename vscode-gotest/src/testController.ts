import * as vscode from "vscode";
import type { DiscoveryCache } from "./discovery.js";

export class GoTestController implements vscode.Disposable {
  private controller: vscode.TestController;
  private disposables: vscode.Disposable[] = [];

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

    this.disposables.push(
      this.cache.onDidUpdate(() => this.rebuild()),
    );
  }

  get testController(): vscode.TestController {
    return this.controller;
  }

  rebuild(): void {
    const packages = this.cache.packages;
    const seenPackageIds = new Set<string>();

    for (const pkg of packages) {
      const pkgId = pkg.importPath;
      seenPackageIds.add(pkgId);

      let pkgItem = this.controller.items.get(pkgId);
      if (!pkgItem) {
        pkgItem = this.controller.createTestItem(pkgId, pkgId);
      }
      this.controller.items.add(pkgItem);

      const seenSuiteIds = new Set<string>();

      for (const suite of pkg.suites) {
        const suiteId = `${pkgId}/${suite.name}`;
        seenSuiteIds.add(suiteId);

        const suiteUri = vscode.Uri.file(suite.file);
        let suiteItem = pkgItem.children.get(suiteId);
        if (!suiteItem) {
          suiteItem = this.controller.createTestItem(suiteId, suite.name, suiteUri);
        }
        suiteItem.range = new vscode.Range(
          new vscode.Position(suite.line - 1, suite.col - 1),
          new vscode.Position(suite.line - 1, suite.col - 1),
        );
        suiteItem.tags = this.buildTags(
          suite.focused,
          suite.excluded,
          suite.parallel,
        );
        pkgItem.children.add(suiteItem);

        const seenMethodIds = new Set<string>();

        for (const method of suite.methods) {
          const methodId = `${suiteId}/${method.name}`;
          seenMethodIds.add(methodId);

          const methodUri = vscode.Uri.file(method.file);
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

        // Remove stale methods, but preserve dynamic subtests
        suiteItem.children.forEach((child) => {
          if (!seenMethodIds.has(child.id) && !child.id.includes("/dynamic/")) {
            suiteItem.children.delete(child.id);
          }
        });
      }

      // Remove stale suites, but preserve dynamic subtests
      pkgItem.children.forEach((child) => {
        if (!seenSuiteIds.has(child.id) && !child.id.includes("/dynamic/")) {
          pkgItem.children.delete(child.id);
        }
      });
    }

    // Remove stale packages, but preserve dynamic subtests
    this.controller.items.forEach((child) => {
      if (
        !seenPackageIds.has(child.id) &&
        !child.id.includes("/dynamic/")
      ) {
        this.controller.items.delete(child.id);
      }
    });
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

  createTestRun(
    request: vscode.TestRunRequest,
    name: string,
  ): vscode.TestRun {
    return this.controller.createTestRun(request, name);
  }

  findItem(id: string): vscode.TestItem | undefined {
    return this.findItemRecursive(this.controller.items, id);
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
    return tags;
  }
}
