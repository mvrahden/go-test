import * as vscode from "vscode";
import {
  EXTERNAL_ID,
  methodItemId,
  suiteItemId,
  type DiscoverBehavior,
} from "./types.js";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import { TestResultStore, type TestResult } from "./testResultStore.js";
import { type PathNode, buildPathTrie, collapsePathTrie } from "./pathTrie.js";
import { tracked } from "./runStates.js";

export class GoTestController implements vscode.Disposable {
  private static readonly MAX_DYNAMIC_SUBTESTS = 100;

  private controller: vscode.TestController;
  private disposables: vscode.Disposable[] = [];
  private coverageProfile: vscode.TestRunProfile | undefined;
  private dynamicOverflow = new Map<string, number>();
  // Items created from run events rather than from source. Behavior ids no
  // longer carry a marker segment — they are the go test path — so membership
  // is tracked instead of inferred from the string.
  private dynamicIds = new Set<string>();
  // Open brackets: nodes go test has started and not yet reported on, and
  // which of them parked themselves with t.Parallel.
  private pendingStarts = new Map<string, number>();
  private pendingPaused = new Set<string>();
  // Every item in the tree by id, kept by attach and detach. A list because
  // two modules can each hold a dir: item of the same id; the first answers.
  private index = new Map<string, vscode.TestItem[]>();

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
    updateSnapshotsHandler: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
    benchHandler?: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
    fuzzHandler?: (
      request: vscode.TestRunRequest,
      token: vscode.CancellationToken,
    ) => Promise<void>,
  ) {
    this.controller = vscode.tests.createTestController("gotest", "gotest");

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

    this.controller.createRunProfile(
      "Update Snapshots",
      vscode.TestRunProfileKind.Run,
      (request, token) => updateSnapshotsHandler(request, token),
      false,
    );

    // Benchmarks get their own profile, scoped to the "benchmark" tag so it
    // is only offered on benchmark items — and benchmark items never run as
    // part of a normal test run (the run handler filters the tag out).
    // A benchmark answers with a number, not pass/fail; mixing the two run
    // kinds would produce meaningless timings on loaded machines.
    if (benchHandler) {
      this.controller.createRunProfile(
        "Bench",
        vscode.TestRunProfileKind.Run,
        (request, token) => benchHandler(request, token),
        false,
        new vscode.TestTag("benchmark"),
      );
    }

    // Fuzz mirrors Bench: opt-in from the dropdown, offered only where fuzz
    // targets are selected, never part of plain Run (which replays seeds).
    if (fuzzHandler) {
      this.controller.createRunProfile(
        "Fuzz",
        vscode.TestRunProfileKind.Run,
        (request, token) => fuzzHandler(request, token),
        false,
        new vscode.TestTag("fuzz"),
      );
    }

    // Coalesces bursts — a watch event can touch several packages at once, and
    // each one fires the cache's update. Anything that reads the tree flushes
    // it first, so the delay never becomes visible as a missing item.
    this.disposables.push(
      this.cache.onDidUpdate(() => {
        if (this.rebuildTimer) clearTimeout(this.rebuildTimer);
        this.rebuildTimer = setTimeout(() => {
          this.rebuildTimer = undefined;
          this.rebuild();
        }, 50);
      }),
    );
  }

  private rebuildTimer: ReturnType<typeof setTimeout> | undefined;

  // flushPendingRebuild applies a debounced rebuild that has not fired yet, so
  // a caller reading the tree never observes the delay as a missing item.
  private flushPendingRebuild(): void {
    if (!this.rebuildTimer) return;
    clearTimeout(this.rebuildTimer);
    this.rebuildTimer = undefined;
    this.rebuild();
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
      const modules = this.cache.getModules(wsDir);
      const isMultiModule = modules.length > 1;
      const target = isMultiFolder
        ? this.getOrCreateFolderItem(wsDir, seenIds, rootIds)
        : this.controller.items;

      if (isMultiModule) {
        // Group entries by module
        const byModule = new Map<string, typeof entries>();
        for (const entry of entries) {
          const mod = this.cache.getModulePath(entry.importPath) ?? "";
          let group = byModule.get(mod);
          if (!group) {
            group = [];
            byModule.set(mod, group);
          }
          group.push(entry);
        }

        for (const [modulePath, moduleEntries] of byModule) {
          const moduleDir = this.cache.getModuleDir(modulePath);

          // Compute module-relative prefix (e.g. "examples" for a module at /ws/examples)
          const moduleRelPrefix =
            moduleDir && moduleDir.startsWith(wsDir)
              ? moduleDir.slice(wsDir.length).replace(/^[/\\]+/, "")
              : "";

          // Rewrite entries so relativePath is relative to module dir, not workspace dir
          const moduleRelEntries = moduleEntries.map((e) => ({
            relativePath:
              moduleRelPrefix && e.relativePath.startsWith(moduleRelPrefix)
                ? e.relativePath
                    .slice(moduleRelPrefix.length)
                    .replace(/^[/\\]+/, "") || "."
                : e.relativePath,
            importPath: e.importPath,
          }));

          // Create module node
          const moduleLabel =
            moduleRelPrefix || modulePath.split("/").pop() || modulePath;
          const moduleId = `module:${modulePath}`;
          seenIds.add(moduleId);
          if (!isMultiFolder) rootIds.add(moduleId);

          let moduleItem = target.get(moduleId);
          if (!moduleItem) {
            moduleItem = this.controller.createTestItem(moduleId, moduleLabel);
          }
          moduleItem.tags = [new vscode.TestTag("module")];
          moduleItem.description = "module";
          this.attach(target, moduleItem);

          // Build path trie within this module
          const trie = buildPathTrie(moduleRelEntries);
          collapsePathTrie(trie);

          const seenModuleChildIds = new Set<string>();

          if (trie.importPath && trie.children.size === 0) {
            this.addPackageItem(trie.importPath, moduleItem.children, seenIds);
            seenModuleChildIds.add(trie.importPath);
          } else if (trie.importPath) {
            this.addPackageItem(trie.importPath, moduleItem.children, seenIds);
            seenModuleChildIds.add(trie.importPath);
            for (const child of trie.children.values()) {
              this.addTrieNode(child, moduleItem.children, seenIds, "");
              seenModuleChildIds.add(
                child.importPath && child.children.size === 0
                  ? child.importPath
                  : `dir:${child.segment}`,
              );
            }
          } else {
            for (const child of trie.children.values()) {
              this.addTrieNode(child, moduleItem.children, seenIds, "");
              seenModuleChildIds.add(
                child.importPath && child.children.size === 0
                  ? child.importPath
                  : `dir:${child.segment}`,
              );
            }
          }

          // Clean up stale children within the module node
          moduleItem.children.forEach((child: vscode.TestItem) => {
            if (
              !seenModuleChildIds.has(child.id) &&
              !child.id.includes("/dynamic/")
            ) {
              this.detach(moduleItem!.children, child.id);
            }
          });
        }
      } else {
        // Single module — existing behavior (no module node)
        const trie = buildPathTrie(entries);
        collapsePathTrie(trie);

        const idPrefix = isMultiFolder ? `${this.folderName(wsDir)}/` : "";

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
    }

    this.controller.items.forEach((child) => {
      if (!rootIds.has(child.id) && !child.id.includes("/dynamic/")) {
        this.detach(this.controller.items, child.id);
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
    this.attach(parent, dirItem);

    if (node.importPath) {
      this.addPackageItem(node.importPath, dirItem.children, seenIds);
    }

    const seenChildIds = new Set<string>();
    if (node.importPath) {
      seenChildIds.add(node.importPath);
    }
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
        this.detach(dirItem.children, child.id);
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
    this.attach(this.controller.items, item);
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
    // A broken package keeps its last known suites in the tree but carries an
    // error badge with the build diagnostics: the tests still exist in the
    // code, they just cannot run until the package builds again.
    if (pkg.broken) {
      const diagnostics = this.cache
        .getWarnings(importPath)
        .map((w) => w.message)
        .join("\n");
      pkgItem.error = diagnostics || "package failed to build";
    } else {
      pkgItem.error = undefined;
    }
    this.attach(parent, pkgItem);

    const seenSuiteIds = new Set<string>();

    for (const suite of pkg.suites) {
      const suiteId = suiteItemId(importPath, suite, pkg.suites);
      const isTwin = suiteId.endsWith(EXTERNAL_ID);
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
        isTwin,
      );
      suiteItem.description = suite.guarded ? "guarded" : undefined;
      this.attach(pkgItem.children, suiteItem);

      const seenMethodIds = new Set<string>();

      for (const method of suite.methods) {
        const methodId = methodItemId(suiteId, method);
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
        // Deliberately not canResolveChildren: that tells VS Code a handler can
        // fetch the missing children on demand, and they cannot be known
        // without running the test. An expand arrow that resolves to nothing is
        // a worse lie than a partial list. Say it in words instead.
        this.addBehaviors(
          methodItem,
          method.behaviors ?? [],
          methodUri,
          method.behaviorsComplete === false,
        );
        this.attach(suiteItem.children, methodItem);
      }

      // Fuzz targets sit beside test methods in the tree; their names are
      // disjoint by prefix (Fuzz* vs Test*), so they share the id
      // namespace. Running one from the explorer replays its seed corpus
      // as ordinary subtests — searching for new inputs is the CodeLens
      // "Fuzz" action, deliberately not a Test Explorer run.
      for (const fuzzer of suite.fuzzers ?? []) {
        const fuzzerId = `${suiteId}/${fuzzer.name}`;
        seenMethodIds.add(fuzzerId);

        const fuzzerUri = vscode.Uri.file(path.join(pkg.dir, fuzzer.file));
        let fuzzerItem = suiteItem.children.get(fuzzerId);
        if (!fuzzerItem) {
          fuzzerItem = this.controller.createTestItem(
            fuzzerId,
            fuzzer.name,
            fuzzerUri,
          );
        }
        fuzzerItem.range = new vscode.Range(
          new vscode.Position(fuzzer.line - 1, fuzzer.col - 1),
          new vscode.Position(fuzzer.line - 1, fuzzer.col - 1),
        );
        // The "fuzz" tag routes the item to the Fuzz run profile; plain Run
        // still replays its seeds, since only "benchmark" is filtered there.
        fuzzerItem.tags = [
          ...this.buildTags(fuzzer.focused, fuzzer.excluded, fuzzer.parallel),
          new vscode.TestTag("fuzz"),
        ];
        fuzzerItem.description = "fuzz";
        this.attach(suiteItem.children, fuzzerItem);
      }

      // Benchmark methods sit beside tests and fuzzers under their suite —
      // names are disjoint by prefix (Benchmark*), so they share the id
      // namespace. They carry the "benchmark" tag: the tag routes them to
      // the Bench run profile and keeps them out of ordinary runs.
      for (const bench of suite.benchmarks ?? []) {
        const benchId = `${suiteId}/${bench.name}`;
        seenMethodIds.add(benchId);

        const benchUri = vscode.Uri.file(path.join(pkg.dir, bench.file));
        let benchItem = suiteItem.children.get(benchId);
        if (!benchItem) {
          benchItem = this.controller.createTestItem(
            benchId,
            bench.name,
            benchUri,
          );
        }
        benchItem.range = new vscode.Range(
          new vscode.Position(bench.line - 1, bench.col - 1),
          new vscode.Position(bench.line - 1, bench.col - 1),
        );
        benchItem.tags = [
          new vscode.TestTag("benchmark"),
          ...this.buildTags(bench.focused, bench.excluded, bench.parallel),
        ];
        benchItem.description = "bench";
        this.attach(suiteItem.children, benchItem);
      }

      suiteItem.children.forEach((child) => {
        if (!seenMethodIds.has(child.id) && !this.dynamicIds.has(child.id)) {
          this.forgetDynamic(child);
          this.detach(suiteItem.children, child.id);
        }
      });
    }

    pkgItem.children.forEach((child) => {
      if (!seenSuiteIds.has(child.id) && !this.dynamicIds.has(child.id)) {
        this.forgetDynamic(child);
        this.detach(pkgItem.children, child.id);
      }
    });
  }

  clearDynamicChildren(item: vscode.TestItem): void {
    const toDelete: vscode.TestItem[] = [];
    item.children.forEach((child) => {
      if (this.dynamicIds.has(child.id)) {
        toDelete.push(child);
      }
    });
    for (const child of toDelete) {
      this.forgetDynamic(child);
      this.detach(item.children, child.id);
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
    // The id is the go test path: one segment appended to the parent. A
    // statically discovered behavior has the identical id, so an observed
    // result lands on the declared item instead of creating a second one.
    const id = `${parentItem.id}/${subtestPath}`;
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
    this.dynamicIds.add(id);
    this.attach(parentItem.children, item);
    return item;
  }

  // addBehaviors builds the specification a method declares, so the tree shows
  // behaviors before anything has run and the run counter has a real total.
  private addBehaviors(
    parent: vscode.TestItem,
    behaviors: DiscoverBehavior[],
    uri: vscode.Uri,
    incomplete = false,
  ): void {
    const seen = new Set<string>();
    // The same ceiling the runtime path uses. A table with thousands of rows
    // would otherwise materialise thousands of tree items at discovery, before
    // the developer has asked for anything — one policy for declared and
    // observed behaviors rather than two.
    const shown = behaviors.slice(0, GoTestController.MAX_DYNAMIC_SUBTESTS);
    const hidden = behaviors.length - shown.length;
    // Both truths fit in one description, and a method can hold both: a table
    // longer than the ceiling whose rows also depend on runtime values. Setting
    // them separately meant the second overwrote the first.
    const notes: string[] = [];
    if (hidden > 0) {
      notes.push(`${behaviors.length} behaviors (${shown.length} shown)`);
    }
    if (incomplete) {
      notes.push("+ behaviors known only at run time");
    }
    parent.description = notes.length > 0 ? notes.join(", ") : undefined;
    for (const behavior of shown) {
      const id = `${parent.id}/${behavior.name}`;
      seen.add(id);
      let item = parent.children.get(id);
      if (!item) {
        item = this.controller.createTestItem(id, behavior.display, uri);
      }
      // Source now claims this id, so it is no longer a run-time discovery:
      // forgetting that would exempt it from pruning when it leaves the source.
      this.dynamicIds.delete(id);
      if (behavior.line > 0) {
        item.range = new vscode.Range(
          new vscode.Position(behavior.line - 1, 0),
          new vscode.Position(behavior.line - 1, 0),
        );
      }
      this.attach(parent.children, item);
      this.addBehaviors(item, behavior.children ?? [], uri);
    }
    // Behaviors that no longer exist in source go away; ones discovered at run
    // time stay, because source never claimed them in the first place.
    parent.children.forEach((child) => {
      if (!seen.has(child.id) && !this.dynamicIds.has(child.id)) {
        this.forgetDynamic(child);
        this.detach(parent.children, child.id);
      }
    });
  }

  // forgetDynamic drops a deleted subtree from the dynamic registry. The set
  // decides what survives pruning, so an id left in it after its item is gone
  // would grant a later item of the same name an exemption it never earned.
  private forgetDynamic(item: vscode.TestItem): void {
    this.dynamicIds.delete(item.id);
    this.dynamicOverflow.delete(item.id);
    item.children.forEach((child) => this.forgetDynamic(child));
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
    // A run that died mid-way leaves brackets open on nodes that never
    // reported. They are worthless to the next run and would otherwise sit in
    // the map for the rest of the session.
    this.pendingStarts.clear();
    this.pendingPaused.clear();
    return tracked(this.controller.createTestRun(request, name));
  }

  findItem(id: string): vscode.TestItem | undefined {
    // The tree is the answer to this question, so it has to be current. A
    // caller reading it moments after discovery — restoring the last session's
    // results, say — would otherwise search an empty tree and conclude the
    // tests no longer exist.
    this.flushPendingRebuild();
    return this.index.get(id)?.[0];
  }

  // attach adds item to a collection of the tree and to the index. Callers
  // reuse the item a collection holds under the id, so nothing is replaced.
  private attach(
    collection: vscode.TestItemCollection,
    item: vscode.TestItem,
  ): void {
    collection.add(item);
    this.indexItem(item);
  }

  // detach deletes an item from a collection, its subtree from the index.
  private detach(collection: vscode.TestItemCollection, id: string): void {
    const item = collection.get(id);
    if (item) this.unindex(item);
    collection.delete(id);
  }

  // indexItem records an item and, when it is new to the index, the children
  // it already holds; a re-added item's subtree is indexed already.
  private indexItem(item: vscode.TestItem): void {
    const list = this.index.get(item.id);
    if (list?.includes(item)) return;
    if (list) list.push(item);
    else this.index.set(item.id, [item]);
    item.children.forEach((child) => this.indexItem(child));
  }

  private unindex(item: vscode.TestItem): void {
    const list = this.index.get(item.id);
    if (list) {
      const rest = list.filter((i) => i !== item);
      if (rest.length > 0) this.index.set(item.id, rest);
      else this.index.delete(item.id);
    }
    item.children.forEach((child) => this.unindex(child));
  }

  // noteStart remembers when go test registered a node, so the terminal event
  // can close the bracket. It is kept off the result store on purpose: an
  // in-flight node has no verdict, and a run that dies mid-way must not leave
  // half a result behind to be restored on the next reload.
  noteStart(itemId: string, at: number): void {
    this.pendingStarts.set(itemId, at);
  }

  // notePaused records that the node called t.Parallel. What that costs its
  // bracket is described on TestResult.paused.
  notePaused(itemId: string): void {
    this.pendingPaused.add(itemId);
  }

  recordResult(
    itemId: string,
    status: TestResult["status"],
    duration?: number,
    endedAt?: number,
  ): void {
    const startedAt = this.pendingStarts.get(itemId);
    const paused = this.pendingPaused.has(itemId);
    this.pendingStarts.delete(itemId);
    this.pendingPaused.delete(itemId);
    this.resultStore.record(itemId, status, duration, {
      startedAt,
      endedAt,
      paused,
    });
  }

  getResult(itemId: string): TestResult | undefined {
    return this.resultStore.get(itemId);
  }

  // forgetResult drops one item's stored verdict; its subtree's stay.
  forgetResult(itemId: string): void {
    if (this.resultStore.get(itemId)) this.resultStore.delete(itemId);
  }

  // invalidateResults marks a package's shown verdicts outdated; the stored
  // ones are kept, so a reload restores them as before.
  invalidateResults(importPath: string): void {
    const item = this.findItem(importPath);
    if (!item) return;
    this.controller.invalidateTestResults(item);
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
    this.pendingStarts.clear();
    this.pendingPaused.clear();
    this.index.clear();
    this.controller.dispose();
  }

  private buildTags(
    focused: boolean,
    excluded: boolean,
    parallel: boolean,
    guarded?: boolean,
    external?: boolean,
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
    if (external) {
      tags.push(new vscode.TestTag("external"));
    }
    return tags;
  }
}
