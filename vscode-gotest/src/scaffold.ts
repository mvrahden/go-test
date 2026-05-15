import * as vscode from "vscode";
import * as path from "node:path";
import { spawn } from "node:child_process";
import { buildCliCommand, formatCliCommand } from "./cli.js";

export class ScaffoldCodeActionProvider
  implements vscode.CodeActionProvider, vscode.Disposable
{
  static readonly providedCodeActionKinds = [
    vscode.CodeActionKind.RefactorExtract,
  ];

  private static readonly typePattern =
    /^\s*type\s+([A-Z]\w*)\s+(struct|interface)\b/;
  private static readonly exportedFuncPattern = /^func\s+([A-Z]\w*)\s*\(/;

  provideCodeActions(
    document: vscode.TextDocument,
    range: vscode.Range | vscode.Selection,
  ): vscode.CodeAction[] | undefined {
    if (document.fileName.endsWith("_test.go")) {
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

  const cmd = await buildCliCommand(["scaffold", target], effectiveDir);
  outputChannel.info(`[scaffold] ${formatCliCommand(cmd)}`);

  try {
    const stdout = await spawnScaffold(cmd, effectiveDir);
    const match = /^Generated:\s*(.+)$/m.exec(stdout);
    if (match) {
      const generatedPath = match[1];
      const fullPath = generatedPath.startsWith("/")
        ? generatedPath
        : `${effectiveDir}/${generatedPath}`;
      const doc = await vscode.workspace.openTextDocument(fullPath);
      await vscode.window.showTextDocument(doc);
    }
    discoverCallback();
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(`gotest scaffold failed: ${message}`);
  }
}

function spawnScaffold(
  cmd: { bin: string; args: string[] },
  cwd: string,
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const child = spawn(cmd.bin, cmd.args, { cwd });
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
