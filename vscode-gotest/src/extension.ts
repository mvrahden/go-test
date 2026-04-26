import * as vscode from "vscode";

export function activate(context: vscode.ExtensionContext): void {
  const outputChannel = vscode.window.createOutputChannel("Go Test Suites");
  outputChannel.appendLine("Go Test Suites extension activated");
}

export function deactivate(): void {}
