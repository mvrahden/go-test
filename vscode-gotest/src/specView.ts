import * as vscode from "vscode";
import * as path from "node:path";
import { existsSync } from "node:fs";
import {
  buildCliCommand,
  hasRuns,
  offerCliUpgrade,
  stripGoRunExitEcho,
} from "./cli.js";
import { specRenderTimeoutSeconds } from "./config.js";
import { ManagedChild } from "./managedChild.js";
import { readModulePath } from "./gomod.js";
import { JsonStore } from "./jsonStore.js";
import { SpecStore, type RunPiece } from "./specStore.js";
import {
  elapsedText,
  liveRuns,
  tallyText,
  type LiveRuns,
  type RunProgress,
} from "./liveProgress.js";
import type { DiscoveryCache } from "./discovery.js";
import type { ChangeScope } from "./changeScope.js";
import type { RunsScope } from "./runsScope.js";
import { buildRunsBody, RUNS_CSS } from "./runsView.js";
import { buildChangeBody, CHANGE_CSS } from "./changeView.js";
import {
  CHECK_FRESH_ARGS,
  parseFreshness,
  STALE_LABEL,
  UNKNOWN_TITLE,
  staleHeadline,
  type Freshness,
} from "./freshness.js";
import {
  EXTERNAL_ID,
  externalInstance,
  externalTwinNode,
  methodItemId,
  type DiscoverPackage,
} from "./types.js";

// The kept runs on disk, so a reload does not forget the packages that ran
// before it.
const RUNS_FILE = "specRuns.json";
// 2: lines are kept per suite instance and in the order they were merged.
const RUNS_VERSION = 2;

// The least time between two renders of a run in progress, and how often its
// banner's elapsed time and counts are redrawn.
export const LIVE_RENDER_MS = 3000;
export const LIVE_BANNER_MS = 1000;

// How long a burst of file changes settles before freshness is asked again.
const FRESH_DEBOUNCE_MS = 750;

// What the panel's node buttons do, given Test Explorer item ids.
const GITHUB_COMMENT_LIMIT = 65536;

export interface SpecViewActions {
  run(ids: string[]): Promise<void>;
  debug(id: string): Promise<void>;
}

// The panel's scopes: the specification of the runs, and the audit of the
// change against a base.
export type Scope = "spec" | "change" | "runs";
const SCOPES: { id: Scope; label: string; title: string }[] = [
  {
    id: "spec",
    label: "Spec",
    title: "What the suites declare, with the newest verdicts",
  },
  {
    id: "change",
    label: "Change",
    title: "What the working tree changed in the tests, against a base",
  },
  {
    id: "runs",
    label: "Runs",
    title:
      "The runs recorded in this folder, and what each says of the tree now",
  },
];

const isScope = (v: unknown): v is Scope => SCOPES.some((sc) => sc.id === v);

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
  // Why the last render failed; cleared by the next one that works.
  private renderError: string | undefined;
  private rendering: Promise<void> = Promise.resolve();
  private actions: SpecViewActions | undefined;
  // The statuses the toolbar hides, as the CLI's --hide names them.
  private readonly hidden = new Set<string>();
  private scope: Scope = "spec";
  private change: ChangeScope | undefined;
  private runsScope: RunsScope | undefined;
  // Per source on show ("" for the workspace's runs), each package's
  // freshness as the CLI last judged it.
  private readonly freshness = new Map<string, Map<string, Freshness>>();
  // Folders whose freshness is to be asked again, the timer that gathers a
  // burst of changes, and the check running per folder.
  private readonly freshPending = new Set<string>();
  private freshTimer: ReturnType<typeof setTimeout> | undefined;
  private readonly freshChecks = new Map<
    string,
    { child?: ManagedChild; gen: number }
  >();
  // Each folder's document as the last render drew it, so a render of a run
  // in progress redraws only the folders the run streams into.
  private folderDocs = new Map<string, SpecData>();
  // The render of runs in progress: the timer that spaces renders, the
  // render running and its children, and whether one waits behind it.
  private liveTimer: ReturnType<typeof setTimeout> | undefined;
  private liveBannerTimer: ReturnType<typeof setInterval> | undefined;
  private liveGen = 0;
  private liveRunning = false;
  private liveWaiting = false;
  private readonly liveChildren = new Set<ManagedChild>();
  // The picture on show holds a run in progress.
  private liveDrawn = false;

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
    // The folders that use gotest: one nothing ran in yet shows what its
    // source declares.
    private readonly declaredFolders?: () => string[],
    // The window's runs in progress.
    private readonly live: LiveRuns = liveRuns,
  ) {
    this.persisted = new JsonStore(storageDir, RUNS_FILE, RUNS_VERSION);
    this.disposables.push(live.onDidChange((run) => this.liveChanged(run)));
    if (cache) {
      this.disposables.push(
        cache.onDidUpdate(() => {
          this.pruneUndiscovered();
          if (this.unranFolders().length > 0) this.stale = true;
          if (this.stale && this.isVisible() && this.autoRefresh()) {
            void this.render();
          } else {
            this.rebuildIfVisible();
          }
        }),
      );
    }
  }

  // setChangeScope gives the panel its Change scope.
  setChangeScope(change: ChangeScope): void {
    this.change = change;
  }

  // changeChanged redraws the Change scope when it is on show.
  changeChanged(): void {
    if (this.panel && this.scope === "change") this.draw();
  }

  // setRunsScope gives the panel its Runs scope.
  setRunsScope(runs: RunsScope): void {
    this.runsScope = runs;
  }

  // runsChanged redraws the Runs scope when it is on show; the Change
  // scope shows the runs taken as evidence, so it is redrawn too.
  runsChanged(): void {
    if (this.panel && this.scope !== "spec") this.draw();
  }

  // ensureScope has the scope on show fetch what it lacks.
  private ensureScope(): void {
    if (this.scope === "change") this.change?.ensure();
    else if (this.scope === "runs") this.runsScope?.ensure();
  }

  private isVisible(): boolean {
    return !!this.panel && this.panel.visible !== false;
  }

  // folders lists every folder the spec renders: those with kept runs and,
  // on the workspace's own source, those nothing ran in yet.
  private renderFolders(): string[] {
    const ran = this.currentRuns().folders();
    return [...ran, ...this.unranFolders().filter((f) => !ran.includes(f))];
  }

  private unranFolders(): string[] {
    if (this.selected || !this.declaredFolders) return [];
    const ran = new Set(this.runs.folders());
    return this.declaredFolders().filter((f) => !ran.has(f));
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
      // Within a package, rows of a suite or method no longer declared.
      for (const pkg of known) {
        const declared = declaredTests(pkg);
        if (declared) removed += this.runs.pruneTests(pkg.importPath, declared);
      }
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
  // Loaded after the snapshot restore: only a live discovery prunes kept rows.
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
    if (!this.panel) return;
    if (this.scope !== "spec" || this.lastSpecData) this.draw();
  }

  // draw writes the panel for its scope from what it holds.
  private draw(): void {
    if (!this.panel) return;
    this.panel.webview.html = this.lastSpecData
      ? this.buildHtml({ type: "specData", data: this.lastSpecData })
      : this.buildHtml({ type: "empty" });
  }

  setExtensionUri(uri: vscode.Uri): void {
    this.extensionUri = uri;
  }

  // show reveals the panel, in scope when one is given.
  async show(scope?: Scope): Promise<void> {
    if (scope) this.scope = scope;
    if (!this.lastSpecData && this.unranFolders().length > 0) this.stale = true;
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside);
      if (scope) this.draw();
      this.ensureScope();
      if (this.stale) await this.render();
      return;
    }

    this.createPanel();
    await this.resolveModulePaths();

    this.draw();
    this.ensureScope();
    if (this.stale) await this.render();
  }

  // showChange reveals the panel in its Change scope.
  showChange(): Promise<void> {
    return this.show("change");
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

    const stored = parseStoredState(state);
    if (stored.spec) this.lastSpecData = stored.spec;
    if (stored.scope) this.scope = stored.scope;
    if (stored.folder) this.change?.select(stored.folder);
    if (stored.runsFolder) this.runsScope?.select(stored.runsFolder);
    if (!this.lastSpecData && this.unranFolders().length > 0) this.stale = true;
    this.draw();
    this.ensureScope();
    // A panel with a picture to show renders again when it is seen; one with
    // nothing but a declared tree to show renders now.
    if (!this.lastSpecData && this.stale && this.autoRefresh()) {
      await this.render();
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
        if (!e.webviewPanel.visible) return;
        this.ensureScope();
        this.checkFreshSoon([], 0);
        if (this.stale && this.autoRefresh()) void this.render();
        this.liveSoon();
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

  // The item ids above each row a button acted on, as the rows list them: a
  // behavior's name may hold slashes, so its ancestors are never found by
  // cutting its id.
  private readonly ancestry = new Map<string, string[]>();

  private noteAncestors(raw: unknown): void {
    if (!raw || typeof raw !== "object") return;
    for (const [id, list] of Object.entries(raw)) {
      if (Array.isArray(list) && list.every((a) => typeof a === "string"))
        this.ancestry.set(id, list);
    }
  }

  // ancestorsOf lists the item ids above a row the panel last acted on.
  ancestorsOf(id: string): string[] | undefined {
    return this.ancestry.get(id);
  }

  // The webview posts plain objects; each branch checks what it reads.
  private handleMessage(msg: any): void {
    if (msg.type === "run" && this.actions) {
      const ids: string[] = Array.isArray(msg.ids) ? msg.ids : [msg.id];
      this.noteAncestors(msg.ancestors);
      void this.actions.run(ids.filter((id) => typeof id === "string"));
    } else if (msg.type === "debug" && this.actions) {
      this.noteAncestors(msg.ancestors);
      if (typeof msg.id === "string") void this.actions.debug(msg.id);
    } else if (msg.type === "selectSource") {
      this.select(typeof msg.id === "string" ? msg.id : undefined);
    } else if (msg.type === "runTests") {
      vscode.commands.executeCommand("testing.runAll");
    } else if (msg.type === "runChanged") {
      void vscode.commands.executeCommand("gotest.runChanged");
    } else if (msg.type === "refresh") {
      this.stale = true;
      void this.render();
    } else if (msg.type === "copySpec") {
      void this.copy(typeof msg.search === "string" ? msg.search : "");
    } else if (msg.type === "copyNode") {
      const ok =
        typeof msg.pkg === "string" &&
        Array.isArray(msg.position) &&
        msg.position.every(
          (i: unknown) => Number.isInteger(i) && (i as number) >= 0,
        );
      if (ok) void this.copy("", { pkg: msg.pkg, position: msg.position });
    } else if (msg.type === "toggleHide") {
      if (typeof msg.status !== "string") return;
      if (!this.hidden.delete(msg.status)) this.hidden.add(msg.status);
      this.stale = true;
      void this.render();
    } else if (msg.type === "scope") {
      if (!isScope(msg.scope)) return;
      this.scope = msg.scope;
      this.draw();
      if (this.scope !== "spec") this.ensureScope();
      else if (this.stale) void this.render();
      else this.checkFreshSoon([], 0);
      this.liveSoon();
    } else if (msg.type === "change") {
      void this.change?.act(msg);
    } else if (msg.type === "runs") {
      void this.runsScope?.act(msg);
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
    this.freshness.clear();
    this.selected = undefined;
    this.stale = false;
    this.renderError = undefined;
    this.persist();
    if (this.unranFolders().length > 0) this.stale = true;
    this.draw();
    if (this.stale && this.isVisible()) void this.render();
  }

  // refresh folds finished runs into what the panel keeps and renders the
  // result: every package seen, each test with its newest verdict.
  async refresh(pieces: RunPiece | RunPiece[]): Promise<void> {
    const list = Array.isArray(pieces) ? pieces : [pieces];
    // What a run just judged is the tree's until the CLI says otherwise.
    const own = this.freshness.get("");
    for (const piece of list) {
      for (const pkg of this.runs.merge(piece)) own?.delete(pkg);
    }
    this.change?.runFinished(
      list.map((p) => p.folder),
      this.isVisible() && this.scope === "change",
    );
    this.runsScope?.invalidate(
      list.map((p) => p.folder),
      this.isVisible() && this.scope === "runs",
    );
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
  async openStream(
    id: string,
    label: string,
    piece: RunPiece,
    scope?: Scope,
  ): Promise<void> {
    const runs = new SpecStore();
    runs.merge(piece);
    this.imports.set(id, { label, runs });
    this.freshness.delete(id);
    this.selected = id;
    this.stale = true;
    await this.show(scope);
  }

  // sourceChanged asks again, once a burst of edits settles, whether the
  // verdicts the panel shows for folder still belong to its tree.
  sourceChanged(folder: string): void {
    // What a record says of the tree moves with it too.
    this.runsScope?.invalidate([folder], false);
    if (!this.renderFolders().includes(folder)) return;
    this.checkFreshSoon([folder], FRESH_DEBOUNCE_MS);
  }

  private freshnessOf(pkg: string): Freshness | undefined {
    return this.freshness.get(this.selected ?? "")?.get(pkg);
  }

  // checkFreshSoon gathers folders and checks them after delay, while the
  // panel is on show; a hidden panel checks when it is seen again.
  private checkFreshSoon(folders: string[], delay: number): void {
    for (const f of folders) this.freshPending.add(f);
    if (this.freshPending.size === 0) return;
    clearTimeout(this.freshTimer);
    this.freshTimer = setTimeout(() => {
      this.freshTimer = undefined;
      if (!this.isVisible() || this.scope !== "spec") return;
      const due = Array.from(this.freshPending);
      this.freshPending.clear();
      for (const folder of due) this.trackFresh(this.checkFresh(folder));
    }, delay);
  }

  private readonly freshWork = new Set<Promise<void>>();

  private trackFresh(p: Promise<void>): void {
    const done = p.catch(() => {}).finally(() => this.freshWork.delete(done));
    this.freshWork.add(done);
  }

  // freshIdle resolves once no freshness check is due or running.
  async freshIdle(): Promise<void> {
    for (;;) {
      if (this.freshTimer) await new Promise((r) => setTimeout(r, 50));
      else if (this.freshWork.size > 0) await Promise.all(this.freshWork);
      else return;
    }
  }

  // checkFresh has the folder's CLI judge the shown stream's packages
  // against the working tree. A newer check stops the one it supersedes.
  private async checkFresh(folder: string): Promise<void> {
    const prev = this.freshChecks.get(folder);
    const gen = (prev?.gen ?? 0) + 1;
    void prev?.child?.terminate("prompt");
    const slot: { child?: ManagedChild; gen: number } = { gen };
    this.freshChecks.set(folder, slot);
    const source = this.selected ?? "";
    const stream = this.currentRuns().compose(folder);
    if (!stream) return;
    if (!(await hasRuns(folder, this.outputChannel))) return;
    if (this.freshChecks.get(folder) !== slot) return;
    let cmd;
    try {
      cmd = await buildCliCommand(CHECK_FRESH_ARGS, folder, this.outputChannel);
    } catch {
      return;
    }
    if (this.freshChecks.get(folder) !== slot) return;
    const mc = new ManagedChild(cmd.bin, cmd.args, {
      cwd: folder,
      kind: "spec",
    });
    slot.child = mc;
    let stdout = "";
    mc.child.stdout?.on("data", (c: string) => (stdout += c));
    mc.child.stderr?.on("data", () => {});
    mc.child.on("error", () => {});
    mc.child.stdin?.on("error", () => {});
    mc.child.stdin?.end(stream);
    const budget = setTimeout(
      () => void mc.terminate("prompt"),
      specRenderTimeoutSeconds(folder) * 1000,
    );
    try {
      await mc.exited;
    } finally {
      clearTimeout(budget);
      mc.dispose();
    }
    if (this.freshChecks.get(folder) !== slot) return;
    slot.child = undefined;
    const judged = parseFreshness(stdout);
    if (!judged || (this.selected ?? "") !== source) return;
    let kept = this.freshness.get(source);
    if (!kept) {
      kept = new Map();
      this.freshness.set(source, kept);
    }
    for (const [pkg, f] of judged) kept.set(pkg, f);
    if (this.scope === "spec") this.draw();
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
    // A render of a run in progress would draw over this one.
    this.cancelLive();
    try {
      const data = await this.renderRuns();
      this.lastSpecData = data;
      this.renderError = undefined;
      await this.resolveModulePaths();
      this.draw();
      this.checkFreshSoon(this.renderFolders(), 0);
      this.liveSoon();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[specView] ${message}`);
      // The last picture stays, under a notice; the next reveal retries.
      this.renderError = message;
      this.stale = true;
      this.draw();
    }
  }

  // renderRuns has each folder's CLI render the packages that folder ran,
  // and what the source of a folder nothing ran in declares.
  private async renderRuns(): Promise<SpecData | undefined> {
    const runs = this.currentRuns();
    const docs = new Map<string, SpecData>();
    for (const folder of this.renderFolders()) {
      const raw = await this.runSpec(runs.compose(folder), folder, "json");
      docs.set(folder, JSON.parse(raw) as SpecData);
    }
    this.folderDocs = docs;
    this.liveDrawn = false;
    return docs.size > 0
      ? combineSpecData(Array.from(docs.values()))
      : undefined;
  }

  // copy puts on the clipboard what the panel shows, as the CLI writes it in
  // Markdown: the same streams, rendered with the same flags. A search keeps
  // the rows the panel's search keeps; a node is rendered alone, by the CLI of
  // the folder its package belongs to.
  private async copy(
    search = "",
    node?: { pkg: string; position: number[] },
  ): Promise<void> {
    const runs = this.currentRuns();
    const parts: string[] = [];
    try {
      if (node) {
        const folder = runs.folderOf(node.pkg);
        if (folder === undefined) return;
        const only = [node.pkg, node.position.join(".")]
          .filter((_, i) => i === 0 || node.position.length > 0)
          .join(":");
        parts.push(
          await this.runSpec(runs.compose(folder), folder, "md", [
            `--only=${only}`,
          ]),
        );
      } else {
        const match = search.trim();
        const extra = match ? [`--match=${match}`] : [];
        for (const folder of this.renderFolders()) {
          parts.push(
            await this.runSpec(runs.compose(folder), folder, "md", extra),
          );
        }
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[specView] copy: ${message}`);
      return;
    }
    if (parts.length === 0) return;
    const text = parts.join("\n");
    await vscode.env.clipboard.writeText(text);
    const tokens = Math.round(text.length / 4);
    void vscode.window.showInformationMessage(
      `Copied ${text.length} characters (about ${tokens} tokens).`,
    );
    if (text.length > GITHUB_COMMENT_LIMIT) {
      void vscode.window.showWarningMessage(
        `The copy is above GitHub's 65,536-character comment limit.`,
      );
    }
  }

  // specArgs are the CLI's flags for a folder's render: the declared
  // specification under the workspace's runs, never under an imported
  // stream, and the statuses the toolbar hides. --hide is always given, so
  // every package carries the CLI's counts.
  private async specArgs(
    folder: string,
    format: string,
    extra: string[] = [],
  ): Promise<string[]> {
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
    // --declared takes the rest of the line, so the flags come before it.
    args.push(...extra);
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
    this.runsScope?.dispose();
    this.cancelLive();
    clearInterval(this.liveBannerTimer);
    clearTimeout(this.freshTimer);
    for (const c of this.freshChecks.values())
      void c.child?.terminate("prompt");
    this.change?.dispose();
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

    const unran = new Set(this.unranFolders());
    const specBody =
      msg.type === "empty"
        ? buildEmptyBody(gopherUri)
        : buildSpecBody(
            msg.data,
            this.modulePaths,
            {
              dirOf: (pkgPath) => this.cache?.getPackage(pkgPath)?.dir,
              ranAtOf: (pkgPath) => this.currentRuns().ranAtOf(pkgPath),
              neverRan: (pkgPath) => {
                if (this.selected || this.runs.folderOf(pkgPath)) return false;
                const dir = this.cache?.getWorkspaceDir(pkgPath);
                return dir === undefined || unran.has(dir);
              },
              now: Date.now(),
              freshnessOf: (pkgPath) => this.freshnessOf(pkgPath),
            },
            this.sourcesForToolbar(),
            this.hidden,
          );
    const scopeBody =
      this.scope === "change" && this.change
        ? buildChangeBody(this.change.view())
        : this.scope === "runs" && this.runsScope
          ? buildRunsBody(this.runsScope.view())
          : liveBannerHtml(this.live.active(), Date.now()) + specBody;
    const body =
      (this.change || this.runsScope
        ? buildScopeTabs(this.scope, {
            change: !!this.change,
            runs: !!this.runsScope,
          })
        : "") + scopeBody;
    const stateJson =
      msg.type === "specData" ? encodeStateForScript(msg.data) : "null";
    const panelJson = encodeStateForScript({
      scope: this.scope,
      folder: this.change?.folder,
      runsFolder: this.runsScope?.folder,
    });

    return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${this.panel!.webview.cspSource}; style-src 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
<style nonce="${nonce}">
${CSS}
${CHANGE_CSS}
${RUNS_CSS}
</style>
</head>
<body>
${this.scope === "spec" ? renderErrorHtml(this.renderError) : ""}
${body}
<script nonce="${nonce}">
const SPEC_STATE = ${stateJson};
const PANEL_STATE = ${panelJson};
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
    extra: string[] = [],
    // Told of the child, so a superseded render can be stopped.
    track?: Set<ManagedChild>,
  ): Promise<string> {
    const workspaceDir =
      folder ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    let cmd;
    try {
      cmd = await buildCliCommand(
        await this.specArgs(workspaceDir ?? "", format, extra),
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
    track?.add(mc);
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
      track?.delete(mc);
      mc.dispose();
    }
  }

  // liveChanged follows a run in progress: its banner, and a render of the
  // rows it has reached. A run that ended stops its render; the finished
  // run's refresh renders what it judged.
  private liveChanged(run: RunProgress): void {
    if (!run.active) {
      this.cancelLive();
      if (this.liveDrawn) this.stale = true;
      if (this.live.active().length === 0) this.stopBanner();
      if (this.panel && this.scope === "spec") this.draw();
      return;
    }
    this.startBanner();
    this.liveSoon();
  }

  // liveOn says whether a run in progress is the panel's to render: the
  // workspace's runs on show in the Spec scope, which refreshes itself.
  private liveOn(): boolean {
    return (
      this.live.active().length > 0 &&
      this.isVisible() &&
      this.scope === "spec" &&
      !this.selected &&
      this.autoRefresh()
    );
  }

  // liveSoon renders the runs in progress LIVE_RENDER_MS from now, unless a
  // render is due already; one running has the next wait behind it.
  private liveSoon(): void {
    if (!this.liveOn() || this.liveTimer) return;
    if (this.liveRunning) {
      this.liveWaiting = true;
      return;
    }
    this.liveTimer = setTimeout(() => {
      this.liveTimer = undefined;
      void this.renderLive();
    }, LIVE_RENDER_MS);
  }

  // cancelLive stops the render of runs in progress, the one running too:
  // what it would draw is superseded.
  private cancelLive(): void {
    clearTimeout(this.liveTimer);
    this.liveTimer = undefined;
    this.liveWaiting = false;
    this.liveGen++;
    for (const mc of this.liveChildren) void mc.terminate("prompt");
  }

  // renderLive has the CLI render the kept runs with the runs in progress
  // laid over them, in the folders those runs stream into.
  private async renderLive(): Promise<void> {
    if (!this.liveOn()) return;
    const gen = ++this.liveGen;
    const runs = this.live.active();
    const overlay = new SpecStore();
    overlay.restore(this.runs.serialize());
    const folders = new Set<string>();
    for (const run of runs) {
      for (const piece of run.pieces()) {
        if (!piece.stream) continue;
        overlay.merge(piece);
        folders.add(piece.folder);
      }
    }
    if (folders.size === 0) return;
    this.liveRunning = true;
    try {
      const docs = new Map(this.folderDocs);
      for (const folder of folders) {
        const raw = await this.runSpec(
          overlay.compose(folder),
          folder,
          "json",
          [],
          this.liveChildren,
        );
        if (gen !== this.liveGen) return;
        docs.set(folder, JSON.parse(raw) as SpecData);
      }
      if (gen !== this.liveGen) return;
      const order = [
        ...this.renderFolders(),
        ...Array.from(folders).filter((f) => !this.renderFolders().includes(f)),
      ];
      const shown = order
        .map((f) => docs.get(f))
        .filter((d): d is SpecData => d !== undefined);
      this.lastSpecData = combineSpecData(shown);
      this.liveDrawn = true;
      this.draw();
    } catch (err: unknown) {
      // The run's own refresh renders it; a render that fails here waits.
      if (gen === this.liveGen) {
        const message = err instanceof Error ? err.message : String(err);
        this.outputChannel.warn(`[specView] live render: ${message}`);
      }
    } finally {
      this.liveRunning = false;
      if (this.liveWaiting) {
        this.liveWaiting = false;
        this.liveSoon();
      }
    }
  }

  // startBanner redraws the banner of the runs in progress each
  // LIVE_BANNER_MS, while the panel shows it.
  private startBanner(): void {
    if (this.liveBannerTimer) return;
    this.postBanner();
    this.liveBannerTimer = setInterval(() => this.postBanner(), LIVE_BANNER_MS);
  }

  private stopBanner(): void {
    clearInterval(this.liveBannerTimer);
    this.liveBannerTimer = undefined;
    this.postBanner();
  }

  private postBanner(): void {
    if (!this.panel || !this.isVisible() || this.scope !== "spec") return;
    void this.panel.webview.postMessage?.({
      type: "liveBanner",
      html: liveBannerInner(this.live.active(), Date.now()),
    });
  }
}

// liveBannerInner is the banner's content: a line per run in progress, with
// its elapsed time and the test events it applied so far; empty when none.
export function liveBannerInner(runs: RunProgress[], now: number): string {
  return runs
    .map(
      (r) =>
        `<div class="live-run"><span class="live-dot" aria-hidden="true"></span>Running ${escapeHtml(r.label)} · ${elapsedText(now - r.startedAt)} · ${tallyText(r.tally)}</div>`,
    )
    .join("");
}

// liveBannerHtml is the banner over the Spec scope, hidden without a run.
export function liveBannerHtml(runs: RunProgress[], now: number): string {
  const inner = liveBannerInner(runs, now);
  return `<div id="live-banner" class="live-banner" role="status"${inner ? "" : " hidden"}>${inner}</div>`;
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
    vacuous: 0,
    assertsBy: 0,
    excluded: 0,
  };
  const packages: SpecPackage[] = [];
  const groups: SpecGroup[] = [];
  const runs: SpecRun[] = [];
  for (const doc of docs) {
    packages.push(...doc.packages);
    groups.push(...(doc.groups ?? []));
    if (doc.run) runs.push(doc.run);
    for (const key of Object.keys(stats) as (keyof SpecStats)[]) {
      stats[key] = (stats[key] ?? 0) + (doc.stats[key] ?? 0);
    }
  }
  packages.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  return {
    version: docs[0]?.version,
    packages,
    stats,
    ...(groups.length > 0 ? { groups } : {}),
    ...(runs.length > 0 ? { runs } : {}),
  };
}

// --- Types ---

export interface SpecData {
  /** The CLI that produced the document; absent before v1.30.2. */
  version?: string;
  // The run the document renders, as its stream recorded it.
  run?: SpecRun;
  packages: SpecPackage[];
  stats: SpecStats;
  // What each named ExclusiveGroup took.
  groups?: SpecGroup[];
  // The runs of the documents combined into this one, one per folder that
  // had one; set by combineSpecData, not by the CLI.
  runs?: SpecRun[];
}

export interface SpecRun {
  version?: string;
  goVersion?: string;
  args?: string[];
  commit?: string;
  dirty?: boolean;
  agent?: string;
  exitCode?: number;
  // A --changed run: how many packages it ran and left out unchanged.
  changed?: { ran: number; unchanged: number; noRecord?: boolean };
}

export interface SpecGroup {
  name: string;
  suites: number;
  // Seconds from the group's first suite start to its last end.
  wall: number;
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
  // The instance of its top-level name in the run, from 2 on.
  variant?: number;
  // Which run of a repeated suite this is (-count); absent when it ran once
  // and from an older CLI's document.
  run?: number;
  runs?: number;
  // The own level go test reported, when it is not name: a twin's
  // TestCreate#01.
  reported?: string;
  // The children are a floor: more behaviors are known only at run time.
  incomplete?: boolean;
  skipReason?: string;
  // Assertions a leaf made; absent when unknown, which is not zero.
  assertions?: number;
  // Passed without an assertion and declared no other means.
  vacuous?: boolean;
  // The means of asserting a leaf declared instead.
  assertsBy?: string;
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
  vacuous?: number;
  assertsBy?: number;
  excluded?: number;
  benchmarks?: number;
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
  // When a package last ran, and the clock that is read against.
  ranAtOf?: (pkgPath: string) => number | undefined;
  // A package of a folder nothing ran in: its rows are what source declares.
  neverRan?: (pkgPath: string) => boolean;
  now?: number;
  // Whether a package's verdict still belongs to the working tree, as the
  // CLI judged it; undefined before it has.
  freshnessOf?: (pkgPath: string) => Freshness | undefined;
}

// staleClass is " stale" for a package the CLI judged stale.
function staleClass(pkgPath: string, env: RenderEnv): string {
  return env.freshnessOf?.(pkgPath) === "stale" ? " stale" : "";
}

// freshHtml is what a package's header says of its freshness: a stale one
// its label; an unknown one only a tooltip.
function freshHtml(pkgPath: string, env: RenderEnv): string {
  const f = env.freshnessOf?.(pkgPath);
  if (f === "stale")
    return ` <span class="tag stale" title="The CLI hashed this package's inputs now and they differ from the run's">${escapeHtml(STALE_LABEL)}</span>`;
  if (f === "unknown")
    return ` <span class="fresh-unknown" title="${escapeAttr(UNKNOWN_TITLE)}">·</span>`;
  return "";
}

// formatRanAt is a run's local time: HH:MM on the day of now, with the date
// on any other day.
export function formatRanAt(at: number, now: number): string {
  const d = new Date(at);
  const n = new Date(now);
  const two = (v: number) => String(v).padStart(2, "0");
  const time = `${two(d.getHours())}:${two(d.getMinutes())}`;
  const sameDay =
    d.getFullYear() === n.getFullYear() &&
    d.getMonth() === n.getMonth() &&
    d.getDate() === n.getDate();
  return sameDay
    ? time
    : `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${time}`;
}

// ranAtText is a package's "last run" text, when the time is known.
function ranAtText(pkgPath: string, env: RenderEnv): string | undefined {
  const at = env.ranAtOf?.(pkgPath);
  if (at === undefined) return env.neverRan?.(pkgPath) ? NEVER_RAN : undefined;
  return `last run ${formatRanAt(at, env.now ?? Date.now())}`;
}

const NEVER_RAN = "declared, not run yet";

function ranAtHtml(pkgPath: string, env: RenderEnv): string {
  const at = env.ranAtOf?.(pkgPath);
  if (at === undefined) {
    return env.neverRan?.(pkgPath)
      ? ` <span class="ran-at">${NEVER_RAN}</span>`
      : "";
  }
  const text = `last run ${formatRanAt(at, env.now ?? Date.now())}`;
  return ` <span class="ran-at" title="${escapeAttr(new Date(at).toISOString())}">${escapeHtml(text)}</span>`;
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
    const ranAt = ranAtText(pkg.path, env);
    const fresh = freshHtml(pkg.path, env);
    tree =
      (ranAt || fresh
        ? `<div class="ran-at-line">${escapeHtml(ranAt ?? "")}${fresh}</div>`
        : "") +
      `<div class="pkg-body${staleClass(pkg.path, env)}">` +
      buildPackageFailureHtml(pkg, env) +
      pkg.nodes
        .map((n, i) =>
          buildNodeHtml(n, null, nodeCtx(env, pkg), [i], [pkg.path]),
        )
        .join("") +
      `</div>`;
  } else if (groups.length === 1) {
    tree = groups[0].entries
      .map((e) => buildPackageSectionHtml(e.displayPath, e.pkg, env))
      .join("");
  } else {
    tree = groups.map((g) => buildModuleSectionHtml(g, env)).join("");
  }
  const summary = buildSummary(data.stats, data.groups);
  const stale = env.freshnessOf
    ? staleHeadline(
        data.packages.map((p) => p.path),
        env.freshnessOf,
      )
    : undefined;
  const staleLine = stale
    ? `<div class="stale-headline" role="status">${escapeHtml(stale)}</div>`
    : "";
  return `<div class="spec-container">
  ${toolbar}
  ${buildProvenanceHtml(data)}
  ${staleLine}
  <div class="spec-tree" id="spec-tree">${tree}</div>
  ${summary}
</div>`;
}

// provenanceText says which run a document renders, from its run header:
// the commit and whether the tree was dirty, the agent, the exit code.
export function provenanceText(run: SpecRun): string {
  const parts: string[] = [];
  if (run.commit) {
    parts.push(run.dirty ? `${run.commit}, uncommitted changes` : run.commit);
  }
  if (run.agent) parts.push(`agent ${run.agent}`);
  if (run.exitCode !== undefined) parts.push(`exit ${run.exitCode}`);
  else parts.push("run did not finish");
  const c = run.changed;
  if (c)
    parts.push(
      c.noRecord
        ? "--changed found no record and ran everything"
        : `narrowed by --changed: ran ${countNoun(c.ran, "package")}, left out ${c.unchanged} unchanged`,
    );
  if (run.version) parts.push(`gotest ${run.version}`);
  return parts.join(" · ");
}

function buildProvenanceHtml(data: SpecData): string {
  const runs = data.runs ?? (data.run ? [data.run] : []);
  if (runs.length === 0) return "";
  const lines = runs
    .map((r) => {
      const args = r.args && r.args.length > 0 ? r.args.join(" ") : "";
      return `<div class="provenance-line" title="${escapeAttr(args)}">Run: ${escapeHtml(provenanceText(r))}</div>`;
    })
    .join("");
  return `<div class="provenance">${lines}</div>`;
}

// groupText is a group's line as the CLI's summary words it.
export function groupText(g: SpecGroup): string {
  return `group ${g.name}: ${countNoun(g.suites, "suite")}, ${formatDuration(g.wall)}, run apart`;
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
    <button class="tool-btn" id="run-changed-btn" title="Run only the packages whose inputs changed since the last record, and those that failed or never ran (--changed)">Run changed</button>
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
  if ((stats.vacuous ?? 0) > 0) parts.push(`${stats.vacuous} vacuous`);
  if ((stats.assertsBy ?? 0) > 0)
    parts.push(`${stats.assertsBy} asserting by other means`);
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
    vacuous: 0,
    assertsBy: 0,
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
    sum.vacuous = (sum.vacuous ?? 0) + (s.vacuous ?? 0);
    sum.assertsBy = (sum.assertsBy ?? 0) + (s.assertsBy ?? 0);
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
    .map((n, i) => buildNodeHtml(n, null, nodeCtx(env, pkg), [i], [pkg.path]))
    .join("");
  return `<details class="pkg-section${staleClass(pkg.path, env)}" open>
  <summary class="pkg-header">${escapeHtml(displayPath)}${statsHtml(pkg.stats)}${ranAtHtml(pkg.path, env)}${freshHtml(pkg.path, env)}</summary>
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
        only.nodes
          .map((n, i) =>
            buildNodeHtml(n, null, nodeCtx(env, only), [i], [only.path]),
          )
          .join("")
      : group.entries
          .map((e) => buildPackageSectionHtml(e.displayPath, e.pkg, env))
          .join("");
  if (!inner) return "";
  const stats = addStats(group.entries.map((e) => e.pkg.stats));
  const lone = group.entries.length === 1 ? only.path : undefined;
  const cls = lone ? staleClass(lone, env) : "";
  const fresh = lone ? freshHtml(lone, env) : "";
  return `<details class="module-section${cls}" open>
  <summary class="module-header">${escapeHtml(group.modulePath || "unknown")}${statsHtml(stats)}${fresh}</summary>
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

// itemIdOf is the Test Explorer id of a node under its parent's (null at the
// top, undefined when the parent has none): a suite as discovery names it,
// its Test prefix dropped and an external twin's run marked; below it the
// level go test reported, as the router names a subtest's item. A fuzz
// target sits under its suite in both trees.
export function itemIdOf(
  pkgPath: string,
  parentId: string | null | undefined,
  node: Pick<SpecNode, "name" | "reported" | "external" | "variant" | "run">,
): string | undefined {
  if (parentId === undefined) return undefined;
  if (parentId !== null) {
    return methodItemId(parentId, { name: node.name, testName: node.reported });
  }
  if (!node.name.startsWith("Test") || node.name.startsWith("Test_")) {
    return undefined;
  }
  const twin = externalTwinNode(node);
  return `${pkgPath}/${node.name.slice(4)}${twin ? EXTERNAL_ID : ""}`;
}

// runTagHtml names which run of a repeated suite a node is, as the CLI's
// own views do; a suite run once has none.
function runTagHtml(node: SpecNode): string {
  if (!node.runs || !node.run) return "";
  return ` <span class="tag run">RUN ${node.run} OF ${node.runs}</span>`;
}

// skipReasonText is ": reason" for a skipped node whose skip gave one.
function skipReasonText(node: SpecNode): string {
  return node.status === "skip" && node.skipReason
    ? `: ${node.skipReason}`
    : "";
}

// incompleteHtml marks a row whose children are a floor, as the CLI does.
function incompleteHtml(node: SpecNode): string {
  return node.incomplete
    ? ` <span class="tag incomplete">INCOMPLETE: behaviors known only at run time</span>`
    : "";
}

// assertionHtml is what a leaf says about its assertions: vacuous, the
// means it declared, or how many it made when the run said.
function assertionHtml(node: SpecNode): string {
  if (node.vacuous) {
    return ` <span class="tag vacuous">VACUOUS: passed without an assertion</span>`;
  }
  if (node.assertsBy) {
    return ` <span class="tag asserts-by">ASSERTS BY: ${escapeHtml(node.assertsBy)}</span>`;
  }
  if (node.assertions !== undefined) {
    return ` <span class="tag assertions" title="Assertions the run counted">${countNoun(node.assertions, "assertion")}</span>`;
  }
  return "";
}

// nearestItemId is id itself when the tree has it, else its closest
// ancestor that it has (the row's own list of them when it has one, else cut
// at slashes, which a behavior's name may hold): behaviors past the tree's per-method cap are not
// items, and running one runs its method.
export function nearestItemId(
  id: string,
  has: (id: string) => boolean,
  ancestors?: string[],
): string | undefined {
  if (ancestors) {
    if (has(id)) return id;
    for (let i = ancestors.length - 1; i >= 0; i--) {
      if (has(ancestors[i])) return ancestors[i];
    }
    return undefined;
  }
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

function copyNodeHtml(): string {
  return `<span class="node-actions"><button class="act" data-act="copyNode" title="Copy this node as Markdown">Copy</button></span>`;
}

function idAttr(id: string | undefined, ancestors: string[] = []): string {
  if (!id) return "";
  const above =
    ancestors.length > 0
      ? ` data-ancestors="${escapeAttr(JSON.stringify(ancestors))}"`
      : "";
  return ` data-id="${escapeAttr(id)}"${above}`;
}

// Nodes are rendered under the item id their parent's path names (null at
// the top), which with their own reported level names their Test Explorer
// item. A fixture row has no item, though the rows beneath it may.
function buildNodeHtml(
  node: SpecNode,
  parentBase: string | null | undefined,
  ctx: NodeCtx,
  position: number[],
  ancestors: string[],
): string {
  const base = itemIdOf(ctx.pkgPath, parentBase, node);
  if (node.children.length === 0) {
    return buildLeafHtml(node, base, ctx, ancestors);
  }
  return buildBranchHtml(node, base, ctx, position, ancestors);
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

function buildLeafHtml(
  node: SpecNode,
  base: string | undefined,
  ctx: NodeCtx,
  ancestors: string[],
): string {
  const iconHtml = buildIconHtml(node, ctx);
  const id = node.kind === "fixture" ? undefined : base;
  let suffix = fuzzTagHtml(node);
  if (node.external) suffix += ` <span class="tag external">EXT.</span>`;
  suffix += runTagHtml(node);
  if (node.focused) suffix += ` <span class="tag focused">FOCUSED</span>`;
  else if (node.excluded || node.status === "skip")
    suffix += ` — SKIPPED${skipReasonText(node)}`;
  suffix += incompleteHtml(node) + assertionHtml(node);
  // A declared behavior the run did not report took no time.
  const dur =
    node.status === "none"
      ? ` <span class="tag none">NOT RUN</span>`
      : durationHtml(node);

  const errorBlock =
    node.status === "fail" ? errorBlockHtml(node, ctx.outputDir) : "";

  return `<div class="leaf ${node.kind} ${node.status}" data-display="${escapeAttr(node.display)}"${idAttr(id, ancestors)}>
  ${iconHtml} ${escapeHtml(node.display)}${suffix}${dur}${actionsHtml(id)}
  ${errorBlock}
</div>`;
}

// The kinds whose rows show their time at any length, as the CLI's spec does.
const TIMED_KINDS = new Set(["suite", "method", "test", "fuzz"]);

// durationHtml is a row's time as the CLI's spec shows it: a suite, a
// method, a stdlib test or a fuzz target with a verdict at any length, any
// other row from one second up, a row without a verdict none.
function durationHtml(node: SpecNode): string {
  if (!["pass", "fail", "skip"].includes(node.status)) return "";
  if (!TIMED_KINDS.has(node.kind) && node.duration < 1) return "";
  return ` <span class="dur">(${formatDuration(node.duration)})</span>`;
}

function buildBranchHtml(
  node: SpecNode,
  base: string | undefined,
  ctx: NodeCtx,
  position: number[],
  ancestors: string[],
): string {
  const openAttr = node.status === "fail" ? " open" : "";
  const iconHtml = buildIconHtml(node, ctx);
  const id = node.kind === "fixture" ? undefined : base;
  const below = id ? [...ancestors, id] : ancestors;
  const children = node.children
    .map((c, i) => buildNodeHtml(c, base, ctx, [...position, i], below))
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
  suffix += runTagHtml(node);
  if (node.focused) suffix += ` <span class="tag focused">FOCUSED</span>`;
  else if (node.excluded || node.status === "skip")
    suffix += ` <span class="tag skipped">SKIPPED${escapeHtml(skipReasonText(node))}</span>`;
  suffix += incompleteHtml(node);

  // What the branch itself said when it failed: a failed AfterAll, a panic
  // booked on the suite rather than on the behavior that raised it.
  const errorBlock =
    node.status === "fail" ? errorBlockHtml(node, ctx.outputDir) : "";

  return `<details class="branch ${node.status}" data-display="${escapeAttr(node.display)}"${openAttr}>
  <summary class="node ${node.kind}"${idAttr(id, ancestors)} data-pkg="${escapeAttr(ctx.pkgPath)}" data-position="${position.join(".")}">${iconHtml} ${label}${suffix}${durationHtml(node)}${actionsHtml(id)}${copyNodeHtml()}</summary>
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

function buildSummary(stats: SpecStats, groups: SpecGroup[] = []): string {
  const counts: string[] = [];
  if (stats.suites > 0) counts.push(countNoun(stats.suites, "suite"));
  if (stats.behaviors > 0) counts.push(countNoun(stats.behaviors, "behavior"));
  if ((stats.fuzzers ?? 0) > 0)
    counts.push(countNoun(stats.fuzzers ?? 0, "fuzz target"));
  if (stats.tests > 0) counts.push(`${stats.tests} stdlib tests`);
  if ((stats.benchmarks ?? 0) > 0)
    counts.push(countNoun(stats.benchmarks ?? 0, "benchmark"));
  const groupLines = groups
    .map(
      (g) => `<span class="summary-group">${escapeHtml(groupText(g))}</span>`,
    )
    .join("");

  const failedPackages = stats.failedPackages ?? 0;
  const notRun = stats.notRun ?? 0;
  return `<div class="summary">
  <span class="summary-counts">${counts.join(", ")}${groupLines}</span>
  <span class="summary-results">
    ${stats.passed > 0 ? `<span class="pass">${stats.passed} passed</span>` : ""}
    ${stats.failed > 0 ? `<span class="fail">${stats.failed} failed</span>` : ""}
    ${stats.skipped > 0 ? `<span class="skip">${stats.skipped} skipped</span>` : ""}
    ${notRun > 0 ? `<span class="none">${notRun} not run</span>` : ""}
    ${(stats.vacuous ?? 0) > 0 ? `<span class="skip">${stats.vacuous} vacuous</span>` : ""}
    ${(stats.assertsBy ?? 0) > 0 ? `<span class="none">${stats.assertsBy} asserting by other means</span>` : ""}
    ${(stats.excluded ?? 0) > 0 ? `<span class="none">${stats.excluded} excluded</span>` : ""}
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

// renderErrorHtml is the notice over a picture the last render could not
// replace: empty when that render worked.
export function renderErrorHtml(message: string | undefined): string {
  if (message === undefined) return "";
  const reason = escapeHtml(message.split("\n")[0]);
  return `<div class="render-error" role="alert">This view is out of date: the last render failed. <span class="render-error-reason">${reason}</span> <button class="tool-btn" id="retry-render-btn">Retry</button></div>`;
}

// declaredTests says whether a kept test path is still declared in pkg: a
// suite's own line and its methods' rows by the suite and method names, a
// fuzz target by its wrapper's name. Paths discovery does not list (plain
// test functions, benchmarks) are kept. A twin's row is matched against
// its own twin's methods. Undefined for a package that does not load: its
// suites are unknown.
export function declaredTests(
  pkg: DiscoverPackage,
): ((instance: number, testPath: string) => boolean) | undefined {
  if (pkg.broken) return undefined;
  const suites = pkg.suites ?? [];
  const hasSuite = (name: string) => suites.some((s) => s.name === name);
  // The suites a row of this instance belongs to, read as the router reads
  // the stream; a lone suite takes every instance.
  const suitesOf = (name: string, instance: number) => {
    const named = suites.filter((s) => s.name === name);
    const twins =
      named.some((s) => s.external) && named.some((s) => !s.external);
    if (!twins) return named;
    const external = externalInstance(instance, true);
    return named.filter((s) => !!s.external === external);
  };
  const hasMethod = (name: string, instance: number, method: string) =>
    suitesOf(name, instance).some((s) =>
      [...(s.methods ?? []), ...(s.fuzzers ?? [])].some(
        (m) => (m.testName ?? m.name) === method,
      ),
    );
  const hasFuzzer = (name: string, fuzzer: string) =>
    suites.some(
      (s) =>
        s.name === name && (s.fuzzers ?? []).some((f) => f.name === fuzzer),
    );
  return (instance, testPath) => {
    const slash = testPath.indexOf("/");
    const top = slash < 0 ? testPath : testPath.slice(0, slash);
    if (top.startsWith("Test")) {
      const name = top.slice(4);
      if (!hasSuite(name)) return !name.endsWith("TestSuite");
      if (slash < 0) return true;
      const rest = testPath.slice(slash + 1);
      const next = rest.indexOf("/");
      const method = next < 0 ? rest : rest.slice(0, next);
      // Under -count a repeat's method carries testing's #NN suffix.
      return (
        hasMethod(name, instance, method) ||
        hasMethod(name, instance, method.replace(/#\d+$/, ""))
      );
    }
    if (top.startsWith("Fuzz")) {
      const body = top.slice(4);
      let suiteLike = false;
      for (
        let idx = body.indexOf("_Fuzz");
        idx >= 0;
        idx = body.indexOf("_Fuzz", idx + 1)
      ) {
        const name = body.slice(0, idx);
        if (hasFuzzer(name, body.slice(idx + 1))) return true;
        if (name.endsWith("TestSuite")) suiteLike = true;
      }
      return !suiteLike;
    }
    return true;
  };
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/"/g, "&quot;");
}

// parseStoredState reads what the webview kept: the scope, the folder of
// the Change scope and the spec; an older panel kept the spec alone.
export function parseStoredState(state: unknown): {
  spec?: SpecData;
  scope?: Scope;
  folder?: string;
  runsFolder?: string;
} {
  const isSpec = (v: unknown): v is SpecData =>
    !!v &&
    typeof v === "object" &&
    "packages" in (v as Record<string, unknown>) &&
    "stats" in (v as Record<string, unknown>);
  if (isSpec(state)) return { spec: state };
  if (!state || typeof state !== "object") return {};
  const s = state as Record<string, unknown>;
  return {
    spec: isSpec(s.spec) ? s.spec : undefined,
    scope: isScope(s.scope) ? s.scope : undefined,
    folder: typeof s.folder === "string" ? s.folder : undefined,
    runsFolder: typeof s.runsFolder === "string" ? s.runsFolder : undefined,
  };
}

// buildScopeTabs is the panel's scope switch.
export function buildScopeTabs(
  scope: Scope,
  has: { change: boolean; runs: boolean } = { change: true, runs: true },
): string {
  const shown = SCOPES.filter(
    (sc) => sc.id === "spec" || has[sc.id as "change" | "runs"],
  );
  return `<div class="scopes" role="tablist" aria-label="Scope">${shown
    .map(
      (sc) =>
        `<button class="scope-tab" role="tab" data-scope="${sc.id}" aria-selected="${sc.id === scope}" title="${escapeAttr(sc.title)}">${sc.label}</button>`,
    )
    .join("")}</div>`;
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
.tag.run { color: var(--vscode-descriptionForeground); }
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
.ran-at, .ran-at-line { opacity: 0.7; font-size: 0.9em; }
.ran-at-line { margin: 0 0 4px 4px; }
.live-banner { margin: 8px 12px; padding: 6px 10px; border: 1px solid var(--vscode-focusBorder); border-radius: 3px; font-variant-numeric: tabular-nums; }
.live-banner[hidden] { display: none; }
.live-dot { display: inline-block; width: 8px; height: 8px; margin-right: 8px; border-radius: 50%; background: var(--vscode-progressBar-background); animation: live-pulse 1.2s ease-in-out infinite; }
@keyframes live-pulse { 50% { opacity: 0.3; } }
@media (prefers-reduced-motion: reduce) { .live-dot { animation: none; } }
.render-error { margin: 8px 12px; padding: 6px 10px; border: 1px solid var(--vscode-inputValidation-warningBorder); background: var(--vscode-inputValidation-warningBackground); }
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

.tag.incomplete, .tag.assertions, .tag.asserts-by { color: var(--vscode-descriptionForeground); }
.tag.vacuous { color: var(--vscode-testing-iconSkipped); }
.provenance { padding: 4px 12px; border-bottom: 1px solid var(--vscode-panel-border); flex-shrink: 0; }
.provenance-line { color: var(--vscode-descriptionForeground); font-size: 0.85em; overflow-wrap: anywhere; }
.summary-group { display: block; }

/* Verdicts whose inputs changed since the run */
.tag.stale { color: var(--vscode-testing-iconSkipped); font-weight: normal; }
.fresh-unknown { color: var(--vscode-descriptionForeground); margin-left: 6px; cursor: help; }
.pkg-body.stale,
details.pkg-section.stale > :not(summary),
details.module-section.stale > :not(summary) { opacity: 0.55; }
.stale-headline { padding: 4px 12px; font-size: 0.85em; color: var(--vscode-testing-iconSkipped); border-bottom: 1px solid var(--vscode-panel-border); flex-shrink: 0; }

/* Filter hiding */
.search-hidden { display: none; }
`;

// --- Embedded Script ---

const SCRIPT = `
const vscode = acquireVsCodeApi();
const PREV_STATE = vscode.getState() || {};
vscode.setState(Object.assign({}, PANEL_STATE, { spec: SPEC_STATE }));

// A redraw of the same scope keeps where the reader was: a run in progress
// redraws the rows every few seconds.
const sameScope = PREV_STATE.scope === PANEL_STATE.scope;
if (sameScope && typeof PREV_STATE.scrollY === 'number') {
  window.scrollTo(0, PREV_STATE.scrollY);
  vscode.setState(Object.assign({}, vscode.getState() || {}, { scrollY: PREV_STATE.scrollY }));
}
let scrollSave = null;
window.addEventListener('scroll', () => {
  clearTimeout(scrollSave);
  scrollSave = setTimeout(() => {
    vscode.setState(Object.assign({}, vscode.getState() || {}, { scrollY: window.scrollY }));
  }, 100);
});

// The banner of a run in progress, redrawn in place.
window.addEventListener('message', (e) => {
  const m = e.data;
  if (!m || m.type !== 'liveBanner') return;
  const el = document.getElementById('live-banner');
  if (!el) return;
  el.innerHTML = m.html;
  el.hidden = !m.html;
});

// The item ids above each row, as the rows list them.
function ancestorsOf(rows) {
  const out = {};
  for (const el of rows) {
    if (!el.dataset.ancestors) continue;
    try { out[el.dataset.id] = JSON.parse(el.dataset.ancestors); } catch (_) {}
  }
  return out;
}

document.addEventListener('click', (e) => {
  const t = e.target;
  if (!t || !(t instanceof HTMLElement)) return;

  if (t.id === 'run-tests-btn') {
    vscode.postMessage({ type: 'runTests' });
    return;
  }
  const scopeTab = t.closest('[data-scope]');
  if (scopeTab) {
    vscode.postMessage({ type: 'scope', scope: scopeTab.dataset.scope });
    return;
  }
  const changeEl = t.closest('[data-change]');
  if (changeEl) {
    if (changeEl.disabled) return;
    vscode.postMessage({ type: 'change', act: changeEl.dataset.change, key: changeEl.dataset.key });
    e.preventDefault();
    e.stopPropagation();
    return;
  }
  const runsEl = t.closest('[data-runs]');
  if (runsEl) {
    if (runsEl.disabled) return;
    vscode.postMessage({ type: 'runs', act: runsEl.dataset.runs, key: runsEl.dataset.key });
    e.preventDefault();
    e.stopPropagation();
    return;
  }
  const act = t.closest('.act');
  if (act) {
    if (act.dataset.act === 'copyNode') {
      const row = act.closest('[data-position]');
      if (row) {
        const raw = row.dataset.position;
        vscode.postMessage({
          type: 'copyNode',
          pkg: row.dataset.pkg,
          position: raw === '' ? [] : raw.split('.').map(Number),
        });
      }
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    const owner = act.closest('[data-id]');
    if (owner) vscode.postMessage({ type: act.dataset.act, id: owner.dataset.id, ancestors: ancestorsOf([owner]) });
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
    if (ids.length > 0) {
      const owners = Array.from(document.querySelectorAll('[data-id]')).filter(el => ids.includes(el.dataset.id));
      vscode.postMessage({ type: 'run', ids: ids, ancestors: ancestorsOf(owners) });
    }
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
  if (t.id === 'run-changed-btn') {
    vscode.postMessage({ type: 'runChanged' });
    return;
  }
  if (t.id === 'copy-spec-btn') {
    const box = document.getElementById('search-input');
    vscode.postMessage({ type: 'copySpec', search: box ? box.value.trim() : '' });
    return;
  }
  if (t.id === 'refresh-btn' || t.id === 'retry-render-btn') {
    vscode.postMessage({ type: 'refresh' });
    return;
  }
  if (t.id === 'clear-results-btn') {
    vscode.setState(Object.assign({}, PANEL_STATE, { spec: null }));
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

const changeFolder = document.getElementById('change-folder');
if (changeFolder) {
  changeFolder.addEventListener('change', () => {
    vscode.postMessage({ type: 'change', act: 'folder', folder: changeFolder.value });
  });
}

const runsFolder = document.getElementById('runs-folder');
if (runsFolder) {
  runsFolder.addEventListener('change', () => {
    vscode.postMessage({ type: 'runs', act: 'folder', folder: runsFolder.value });
  });
}

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
    vscode.setState(Object.assign({}, vscode.getState() || {}, { search: searchInput.value }));
  });
  if (sameScope && typeof PREV_STATE.search === 'string' && PREV_STATE.search) {
    searchInput.value = PREV_STATE.search;
    vscode.setState(Object.assign({}, vscode.getState() || {}, { search: PREV_STATE.search }));
    applySearch();
  }
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
