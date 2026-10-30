import * as vscode from "vscode";
import * as path from "node:path";
import { buildCliCommand, formatCliCommand, offerCliUpgrade } from "./cli.js";
import { captureStdout } from "./capture.js";
import type { DiscoveryCache } from "./discovery.js";
import type { DiscoverMethod, DiscoverSuite } from "./types.js";

export const TOGGLE_FOCUS_COMMAND = "gotest.toggleFocus";

export class FocusExcludeProvider
  implements vscode.CodeActionProvider, vscode.Disposable
{
  static readonly providedCodeActionKinds = [vscode.CodeActionKind.QuickFix];

  constructor(private readonly cache: DiscoveryCache) {}

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range,
  ): vscode.CodeAction[] {
    if (!document.fileName.endsWith("_test.go")) {
      return [];
    }

    const line = range.start.line + 1; // convert to 1-based
    const docPath = document.fileName;

    for (const pkg of this.cache.packages) {
      if (!docPath.startsWith(pkg.dir)) {
        continue;
      }
      for (const suite of pkg.suites) {
        if (path.join(pkg.dir, suite.file) === docPath && suite.line === line) {
          return this.actionsForSuite(suite, document);
        }
        for (const method of suite.methods) {
          if (
            path.join(pkg.dir, method.file) === docPath &&
            method.line === line
          ) {
            return this.actionsForMethod(suite, method, document);
          }
        }
      }
    }

    return [];
  }

  dispose(): void {}

  private actionsForMethod(
    suite: DiscoverSuite,
    method: DiscoverMethod,
    document: vscode.TextDocument,
  ): vscode.CodeAction[] {
    const actions: vscode.CodeAction[] = [];
    // The receiver names the type the method is declared on, which an alias
    // suite does not: its methods live on the generic type it instantiates.
    const receiver =
      receiverTypeName(document.lineAt(method.line - 1).text) ?? suite.name;
    const identifier = `${receiver}.${method.name}`;

    if (method.focused) {
      actions.push(
        toggleFocusAction("Unfocus this test", document, identifier),
      );
    } else if (method.excluded) {
      const newName = method.name.replace(/^X_/, "");
      actions.push(
        this.createMethodAction("Include this test", method, newName, document),
      );
    } else {
      actions.push(toggleFocusAction("Focus this test", document, identifier));
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
      actions.push(
        toggleFocusAction("Unfocus this suite", document, suite.name),
      );
    } else if (suite.excluded) {
      const newName = suite.name.replace(/^X_/, "");
      actions.push(
        this.createSuiteAction("Include this suite", suite, newName, document),
      );
    } else {
      actions.push(toggleFocusAction("Focus this suite", document, suite.name));
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
    const action = new vscode.CodeAction(title, vscode.CodeActionKind.QuickFix);
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
    const action = new vscode.CodeAction(title, vscode.CodeActionKind.QuickFix);
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

// toggleFocusAction delegates the rename to the CLI, which follows the name
// into every file of the package; an in-document edit would break a suite
// whose methods live in other files.
function toggleFocusAction(
  title: string,
  document: vscode.TextDocument,
  identifier: string,
): vscode.CodeAction {
  const action = new vscode.CodeAction(title, vscode.CodeActionKind.QuickFix);
  action.command = {
    command: TOGGLE_FOCUS_COMMAND,
    title,
    arguments: [
      document.uri.fsPath,
      identifier,
      vscode.workspace.getWorkspaceFolder(document.uri)?.uri.fsPath,
    ],
  };
  return action;
}

// receiverTypeName reads the receiver's type name off a method declaration
// line, type parameters dropped.
export function receiverTypeName(line: string): string | undefined {
  return /^\s*func\s*\(\s*(?:\w+\s+)?\*?\s*(\w+)/.exec(line)?.[1];
}

// executeToggleFocus runs `gotest refactor toggle-focus`. The CLI edits files
// on disk, so unsaved changes are saved first; a refusal surfaces its stderr.
export async function executeToggleFocus(
  file: string,
  identifier: string,
  outputChannel: vscode.LogOutputChannel,
  discoverCallback: () => void,
  workspaceDir?: string,
): Promise<void> {
  const effectiveDir =
    workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!effectiveDir) {
    return;
  }
  if (!(await vscode.workspace.saveAll(false))) {
    vscode.window.showErrorMessage(
      `gotest refactor toggle-focus not run: unsaved changes could not be saved`,
    );
    return;
  }

  try {
    const cmd = await buildCliCommand(
      ["refactor", "toggle-focus", file, identifier],
      effectiveDir,
    );
    outputChannel.info(`[toggle-focus] ${formatCliCommand(cmd)}`);
    await captureStdout(cmd.bin, cmd.args, { cwd: effectiveDir });
    discoverCallback();
  } catch (err: unknown) {
    if (offerCliUpgrade(err)) return;
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(
      `gotest refactor toggle-focus failed: ${message}`,
    );
  }
}
