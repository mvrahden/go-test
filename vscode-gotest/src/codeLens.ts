import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import { hostPlatform, type BenchResultStore } from "./benchResultStore.js";
import { formatBenchAnnotation } from "./benchReport.js";

export class GoTestCodeLensProvider
  implements vscode.CodeLensProvider, vscode.Disposable
{
  private _onDidChangeCodeLenses = new vscode.EventEmitter<void>();
  readonly onDidChangeCodeLenses: vscode.Event<void> =
    this._onDidChangeCodeLenses.event;

  private subscriptions: vscode.Disposable[] = [];

  constructor(
    private readonly cache: DiscoveryCache,
    private readonly benchStore?: BenchResultStore,
  ) {
    this.subscriptions.push(
      cache.onDidUpdate(() => this._onDidChangeCodeLenses.fire()),
    );
    if (benchStore) {
      this.subscriptions.push(
        benchStore.onDidUpdate(() => this._onDidChangeCodeLenses.fire()),
      );
    }
  }

  provideCodeLenses(
    document: vscode.TextDocument,
    _token: vscode.CancellationToken,
  ): vscode.CodeLens[] {
    if (!document.fileName.endsWith("_test.go")) {
      return [];
    }

    const enabled =
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("showCodeLens") ?? true;
    if (!enabled) {
      return [];
    }

    const docPath = document.fileName;
    const importPath = this.cache.resolveFileToPackage(docPath);
    if (!importPath) return [];

    const pkg = this.cache.getPackage(importPath);
    if (!pkg) return [];

    const lenses: vscode.CodeLens[] = [];
    const packageLine = new vscode.Range(0, 0, 0, 0);

    lenses.push(
      new vscode.CodeLens(packageLine, {
        title: "▶ Run Package",
        command: "gotest.runTest",
        arguments: [importPath],
      }),
    );

    const fileSuiteIds = pkg.suites
      .filter((s) => path.join(pkg.dir, s.file) === docPath)
      .map((s) => `${importPath}/${s.name}`);

    if (fileSuiteIds.length > 1) {
      lenses.push(
        new vscode.CodeLens(packageLine, {
          title: "▶ Run File",
          command: "gotest.runFile",
          arguments: [fileSuiteIds],
        }),
      );
    }

    const docText = document.getText();
    const fileSuites = pkg.suites.filter(
      (s) => path.join(pkg.dir, s.file) === docPath,
    );

    for (const suite of pkg.suites) {
      const suiteInFile = path.join(pkg.dir, suite.file) === docPath;
      if (suiteInFile) {
        const range = new vscode.Range(suite.line - 1, 0, suite.line - 1, 0);
        const testPath = `${importPath}/${suite.name}`;

        lenses.push(
          new vscode.CodeLens(range, {
            title: "▶ Run Suite",
            command: "gotest.runTest",
            arguments: [testPath],
          }),
          new vscode.CodeLens(range, {
            title: "Debug Suite",
            command: "gotest.debugTest",
            arguments: [testPath],
          }),
        );
      }

      let suiteHasSnapshots = false;

      const fileMethods = suite.methods.filter(
        (m) => path.join(pkg.dir, m.file) === docPath,
      );

      for (let i = 0; i < fileMethods.length; i++) {
        const method = fileMethods[i];
        const range = new vscode.Range(method.line - 1, 0, method.line - 1, 0);
        const testPath = `${importPath}/${suite.name}/${method.name}`;

        lenses.push(
          new vscode.CodeLens(range, {
            title: "▶ Run",
            command: "gotest.runTest",
            arguments: [testPath],
          }),
          new vscode.CodeLens(range, {
            title: "Debug",
            command: "gotest.debugTest",
            arguments: [testPath],
          }),
        );

        const startOffset = document.offsetAt(range.start);
        const nextSuiteIdx = fileSuites.indexOf(suite) + 1;
        const suiteEndLine =
          nextSuiteIdx < fileSuites.length
            ? fileSuites[nextSuiteIdx].line - 2
            : document.lineCount - 1;
        const endLine = fileMethods[i + 1]
          ? fileMethods[i + 1].line - 2
          : suiteEndLine;
        const endOffset = document.offsetAt(
          new vscode.Position(endLine, Number.MAX_SAFE_INTEGER),
        );
        const methodText = docText.slice(startOffset, endOffset);

        if (methodText.includes("MatchSnapshot")) {
          suiteHasSnapshots = true;
          lenses.push(
            new vscode.CodeLens(range, {
              title: "↻ Update Snapshots",
              command: "gotest.updateSnapshots",
              arguments: [testPath],
            }),
          );
        }
      }

      const fileBenchmarks = suite.benchmarks.filter(
        (m) => path.join(pkg.dir, m.file) === docPath,
      );

      if (suiteInFile && fileBenchmarks.length > 1) {
        const range = new vscode.Range(suite.line - 1, 0, suite.line - 1, 0);
        lenses.push(
          new vscode.CodeLens(range, {
            title: "▶ Bench Suite",
            command: "gotest.runBench",
            arguments: [importPath, suite.name],
          }),
        );
      }

      const platform = hostPlatform();
      for (const method of fileBenchmarks) {
        const range = new vscode.Range(method.line - 1, 0, method.line - 1, 0);

        lenses.push(
          new vscode.CodeLens(range, {
            title: "▶ Bench",
            command: "gotest.runBench",
            arguments: [importPath, suite.name, method.name],
          }),
          // Five repetitions give the CLI enough samples for a trustworthy
          // Welch comparison and an honest ± spread on the annotation.
          new vscode.CodeLens(range, {
            title: "5×",
            command: "gotest.runBenchStable",
            arguments: [importPath, suite.name, method.name],
          }),
        );

        // The last measured numbers, right where the code is — but only
        // numbers taken on this host's goos/goarch: a result from another
        // platform is a different number and never shown here.
        const latest = this.benchStore?.getLatest(
          importPath,
          suite.name,
          method.name,
          platform,
        );
        if (latest) {
          lenses.push(
            new vscode.CodeLens(range, {
              title: formatBenchAnnotation(
                latest,
                latest.recordedAt,
                Date.now(),
                latest.delta,
              ),
              command: "",
            }),
          );
        }
      }

      if (suiteInFile && suiteHasSnapshots) {
        const range = new vscode.Range(suite.line - 1, 0, suite.line - 1, 0);
        const testPath = `${importPath}/${suite.name}`;
        lenses.push(
          new vscode.CodeLens(range, {
            title: "↻ Update Snapshots",
            command: "gotest.updateSnapshots",
            arguments: [testPath],
          }),
        );
      }
    }

    return lenses;
  }

  dispose(): void {
    for (const sub of this.subscriptions) {
      sub.dispose();
    }
    this.subscriptions = [];
    this._onDidChangeCodeLenses.dispose();
  }
}
