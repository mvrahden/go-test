import * as vscode from "vscode";
import type { DiscoveryCache } from "./discovery.js";

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

  provideCodeLenses(
    document: vscode.TextDocument,
    _token: vscode.CancellationToken,
  ): vscode.CodeLens[] {
    if (!document.fileName.endsWith("_test.go")) {
      return [];
    }

    const enabled =
      vscode.workspace.getConfiguration("gotest").get<boolean>("showCodeLens") ??
      true;
    if (!enabled) {
      return [];
    }

    const lenses: vscode.CodeLens[] = [];

    for (const pkg of this.cache.packages) {
      if (!document.fileName.startsWith(pkg.dir)) {
        continue;
      }

      for (const suite of pkg.suites) {
        if (suite.file === document.fileName) {
          const range = new vscode.Range(
            suite.line - 1,
            0,
            suite.line - 1,
            0,
          );
          const testPath = `${pkg.importPath}/${suite.name}`;

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

        for (const method of suite.methods) {
          if (method.file === document.fileName) {
            const range = new vscode.Range(
              method.line - 1,
              0,
              method.line - 1,
              0,
            );
            const testPath = `${pkg.importPath}/${suite.name}/${method.name}`;

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
          }
        }
      }
    }

    return lenses;
  }

  dispose(): void {
    this.subscription.dispose();
    this._onDidChangeCodeLenses.dispose();
  }
}
