import * as vscode from "vscode";
import * as path from "node:path";
import {
  buildCliCommand,
  formatCliCommand,
  GotestNotRequiredError,
  offerCliUpgrade,
  runGoInModule,
} from "./cli.js";
import { captureStdout } from "./capture.js";
import { moduleRootFor, readModuleRoots } from "./gomod.js";

export class ScaffoldCodeActionProvider
  implements vscode.CodeActionProvider, vscode.Disposable
{
  static readonly providedCodeActionKinds = [
    vscode.CodeActionKind.RefactorExtract,
  ];

  private static readonly typePattern =
    /^\s*type\s+([A-Z]\w*)\s+(struct|interface)\b/;
  private static readonly exportedFuncPattern = /^func\s+([A-Z]\w*)\s*\(/;

  // isActive says whether a workspace folder uses gotest: elsewhere no
  // action is offered.
  constructor(private readonly isActive: (folder: string) => boolean) {}

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range | vscode.Selection,
  ): vscode.CodeAction[] | undefined {
    if (document.fileName.endsWith("_test.go")) {
      return undefined;
    }
    const folder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!folder || !this.isActive(folder.uri.fsPath)) {
      return undefined;
    }

    const actions: vscode.CodeAction[] = [];
    const line = document.lineAt(range.start.line);
    const typeMatch = ScaffoldCodeActionProvider.typePattern.exec(line.text);

    if (typeMatch) {
      const typeName = typeMatch[1];
      const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
      const action = new vscode.CodeAction(
        `Generate test suite for ${typeName}`,
        vscode.CodeActionKind.RefactorExtract,
      );
      action.command = {
        command: "gotest.scaffoldTarget",
        title: `Scaffold ${typeName}`,
        arguments: [
          this.buildTypeTarget(document, typeName),
          workspaceFolder?.uri.fsPath,
        ],
      };
      actions.push(action);
    }

    if (this.hasScaffoldableDeclarations(document)) {
      const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
      const fileAction = new vscode.CodeAction(
        "Generate test suite for this file",
        vscode.CodeActionKind.RefactorExtract,
      );
      fileAction.command = {
        command: "gotest.scaffoldTarget",
        title: "Scaffold file",
        arguments: [
          this.buildFileTarget(document),
          workspaceFolder?.uri.fsPath,
        ],
      };
      actions.push(fileAction);
    }

    return actions.length > 0 ? actions : undefined;
  }

  dispose(): void {}

  private hasScaffoldableDeclarations(document: vscode.TextDocument): boolean {
    for (let i = 0; i < document.lineCount; i++) {
      const text = document.lineAt(i).text;
      if (ScaffoldCodeActionProvider.typePattern.test(text)) {
        return true;
      }
      if (ScaffoldCodeActionProvider.exportedFuncPattern.test(text)) {
        return true;
      }
    }
    return false;
  }

  private buildTypeTarget(
    document: vscode.TextDocument,
    typeName: string,
  ): string {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!workspaceFolder) {
      return "";
    }
    const relPath = path.relative(
      workspaceFolder.uri.fsPath,
      document.uri.fsPath,
    );
    const relDir = path.posix.normalize(
      path.dirname(relPath).split(path.sep).join("/"),
    );
    return `./${relDir}.${typeName}`;
  }

  private buildFileTarget(document: vscode.TextDocument): string {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!workspaceFolder) {
      return "";
    }
    const relPath = path.relative(
      workspaceFolder.uri.fsPath,
      document.uri.fsPath,
    );
    return `./${relPath.split(path.sep).join("/")}`;
  }
}

export async function runScaffoldCommand(
  outputChannel: vscode.LogOutputChannel,
  discoverCallback: () => void,
  workspaceDir?: string,
): Promise<void> {
  const target = await vscode.window.showInputBox({
    prompt: "Scaffold target",
    placeHolder: "./pkg/path.TypeName or ./pkg/path/file.go",
  });

  if (!target) {
    return;
  }

  await executeScaffold(target, outputChannel, discoverCallback, workspaceDir);
}

// scaffoldTargetDir is the directory a scaffold target names: a file's
// directory, or the package path before .TypeName.
export function scaffoldTargetDir(target: string): string {
  if (target.endsWith(".go")) return path.posix.dirname(target);
  const slash = target.lastIndexOf("/");
  const dot = target.indexOf(".", slash + 1);
  return dot > 0 ? target.slice(0, dot) : target;
}

export async function executeScaffold(
  target: string,
  outputChannel: vscode.LogOutputChannel,
  discoverCallback: () => void,
  workspaceDir?: string,
): Promise<void> {
  const effectiveDir =
    workspaceDir ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!effectiveDir) {
    return;
  }

  try {
    await scaffoldOnce(target, outputChannel, discoverCallback, effectiveDir);
  } catch (err: unknown) {
    if (!(err instanceof GotestNotRequiredError)) {
      reportScaffoldError(err);
      return;
    }
    // No module requires gotest: offer to add it, then scaffold once more.
    const roots = await readModuleRoots(effectiveDir);
    const root =
      moduleRootFor(
        path.resolve(effectiveDir, scaffoldTargetDir(target)),
        roots,
      ) ?? roots[0];
    if (!root) {
      vscode.window.showErrorMessage(
        `gotest scaffold failed: no go.mod in ${effectiveDir}`,
      );
      return;
    }
    const add = `${err.modulePath}@latest`;
    const choice = await vscode.window.showWarningMessage(
      `gotest: no go.mod here requires gotest. Add it to scaffold a suite? This runs: go get -tool ${add}`,
      "Add to go.mod",
    );
    if (choice !== "Add to go.mod") return;
    try {
      await runGoInModule(root.dir, ["get", "-tool", add], outputChannel);
    } catch (getErr: unknown) {
      const message = getErr instanceof Error ? getErr.message : String(getErr);
      vscode.window.showErrorMessage(
        `gotest: adding gotest failed. ${message}`,
      );
      return;
    }
    try {
      await scaffoldOnce(target, outputChannel, discoverCallback, effectiveDir);
    } catch (again: unknown) {
      reportScaffoldError(again);
    }
  }
}

function reportScaffoldError(err: unknown): void {
  if (offerCliUpgrade(err)) return;
  const message = err instanceof Error ? err.message : String(err);
  vscode.window.showErrorMessage(`gotest scaffold failed: ${message}`);
}

async function scaffoldOnce(
  target: string,
  outputChannel: vscode.LogOutputChannel,
  discoverCallback: () => void,
  effectiveDir: string,
): Promise<void> {
  const cmd = await buildCliCommand(["scaffold", target], effectiveDir);
  outputChannel.info(`[scaffold] ${formatCliCommand(cmd)}`);
  const stdout = await captureStdout(cmd.bin, cmd.args, {
    cwd: effectiveDir,
  });
  const match = /^Generated:\s*(.+)$/m.exec(stdout);
  if (match) {
    const generatedPath = match[1];
    const fullPath = path.isAbsolute(generatedPath)
      ? generatedPath
      : path.join(effectiveDir, generatedPath);
    const doc = await vscode.workspace.openTextDocument(fullPath);
    await vscode.window.showTextDocument(doc);
  }
  discoverCallback();
}
