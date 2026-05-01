import * as vscode from "vscode";
import { DiscoveryCache, DiscoveryService } from "./discovery.js";
import { GoTestController } from "./testController.js";
import { TestRunner } from "./runner.js";
import { GoTestCodeLensProvider } from "./codeLens.js";
import { DebugLauncher } from "./debug.js";
import { FocusExcludeProvider } from "./focusExclude.js";
import { FocusDiagnostics } from "./diagnostics.js";
import { SpecViewPanel } from "./specView.js";
import { WatchManager } from "./watch.js";
import { ScaffoldCodeActionProvider, runScaffoldCommand, executeScaffold } from "./scaffold.js";
import { CoverageRunner } from "./coverage.js";
import { CoverageStore } from "./coverageStore.js";

export function activate(context: vscode.ExtensionContext): void {
  const outputChannel = vscode.window.createOutputChannel("Go Test Suites");
  outputChannel.appendLine("Go Test Suites extension activated");

  const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!workspaceDir) {
    outputChannel.appendLine("[activate] No workspace folder found, skipping activation.");
    return;
  }

  const cache = new DiscoveryCache();
  const discoveryService = new DiscoveryService(cache, outputChannel);
  const debugLauncher = new DebugLauncher(outputChannel);

  // Create controller with inline handlers that close over runner
  let runner: TestRunner;
  let coverageRunner: CoverageRunner;

  const controller = new GoTestController(
    cache,
    outputChannel,
    (request, token) => runner.run(request, token),
    (request, token) =>
      debugLauncher.debug(
        request,
        token,
        (items) => buildRunFilter(items),
        (item) => getPackageDir(item, cache),
      ),
    (request, token) => coverageRunner.run(request, token),
  );

  runner = new TestRunner(controller, cache, outputChannel);

  const specView = new SpecViewPanel(outputChannel);

  const coverageStore = new CoverageStore(context.storageUri);

  coverageRunner = new CoverageRunner(controller, cache, coverageStore, outputChannel, (jsonOutput) => {
    specView.refresh(jsonOutput);
  });

  const specViewRefreshDisposable = runner.onDidComplete((jsonOutput) => {
    specView.refresh(jsonOutput);
  });

  const watchManager = new WatchManager(controller, cache, outputChannel, (jsonOutput) => {
    specView.refresh(jsonOutput);
  });

  // Register CodeLens provider
  const codeLensProvider = new GoTestCodeLensProvider(cache);
  const codeLensDisposable = vscode.languages.registerCodeLensProvider(
    { language: "go", pattern: "**/*_test.go" },
    codeLensProvider,
  );

  // Register FocusExclude code action provider
  const focusExcludeProvider = new FocusExcludeProvider(cache);
  const codeActionsDisposable = vscode.languages.registerCodeActionsProvider(
    { language: "go", pattern: "**/*_test.go" },
    focusExcludeProvider,
    { providedCodeActionKinds: FocusExcludeProvider.providedCodeActionKinds },
  );

  // Register scaffold Code Action provider
  const scaffoldProvider = new ScaffoldCodeActionProvider();
  const scaffoldCodeActionsDisposable = vscode.languages.registerCodeActionsProvider(
    { language: "go", pattern: "**/*.go" },
    scaffoldProvider,
    { providedCodeActionKinds: ScaffoldCodeActionProvider.providedCodeActionKinds },
  );

  // Create FocusDiagnostics
  const diagnostics = new FocusDiagnostics(cache);

  // Register debug session cleanup
  debugLauncher.registerCleanupOnSessionEnd(context);

  // Register commands
  const runTestCmd = vscode.commands.registerCommand(
    "gotest.runTest",
    async (testId: string) => {
      const item = controller.findItem(testId);
      if (!item) {
        outputChannel.appendLine(`[command] runTest: item not found: ${testId}`);
        return;
      }
      const cts = new vscode.CancellationTokenSource();
      try {
        const request = new vscode.TestRunRequest([item]);
        await runner.run(request, cts.token);
      } finally {
        cts.dispose();
      }
    },
  );

  const debugTestCmd = vscode.commands.registerCommand(
    "gotest.debugTest",
    async (testId: string) => {
      const item = controller.findItem(testId);
      if (!item) {
        outputChannel.appendLine(`[command] debugTest: item not found: ${testId}`);
        return;
      }
      const cts = new vscode.CancellationTokenSource();
      try {
        const request = new vscode.TestRunRequest([item]);
        await debugLauncher.debug(
          request,
          cts.token,
          (items) => buildRunFilter(items),
          (i) => getPackageDir(i, cache),
        );
      } finally {
        cts.dispose();
      }
    },
  );

  const refreshTestsCmd = vscode.commands.registerCommand(
    "gotest.refreshTests",
    async () => {
      await discoveryService.discover(workspaceDir);
    },
  );

  const showFocusedTestsCmd = vscode.commands.registerCommand(
    "gotest.showFocusedTests",
    async () => {
      await diagnostics.showFocusedTests();
    },
  );

  const showSpecViewCmd = vscode.commands.registerCommand(
    "gotest.showSpecView",
    () => {
      specView.show();
    },
  );

  const startWatchCmd = vscode.commands.registerCommand(
    "gotest.startWatch",
    async () => {
      const defaultScope = vscode.workspace
        .getConfiguration("gotest")
        .get<string>("watch.scope") ?? "./...";

      const scope = await vscode.window.showInputBox({
        prompt: "Package scope to watch",
        value: defaultScope,
        placeHolder: "./...",
      });

      if (scope) {
        await watchManager.start(scope, workspaceDir);
      }
    },
  );

  const stopWatchCmd = vscode.commands.registerCommand(
    "gotest.stopWatch",
    async () => {
      if (watchManager.activeCount === 0) {
        vscode.window.showInformationMessage("No active watchers.");
        return;
      }
      watchManager.stopAll();
      vscode.window.showInformationMessage("All watchers stopped.");
    },
  );

  const scaffoldCmd = vscode.commands.registerCommand(
    "gotest.scaffold",
    () => runScaffoldCommand(outputChannel, () => discoveryService.discover(workspaceDir)),
  );

  const scaffoldTargetCmd = vscode.commands.registerCommand(
    "gotest.scaffoldTarget",
    (target: string) => executeScaffold(target, outputChannel, () => discoveryService.discover(workspaceDir)),
  );

  const copyCoverageCmd = vscode.commands.registerCommand(
    "gotest.copyCoverage",
    () => coverageRunner.copyCoverageSummary(),
  );

  // Set up FileSystemWatcher for *_test.go files
  const watcher = vscode.workspace.createFileSystemWatcher("**/*_test.go");
  const onFileChange = () => {
    const discoverOnSave =
      vscode.workspace.getConfiguration("gotest").get<boolean>("discoverOnSave") ?? true;
    if (discoverOnSave) {
      discoveryService.discover(workspaceDir);
    }
  };
  const watcherChangeDisposable = watcher.onDidChange(onFileChange);
  const watcherCreateDisposable = watcher.onDidCreate(onFileChange);
  const watcherDeleteDisposable = watcher.onDidDelete(onFileChange);

  // Invalidate coverage when source files change
  const goSourceWatcher = vscode.workspace.createFileSystemWatcher("**/*.go");
  const onSourceChange = (uri: vscode.Uri) => {
    if (uri.fsPath.endsWith("_test.go")) {
      return;
    }
    const importPath = cache.resolveFileToPackage(uri.fsPath);
    if (importPath && coverageStore.invalidate(importPath)) {
      outputChannel.appendLine(`[coverage] invalidated ${importPath}`);
      coverageStore.save();
    }
  };
  const sourceChangeDisposable = goSourceWatcher.onDidChange(onSourceChange);
  const sourceCreateDisposable = goSourceWatcher.onDidCreate(onSourceChange);
  const sourceDeleteDisposable = goSourceWatcher.onDidDelete(onSourceChange);

  // Push all disposables to context.subscriptions
  context.subscriptions.push(
    outputChannel,
    cache,
    controller,
    codeLensProvider,
    codeLensDisposable,
    focusExcludeProvider,
    codeActionsDisposable,
    diagnostics,
    debugLauncher,
    runner,
    coverageRunner,
    coverageStore,
    specView,
    specViewRefreshDisposable,
    watchManager,
    runTestCmd,
    debugTestCmd,
    refreshTestsCmd,
    showFocusedTestsCmd,
    showSpecViewCmd,
    startWatchCmd,
    stopWatchCmd,
    scaffoldProvider,
    scaffoldCodeActionsDisposable,
    scaffoldCmd,
    scaffoldTargetCmd,
    copyCoverageCmd,
    watcher,
    watcherChangeDisposable,
    watcherCreateDisposable,
    watcherDeleteDisposable,
    goSourceWatcher,
    sourceChangeDisposable,
    sourceCreateDisposable,
    sourceDeleteDisposable,
  );

  // Run initial discovery, then restore persisted coverage
  discoveryService.discover(workspaceDir).then(async () => {
    await coverageStore.load();
    if (coverageStore.size > 0) {
      const coverages = coverageStore.buildFileCoverages(cache);
      if (coverages.length > 0) {
        const request = new vscode.TestRunRequest();
        const run = controller.createTestRun(request, "Restored Coverage");
        for (const fc of coverages) {
          run.addCoverage(fc);
        }
        run.end();
        outputChannel.appendLine(`[coverage] restored ${coverages.length} file(s) from storage`);
      }
    }
  });
}

export function deactivate(): void {}

function buildRunFilter(items: readonly vscode.TestItem[]): string | undefined {
  if (!items || items.length === 0) {
    return undefined;
  }
  const item = items[0];

  // Determine depth: 0=package, 1=suite, 2=method, 3+=dynamic
  let depth = 0;
  let current: vscode.TestItem | undefined = item.parent;
  while (current) {
    depth++;
    current = current.parent;
  }

  if (depth === 0) {
    return undefined; // package level — run all
  }

  if (depth === 1) {
    // suite level
    return `^Test${item.label}$`;
  }

  // method level or deeper
  const suite = item.parent!;
  return `^Test${suite.label}$/^${item.label}$`;
}

function getPackageDir(item: vscode.TestItem, cache: DiscoveryCache): string | undefined {
  let current: vscode.TestItem | undefined = item;
  while (current?.parent) {
    current = current.parent;
  }
  return cache.getPackage(current?.id || "")?.dir;
}
