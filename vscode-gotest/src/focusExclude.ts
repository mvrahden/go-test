import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import type { DiscoverMethod, DiscoverSuite } from "./types.js";

export class FocusExcludeProvider
  implements vscode.CodeActionProvider, vscode.Disposable
{
  static readonly providedCodeActionKinds = [vscode.CodeActionKind.QuickFix];

  private disposables: vscode.Disposable[] = [];

  constructor(private readonly cache: DiscoveryCache) {}

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range,
  ): vscode.CodeAction[] {
    if (!document.fileName.endsWith("_test.go")) {
      return [];
    }

    const line = range.start.line + 1; // convert to 1-based
    const docBasename = path.basename(document.fileName);

    for (const pkg of this.cache.packages) {
      if (!document.fileName.startsWith(pkg.dir)) {
        continue;
      }
      for (const suite of pkg.suites) {
        if (suite.file === docBasename && suite.line === line) {
          return this.actionsForSuite(suite, document);
        }
        for (const method of suite.methods) {
          if (method.file === docBasename && method.line === line) {
            return this.actionsForMethod(method, document);
          }
        }
      }
    }

    return [];
  }

  dispose(): void {
    for (const d of this.disposables) {
      d.dispose();
    }
  }

  private actionsForMethod(
    method: DiscoverMethod,
    document: vscode.TextDocument,
  ): vscode.CodeAction[] {
    const actions: vscode.CodeAction[] = [];

    if (method.focused) {
      const newName = method.name.replace(/^F_/, "");
      actions.push(
        this.createMethodAction("Unfocus this test", method, newName, document),
      );
    } else if (method.excluded) {
      const newName = method.name.replace(/^X_/, "");
      actions.push(
        this.createMethodAction(
          "Include this test",
          method,
          newName,
          document,
        ),
      );
    } else {
      actions.push(
        this.createMethodAction(
          "Focus this test",
          method,
          `F_${method.name}`,
          document,
        ),
      );
      actions.push(
        this.createMethodAction(
          "Exclude this test",
          method,
          `X_${method.name}`,
          document,
        ),
      );
    }

    return actions;
  }

  private actionsForSuite(
    suite: DiscoverSuite,
    document: vscode.TextDocument,
  ): vscode.CodeAction[] {
    const actions: vscode.CodeAction[] = [];

    if (suite.focused) {
      const newName = suite.name.replace(/^F_/, "");
      actions.push(
        this.createSuiteAction("Unfocus this suite", suite, newName, document),
      );
    } else if (suite.excluded) {
      const newName = suite.name.replace(/^X_/, "");
      actions.push(
        this.createSuiteAction(
          "Include this suite",
          suite,
          newName,
          document,
        ),
      );
    } else {
      actions.push(
        this.createSuiteAction(
          "Focus this suite",
          suite,
          `F_${suite.name}`,
          document,
        ),
      );
      actions.push(
        this.createSuiteAction(
          "Exclude this suite",
          suite,
          `X_${suite.name}`,
          document,
        ),
      );
    }

    return actions;
  }

  private createMethodAction(
    title: string,
    method: DiscoverMethod,
    newName: string,
    document: vscode.TextDocument,
  ): vscode.CodeAction {
    const action = new vscode.CodeAction(
      title,
      vscode.CodeActionKind.QuickFix,
    );
    const edit = new vscode.WorkspaceEdit();

    const lineIndex = method.line - 1;
    const lineText = document.lineAt(lineIndex).text;
    const col = lineText.indexOf(method.name);
    if (col >= 0) {
      const range = new vscode.Range(
        lineIndex,
        col,
        lineIndex,
        col + method.name.length,
      );
      edit.replace(document.uri, range, newName);
    }

    action.edit = edit;
    return action;
  }

  private createSuiteAction(
    title: string,
    suite: DiscoverSuite,
    newName: string,
    document: vscode.TextDocument,
  ): vscode.CodeAction {
    const action = new vscode.CodeAction(
      title,
      vscode.CodeActionKind.QuickFix,
    );
    const edit = new vscode.WorkspaceEdit();

    // Replace at the struct type declaration line
    const declLineIndex = suite.line - 1;
    const declLineText = document.lineAt(declLineIndex).text;
    const declCol = declLineText.indexOf(suite.name);
    if (declCol >= 0) {
      const range = new vscode.Range(
        declLineIndex,
        declCol,
        declLineIndex,
        declCol + suite.name.length,
      );
      edit.replace(document.uri, range, newName);
    }

    // Replace in all method receiver declarations throughout the document
    const receiverPattern = `*${suite.name}`;
    for (let i = 0; i < document.lineCount; i++) {
      if (i === declLineIndex) {
        continue;
      }
      const text = document.lineAt(i).text;
      let searchFrom = 0;
      while (true) {
        const idx = text.indexOf(receiverPattern, searchFrom);
        if (idx < 0) {
          break;
        }
        // Check that this is followed by ')' (after possible whitespace)
        const afterName = idx + receiverPattern.length;
        const rest = text.substring(afterName);
        if (/^\s*\)/.test(rest)) {
          // Replace OldName with NewName (skip the *)
          const nameStart = idx + 1; // skip '*'
          const range = new vscode.Range(
            i,
            nameStart,
            i,
            nameStart + suite.name.length,
          );
          edit.replace(document.uri, range, newName);
        }
        searchFrom = afterName;
      }
    }

    action.edit = edit;
    return action;
  }
}
