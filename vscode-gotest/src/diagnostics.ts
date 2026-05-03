import * as vscode from "vscode";
import type { DiscoveryCache } from "./discovery.js";

export class FocusDiagnostics implements vscode.Disposable {
  private readonly diagnosticCollection: vscode.DiagnosticCollection;
  private readonly statusBarItem: vscode.StatusBarItem;
  private readonly disposables: vscode.Disposable[] = [];

  constructor(private readonly cache: DiscoveryCache) {
    this.diagnosticCollection =
      vscode.languages.createDiagnosticCollection("gotest");
    this.statusBarItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      50,
    );
    this.statusBarItem.command = "gotest.showFocusedTests";

    this.disposables.push(
      this.diagnosticCollection,
      this.statusBarItem,
      cache.onDidUpdate(() => this.refresh()),
    );
  }

  refresh(): void {
    const config = vscode.workspace.getConfiguration("gotest");
    const showWarnings = config.get<boolean>("showFocusWarnings", true);

    this.diagnosticCollection.clear();

    if (!showWarnings) {
      this.statusBarItem.hide();
      return;
    }

    const diagnosticsMap = new Map<string, vscode.Diagnostic[]>();
    let focusCount = 0;

    for (const pkg of this.cache.packages) {
      for (const suite of pkg.suites) {
        if (suite.focused) {
          focusCount++;
          this.addDiagnostic(
            diagnosticsMap,
            suite.file,
            suite.line,
            suite.name,
            true,
          );
        }
        for (const method of suite.methods) {
          if (method.focused) {
            focusCount++;
            this.addDiagnostic(
              diagnosticsMap,
              method.file,
              method.line,
              method.name,
              false,
            );
          }
        }
      }
    }

    for (const warning of this.cache.warnings) {
      if (!warning.file) {
        continue;
      }
      const pkg = this.cache.getPackage(warning.importPath);
      if (!pkg) {
        continue;
      }
      const absFile = `${pkg.dir}/${warning.file}`;
      const line = (warning.line ?? 1) - 1;
      const col = (warning.col ?? 1) - 1;
      const range = new vscode.Range(line, col, line, col + 1);
      const diagnostic = new vscode.Diagnostic(
        range,
        warning.message,
        vscode.DiagnosticSeverity.Warning,
      );
      diagnostic.source = "gotest";
      const existing = diagnosticsMap.get(absFile) ?? [];
      existing.push(diagnostic);
      diagnosticsMap.set(absFile, existing);
    }

    for (const [file, diagnostics] of diagnosticsMap) {
      this.diagnosticCollection.set(vscode.Uri.file(file), diagnostics);
    }

    if (focusCount > 0) {
      this.statusBarItem.text = `$(warning) gotest: ${focusCount} focused test(s)`;
      this.statusBarItem.tooltip = `${focusCount} focused test(s) detected — will cause CI failure`;
      this.statusBarItem.show();
    } else {
      this.statusBarItem.hide();
    }
  }

  async showFocusedTests(): Promise<void> {
    const items: (vscode.QuickPickItem & { file: string; line: number })[] = [];

    for (const pkg of this.cache.packages) {
      for (const suite of pkg.suites) {
        if (suite.focused) {
          items.push({
            label: suite.name,
            description: `${suite.file}:${suite.line}`,
            detail: "Focused suite",
            file: suite.file,
            line: suite.line,
          });
        }
        for (const method of suite.methods) {
          if (method.focused) {
            items.push({
              label: method.name,
              description: `${method.file}:${method.line}`,
              detail: `Focused method in ${suite.name}`,
              file: method.file,
              line: method.line,
            });
          }
        }
      }
    }

    const selected = await vscode.window.showQuickPick(items, {
      placeHolder: "Select a focused test to navigate to",
    });

    if (selected) {
      const doc = await vscode.workspace.openTextDocument(selected.file);
      const editor = await vscode.window.showTextDocument(doc);
      const position = new vscode.Position(selected.line - 1, 0);
      editor.selection = new vscode.Selection(position, position);
      editor.revealRange(
        new vscode.Range(position, position),
        vscode.TextEditorRevealType.InCenter,
      );
    }
  }

  dispose(): void {
    for (const d of this.disposables) {
      d.dispose();
    }
  }

  private addDiagnostic(
    map: Map<string, vscode.Diagnostic[]>,
    file: string,
    line: number,
    name: string,
    isSuite: boolean,
  ): void {
    const zeroLine = line - 1;
    const range = new vscode.Range(zeroLine, 0, zeroLine, name.length);
    const message = isSuite
      ? "Focused test suite — will cause CI failure (gotest --ci)"
      : "Focused test — will cause CI failure (gotest --ci)";
    const diagnostic = new vscode.Diagnostic(
      range,
      message,
      vscode.DiagnosticSeverity.Warning,
    );
    diagnostic.source = "gotest";

    const existing = map.get(file) ?? [];
    existing.push(diagnostic);
    map.set(file, existing);
  }
}
