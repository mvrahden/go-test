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
import {
  ScaffoldCodeActionProvider,
  runScaffoldCommand,
  executeScaffold,
} from "./scaffold.js";
import { CoverageRunner } from "./coverage.js";
import { CoverageStore } from "./coverageStore.js";
import { validateGoBinary, scopedConfig } from "./cli.js";
import { buildRunFilter, getPackageDir } from "./runnerUtils.js";

export function activate(context: vscode.ExtensionContext): void {
  const outputChannel = vscode.window.createOutputChannel("Go Test Suites");
  outputChannel.appendLine("Go Test Suites extension activated");

  const workspaceFolders = vscode.workspace.workspaceFolders;
  if (!workspaceFolders || workspaceFolders.length === 0) {
    outputChannel.appendLine(
      "[activate] No workspace folder found, skipping activation.",
    );
    return;
  }

  const cache = new DiscoveryCache();
  const discoveryService = new DiscoveryService(cache, outputChannel);
  const debugLauncher = new DebugLauncher(outputChannel);

  let runner!: TestRunner;
  let coverageRunner!: CoverageRunner;

  const controller = new GoTestController(
    cache,
    outputChannel,
    (request, token) => runner.run(request, token),
    (request, token) =>
      debugLauncher.debug(
        request,
        token,
        (items) => {
          if (!items || items.length === 0) return undefined;
          return buildRunFilter(Array.from(items));
        },
        (item) => getPackageDir(item, cache),
      ),
    (request, token) => coverageRunner.run(request, token),
  );

  controller.testController.refreshHandler = async () => {
    for (const folder of vscode.workspace.workspaceFolders ?? []) {
      await discoveryService.discover(folder.uri.fsPath);
    }
  };

  const coverageStore = new CoverageStore(context.storageUri);

  const specView = new SpecViewPanel(outputChannel);

  coverageRunner = new CoverageRunner(
    controller,
    cache,
    coverageStore,
    outputChannel,
    (jsonOutput) => {
      specView.refresh(jsonOutput);
    },
  );

  controller.setCoverageDetailProvider((uri) =>
    coverageRunner.getDetailedCoverage(uri),
  );

  runner = new TestRunner(controller, cache, outputChannel, coverageStore, coverageRunner);

  const specViewRefreshDisposable = runner.onDidComplete((jsonOutput) => {
    specView.refresh(jsonOutput);
  });

  const watchManager = new WatchManager(
    controller,
    cache,
    outputChannel,
    (jsonOutput) => {
      specView.refresh(jsonOutput);
    },
  );

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
  const scaffoldCodeActionsDisposable =
    vscode.languages.registerCodeActionsProvider(
      { language: "go", pattern: "**/*.go" },
      scaffoldProvider,
      {
        providedCodeActionKinds:
          ScaffoldCodeActionProvider.providedCodeActionKinds,
      },
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
        outputChannel.appendLine(
          `[command] runTest: item not found: ${testId}`,
        );
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
        outputChannel.appendLine(
          `[command] debugTest: item not found: ${testId}`,
        );
        return;
      }
      const cts = new vscode.CancellationTokenSource();
      try {
        const request = new vscode.TestRunRequest([item]);
        await debugLauncher.debug(
          request,
          cts.token,
          (items) => {
            if (!items || items.length === 0) return undefined;
            return buildRunFilter(Array.from(items));
          },
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
      for (const folder of vscode.workspace.workspaceFolders ?? []) {
        await discoveryService.discover(folder.uri.fsPath);
      }
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
      const defaultScope =
        vscode.workspace
          .getConfiguration("gotest")
          .get<string>("watch.scope") ?? "./...";

      const scope = await vscode.window.showInputBox({
        prompt: "Package scope to watch",
        value: defaultScope,
        placeHolder: "./...",
      });

      if (scope) {
        const wsDir = resolveActiveWorkspaceDir();
        if (wsDir) {
          await watchManager.start(scope, wsDir);
        }
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

  const scaffoldCmd = vscode.commands.registerCommand("gotest.scaffold", () => {
    const wsDir = resolveActiveWorkspaceDir();
    return runScaffoldCommand(
      outputChannel,
      () => {
        if (wsDir) discoveryService.discover(wsDir);
      },
      wsDir,
    );
  });

  const scaffoldTargetCmd = vscode.commands.registerCommand(
    "gotest.scaffoldTarget",
    (target: string, workspaceDir?: string) => {
      const wsDir = workspaceDir ?? resolveActiveWorkspaceDir();
      return executeScaffold(
        target,
        outputChannel,
        () => {
          if (wsDir) discoveryService.discover(wsDir);
        },
        wsDir,
      );
    },
  );

  const copyCoverageCmd = vscode.commands.registerCommand(
    "gotest.copyCoverage",
    () => coverageRunner.copyCoverageSummary(),
  );

  const copyTestResultsCmd = vscode.commands.registerCommand(
    "gotest.copyTestResults",
    (item?: vscode.TestItem) => controller.copyTestResults(item),
  );

  // Cover on save: debounced per-package trigger
  const coverOnSaveTimers = new Map<string, ReturnType<typeof setTimeout>>();
  const triggerCoverOnSave = (importPath: string, uri: vscode.Uri) => {
    const folder = vscode.workspace.getWorkspaceFolder(uri);
    if (!folder) return;
    const config = scopedConfig(folder.uri.fsPath);
    if (!(config.get<boolean>("coverOnSave") ?? false)) return;

    const existing = coverOnSaveTimers.get(importPath);
    if (existing) clearTimeout(existing);
    coverOnSaveTimers.set(
      importPath,
      setTimeout(() => {
        coverOnSaveTimers.delete(importPath);
        coverageRunner.runPackage(importPath);
      }, 500),
    );
  };

  // Set up FileSystemWatcher for *_test.go files
  const watcher = vscode.workspace.createFileSystemWatcher("**/*_test.go");
  const onFileChange = (uri: vscode.Uri) => {
    const folder = vscode.workspace.getWorkspaceFolder(uri);
    if (!folder) return;
    const wsDir = folder.uri.fsPath;
    const importPath = cache.resolveFileToPackage(uri.fsPath);

    const discoverOnSave =
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("discoverOnSave") ?? true;
    if (discoverOnSave) {
      if (importPath) {
        discoveryService.discoverPackage(wsDir, importPath);
      } else {
        discoveryService.discover(wsDir);
      }
    }

    if (importPath) triggerCoverOnSave(importPath, uri);
  };
  const onFileDelete = (uri: vscode.Uri) => {
    const discoverOnSave =
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("discoverOnSave") ?? true;
    if (!discoverOnSave) {
      return;
    }
    const folder = vscode.workspace.getWorkspaceFolder(uri);
    if (folder) {
      discoveryService.discover(folder.uri.fsPath);
    }
  };
  const watcherChangeDisposable = watcher.onDidChange(onFileChange);
  const watcherCreateDisposable = watcher.onDidCreate(onFileChange);
  const watcherDeleteDisposable = watcher.onDidDelete(onFileDelete);

  // Invalidate coverage when source files change
  const goSourceWatcher = vscode.workspace.createFileSystemWatcher("**/*.go");
  const onSourceChange = (uri: vscode.Uri) => {
    if (uri.fsPath.endsWith("_test.go")) return;
    const importPath = cache.resolveFileToPackage(uri.fsPath);
    if (!importPath) return;

    if (coverageStore.invalidate(importPath)) {
      outputChannel.appendLine(`[coverage] invalidated ${importPath}`);
      coverageStore.save();
    }

    triggerCoverOnSave(importPath, uri);
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
    copyTestResultsCmd,
    watcher,
    watcherChangeDisposable,
    watcherCreateDisposable,
    watcherDeleteDisposable,
    goSourceWatcher,
    sourceChangeDisposable,
    sourceCreateDisposable,
    sourceDeleteDisposable,
    {
      dispose() {
        for (const t of coverOnSaveTimers.values()) clearTimeout(t);
      },
    },
  );

  // Validate go binary, then run initial discovery and restore persisted coverage
  (async () => {
    const firstDir = workspaceFolders[0].uri.fsPath;
    const goBin = await validateGoBinary(outputChannel, firstDir);
    if (!goBin) {
      outputChannel.appendLine("[activate] Go binary not found or not working");
      vscode.window
        .showErrorMessage(
          "Go Test Suites: could not find a working 'go' installation. Ensure Go is installed and on your PATH.",
          "Open Output",
        )
        .then((choice) => {
          if (choice === "Open Output") outputChannel.show();
        });
      return;
    }

    for (const folder of workspaceFolders) {
      await discoveryService.discover(folder.uri.fsPath);
    }
    await coverageStore.load();
    if (coverageStore.size > 0) {
      const { coverages, details } = coverageStore.buildFileCoverages(cache);
      coverageRunner.mergeDetails(details);
      if (coverages.length > 0) {
        const request = new vscode.TestRunRequest();
        const run = controller.createTestRun(request, "Restored Coverage");
        for (const fc of coverages) {
          run.addCoverage(fc);
        }
        run.end();
        outputChannel.appendLine(
          `[coverage] restored ${coverages.length} file(s) from storage`,
        );
      }
    }
  })();
}

export function deactivate(): void {}

function resolveActiveWorkspaceDir(): string | undefined {
  const activeUri = vscode.window.activeTextEditor?.document.uri;
  const folder = activeUri
    ? (vscode.workspace.getWorkspaceFolder(activeUri) ??
      vscode.workspace.workspaceFolders?.[0])
    : vscode.workspace.workspaceFolders?.[0];
  return folder?.uri.fsPath;
}
