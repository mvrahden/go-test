import * as vscode from "vscode";
import { spawn } from "node:child_process";
import { buildCliCommand } from "./cli.js";

const ansiClassMap: Record<number, string> = {
  1: "ansi-bold",
  2: "ansi-dim",
  3: "ansi-italic",
  4: "ansi-underline",
  31: "ansi-red",
  32: "ansi-green",
  33: "ansi-yellow",
  34: "ansi-blue",
  35: "ansi-magenta",
  36: "ansi-cyan",
  37: "ansi-white",
  90: "ansi-bright-black",
  91: "ansi-bright-red",
  92: "ansi-bright-green",
  93: "ansi-bright-yellow",
  94: "ansi-bright-blue",
  95: "ansi-bright-magenta",
  96: "ansi-bright-cyan",
  97: "ansi-bright-white",
};

const resetCodes = new Set([0, 22, 23, 24, 39, 49]);

/**
 * Convert ANSI escape codes to HTML spans with CSS classes.
 * Supports bold, dim, italic, underline, standard and bright colors,
 * and multi-parameter sequences (e.g. \x1b[1;31m).
 */
export function ansiToHtml(text: string): string {
  let escaped = text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");

  let result = "";
  let openSpans = 0;

  const ansiRegex = /\x1b\[([\d;]*)m/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = ansiRegex.exec(escaped)) !== null) {
    result += escaped.slice(lastIndex, match.index);
    lastIndex = match.index + match[0].length;

    const params = match[1]
      ? match[1].split(";").map((s) => parseInt(s, 10))
      : [0];

    for (const code of params) {
      if (resetCodes.has(code)) {
        for (let i = 0; i < openSpans; i++) {
          result += "</span>";
        }
        openSpans = 0;
      } else {
        const cls = ansiClassMap[code];
        if (cls) {
          result += `<span class="${cls}">`;
          openSpans++;
        }
      }
    }
  }

  result += escaped.slice(lastIndex);

  for (let i = 0; i < openSpans; i++) {
    result += "</span>";
  }

  return result;
}

export class SpecViewPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  private disposables: vscode.Disposable[] = [];

  constructor(private readonly outputChannel: vscode.LogOutputChannel) {}

  show(): void {
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside);
      return;
    }

    this.panel = vscode.window.createWebviewPanel(
      "gotestSpecView",
      "Go Test: Spec View",
      vscode.ViewColumn.Beside,
      {},
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
      this.outputChannel.error(`[specView] ${message}`);
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
  .ansi-italic { font-style: italic; }
  .ansi-underline { text-decoration: underline; }
  .ansi-red { color: var(--vscode-testing-iconFailed); }
  .ansi-green { color: var(--vscode-testing-iconPassed); }
  .ansi-yellow { color: var(--vscode-testing-iconSkipped); }
  .ansi-blue { color: var(--vscode-terminal-ansiBlue); }
  .ansi-magenta { color: var(--vscode-terminal-ansiMagenta); }
  .ansi-cyan { color: var(--vscode-terminal-ansiCyan); }
  .ansi-white { color: var(--vscode-terminal-ansiWhite); }
  .ansi-bright-black { color: var(--vscode-terminal-ansiBrightBlack); }
  .ansi-bright-red { color: var(--vscode-terminal-ansiBrightRed); }
  .ansi-bright-green { color: var(--vscode-terminal-ansiBrightGreen); }
  .ansi-bright-yellow { color: var(--vscode-terminal-ansiBrightYellow); }
  .ansi-bright-blue { color: var(--vscode-terminal-ansiBrightBlue); }
  .ansi-bright-magenta { color: var(--vscode-terminal-ansiBrightMagenta); }
  .ansi-bright-cyan { color: var(--vscode-terminal-ansiBrightCyan); }
  .ansi-bright-white { color: var(--vscode-terminal-ansiBrightWhite); }
</style>
</head>
<body>
  <pre class="spec-output">${content}</pre>
</body>
</html>`;
  }

  private async runSpecFromInput(jsonInput: string): Promise<string> {
    const cmd = await buildCliCommand([
      "spec",
      "--input=-",
      "--format=terminal",
    ]);
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
