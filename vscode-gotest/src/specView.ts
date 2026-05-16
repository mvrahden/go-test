import * as vscode from "vscode";
import { spawn } from "node:child_process";
import { buildCliCommand } from "./cli.js";

export class SpecViewPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  private disposables: vscode.Disposable[] = [];
  private extensionUri: vscode.Uri | undefined;

  constructor(private readonly outputChannel: vscode.LogOutputChannel) {}

  setExtensionUri(uri: vscode.Uri): void {
    this.extensionUri = uri;
  }

  show(): void {
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside);
      return;
    }

    const localResourceRoots: vscode.Uri[] = [];
    if (this.extensionUri) {
      localResourceRoots.push(
        vscode.Uri.joinPath(this.extensionUri, "static"),
      );
    }

    this.panel = vscode.window.createWebviewPanel(
      "gotestSpecView",
      "Go Test: Spec View",
      vscode.ViewColumn.Beside,
      { enableScripts: true, localResourceRoots },
    );

    this.panel.webview.onDidReceiveMessage(
      (msg) => {
        if (msg.type === "runTests") {
          vscode.commands.executeCommand("testing.runAll");
        }
      },
      null,
      this.disposables,
    );

    this.panel.onDidDispose(
      () => {
        this.panel = undefined;
      },
      null,
      this.disposables,
    );

    this.panel.webview.html = this.buildHtml({ type: "empty" });
  }

  get isVisible(): boolean {
    return this.panel?.visible ?? false;
  }

  async refresh(jsonOutput: string): Promise<void> {
    const autoRefresh =
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("specView.autoRefresh") ?? true;

    if (!autoRefresh || !this.panel) {
      return;
    }

    try {
      const raw = await this.runSpecFromInput(jsonOutput);
      const data = JSON.parse(raw);
      this.panel.webview.html = this.buildHtml({
        type: "specData",
        data,
      });
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

  private getGopherUri(): string {
    if (!this.panel || !this.extensionUri) return "";
    return this.panel.webview
      .asWebviewUri(vscode.Uri.joinPath(this.extensionUri, "static", "icon.png"))
      .toString();
  }

  private buildHtml(
    msg: { type: "empty" } | { type: "specData"; data: SpecData },
  ): string {
    const gopherUri = this.getGopherUri();
    const nonce = getNonce();

    const body =
      msg.type === "empty"
        ? buildEmptyBody(gopherUri)
        : buildSpecBody(msg.data);

    return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${this.panel!.webview.cspSource}; style-src 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
<style nonce="${nonce}">
${CSS}
</style>
</head>
<body>
${body}
<script nonce="${nonce}">
${SCRIPT}
</script>
</body>
</html>`;
  }

  private async runSpecFromInput(jsonInput: string): Promise<string> {
    const cmd = await buildCliCommand(["spec", "--input=-", "--format=json"]);
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

// --- Types ---

interface SpecData {
  packages: SpecPackage[];
  stats: SpecStats;
}

interface SpecPackage {
  path: string;
  status: string;
  duration: number;
  nodes: SpecNode[];
}

interface SpecNode {
  name: string;
  display: string;
  kind: string;
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  output: string[];
  children: SpecNode[];
}

interface SpecStats {
  suites: number;
  behaviors: number;
  tests: number;
  passed: number;
  failed: number;
  skipped: number;
}

// --- HTML Builders ---

function buildEmptyBody(gopherUri: string): string {
  const img = gopherUri
    ? `<img src="${gopherUri}" alt="gotest gopher" class="empty-logo" />`
    : "";
  return `<div class="empty-state">
  ${img}
  <h1 class="empty-title">Behavioral Specification</h1>
  <p class="empty-text">Run your tests to see the spec view.<br/>Each suite, method, and behavior will appear here as an interactive tree.</p>
  <button class="empty-button" onclick="postRunTests()">Run Tests</button>
  <div class="empty-legend">
    <span class="legend-item pass">✓ passed</span>
    <span class="legend-item fail">✗ failed</span>
    <span class="legend-item skip">~ skipped</span>
  </div>
</div>`;
}

function buildSpecBody(data: SpecData): string {
  const toolbar = buildToolbar();
  const tree = data.packages.map((pkg) => buildPackageHtml(pkg, data.packages.length > 1)).join("");
  const summary = buildSummary(data.stats);
  return `<div class="spec-container">
  ${toolbar}
  <div class="spec-tree" id="spec-tree">${tree}</div>
  ${summary}
</div>`;
}

function buildToolbar(): string {
  return `<div class="toolbar">
  <div class="toolbar-group">
    <button class="tool-btn" onclick="expandAll()" title="Expand all">▼ All</button>
    <button class="tool-btn" onclick="collapseAll()" title="Collapse all">▲ None</button>
  </div>
  <div class="toolbar-group">
    <button class="filter-btn pass active" onclick="toggleFilter('pass')" title="Toggle passed">✓</button>
    <button class="filter-btn fail active" onclick="toggleFilter('fail')" title="Toggle failed">✗</button>
    <button class="filter-btn skip active" onclick="toggleFilter('skip')" title="Toggle skipped">~</button>
  </div>
  <div class="toolbar-group search-group">
    <input type="text" class="search-input" id="search-input" placeholder="Search behaviors..." oninput="onSearchInput()" />
  </div>
</div>`;
}

function buildPackageHtml(pkg: SpecPackage, multiPkg: boolean): string {
  const header = multiPkg ? `<div class="pkg-header">=== ${escapeHtml(pkg.path)} ===</div>` : "";
  const nodes = pkg.nodes.map((n) => buildNodeHtml(n, 0)).join("");
  return `${header}${nodes}`;
}

function buildNodeHtml(node: SpecNode, depth: number): string {
  const isLeaf = node.children.length === 0;

  if (isLeaf) {
    return buildLeafHtml(node, depth);
  }
  return buildBranchHtml(node, depth);
}

function buildLeafHtml(node: SpecNode, depth: number): string {
  const icon = statusIcon(node.status);
  const dur = formatDuration(node.duration);
  const suffix =
    node.excluded || node.status === "skip" ? " — SKIPPED" : "";

  let errorBlock = "";
  if (node.status === "fail" && node.output.length > 0) {
    const lines = node.output
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith("=== ") && !l.startsWith("--- "))
      .map(escapeHtml)
      .join("\n");
    if (lines) {
      errorBlock = `<details class="error-details"><summary class="error-summary">Error details</summary><pre class="error-output">${lines}</pre></details>`;
    }
  }

  return `<div class="leaf ${node.status}" data-display="${escapeAttr(node.display)}" style="padding-left:${(depth + 1) * 16}px">
  <span class="icon ${node.status}">${icon}</span> ${escapeHtml(node.display)}${suffix} <span class="dur">(${dur})</span>
  ${errorBlock}
</div>`;
}

function buildBranchHtml(node: SpecNode, depth: number): string {
  const shouldOpen = hasFailure(node);
  const openAttr = shouldOpen ? " open" : "";
  const icon = derivedStatusIcon(node);
  const children = node.children.map((c) => buildNodeHtml(c, depth + 1)).join("");

  let label = escapeHtml(node.display);
  if (
    node.kind === "suite" ||
    node.kind === "fixture" ||
    node.kind === "method" ||
    node.kind === "test"
  ) {
    label = `<strong>${label}</strong>`;
  }

  let suffix = "";
  if (node.focused) suffix = ` <span class="tag focused">FOCUSED</span>`;
  else if (node.excluded) suffix = ` <span class="tag skipped">SKIPPED</span>`;

  return `<details class="branch" data-display="${escapeAttr(node.display)}"${openAttr} style="padding-left:${depth * 16}px">
  <summary class="node ${node.kind}"><span class="icon ${derivedStatus(node)}">${icon}</span> ${label}${suffix}</summary>
  ${children}
</details>`;
}

function hasFailure(node: SpecNode): boolean {
  if (node.status === "fail") return true;
  return node.children.some(hasFailure);
}

function derivedStatus(node: SpecNode): string {
  if (node.children.some((c) => c.status === "fail" || derivedStatus(c) === "fail")) return "fail";
  if (node.children.every((c) => c.status === "skip" || (c.children.length > 0 && derivedStatus(c) === "skip"))) return "skip";
  return "pass";
}

function derivedStatusIcon(node: SpecNode): string {
  return statusIcon(derivedStatus(node));
}

function statusIcon(status: string): string {
  switch (status) {
    case "pass": return "✓";
    case "fail": return "✗";
    case "skip": return "~";
    default: return "?";
  }
}

function formatDuration(seconds: number): string {
  const ms = Math.round(seconds * 1000);
  if (ms < 1) return "<1ms";
  if (ms < 1000) return `${ms}ms`;
  return `${seconds.toFixed(1)}s`;
}

function buildSummary(stats: SpecStats): string {
  const counts: string[] = [];
  if (stats.suites > 0) counts.push(`${stats.suites} suites`);
  if (stats.behaviors > 0) counts.push(`${stats.behaviors} behaviors`);
  if (stats.tests > 0) counts.push(`${stats.tests} stdlib tests`);

  return `<div class="summary">
  <span class="summary-counts">${counts.join(", ")}</span>
  <span class="summary-results">
    ${stats.passed > 0 ? `<span class="pass">${stats.passed} passed</span>` : ""}
    ${stats.failed > 0 ? `<span class="fail">${stats.failed} failed</span>` : ""}
    ${stats.skipped > 0 ? `<span class="skip">${stats.skipped} skipped</span>` : ""}
  </span>
</div>`;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/"/g, "&quot;");
}

function getNonce(): string {
  const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  let result = "";
  for (let i = 0; i < 32; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

// --- Embedded CSS ---

const CSS = `
body {
  background: var(--vscode-editor-background);
  color: var(--vscode-editor-foreground);
  font-family: var(--vscode-editor-font-family);
  font-size: var(--vscode-editor-font-size);
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  height: 100vh;
}

/* Empty state */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  text-align: center;
  padding: 32px;
}
.empty-logo {
  width: 120px;
  height: 120px;
  margin-bottom: 24px;
  opacity: 0.85;
}
.empty-title {
  font-size: 1.4em;
  font-weight: 600;
  margin: 0 0 12px 0;
  color: var(--vscode-editor-foreground);
}
.empty-text {
  color: var(--vscode-descriptionForeground);
  margin: 0 0 24px 0;
  line-height: 1.6;
  max-width: 340px;
}
.empty-button {
  background: var(--vscode-button-background);
  color: var(--vscode-button-foreground);
  border: none;
  padding: 8px 20px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.95em;
  margin-bottom: 24px;
}
.empty-button:hover {
  background: var(--vscode-button-hoverBackground);
}
.empty-legend {
  display: flex;
  gap: 16px;
  color: var(--vscode-descriptionForeground);
  font-size: 0.85em;
}
.legend-item.pass { color: var(--vscode-testing-iconPassed); }
.legend-item.fail { color: var(--vscode-testing-iconFailed); }
.legend-item.skip { color: var(--vscode-testing-iconSkipped); }

/* Toolbar */
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--vscode-panel-border);
  flex-shrink: 0;
}
.toolbar-group {
  display: flex;
  gap: 4px;
}
.tool-btn {
  background: var(--vscode-button-secondaryBackground);
  color: var(--vscode-button-secondaryForeground);
  border: none;
  padding: 4px 10px;
  border-radius: 3px;
  cursor: pointer;
  font-size: 0.85em;
}
.tool-btn:hover {
  background: var(--vscode-button-secondaryHoverBackground);
}
.filter-btn {
  border: 1px solid transparent;
  padding: 4px 8px;
  border-radius: 3px;
  cursor: pointer;
  font-size: 0.95em;
  background: transparent;
  opacity: 0.4;
}
.filter-btn.active { opacity: 1; }
.filter-btn.pass { color: var(--vscode-testing-iconPassed); }
.filter-btn.pass.active { border-color: var(--vscode-testing-iconPassed); }
.filter-btn.fail { color: var(--vscode-testing-iconFailed); }
.filter-btn.fail.active { border-color: var(--vscode-testing-iconFailed); }
.filter-btn.skip { color: var(--vscode-testing-iconSkipped); }
.filter-btn.skip.active { border-color: var(--vscode-testing-iconSkipped); }
.search-group { flex: 1; }
.search-input {
  width: 100%;
  background: var(--vscode-input-background);
  color: var(--vscode-input-foreground);
  border: 1px solid var(--vscode-input-border);
  padding: 4px 8px;
  border-radius: 3px;
  font-size: 0.85em;
  outline: none;
  box-sizing: border-box;
}
.search-input:focus {
  border-color: var(--vscode-focusBorder);
}

/* Spec tree */
.spec-container {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.spec-tree {
  flex: 1;
  overflow-y: auto;
  padding: 8px 12px;
  line-height: 1.7;
}
.pkg-header {
  color: var(--vscode-descriptionForeground);
  margin: 12px 0 4px 0;
}

/* Nodes */
details.branch { list-style: none; }
details.branch > summary { cursor: pointer; list-style: none; }
details.branch > summary::-webkit-details-marker { display: none; }
summary.node { padding: 1px 0; }

.leaf { padding: 1px 0; white-space: nowrap; }

.icon.pass { color: var(--vscode-testing-iconPassed); }
.icon.fail { color: var(--vscode-testing-iconFailed); }
.icon.skip { color: var(--vscode-testing-iconSkipped); }
.icon.none { opacity: 0.4; }

.dur { color: var(--vscode-descriptionForeground); font-size: 0.85em; }

.tag { font-size: 0.8em; margin-left: 6px; }
.tag.focused { color: var(--vscode-testing-iconSkipped); }
.tag.skipped { color: var(--vscode-testing-iconSkipped); }

/* Error output */
.error-details { margin: 2px 0 4px 20px; }
.error-summary {
  color: var(--vscode-descriptionForeground);
  font-size: 0.85em;
  cursor: pointer;
}
.error-output {
  color: var(--vscode-testing-iconFailed);
  font-size: 0.85em;
  margin: 4px 0;
  white-space: pre-wrap;
}

/* Summary */
.summary {
  display: flex;
  justify-content: space-between;
  padding: 8px 12px;
  border-top: 1px solid var(--vscode-panel-border);
  font-size: 0.85em;
  color: var(--vscode-descriptionForeground);
  flex-shrink: 0;
}
.summary-results { display: flex; gap: 12px; }
.summary .pass { color: var(--vscode-testing-iconPassed); }
.summary .fail { color: var(--vscode-testing-iconFailed); }
.summary .skip { color: var(--vscode-testing-iconSkipped); }

/* Filter hiding */
.hide-pass .leaf.pass { display: none; }
.hide-fail .leaf.fail { display: none; }
.hide-skip .leaf.skip { display: none; }
.branch-hidden { display: none; }
.search-hidden { display: none; }
`;

// --- Embedded Script ---

const SCRIPT = `
const vscode = acquireVsCodeApi();

function postRunTests() {
  vscode.postMessage({ type: 'runTests' });
}

function expandAll() {
  document.querySelectorAll('details.branch').forEach(d => d.open = true);
}

function collapseAll() {
  document.querySelectorAll('details.branch').forEach(d => d.open = false);
}

function toggleFilter(status) {
  const tree = document.getElementById('spec-tree');
  if (!tree) return;

  const btn = document.querySelector('.filter-btn.' + status);
  if (!btn) return;

  btn.classList.toggle('active');
  tree.classList.toggle('hide-' + status);

  updateBranchVisibility();
}

function updateBranchVisibility() {
  const branches = document.querySelectorAll('details.branch');
  // Process bottom-up: reverse the NodeList
  const arr = Array.from(branches).reverse();
  for (const branch of arr) {
    const children = branch.querySelectorAll(':scope > .leaf:not([style*="display: none"]):not(.search-hidden), :scope > details.branch:not(.branch-hidden):not(.search-hidden)');
    const visibleChildren = Array.from(children).filter(el => {
      if (el.classList.contains('branch-hidden') || el.classList.contains('search-hidden')) return false;
      if (el.classList.contains('leaf')) {
        return getComputedStyle(el).display !== 'none';
      }
      return true;
    });
    if (visibleChildren.length === 0) {
      branch.classList.add('branch-hidden');
    } else {
      branch.classList.remove('branch-hidden');
    }
  }
}

let searchTimeout = null;
function onSearchInput() {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(applySearch, 200);
}

function applySearch() {
  const input = document.getElementById('search-input');
  if (!input) return;
  const query = input.value.trim().toLowerCase();
  const tree = document.getElementById('spec-tree');
  if (!tree) return;

  // Clear previous search state
  tree.querySelectorAll('.search-hidden').forEach(el => el.classList.remove('search-hidden'));
  tree.querySelectorAll('.branch-hidden').forEach(el => el.classList.remove('branch-hidden'));

  if (!query) {
    // Restore smart defaults: close all, open those with failures
    tree.querySelectorAll('details.branch').forEach(d => {
      d.open = d.querySelector('.leaf.fail') !== null;
    });
    updateBranchVisibility();
    return;
  }

  // Hide non-matching leaves
  tree.querySelectorAll('.leaf').forEach(leaf => {
    const display = (leaf.getAttribute('data-display') || '').toLowerCase();
    if (!display.includes(query)) {
      leaf.classList.add('search-hidden');
    }
  });

  // Hide branches with no visible descendants, expand those with matches
  const branches = Array.from(tree.querySelectorAll('details.branch')).reverse();
  for (const branch of branches) {
    const hasVisible = branch.querySelector('.leaf:not(.search-hidden):not(.branch-hidden), details.branch:not(.search-hidden):not(.branch-hidden)');
    if (!hasVisible) {
      branch.classList.add('search-hidden');
    } else {
      branch.open = true;
    }
  }

  updateBranchVisibility();
}
`;
