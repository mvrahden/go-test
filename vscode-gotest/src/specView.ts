import * as vscode from "vscode";
import { spawn } from "node:child_process";
import { buildCliCommand } from "./cli.js";

/**
 * Convert ANSI escape codes to HTML spans with CSS classes.
 * Supports bold, dim, and colors (red, green, yellow).
 */
export function ansiToHtml(text: string): string {
  // HTML-escape first
  let escaped = text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");

  let result = "";
  let openSpans = 0;

  // Match ANSI escape sequences
  const ansiRegex = /\x1b\[(\d+)m/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = ansiRegex.exec(escaped)) !== null) {
    // Append text before this match
    result += escaped.slice(lastIndex, match.index);
    lastIndex = match.index + match[0].length;

    const code = parseInt(match[1], 10);

    switch (code) {
      case 0:
        // Reset: close all open spans
        for (let i = 0; i < openSpans; i++) {
          result += "</span>";
        }
        openSpans = 0;
        break;
      case 1:
        result += '<span class="ansi-bold">';
        openSpans++;
        break;
      case 2:
        result += '<span class="ansi-dim">';
        openSpans++;
        break;
      case 31:
        result += '<span class="ansi-red">';
        openSpans++;
        break;
      case 32:
        result += '<span class="ansi-green">';
        openSpans++;
        break;
      case 33:
        result += '<span class="ansi-yellow">';
        openSpans++;
        break;
      default:
        // Unknown code — ignore
        break;
    }
  }

  // Append remaining text
  result += escaped.slice(lastIndex);

  // Close any unclosed spans
  for (let i = 0; i < openSpans; i++) {
    result += "</span>";
  }

  return result;
}

export class SpecViewPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  private disposables: vscode.Disposable[] = [];

  constructor(private readonly outputChannel: vscode.OutputChannel) {}

  show(): void {
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside);
      return;
    }

    this.panel = vscode.window.createWebviewPanel(
      "gotestSpecView",
      "Go Test: Spec View",
      vscode.ViewColumn.Beside,
      { enableScripts: true },
    );

    this.panel.onDidDispose(
      () => {
        this.panel = undefined;
      },
      null,
      this.disposables,
    );
  }

  get isVisible(): boolean {
    return this.panel?.visible ?? false;
  }

  async refresh(jsonOutput: string): Promise<void> {
    const autoRefresh =
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("specView.autoRefresh") ?? true;

    if (!autoRefresh) {
      return;
    }

    if (!this.panel) {
      return;
    }

    try {
      const stdout = await this.runSpecFromInput(jsonOutput);
      const content = ansiToHtml(stdout);
      this.panel.webview.html = this.buildHtml(content);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[specView] error: ${message}`);
    }
  }

  dispose(): void {
    this.panel?.dispose();
    for (const d of this.disposables) {
      d.dispose();
    }
    this.disposables = [];
  }

  private buildHtml(content: string): string {
    return `<!DOCTYPE html>
<html>
<head>
<style>
  body { background: var(--vscode-editor-background); color: var(--vscode-editor-foreground); margin: 0; padding: 16px; }
  .spec-output { font-family: var(--vscode-editor-font-family); font-size: var(--vscode-editor-font-size); line-height: 1.5; white-space: pre-wrap; }
  .ansi-bold { font-weight: bold; }
  .ansi-dim { opacity: 0.6; }
  .ansi-red { color: var(--vscode-testing-iconFailed); }
  .ansi-green { color: var(--vscode-testing-iconPassed); }
  .ansi-yellow { color: var(--vscode-testing-iconSkipped); }
</style>
</head>
<body>
  <pre class="spec-output">${content}</pre>
</body>
</html>`;
  }

  private async runSpecFromInput(jsonInput: string): Promise<string> {
    const cmd = await buildCliCommand(["spec", "--input=-", "--format=terminal"]);
    return new Promise<string>((resolve, reject) => {
      const child = spawn(cmd.bin, cmd.args);
      let stdout = "";
      let stderr = "";

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
      });

      child.stderr.on("data", (data: Buffer) => {
        stderr += data.toString();
      });

      child.on("close", (code) => {
        if (code !== 0 && stderr) {
          reject(new Error(`gotest spec exited with code ${code}: ${stderr}`));
        } else {
          resolve(stdout);
        }
      });

      child.on("error", (err: Error) => {
        reject(err);
      });

      child.stdin.write(jsonInput);
      child.stdin.end();
    });
  }
}
