import * as vscode from "vscode";
import * as path from "node:path";
import { existsSync } from "node:fs";
import { buildCliCommand, offerCliUpgrade, stripGoRunExitEcho } from "./cli.js";
import { specRenderTimeoutSeconds } from "./config.js";
import { ManagedChild } from "./managedChild.js";
import { readModulePath } from "./gomod.js";
import { JsonStore } from "./jsonStore.js";
import { SpecStore, type RunPiece } from "./specStore.js";
import type { DiscoveryCache } from "./discovery.js";

// The kept runs on disk, so a reload does not forget the packages that ran
// before it.
const RUNS_FILE = "specRuns.json";
// 2: lines are kept per suite instance and in the order they were merged.
const RUNS_VERSION = 2;

// What the panel's node buttons do, given Test Explorer item ids.
export interface SpecViewActions {
  run(ids: string[]): Promise<void>;
  debug(id: string): Promise<void>;
}

export class SpecViewPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  private disposables: vscode.Disposable[] = [];
  private extensionUri: vscode.Uri | undefined;
  private lastSpecData: SpecData | undefined;
  // The runs of this workspace, and each saved stream opened beside them.
  private readonly runs = new SpecStore();
  private readonly imports = new Map<
    string,
    { label: string; runs: SpecStore }
  >();
  // The imported stream on show; undefined shows this workspace's runs.
  private selected: string | undefined;
  private readonly persisted: JsonStore<unknown>;
  private modulePaths: string[] = [];
  // Runs merged since the panel last rendered them.
  private stale = false;
  private rendering: Promise<void> = Promise.resolve();
  private actions: SpecViewActions | undefined;
  // The statuses the toolbar hides, as the CLI's --hide names them.
  private readonly hidden = new Set<string>();

  // patternsFor names the packages a folder's discovery reads; the CLI lays
  // the folder's runs over what their source declares. Without it, or for a
  // folder it has no patterns for, the panel shows what ran.
  constructor(
    private readonly outputChannel: vscode.LogOutputChannel,
    private readonly cache?: DiscoveryCache,
    storageDir?: string,
    private readonly patternsFor?: (
      folder: string,
    ) => Promise<string[] | undefined>,
  ) {
    this.persisted = new JsonStore(storageDir, RUNS_FILE, RUNS_VERSION);
    if (cache) {
      this.disposables.push(
        cache.onDidUpdate(() => {
          this.pruneUndiscovered();
          this.rebuildIfVisible();
        }),
      );
    }
  }

  // pruneUndiscovered forgets kept packages that discovery no longer reports
  // in their folder. A folder discovery has said nothing about yet is left
  // alone.
  private pruneUndiscovered(): void {
    if (!this.cache) return;
    let removed = 0;
    for (const folder of this.runs.folders()) {
      const known = this.cache.packagesForWorkspace(folder);
      if (known.length === 0) continue;
      removed += this.runs.prune(
        folder,
        new Set(known.map((p) => p.importPath)),
      ).length;
    }
    if (removed === 0) return;
    this.persist();
    this.stale = true;
    if (this.panel && this.panel.visible !== false && this.autoRefresh()) {
      void this.render();
    }
  }

  // load restores the runs a previous session kept. A run merged before the
  // file was read wins: the store refuses to load over newer memory.
  async load(): Promise<void> {
    const stored = await this.persisted.read();
    if (!stored) return;
    this.runs.restore(stored);
    this.stale = true;
  }

  flush(): Promise<void> {
    return this.persisted.flush();
  }

  private rebuildIfVisible(): void {
    if (!this.panel || !this.lastSpecData) return;
    this.panel.webview.html = this.buildHtml({
      type: "specData",
      data: this.lastSpecData,
    });
  }

  setExtensionUri(uri: vscode.Uri): void {
    this.extensionUri = uri;
  }

  async show(): Promise<void> {
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside);
      if (this.stale) await this.render();
      return;
    }

    this.createPanel();
    await this.resolveModulePaths();

    if (this.stale) {
      await this.render();
    } else if (this.lastSpecData) {
      this.panel!.webview.html = this.buildHtml({
        type: "specData",
        data: this.lastSpecData,
      });
    } else {
      this.panel!.webview.html = this.buildHtml({ type: "empty" });
    }
  }

  async restorePanel(
    panel: vscode.WebviewPanel,
    state: unknown,
  ): Promise<void> {
    this.panel = panel;
    if (this.extensionUri) {
      this.panel.iconPath = vscode.Uri.joinPath(
        this.extensionUri,
        "static",
        "spec-icon.svg",
      );
    }
    this.wirePanel();
    await this.resolveModulePaths();

    const specData = parseStoredState(state);
    if (specData) {
      this.lastSpecData = specData;
      this.panel.webview.html = this.buildHtml({
        type: "specData",
        data: specData,
      });
    } else if (this.lastSpecData) {
      this.panel.webview.html = this.buildHtml({
        type: "specData",
        data: this.lastSpecData,
      });
    } else {
      this.panel.webview.html = this.buildHtml({ type: "empty" });
    }
  }

  private createPanel(): void {
    const localResourceRoots: vscode.Uri[] = [];
    if (this.extensionUri) {
      localResourceRoots.push(vscode.Uri.joinPath(this.extensionUri, "static"));
    }

    this.panel = vscode.window.createWebviewPanel(
      "gotestSpecView",
      "Go Test: Spec View",
      vscode.ViewColumn.Beside,
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots,
      },
    );

    if (this.extensionUri) {
      this.panel.iconPath = vscode.Uri.joinPath(
        this.extensionUri,
        "static",
        "spec-icon.svg",
      );
    }

    this.wirePanel();
  }

  // setActions connects the panel's Run and Debug to the Test Explorer.
  setActions(actions: SpecViewActions): void {
    this.actions = actions;
  }

  private wirePanel(): void {
    this.panel!.webview.onDidReceiveMessage(
      (msg) => this.handleMessage(msg),
      null,
      this.disposables,
    );

    // A hidden panel is not rendered; it catches up when it is seen again.
    this.panel!.onDidChangeViewState?.(
      (e) => {
        if (e.webviewPanel.visible && this.stale && this.autoRefresh()) {
          void this.render();
        }
      },
      null,
      this.disposables,
    );

    this.panel!.onDidDispose(
      () => {
        this.panel = undefined;
      },
      null,
      this.disposables,
    );
  }

  // The webview posts plain objects; each branch checks what it reads.
  private handleMessage(msg: any): void {
    if (msg.type === "run" && this.actions) {
      const ids: string[] = Array.isArray(msg.ids) ? msg.ids : [msg.id];
      void this.actions.run(ids.filter((id) => typeof id === "string"));
    } else if (msg.type === "debug" && this.actions) {
      if (typeof msg.id === "string") void this.actions.debug(msg.id);
    } else if (msg.type === "selectSource") {
      this.select(typeof msg.id === "string" ? msg.id : undefined);
    } else if (msg.type === "runTests") {
      vscode.commands.executeCommand("testing.runAll");
    } else if (msg.type === "refresh") {
      this.stale = true;
      void this.render();
    } else if (msg.type === "copySpec") {
      void this.copy();
    } else if (msg.type === "toggleHide") {
      if (typeof msg.status !== "string") return;
      if (!this.hidden.delete(msg.status)) this.hidden.add(msg.status);
      this.stale = true;
      void this.render();
    } else if (msg.type === "clearResults") {
      this.clear();
    } else if (msg.type === "goToLocation" && msg.file && msg.line) {
      const uri = vscode.Uri.file(msg.file);
      const line = Math.max(0, msg.line - 1);
      const specColumn = this.panel?.viewColumn;
      const other = vscode.window.visibleTextEditors.find(
        (e) => e.viewColumn !== undefined && e.viewColumn !== specColumn,
      );
      const viewColumn =
        other?.viewColumn ??
        (specColumn === vscode.ViewColumn.One
          ? vscode.ViewColumn.Beside
          : vscode.ViewColumn.One);
      vscode.window
        .showTextDocument(uri, {
          viewColumn,
          selection: new vscode.Range(line, 0, line, 0),
        })
        .then(undefined, (err: unknown) => {
          this.outputChannel.error(
            `[specView] showTextDocument failed: ${err}`,
          );
        });
    }
  }

  // clear drops the kept runs and empties the panel. Called both by the
  // panel's own Clear button and when the Test Explorer's results are
  // cleared, so the two surfaces cannot disagree about what has been run.
  clear(): void {
    this.lastSpecData = undefined;
    this.runs.clear();
    this.imports.clear();
    this.selected = undefined;
    this.stale = false;
    this.persist();
    if (this.panel) {
      this.panel.webview.html = this.buildHtml({ type: "empty" });
    }
  }

  // refresh folds finished runs into what the panel keeps and renders the
  // result: every package seen, each test with its newest verdict.
  async refresh(pieces: RunPiece | RunPiece[]): Promise<void> {
    for (const piece of Array.isArray(pieces) ? pieces : [pieces]) {
      this.runs.merge(piece);
    }
    this.persist();
    // The newest change is what the panel shows.
    this.selected = undefined;
    this.stale = true;
    if (!this.panel || this.panel.visible === false || !this.autoRefresh()) {
      return;
    }
    await this.render();
  }

  // openStream shows a saved event stream as a source of its own, beside
  // the workspace's runs and never merged into them: it is evidence about
  // another machine. Opening the same file again replaces it.
  async openStream(id: string, label: string, piece: RunPiece): Promise<void> {
    const runs = new SpecStore();
    runs.merge(piece);
    this.imports.set(id, { label, runs });
    this.selected = id;
    this.stale = true;
    await this.show();
  }

  private select(id: string | undefined): void {
    this.selected = id && this.imports.has(id) ? id : undefined;
    this.stale = true;
    void this.render();
  }

  private currentRuns(): SpecStore {
    return (
      (this.selected && this.imports.get(this.selected)?.runs) || this.runs
    );
  }

  // sourcesForToolbar lists what the panel can show, once there is a choice.
  private sourcesForToolbar(): SourceOption[] {
    if (this.imports.size === 0) return [];
    return [
      { id: "", label: "Workspace runs", selected: !this.selected },
      ...Array.from(this.imports, ([id, imp]) => ({
        id,
        label: `Imported: ${imp.label}`,
        selected: id === this.selected,
      })),
    ];
  }

  private autoRefresh(): boolean {
    return (
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("specView.autoRefresh") ?? true
    );
  }

  private persist(): void {
    this.persisted.markMutated();
    this.persisted.save(() => this.runs.serialize());
  }

  // render draws the kept runs. Renders queue behind each other, and one
  // queued behind a render that already showed the newest runs does nothing.
  private render(): Promise<void> {
    const next = this.rendering.then(() => this.renderNow());
    this.rendering = next.catch(() => {});
    return next;
  }

  private async renderNow(): Promise<void> {
    if (!this.stale) return;
    this.stale = false;
    try {
      const data = await this.renderRuns();
      this.lastSpecData = data;
      await this.resolveModulePaths();
      if (!this.panel) return;
      this.panel.webview.html = this.buildHtml({ type: "specData", data });
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[specView] ${message}`);
    }
  }

  // renderRuns has each folder's CLI render the packages that folder ran.
  private async renderRuns(): Promise<SpecData> {
    const runs = this.currentRuns();
    const docs: SpecData[] = [];
    for (const folder of runs.folders()) {
      const raw = await this.runSpec(runs.compose(folder), folder, "json");
      docs.push(JSON.parse(raw) as SpecData);
    }
    return combineSpecData(docs);
  }

  // copy puts on the clipboard what the panel shows, as the CLI writes it in
  // Markdown: the same streams, rendered with the same flags.
  private async copy(): Promise<void> {
    const runs = this.currentRuns();
    const parts: string[] = [];
    try {
      for (const folder of runs.folders()) {
        parts.push(await this.runSpec(runs.compose(folder), folder, "md"));
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[specView] copy: ${message}`);
      return;
    }
    if (parts.length > 0)
      await vscode.env.clipboard.writeText(parts.join("\n"));
  }

  // specArgs are the CLI's flags for a folder's render: the declared
  // specification under the workspace's runs, never under an imported
  // stream, and the statuses the toolbar hides. --hide is always given, so
  // every package carries the CLI's counts.
  private async specArgs(folder: string, format: string): Promise<string[]> {
    const args = [
      "spec",
      "--input=-",
      `--format=${format}`,
      // A renderer, not a CI gate: without it the exit code also reports
      // whether the tests passed, which says nothing about the rendering.
      "--render-only",
      `--hide=${Array.from(this.hidden).sort().join(",")}`,
    ];
    const patterns = this.selected
      ? undefined
      : await this.patternsFor?.(folder);
    if (patterns && patterns.length > 0) args.push("--declared", ...patterns);
    return args;
  }

  private async resolveModulePaths(): Promise<void> {
    const moduleSet = new Set<string>();
    if (this.cache) {
      const goModCache = new Map<string, string | undefined>();
      for (const pkg of this.cache.packages) {
        const mod = await this.findModuleForDir(pkg.dir, goModCache);
        if (mod) moduleSet.add(mod);
      }
    }
    this.modulePaths = Array.from(moduleSet);
  }

  private async findModuleForDir(
    startDir: string,
    cache: Map<string, string | undefined>,
  ): Promise<string | undefined> {
    let dir = startDir;
    const visited: string[] = [];
    while (true) {
      if (cache.has(dir)) {
        const hit = cache.get(dir);
        for (const v of visited) cache.set(v, hit);
        return hit;
      }
      visited.push(dir);
      const mod = await readModulePath(dir);
      if (mod) {
        for (const v of visited) cache.set(v, mod);
        return mod;
      }
      const parent = path.dirname(dir);
      if (parent === dir) break;
      dir = parent;
    }
    for (const v of visited) cache.set(v, undefined);
    return undefined;
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
      .asWebviewUri(
        vscode.Uri.joinPath(this.extensionUri, "static", "icon.png"),
      )
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
        : buildSpecBody(
            msg.data,
            this.modulePaths,
            { dirOf: (pkgPath) => this.cache?.getPackage(pkgPath)?.dir },
            this.sourcesForToolbar(),
            this.hidden,
          );
    const stateJson =
      msg.type === "specData" ? encodeStateForScript(msg.data) : "null";

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
const SPEC_STATE = ${stateJson};
${SCRIPT}
</script>
</body>
</html>`;
  }

  // runSpec has the folder's CLI render a composed stream in format.
  private async runSpec(
    jsonInput: string,
    folder: string | undefined,
    format: string,
  ): Promise<string> {
    const workspaceDir =
      folder ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    let cmd;
    try {
      cmd = await buildCliCommand(
        await this.specArgs(workspaceDir ?? "", format),
        workspaceDir,
        this.outputChannel,
      );
    } catch (err: unknown) {
      offerCliUpgrade(err);
      throw err;
    }
    const budgetSeconds = specRenderTimeoutSeconds(workspaceDir);
    const mc = new ManagedChild(cmd.bin, cmd.args, {
      cwd: workspaceDir,
      kind: "spec",
    });
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let spawnError: Error | undefined;

    // Decoded on the stream by ManagedChild: a spec tree is large enough to
    // arrive in several reads, and a boundary inside a multi-byte character
    // would corrupt the behavior name it lands in.
    mc.child.stdout?.on("data", (chunk: string) => {
      stdout += chunk;
    });
    mc.child.stderr?.on("data", (chunk: string) => {
      stderr += chunk;
    });
    mc.child.on("error", (err: Error) => {
      spawnError = err;
    });

    // Rendering is a read: it has no fixtures to tear down, so it gets a budget
    // and the prompt escalation. Without them a wedged CLI left this promise
    // pending forever and nothing could end the child.
    const budget = setTimeout(() => {
      timedOut = true;
      void mc.terminate("prompt");
    }, budgetSeconds * 1000);

    mc.child.stdin?.on("error", () => {});
    mc.child.stdin?.write(jsonInput);
    mc.child.stdin?.end();

    try {
      const { code } = await mc.exited;
      if (spawnError) throw spawnError;
      if (timedOut) {
        throw new Error(`spec render timed out after ${budgetSeconds}s`);
      }
      const outcome = interpretSpecExit(code, stdout, stderr);
      if (!outcome.ok) throw new Error(outcome.message);
      return outcome.stdout;
    } finally {
      clearTimeout(budget);
      mc.dispose();
    }
  }
}

// encodeStateForScript serialises the spec for embedding inside a <script>
// block. JSON.stringify leaves "<" alone, so a test whose output contains
// "</script>" would close the block early and spill the rest of the document
// into the page as live markup. Escaping the angle brackets as unicode
// escapes leaves the parsed value identical while making that impossible.
export function encodeStateForScript(data: unknown): string {
  return JSON.stringify(data)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026");
}

export type SpecExitOutcome =
  { ok: true; stdout: string } | { ok: false; message: string };

// interpretSpecExit decides whether a finished `gotest spec --input=-` left a
// usable spec behind. Exit 1 means the piped stream carried failures — a
// verdict about the tests, not a rendering error — so the spec on stdout still
// stands. Only an exit that produced no spec is an error worth reporting.
export function interpretSpecExit(
  code: number | null,
  stdout: string,
  stderr: string,
): SpecExitOutcome {
  if (code === 0 || (code === 1 && stdout.trim() !== "")) {
    return { ok: true, stdout };
  }
  const detail = stripGoRunExitEcho(stderr);
  const suffix = detail ? `: ${detail}` : "";
  return {
    ok: false,
    message: `gotest spec exited with code ${code}${suffix}`,
  };
}

// combineSpecData joins the documents of several folders' CLIs. Each package
// is rendered by exactly one of them and the CLI's counts are sums over
// packages, so the joined counts are sums too.
export function combineSpecData(docs: SpecData[]): SpecData {
  if (docs.length === 1) return docs[0];
  const stats: SpecStats = {
    suites: 0,
    behaviors: 0,
    tests: 0,
    fuzzers: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    failedPackages: 0,
    notRun: 0,
  };
  const packages: SpecPackage[] = [];
  for (const doc of docs) {
    packages.push(...doc.packages);
    for (const key of Object.keys(stats) as (keyof SpecStats)[]) {
      stats[key] = (stats[key] ?? 0) + (doc.stats[key] ?? 0);
    }
  }
  packages.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  return { version: docs[0]?.version, packages, stats };
}

// --- Types ---

export interface SpecData {
  /** The CLI that produced the document; absent before v1.30.2. */
  version?: string;
  packages: SpecPackage[];
  stats: SpecStats;
}

export interface SpecPackage {
  path: string;
  // The package's directory, when the CLI read its source (--declared).
  dir?: string;
  status: string;
  duration: number;
  nodes: SpecNode[];
  output?: string[];
  // The package's own counts; given with --hide or --declared.
  stats?: SpecStats;
  // A package the run booked for itself (shared fixtures, an interrupt).
  runLevel?: boolean;
}

export interface SpecNode {
  name: string;
  display: string;
  kind: string;
  // "none" for a declared node the run did not report.
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  external: boolean;
  incomplete?: boolean;
  // Where the source declares the node, relative to its package's dir.
  file?: string;
  line?: number;
  output: string[];
  children: SpecNode[];
}

export interface SpecStats {
  suites: number;
  behaviors: number;
  tests: number;
  // Fuzz targets: each is one property under its suite, counted once.
  fuzzers?: number;
  passed: number;
  failed: number;
  skipped: number;
  // Packages whose verdict sits on the package itself — a build failure, a
  // death outside any test. They carry no failing behavior, so they are
  // counted separately from `failed`.
  failedPackages?: number;
  // Declared behaviors the run did not report (--declared).
  notRun?: number;
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
  <button class="empty-button" id="run-tests-btn">Run Tests</button>
  <div class="empty-legend">
    <span class="legend-item pass">✓ passed</span>
    <span class="legend-item fail">✗ failed</span>
    <span class="legend-item skip">~ skipped</span>
    <span class="legend-item none">? not run</span>
  </div>
</div>`;
}

// What the tree is rendered against: package directories from discovery,
// for the file:line tokens of output a stream without positions holds.
interface RenderEnv {
  dirOf: (pkgPath: string) => string | undefined;
}

interface SourceOption {
  id: string;
  label: string;
  selected: boolean;
}

export function buildSpecBody(
  data: SpecData,
  modulePaths: string[],
  env: RenderEnv,
  sources: SourceOption[] = [],
  hidden: ReadonlySet<string> = new Set(),
): string {
  const toolbar = buildToolbar(
    data.stats.failed > 0 || (data.stats.failedPackages ?? 0) > 0,
    sources,
    hidden,
  );
  const groups = groupByModule(data.packages, modulePaths);
  let tree: string;
  if (groups.length === 1 && groups[0].entries.length === 1) {
    const pkg = groups[0].entries[0].pkg;
    tree =
      buildPackageFailureHtml(pkg, env) +
      pkg.nodes.map((n) => buildNodeHtml(n, "", nodeCtx(env, pkg))).join("");
  } else if (groups.length === 1) {
    tree = groups[0].entries
      .map((e) => buildPackageSectionHtml(e.displayPath, e.pkg, env))
      .join("");
  } else {
    tree = groups.map((g) => buildModuleSectionHtml(g, env)).join("");
  }
  const summary = buildSummary(data.stats);
  return `<div class="spec-container">
  ${toolbar}
  <div class="spec-tree" id="spec-tree">${tree}</div>
  ${summary}
</div>`;
}

// The toolbar's status toggles, each named as the CLI's trailer words it.
const STATUS_TOGGLES = [
  { status: "pass", glyph: "✓", word: "passed" },
  { status: "fail", glyph: "✗", word: "failed" },
  { status: "skip", glyph: "~", word: "skipped" },
  { status: "none", glyph: "?", word: "not run" },
];

function buildToolbar(
  anyFailed: boolean,
  sources: SourceOption[],
  hidden: ReadonlySet<string>,
): string {
  const rerun = anyFailed ? "" : " disabled";
  const picker =
    sources.length === 0
      ? ""
      : `<div class="toolbar-group"><select class="source-select" id="source-select" title="Which runs to show">${sources
          .map(
            (o) =>
              `<option value="${escapeAttr(o.id)}"${o.selected ? " selected" : ""}>${escapeHtml(o.label)}</option>`,
          )
          .join("")}</select></div>`;
  const toggles = STATUS_TOGGLES.map(
    (t) =>
      `<button class="filter-btn ${t.status}${hidden.has(t.status) ? "" : " active"}" data-filter="${t.status}" title="Show or hide ${t.word}">${t.glyph}</button>`,
  ).join("\n    ");
  return `<div class="toolbar">
  ${picker}
  <div class="toolbar-group">
    <button class="tool-btn" id="expand-all-btn" title="Expand all">▼ All</button>
    <button class="tool-btn" id="collapse-all-btn" title="Collapse all">▲ None</button>
  </div>
  <div class="toolbar-group">
    ${toggles}
  </div>
  <div class="toolbar-group search-group">
    <input type="text" class="search-input" id="search-input" placeholder="Search behaviors..." />
  </div>
  <div class="toolbar-group">
    <button class="tool-btn" id="rerun-failed-btn" title="Run what failed again"${rerun}>Rerun failed</button>
    <button class="tool-btn" id="refresh-btn" title="Render the newest runs">Refresh</button>
    <button class="tool-btn" id="copy-spec-btn" title="Copy the spec as Markdown">Copy</button>
    <button class="tool-btn" id="clear-results-btn" title="Clear results">Clear</button>
  </div>
</div>`;
}

interface ModuleGroup {
  modulePath: string;
  entries: Array<{ pkg: SpecPackage; displayPath: string }>;
}

function groupByModule(
  packages: SpecPackage[],
  modulePaths: string[],
): ModuleGroup[] {
  const sorted = [...modulePaths].sort((a, b) => b.length - a.length);
  const groups = new Map<string, ModuleGroup>();
  for (const pkg of packages) {
    const mod =
      sorted.find((m) => pkg.path === m || pkg.path.startsWith(m + "/")) ?? "";
    let group = groups.get(mod);
    if (!group) {
      group = { modulePath: mod, entries: [] };
      groups.set(mod, group);
    }
    const displayPath =
      mod && pkg.path.length > mod.length
        ? pkg.path.slice(mod.length + 1)
        : pkg.path;
    group.entries.push({ pkg, displayPath });
  }
  return Array.from(groups.values());
}

// statsText words a package's or a module's counts as the CLI's trailer
// does; empty when the CLI gave none.
export function statsText(stats: SpecStats | undefined): string {
  if (!stats) return "";
  const parts: string[] = [];
  if (stats.passed > 0) parts.push(`${stats.passed} passed`);
  if (stats.failed > 0) parts.push(`${stats.failed} failed`);
  if (stats.skipped > 0) parts.push(`${stats.skipped} skipped`);
  if ((stats.notRun ?? 0) > 0) parts.push(`${stats.notRun} not run`);
  const failedPackages = stats.failedPackages ?? 0;
  if (failedPackages > 0)
    parts.push(countNoun(failedPackages, "failed package"));
  return parts.join(", ");
}

// addStats adds the CLI's counts of several packages, for a module's row.
export function addStats(
  all: (SpecStats | undefined)[],
): SpecStats | undefined {
  const given = all.filter((s): s is SpecStats => s !== undefined);
  if (given.length === 0) return undefined;
  const sum: SpecStats = {
    suites: 0,
    behaviors: 0,
    tests: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    notRun: 0,
    failedPackages: 0,
  };
  for (const s of given) {
    sum.suites += s.suites;
    sum.behaviors += s.behaviors;
    sum.tests += s.tests;
    sum.passed += s.passed;
    sum.failed += s.failed;
    sum.skipped += s.skipped;
    sum.notRun = (sum.notRun ?? 0) + (s.notRun ?? 0);
    sum.failedPackages = (sum.failedPackages ?? 0) + (s.failedPackages ?? 0);
  }
  return sum;
}

function statsHtml(stats: SpecStats | undefined): string {
  const text = statsText(stats);
  return text ? ` <span class="row-stats">${escapeHtml(text)}</span>` : "";
}

// A package whose rows the CLI left out, and that says nothing of its own,
// has no row; its counts still reach its module's.
function buildPackageSectionHtml(
  displayPath: string,
  pkg: SpecPackage,
  env: RenderEnv,
): string {
  const failure = buildPackageFailureHtml(pkg, env);
  if (pkg.nodes.length === 0 && !failure) return "";
  const nodes = pkg.nodes
    .map((n) => buildNodeHtml(n, "", nodeCtx(env, pkg)))
    .join("");
  return `<details class="pkg-section" open>
  <summary class="pkg-header">${escapeHtml(displayPath)}${statsHtml(pkg.stats)}</summary>
  ${failure}${nodes}
</details>`;
}

// buildPackageFailureHtml renders a verdict the CLI counts on the package
// itself (a build failure, a death outside any test) with the output the
// CLI gives it; under --hide=fail the CLI gives none and the block goes.
function buildPackageFailureHtml(pkg: SpecPackage, env: RenderEnv): string {
  if ((pkg.stats?.failedPackages ?? 0) === 0) return "";
  const dir = pkg.dir ?? env.dirOf(pkg.path);
  const lines = (pkg.output ?? [])
    .map((l) => l.trimEnd())
    .filter(Boolean)
    .map((l) => linkSources(escapeHtml(l), dir))
    .join("\n");
  if (!lines) return "";
  const errorBlock = `<details class="error-details" open><summary class="error-summary">Error details</summary><pre class="error-output">${lines}</pre></details>`;
  return `<div class="leaf test fail"${idAttr(pkg.path)}>
  <span class="icon fail"><span class="status-text">✗</span></span> <strong>FAIL</strong>
  ${errorBlock}
</div>`;
}

// linkSources turns the file:line tokens of an escaped output line into
// links to the source, when the file is there to open. go test names test
// files relative to the package, the compiler relative to where it ran.
export function linkSources(escaped: string, dir: string | undefined): string {
  return escaped.replace(
    /((?:[A-Za-z]:)?[\w.\-~/\\]+\.go):(\d+)(?::\d+)?/g,
    (token: string, file: string, line: string) => {
      const resolved = resolveSource(file, dir);
      if (!resolved) return token;
      return `<a class="src-link" data-loc-file="${escapeAttr(resolved)}" data-loc-line="${line}">${token}</a>`;
    },
  );
}

function resolveSource(
  file: string,
  dir: string | undefined,
): string | undefined {
  const candidates = path.isAbsolute(file)
    ? [file]
    : dir
      ? [path.join(dir, file), path.join(dir, path.basename(file))]
      : [];
  return candidates.find((c) => existsSync(c));
}

// A module counts every package the CLI gave, rows shown or not; one with
// no row to show has none itself.
function buildModuleSectionHtml(group: ModuleGroup, env: RenderEnv): string {
  const only = group.entries[0].pkg;
  const inner =
    group.entries.length === 1
      ? buildPackageFailureHtml(only, env) +
        only.nodes.map((n) => buildNodeHtml(n, "", nodeCtx(env, only))).join("")
      : group.entries
          .map((e) => buildPackageSectionHtml(e.displayPath, e.pkg, env))
          .join("");
  if (!inner) return "";
  const stats = addStats(group.entries.map((e) => e.pkg.stats));
  return `<details class="module-section" open>
  <summary class="module-header">${escapeHtml(group.modulePath || "unknown")}${statsHtml(stats)}</summary>
  ${inner}
</details>`;
}

// What a node needs from around it: the package it belongs to, which with
// its path names its Test Explorer item, and the package's directory, where
// the source the CLI names lives.
interface NodeCtx {
  pkgPath: string;
  // The directory the CLI read the package's source from; a node's file is
  // relative to it.
  declDir: string | undefined;
  // Where the file:line tokens of output resolve: the CLI's directory, else
  // discovery's.
  outputDir: string | undefined;
}

function nodeCtx(env: RenderEnv, pkg: SpecPackage): NodeCtx {
  return {
    pkgPath: pkg.path,
    declDir: pkg.dir,
    outputDir: pkg.dir ?? env.dirOf(pkg.path),
  };
}

// itemIdOf is the Test Explorer id of a node: the package, then the go test
// path with the top-level test's Test prefix dropped, as discovery names
// suites. A fuzz target sits under its suite in both trees.
export function itemIdOf(
  pkgPath: string,
  nodePath: string,
): string | undefined {
  if (!nodePath.startsWith("Test") || nodePath.startsWith("Test_")) {
    return undefined;
  }
  return `${pkgPath}/${nodePath.slice(4)}`;
}

// nearestItemId is id itself when the tree has it, else its closest
// ancestor that it has: behaviors past the tree's per-method cap are not
// items, and running one runs its method.
export function nearestItemId(
  id: string,
  has: (id: string) => boolean,
): string | undefined {
  let current = id;
  for (;;) {
    if (has(current)) return current;
    const cut = current.lastIndexOf("/");
    if (cut <= 0) return undefined;
    current = current.slice(0, cut);
  }
}

function actionsHtml(id: string | undefined): string {
  if (!id) return "";
  return `<span class="node-actions"><button class="act" data-act="run" title="Run">Run</button><button class="act" data-act="debug" title="Debug">Debug</button></span>`;
}

function idAttr(id: string | undefined): string {
  return id ? ` data-id="${escapeAttr(id)}"` : "";
}

// Nodes are rendered with their go test path, the parent's path and their
// own name, which names their Test Explorer item.
function buildNodeHtml(
  node: SpecNode,
  parentPath: string,
  ctx: NodeCtx,
): string {
  const nodePath = parentPath ? `${parentPath}/${node.name}` : node.name;
  if (node.children.length === 0) {
    return buildLeafHtml(node, nodePath, ctx);
  }
  return buildBranchHtml(node, nodePath, ctx);
}

// sourceOf is where the CLI says the source declares a node; a node only a
// run revealed has no position.
export function sourceOf(
  node: SpecNode,
  dir: string | undefined,
): { file: string; line: number } | undefined {
  if (!dir || !node.file || !node.line) return undefined;
  return { file: path.join(dir, node.file), line: node.line };
}

function buildIconHtml(node: SpecNode, ctx: NodeCtx): string {
  const icon = statusIcon(node.status);
  const loc = sourceOf(node, ctx.declDir);
  const locAttrs = loc
    ? ` data-loc-file="${escapeAttr(loc.file)}" data-loc-line="${loc.line}"`
    : "";
  const gotoSpan = loc
    ? `<span class="goto-text" title="Go to source">↗</span>`
    : "";
  return `<span class="icon ${node.status}"${locAttrs}><span class="status-text">${icon}</span>${gotoSpan}</span>`;
}

function buildLeafHtml(node: SpecNode, nodePath: string, ctx: NodeCtx): string {
  const iconHtml = buildIconHtml(node, ctx);
  const id =
    node.kind === "fixture" ? undefined : itemIdOf(ctx.pkgPath, nodePath);
  let suffix = fuzzTagHtml(node);
  if (node.external) suffix += ` <span class="tag external">EXT.</span>`;
  if (node.focused) suffix += ` <span class="tag focused">FOCUSED</span>`;
  else if (node.excluded || node.status === "skip") suffix += " — SKIPPED";
  // A declared behavior the run did not report took no time.
  const dur =
    node.status === "none"
      ? ` <span class="tag none">NOT RUN</span>`
      : ` <span class="dur">(${formatDuration(node.duration)})</span>`;

  const errorBlock =
    node.status === "fail" ? errorBlockHtml(node, ctx.outputDir) : "";

  return `<div class="leaf ${node.kind} ${node.status}" data-display="${escapeAttr(node.display)}"${idAttr(id)}>
  ${iconHtml} ${escapeHtml(node.display)}${suffix}${dur}${actionsHtml(id)}
  ${errorBlock}
</div>`;
}

function buildBranchHtml(
  node: SpecNode,
  nodePath: string,
  ctx: NodeCtx,
): string {
  const openAttr = node.status === "fail" ? " open" : "";
  const iconHtml = buildIconHtml(node, ctx);
  const id =
    node.kind === "fixture" ? undefined : itemIdOf(ctx.pkgPath, nodePath);
  const children = node.children
    .map((c) => buildNodeHtml(c, nodePath, ctx))
    .join("");

  let label = escapeHtml(node.display);
  if (
    node.kind === "suite" ||
    node.kind === "fixture" ||
    node.kind === "method" ||
    node.kind === "test" ||
    node.kind === "fuzz"
  ) {
    label = `<strong>${label}</strong>`;
  }

  let suffix = fuzzTagHtml(node);
  if (node.external) suffix += ` <span class="tag external">EXT.</span>`;
  if (node.focused) suffix += ` <span class="tag focused">FOCUSED</span>`;
  else if (node.excluded) suffix += ` <span class="tag skipped">SKIPPED</span>`;

  // What the branch itself said when it failed: a failed AfterAll, a panic
  // booked on the suite rather than on the behavior that raised it.
  const errorBlock =
    node.status === "fail" ? errorBlockHtml(node, ctx.outputDir) : "";

  return `<details class="branch ${node.status}" data-display="${escapeAttr(node.display)}"${openAttr}>
  <summary class="node ${node.kind}"${idAttr(id)}>${iconHtml} ${label}${suffix}${actionsHtml(id)}</summary>
  ${errorBlock}${children}
</details>`;
}

// ownOutput is what a node printed, without go test's RUN/PASS/FAIL frames.
function ownOutput(node: SpecNode): string[] {
  return node.output
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("=== ") && !l.startsWith("--- "));
}

function errorBlockHtml(node: SpecNode, dir: string | undefined): string {
  const lines = ownOutput(node)
    .map((l) => linkSources(escapeHtml(l), dir))
    .join("\n");
  if (!lines) return "";
  return `<details class="error-details"><summary class="error-summary">Error details</summary><pre class="error-output">${lines}</pre></details>`;
}

function statusIcon(status: string): string {
  switch (status) {
    case "pass":
      return "✓";
    case "fail":
      return "✗";
    case "skip":
      return "~";
    default:
      return "?";
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
  if (stats.suites > 0) counts.push(countNoun(stats.suites, "suite"));
  if (stats.behaviors > 0) counts.push(countNoun(stats.behaviors, "behavior"));
  if ((stats.fuzzers ?? 0) > 0)
    counts.push(countNoun(stats.fuzzers ?? 0, "fuzz target"));
  if (stats.tests > 0) counts.push(`${stats.tests} stdlib tests`);

  const failedPackages = stats.failedPackages ?? 0;
  const notRun = stats.notRun ?? 0;
  return `<div class="summary">
  <span class="summary-counts">${counts.join(", ")}</span>
  <span class="summary-results">
    ${stats.passed > 0 ? `<span class="pass">${stats.passed} passed</span>` : ""}
    ${stats.failed > 0 ? `<span class="fail">${stats.failed} failed</span>` : ""}
    ${stats.skipped > 0 ? `<span class="skip">${stats.skipped} skipped</span>` : ""}
    ${notRun > 0 ? `<span class="none">${notRun} not run</span>` : ""}
    ${failedPackages > 0 ? `<span class="fail">${countNoun(failedPackages, "failed package")}</span>` : ""}
  </span>
</div>`;
}

/** Spells a count with its noun, singular for exactly one, as the CLI trailer does. */
export function countNoun(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`;
}

// A fuzz target is a property backed by seeds, not an example; the marker
// says so and how much evidence sits beneath it, as the CLI's own view does.
function seedCount(node: SpecNode): number {
  return node.children.length;
}

function fuzzTagHtml(node: SpecNode): string {
  if (node.kind !== "fuzz") return "";
  const n = seedCount(node);
  return ` <span class="tag fuzz">FUZZ · ${n} seed${n === 1 ? "" : "s"}</span>`;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/"/g, "&quot;");
}

function parseStoredState(state: unknown): SpecData | undefined {
  if (
    state &&
    typeof state === "object" &&
    "packages" in (state as Record<string, unknown>) &&
    "stats" in (state as Record<string, unknown>)
  ) {
    return state as SpecData;
  }
  return undefined;
}

function getNonce(): string {
  const chars =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
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
.legend-item.none { color: var(--vscode-descriptionForeground); }

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
.filter-btn.none { color: var(--vscode-descriptionForeground); }
.filter-btn.none.active { border-color: var(--vscode-descriptionForeground); }
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
/* Module sections */
details.module-section { margin-top: 16px; }
details.module-section:first-child { margin-top: 0; }
details.module-section > summary.module-header {
  cursor: pointer;
  list-style: none;
  color: var(--vscode-editor-foreground);
  font-weight: 600;
  font-size: 0.9em;
  padding: 2px 0;
}
details.module-section > summary.module-header::-webkit-details-marker { display: none; }
details.module-section > details.pkg-section,
details.module-section > details.branch,
details.module-section > .leaf { margin-left: 12px; }

/* Package sections */
details.pkg-section { margin-top: 12px; }
details.pkg-section:first-child { margin-top: 0; }
details.pkg-section > summary.pkg-header {
  cursor: pointer;
  list-style: none;
  color: var(--vscode-descriptionForeground);
  font-size: 0.9em;
  padding: 2px 0;
}
details.pkg-section > summary.pkg-header::-webkit-details-marker { display: none; }
details.pkg-section > details.branch,
details.pkg-section > .leaf { margin-left: 12px; }

/* Nodes — nesting via margin */
details.branch { list-style: none; }
details.branch > details.branch,
details.branch > .leaf { margin-left: 20px; }
details.branch > summary { cursor: pointer; list-style: none; }
details.branch > summary::-webkit-details-marker { display: none; }
summary.node { padding: 1px 0; }
summary.node.block { color: var(--vscode-terminal-ansiYellow); }

.leaf { padding: 1px 0; white-space: nowrap; }
.leaf.block { font-style: italic; }

.icon.pass { color: var(--vscode-testing-iconPassed); }
.icon.fail { color: var(--vscode-testing-iconFailed); }
.icon.skip { color: var(--vscode-testing-iconSkipped); }
.icon.none { opacity: 0.4; }

.dur { color: var(--vscode-descriptionForeground); font-size: 0.85em; }

.tag { font-size: 0.8em; margin-left: 6px; }
.tag.focused { color: var(--vscode-testing-iconSkipped); }
.tag.skipped { color: var(--vscode-testing-iconSkipped); }
.tag.external { color: var(--vscode-descriptionForeground); }
.tag.fuzz { color: var(--vscode-textLink-foreground); }
.tag.none { color: var(--vscode-descriptionForeground); }
.leaf.none { color: var(--vscode-descriptionForeground); }
.row-stats { color: var(--vscode-descriptionForeground); font-size: 0.85em; font-weight: normal; margin-left: 8px; }

/* Icon hover swap: status icon → go-to-source */
.icon .goto-text { display: none; }
.icon[data-loc-file] { cursor: pointer; }
summary.node:hover > .icon[data-loc-file] > .status-text,
.leaf:hover > .icon[data-loc-file] > .status-text { display: none; }
summary.node:hover > .icon[data-loc-file] > .goto-text,
.leaf:hover > .icon[data-loc-file] > .goto-text { display: inline; color: var(--vscode-textLink-foreground); }

.source-select {
  background: var(--vscode-dropdown-background);
  color: var(--vscode-dropdown-foreground);
  border: 1px solid var(--vscode-dropdown-border);
  font-size: 0.85em;
}
.src-link { color: var(--vscode-textLink-foreground); cursor: pointer; text-decoration: underline; }

/* Node actions, shown on hover */
.node-actions { display: none; margin-left: 8px; }
summary.node:hover > .node-actions,
.leaf:hover > .node-actions { display: inline; }
.act {
  background: transparent;
  color: var(--vscode-textLink-foreground);
  border: none;
  padding: 0 4px;
  cursor: pointer;
  font-size: 0.8em;
}
.act:hover { text-decoration: underline; }
.tool-btn:disabled { opacity: 0.5; cursor: default; }

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
.summary .none { color: var(--vscode-descriptionForeground); }

/* Filter hiding */
.search-hidden { display: none; }
`;

// --- Embedded Script ---

const SCRIPT = `
const vscode = acquireVsCodeApi();
if (SPEC_STATE) vscode.setState(SPEC_STATE);

document.addEventListener('click', (e) => {
  const t = e.target;
  if (!t || !(t instanceof HTMLElement)) return;

  if (t.id === 'run-tests-btn') {
    vscode.postMessage({ type: 'runTests' });
    return;
  }
  const act = t.closest('.act');
  if (act) {
    const owner = act.closest('[data-id]');
    if (owner) vscode.postMessage({ type: act.dataset.act, id: owner.dataset.id });
    e.preventDefault();
    e.stopPropagation();
    return;
  }
  if (t.id === 'rerun-failed-btn') {
    // The narrowest failed rows: one with a failed row beneath it runs
    // with that row.
    const ids = Array.from(document.querySelectorAll('.leaf.fail, details.branch.fail'))
      .filter(el => !el.querySelector('.leaf.fail, details.branch.fail'))
      .map(el => el.matches('details') ? el.querySelector(':scope > summary') : el)
      .map(el => el && el.dataset.id)
      .filter(id => id);
    if (ids.length > 0) vscode.postMessage({ type: 'run', ids: ids });
    return;
  }
  const iconEl = t.closest('.icon[data-loc-file], .src-link');
  if (iconEl) {
    const file = iconEl.dataset.locFile;
    const line = Number(iconEl.dataset.locLine);
    if (file && line) {
      vscode.postMessage({ type: 'goToLocation', file: file, line: line });
      e.preventDefault();
      e.stopPropagation();
      return;
    }
  }
  if (t.id === 'copy-spec-btn') {
    vscode.postMessage({ type: 'copySpec' });
    return;
  }
  if (t.id === 'refresh-btn') {
    vscode.postMessage({ type: 'refresh' });
    return;
  }
  if (t.id === 'clear-results-btn') {
    vscode.setState(null);
    vscode.postMessage({ type: 'clearResults' });
    return;
  }
  if (t.id === 'expand-all-btn') {
    document.querySelectorAll('details.branch, details.pkg-section, details.module-section').forEach(d => d.open = true);
    return;
  }
  if (t.id === 'collapse-all-btn') {
    document.querySelectorAll('details.branch, details.pkg-section, details.module-section').forEach(d => d.open = false);
    return;
  }
  // The CLI leaves the hidden rows out; the panel renders again.
  if (t.dataset.filter) {
    vscode.postMessage({ type: 'toggleHide', status: t.dataset.filter });
    return;
  }
});

const sourceSelect = document.getElementById('source-select');
if (sourceSelect) {
  sourceSelect.addEventListener('change', () => {
    vscode.postMessage({ type: 'selectSource', id: sourceSelect.value });
  });
}

let searchTimeout = null;
const searchInput = document.getElementById('search-input');
if (searchInput) {
  searchInput.addEventListener('input', () => {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(applySearch, 200);
  });
}

function applySearch() {
  const input = document.getElementById('search-input');
  if (!input) return;
  const query = input.value.trim().toLowerCase();
  const tree = document.getElementById('spec-tree');
  if (!tree) return;

  tree.querySelectorAll('.search-hidden').forEach(el => el.classList.remove('search-hidden'));

  if (!query) {
    tree.querySelectorAll('details.branch').forEach(d => {
      d.open = d.classList.contains('fail');
    });
    return;
  }

  tree.querySelectorAll('.leaf').forEach(leaf => {
    const display = (leaf.getAttribute('data-display') || '').toLowerCase();
    if (!display.includes(query)) {
      leaf.classList.add('search-hidden');
    }
  });

  const branches = Array.from(tree.querySelectorAll('details.branch')).reverse();
  for (const branch of branches) {
    const hasVisible = branch.querySelector('.leaf:not(.search-hidden), details.branch:not(.search-hidden)');
    if (!hasVisible) {
      branch.classList.add('search-hidden');
    } else {
      branch.open = true;
    }
  }
}
`;
