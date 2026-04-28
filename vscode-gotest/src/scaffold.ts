import * as vscode from "vscode";
import * as path from "node:path";
import { spawn } from "node:child_process";

export class ScaffoldCodeActionProvider implements vscode.CodeActionProvider, vscode.Disposable {
  static readonly providedCodeActionKinds = [vscode.CodeActionKind.RefactorExtract];

  private static readonly structPattern = /^\s*type\s+([A-Z]\w*)\s+struct/;

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range | vscode.Selection,
  ): vscode.CodeAction[] | undefined {
    if (document.fileName.endsWith("_test.go")) {
      return undefined;
    }

    const actions: vscode.CodeAction[] = [];
    const line = document.lineAt(range.start.line);
    const structMatch = ScaffoldCodeActionProvider.structPattern.exec(line.text);

    if (structMatch) {
      const typeName = structMatch[1];
      const action = new vscode.CodeAction(
        `Generate test suite for ${typeName}`,
        vscode.CodeActionKind.RefactorExtract,
      );
      action.command = {
        command: "gotest.scaffoldTarget",
        title: `Scaffold ${typeName}`,
        arguments: [this.buildTypeTarget(document, typeName)],
      };
      actions.push(action);
    }

    const fileAction = new vscode.CodeAction(
      "Generate test suite for this file",
      vscode.CodeActionKind.RefactorExtract,
    );
    fileAction.command = {
      command: "gotest.scaffoldTarget",
      title: "Scaffold file",
      arguments: [this.buildFileTarget(document)],
    };
    actions.push(fileAction);

    return actions;
  }

  dispose(): void {}

  private buildTypeTarget(document: vscode.TextDocument, typeName: string): string {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!workspaceFolder) {
      return "";
    }
    const relPath = path.relative(workspaceFolder.uri.fsPath, document.uri.fsPath);
    const relDir = path.posix.normalize(path.dirname(relPath).split(path.sep).join("/"));
    return `./${relDir}.${typeName}`;
  }

  private buildFileTarget(document: vscode.TextDocument): string {
    const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
    if (!workspaceFolder) {
      return "";
    }
    const relPath = path.relative(workspaceFolder.uri.fsPath, document.uri.fsPath);
    return `./${relPath.split(path.sep).join("/")}`;
  }
}

export async function runScaffoldCommand(
  outputChannel: vscode.OutputChannel,
  discoverCallback: () => void,
): Promise<void> {
  const target = await vscode.window.showInputBox({
    prompt: "Scaffold target",
    placeHolder: "./pkg/path.TypeName or ./pkg/path/file.go",
  });

  if (!target) {
    return;
  }

  await executeScaffold(target, outputChannel, discoverCallback);
}

export async function executeScaffold(
  target: string,
  outputChannel: vscode.OutputChannel,
  discoverCallback: () => void,
): Promise<void> {
  const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!workspaceDir) {
    return;
  }

  const cliPath =
    vscode.workspace.getConfiguration("gotest").get<string>("cliPath") ?? "gotest";

  outputChannel.appendLine(`[scaffold] ${cliPath} scaffold ${target}`);

  try {
    const stdout = await spawnScaffold(cliPath, target, workspaceDir);
    const match = /^Generated:\s*(.+)$/m.exec(stdout);
    if (match) {
      const generatedPath = match[1];
      const fullPath = generatedPath.startsWith("/")
        ? generatedPath
        : `${workspaceDir}/${generatedPath}`;
      const doc = await vscode.workspace.openTextDocument(fullPath);
      await vscode.window.showTextDocument(doc);
    }
    discoverCallback();
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(`gotest scaffold failed: ${message}`);
  }
}

function spawnScaffold(cliPath: string, target: string, cwd: string): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const child = spawn(cliPath, ["scaffold", target], { cwd });
    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
    });

    child.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
    });

    child.on("close", (code) => {
      if (code !== 0) {
        reject(new Error(stderr || `scaffold exited with code ${code}`));
      } else {
        resolve(stdout);
      }
    });

    child.on("error", (err: Error) => {
      reject(err);
    });
  });
}
