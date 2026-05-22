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
import { CoverOnSave } from "./coverOnSave.js";
import { CoverageStore } from "./coverageStore.js";
import { TestResultStore } from "./testResultStore.js";
import { RunRegistry } from "./runRegistry.js";
import { validateGoBinary, scopedConfig } from "./cli.js";
import { buildRunFilter, getPackageDir } from "./runnerUtils.js";
import { copyCoverageSummary, copyTestResults } from "./reporting.js";
import { execFile } from "node:child_process";

let flushOnDeactivate: (() => Promise<void>) | undefined;

export function activate(context: vscode.ExtensionContext): void {
  const outputChannel = vscode.window.createOutputChannel("Go - Test Suites", {
    log: true,
  });
  outputChannel.info("[activate] extension activated");
  vscode.commands.executeCommand("setContext", "gotest.active", true);

  const workspaceFolders = vscode.workspace.workspaceFolders;
  if (!workspaceFolders || workspaceFolders.length === 0) {
    outputChannel.info("[activate] no workspace folder, skipping activation");
    return;
  }

  const cache = new DiscoveryCache();
  const discoveryService = new DiscoveryService(cache, outputChannel);
  const debugLauncher = new DebugLauncher(outputChannel);
  const testResultStore = new TestResultStore(context.storageUri);
  const registryDir =
    context.storageUri?.fsPath ?? context.globalStorageUri.fsPath;
  const runRegistry = new RunRegistry(registryDir);

  let runner!: TestRunner;
  let coverageRunner!: CoverageRunner;

  const controller = new GoTestController(
    cache,
    testResultStore,
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
  const coverOnSave = new CoverOnSave(
    controller,
    cache,
    coverageStore,
    outputChannel,
    runRegistry,
  );
  const specView = new SpecViewPanel(outputChannel, cache);
  specView.setExtensionUri(context.extensionUri);

  const specViewSerializer = vscode.window.registerWebviewPanelSerializer(
    "gotestSpecView",
    {
      async deserializeWebviewPanel(
        panel: vscode.WebviewPanel,
        state: unknown,
      ) {
        await specView.restorePanel(panel, state);
      },
    },
  );

  coverageRunner = new CoverageRunner(
    controller,
    cache,
    coverageStore,
    outputChannel,
    (jsonOutput) => {
      specView.refresh(jsonOutput, "coverage");
    },
    runRegistry,
  );

  controller.setCoverageDetailProvider((uri) =>
    coverageStore.getDetails(uri.fsPath),
  );

  runner = new TestRunner(
    controller,
    cache,
    outputChannel,
    runRegistry,
    coverageStore,
  );

  const specViewRefreshDisposable = runner.onDidComplete((jsonOutput) => {
    specView.refresh(jsonOutput, "run");
  });

  const watchManager = new WatchManager(
    controller,
    cache,
    outputChannel,
    (jsonOutput, scope, cwd) => {
      specView.refresh(jsonOutput, `watch:${scope}@${cwd}`);
    },
    runRegistry,
  );

  const diagnostics = new FocusDiagnostics(cache);
  debugLauncher.registerCleanupOnSessionEnd(context);

  const providerDisposables = registerProviders(cache);
  const commandDisposables = registerCommands({
    controller,
    runner,
    debugLauncher,
    discoveryService,
    diagnostics,
    specView,
    watchManager,
    coverageStore,
    testResultStore,
    cache,
    outputChannel,
  });
  const watcherDisposables = setupFileWatchers(
    cache,
    discoveryService,
    coverageStore,
    coverOnSave,
    outputChannel,
  );

  context.subscriptions.push(
    outputChannel,
    cache,
    controller,
    testResultStore,
    ...providerDisposables,
    diagnostics,
    debugLauncher,
    runner,
    coverageRunner,
    coverOnSave,
    coverageStore,
    specView,
    specViewSerializer,
    specViewRefreshDisposable,
    watchManager,
    ...commandDisposables,
    ...watcherDisposables,
  );

  flushOnDeactivate = async () => {
    await Promise.allSettled([
      runRegistry.save(),
      coverageStore.flush(),
      testResultStore.flush(),
    ]);
  };

  initializeAsync({
    workspaceFolders,
    outputChannel,
    discoveryService,
    coverageStore,
    testResultStore,
    controller,
    cache,
    runRegistry,
    context,
  }).catch((err) => {
    outputChannel.error(`[activate] async initialization failed: ${err}`);
  });
}

export async function deactivate(): Promise<void> {
  await flushOnDeactivate?.();
}

function resolveActiveWorkspaceDir(): string | undefined {
  const activeUri = vscode.window.activeTextEditor?.document.uri;
  const folder = activeUri
    ? (vscode.workspace.getWorkspaceFolder(activeUri) ??
      vscode.workspace.workspaceFolders?.[0])
    : vscode.workspace.workspaceFolders?.[0];
  return folder?.uri.fsPath;
}

function registerProviders(cache: DiscoveryCache): vscode.Disposable[] {
  const codeLensProvider = new GoTestCodeLensProvider(cache);
  const codeLensDisposable = vscode.languages.registerCodeLensProvider(
    { language: "go", pattern: "**/*_test.go" },
    codeLensProvider,
  );

  const focusExcludeProvider = new FocusExcludeProvider(cache);
  const codeActionsDisposable = vscode.languages.registerCodeActionsProvider(
    { language: "go", pattern: "**/*_test.go" },
    focusExcludeProvider,
    { providedCodeActionKinds: FocusExcludeProvider.providedCodeActionKinds },
  );

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

  return [
    codeLensProvider,
    codeLensDisposable,
    focusExcludeProvider,
    codeActionsDisposable,
    scaffoldProvider,
    scaffoldCodeActionsDisposable,
  ];
}

function registerCommands(deps: {
  controller: GoTestController;
  runner: TestRunner;
  debugLauncher: DebugLauncher;
  discoveryService: DiscoveryService;
  diagnostics: FocusDiagnostics;
  specView: SpecViewPanel;
  watchManager: WatchManager;
  coverageStore: CoverageStore;
  testResultStore: TestResultStore;
  cache: DiscoveryCache;
  outputChannel: vscode.LogOutputChannel;
}): vscode.Disposable[] {
  const {
    controller,
    runner,
    debugLauncher,
    discoveryService,
    diagnostics,
    specView,
    watchManager,
    coverageStore,
    testResultStore,
    cache,
    outputChannel,
  } = deps;

  return [
    vscode.commands.registerCommand(
      "gotest.runTest",
      async (testId: string) => {
        const item = controller.findItem(testId);
        if (!item) {
          outputChannel.warn(`[command] runTest: item not found: ${testId}`);
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
    ),

    vscode.commands.registerCommand(
      "gotest.debugTest",
      async (testId: string) => {
        const item = controller.findItem(testId);
        if (!item) {
          outputChannel.warn(`[command] debugTest: item not found: ${testId}`);
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
    ),

    vscode.commands.registerCommand(
      "gotest.runFile",
      async (suiteIds: string[]) => {
        const items = suiteIds
          .map((id) => controller.findItem(id))
          .filter((item): item is vscode.TestItem => item !== undefined);
        if (items.length === 0) {
          outputChannel.warn(`[command] runFile: no items found`);
          return;
        }
        const cts = new vscode.CancellationTokenSource();
        try {
          const request = new vscode.TestRunRequest(items);
          await runner.run(request, cts.token);
        } finally {
          cts.dispose();
        }
      },
    ),

    vscode.commands.registerCommand("gotest.refreshTests", async () => {
      for (const folder of vscode.workspace.workspaceFolders ?? []) {
        await discoveryService.discover(folder.uri.fsPath);
      }
    }),

    vscode.commands.registerCommand("gotest.showFocusedTests", async () => {
      await diagnostics.showFocusedTests();
    }),

    vscode.commands.registerCommand("gotest.showSpecView", async () => {
      await specView.show();
    }),

    vscode.commands.registerCommand("gotest.startWatch", async () => {
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
    }),

    vscode.commands.registerCommand("gotest.stopWatch", async () => {
      if (watchManager.activeCount === 0) {
        vscode.window.showInformationMessage("No active watchers.");
        return;
      }
      watchManager.stopAll();
      vscode.window.showInformationMessage("All watchers stopped.");
    }),

    vscode.commands.registerCommand("gotest.scaffold", () => {
      const wsDir = resolveActiveWorkspaceDir();
      return runScaffoldCommand(
        outputChannel,
        () => {
          if (wsDir) discoveryService.discover(wsDir);
        },
        wsDir,
      );
    }),

    vscode.commands.registerCommand(
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
    ),

    vscode.commands.registerCommand("gotest.copyCoverage", () =>
      copyCoverageSummary(coverageStore, cache),
    ),

    vscode.commands.registerCommand(
      "gotest.copyTestResults",
      (item?: vscode.TestItem) =>
        copyTestResults(
          controller.testController.items,
          testResultStore,
          (id) => controller.findItem(id),
          item,
        ),
    ),
  ];
}

function setupFileWatchers(
  cache: DiscoveryCache,
  discoveryService: DiscoveryService,
  coverageStore: CoverageStore,
  coverOnSave: CoverOnSave,
  outputChannel: vscode.LogOutputChannel,
): vscode.Disposable[] {
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
        coverOnSave.run(importPath);
      }, 500),
    );
  };

  const testWatcher = vscode.workspace.createFileSystemWatcher("**/*_test.go");
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

  const sourceWatcher = vscode.workspace.createFileSystemWatcher("**/*.go");
  const onSourceChange = (uri: vscode.Uri) => {
    if (uri.fsPath.endsWith("_test.go")) return;
    const importPath = cache.resolveFileToPackage(uri.fsPath);
    if (!importPath) return;

    if (coverageStore.invalidate(importPath)) {
      outputChannel.debug(`[coverage] invalidated ${importPath}`);
      coverageStore.save().catch((err) => {
        outputChannel.error(`[coverage] save after invalidate failed: ${err}`);
      });
    }

    triggerCoverOnSave(importPath, uri);
  };

  return [
    testWatcher,
    testWatcher.onDidChange(onFileChange),
    testWatcher.onDidCreate(onFileChange),
    testWatcher.onDidDelete(onFileDelete),
    sourceWatcher,
    sourceWatcher.onDidChange(onSourceChange),
    sourceWatcher.onDidCreate(onSourceChange),
    sourceWatcher.onDidDelete(onSourceChange),
    {
      dispose() {
        for (const t of coverOnSaveTimers.values()) clearTimeout(t);
      },
    },
  ];
}

async function initializeAsync(deps: {
  workspaceFolders: readonly vscode.WorkspaceFolder[];
  outputChannel: vscode.LogOutputChannel;
  discoveryService: DiscoveryService;
  coverageStore: CoverageStore;
  testResultStore: TestResultStore;
  controller: GoTestController;
  cache: DiscoveryCache;
  runRegistry: RunRegistry;
  context: vscode.ExtensionContext;
}): Promise<void> {
  const {
    workspaceFolders,
    outputChannel,
    discoveryService,
    coverageStore,
    testResultStore,
    controller,
    cache,
    runRegistry,
    context,
  } = deps;

  const firstDir = workspaceFolders[0].uri.fsPath;
  const goBin = await validateGoBinary(outputChannel, firstDir);
  if (!goBin) {
    outputChannel.error("[activate] Go binary not found or not working");
    const choice = await vscode.window.showErrorMessage(
      "gotest: could not find a working 'go' installation. Ensure Go is installed and on your PATH.",
      "Open Output",
    );
    if (choice === "Open Output") outputChannel.show();
    return;
  }

  for (const folder of workspaceFolders) {
    execFile(
      "git",
      ["config", "diff.snapshot.xfuncname", "^=== SNAP .+ ===$"],
      { cwd: folder.uri.fsPath },
      (err) => {
        if (err) {
          outputChannel.debug(
            `[activate] git config diff.snapshot.xfuncname failed: ${err.message}`,
          );
        }
      },
    );
  }

  await runRegistry.load();
  const crashed = runRegistry.sweepStale();
  if (crashed.length > 0) {
    outputChannel.info(
      `[registry] marked ${crashed.length} stale run(s) as crashed`,
    );
  }
  runRegistry.sweep();
  await runRegistry.save();

  const registrySaveInterval = setInterval(() => {
    runRegistry.sweep();
    runRegistry.save().catch((err) => {
      outputChannel.error(`[registry] periodic save failed: ${err}`);
    });
  }, 60_000);
  context.subscriptions.push({
    dispose() {
      clearInterval(registrySaveInterval);
    },
  });

  for (const folder of workspaceFolders) {
    await discoveryService.discover(folder.uri.fsPath);
  }

  await coverageStore.load();
  if (coverageStore.size > 0) {
    const { coverages } = coverageStore.buildFileCoverages(cache);
    if (coverages.length > 0) {
      const request = new vscode.TestRunRequest();
      const run = controller.createTestRun(request, "Restored Coverage");
      for (const fc of coverages) {
        run.addCoverage(fc);
      }
      run.end();
      outputChannel.info(
        `[coverage] restored ${coverages.length} file(s) from storage`,
      );
    }
  }

  await testResultStore.load();
  if (testResultStore.size > 0) {
    const resultRequest = new vscode.TestRunRequest();
    const resultRun = controller.createTestRun(
      resultRequest,
      "Restored Results",
    );
    testResultStore.forEach((result, itemId) => {
      const item = controller.findItem(itemId);
      if (!item) return;
      if (result.status === "pass") resultRun.passed(item, result.duration);
      else if (result.status === "fail")
        resultRun.failed(
          item,
          [new vscode.TestMessage("(restored from previous session)")],
          result.duration,
        );
      else if (result.status === "skip") resultRun.skipped(item);
    });
    resultRun.end();
    outputChannel.info(
      `[results] restored ${testResultStore.size} result(s) from storage`,
    );
  }
}
