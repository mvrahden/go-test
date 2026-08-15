import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import { countCrasherEntries } from "./fuzz.js";

export class GoTestCodeLensProvider
  implements vscode.CodeLensProvider, vscode.Disposable
{
  private _onDidChangeCodeLenses = new vscode.EventEmitter<void>();
  readonly onDidChangeCodeLenses: vscode.Event<void> =
    this._onDidChangeCodeLenses.event;

  private subscription: vscode.Disposable;

  constructor(private readonly cache: DiscoveryCache) {
    this.subscription = cache.onDidUpdate(() =>
      this._onDidChangeCodeLenses.fire(),
    );
  }

  async provideCodeLenses(
    document: vscode.TextDocument,
    _token: vscode.CancellationToken,
  ): Promise<vscode.CodeLens[]> {
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

      const fileFuzzers = (suite.fuzzers ?? []).filter(
        (m) => path.join(pkg.dir, m.file) === docPath,
      );

      for (const method of fileFuzzers) {
        const range = new vscode.Range(method.line - 1, 0, method.line - 1, 0);

        // "Fuzz" starts a budgeted search; "Debug Seeds" replays the
        // target's seed corpus under the debugger. Plain seed replay runs
        // through the Test Explorer item, like any other test.
        lenses.push(
          new vscode.CodeLens(range, {
            title: "▶ Fuzz",
            command: "gotest.runFuzz",
            arguments: [importPath, suite.name, method.name],
          }),
          new vscode.CodeLens(range, {
            title: "Debug Seeds",
            command: "gotest.debugFuzz",
            arguments: [importPath, suite.name, method.name],
          }),
        );

        // Pending crashers surface exactly where the target lives. For
        // struct-typed targets the corpus files are format-bound (the
        // fuzz-struct-corpus lint rule's concern); promote is the durable
        // answer either way.
        const crasherCount = await countCrasherEntries(
          pkg.dir,
          `Fuzz${suite.name}_${method.name}`,
        );
        if (crasherCount > 0) {
          lenses.push(
            new vscode.CodeLens(range, {
              title: `⚠ Promote ${crasherCount} crasher${crasherCount === 1 ? "" : "s"}`,
              command: "gotest.promoteCrashers",
              arguments: [importPath],
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
    this.subscription.dispose();
    this._onDidChangeCodeLenses.dispose();
  }
}
