// The panel's host: it keeps the runs and the spec document the CLI renders
// from them, sets the webview's document once, and from then on posts each
// tab small, coalesced messages, a package's rows and a test's output only
// when asked.
import * as vscode from "vscode";
import * as path from "node:path";
import { execFile } from "node:child_process";
import { buildCliCommand, hasRuns, offerCliUpgrade } from "../cli.js";
import { specRenderTimeoutSeconds } from "../config.js";
import { ManagedChild } from "../managedChild.js";
import { readModulePath } from "../gomod.js";
import { JsonStore } from "../jsonStore.js";
import { SpecStore, type RunPiece } from "../specStore.js";
import {
  elapsedText,
  liveRuns,
  tallyText,
  type LiveRuns,
  type RunProgress,
} from "../liveProgress.js";
import { sessions as windowSessions, type Sessions } from "../sessions.js";
import type { DiscoveryCache } from "../discovery.js";
import type { ChangeScope } from "../changeScope.js";
import type { RunsScope } from "../runsScope.js";
import { buildChangeTab, changePackageRows } from "../changeView.js";
import { buildRunsTab } from "../runsView.js";
import { flakyHint } from "../runsModel.js";
import {
  CHECK_FRESH_ARGS,
  parseFreshness,
  type Freshness,
} from "../freshness.js";
import {
  buildNowModel,
  failedIds,
  fullOutput,
  liveFailures,
  findByNames,
  leavesUnder,
  namesAt,
  nodeAt,
  packageTree,
  rowOfItem,
  packagesIn,
  searchFound,
  type SourceOption,
} from "./nowModel.js";
import { Poster, type Channel } from "./poster.js";
import {
  FILTER_STATUSES,
  isTab,
  type FilterStatus,
  type HostMessage,
  type NowModel,
  type RunningItem,
  type Tab,
} from "./protocol.js";
import { buildShell, getNonce } from "./shell.js";
import {
  combineSpecData,
  countNoun,
  declaredTests,
  interpretSpecExit,
  type SpecData,
} from "./specData.js";

// The kept runs on disk, so a reload does not forget the packages that ran
// before it.
const RUNS_FILE = "specRuns.json";
// 2: lines are kept per suite instance and in the order they were merged.
const RUNS_VERSION = 2;

// How long a burst of file changes settles before freshness is asked again.
const FRESH_DEBOUNCE_MS = 750;

// How often the Running block's elapsed times are redrawn.
export const RUNNING_TICK_MS = 1000;

const GITHUB_COMMENT_LIMIT = 65536;

export interface PanelActions {
  run(ids: string[]): Promise<void>;
  debug(id: string): Promise<void>;
}

// What show() takes: a tab, or the old name for Now.
export type Scope = Tab | "spec";
const toTab = (s: Scope): Tab => (s === "spec" ? "now" : s);

// The statuses the CLI's --hide knows, which the chips map onto for a copy.
const HIDEABLE: FilterStatus[] = ["fail", "none", "pass", "skip"];

// parseStoredState reads what the webview kept: the tab, its opened
// packages, search and chips, and the folders of Change and Runs. An older
// panel kept a scope ("spec" for Now) and the spec itself, which is dropped.
export function parseStoredState(state: unknown): {
  tab?: Tab;
  open: string[];
  search: string;
  filters: FilterStatus[];
  folder?: string;
  runsFolder?: string;
} {
  const out = {
    open: [] as string[],
    search: "",
    filters: [] as FilterStatus[],
  };
  if (!state || typeof state !== "object") return out;
  const s = state as Record<string, unknown>;
  const strings = (v: unknown) =>
    Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
  const scope = s.tab ?? s.scope;
  return {
    tab: scope === "spec" ? "now" : isTab(scope) ? scope : undefined,
    open: strings(s.open),
    search: typeof s.search === "string" ? s.search : "",
    filters: strings(s.filters).filter((f): f is FilterStatus =>
      FILTER_STATUSES.includes(f as FilterStatus),
    ),
    folder: typeof s.folder === "string" ? s.folder : undefined,
    runsFolder: typeof s.runsFolder === "string" ? s.runsFolder : undefined,
  };
}

// currentBranch names the checked-out branch, undefined when detached.
function currentBranch(dir: string): Promise<string | undefined> {
  return new Promise((resolve) =>
    execFile(
      "git",
      ["rev-parse", "--abbrev-ref", "HEAD"],
      { cwd: dir, timeout: 15_000 },
      (err, stdout) => {
        const b = stdout?.trim();
        resolve(err || !b || b === "HEAD" ? undefined : b);
      },
    ),
  );
}

export class PanelHost implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  private disposables: vscode.Disposable[] = [];
  private extensionUri: vscode.Uri | undefined;
  // The document on show, and its revision: a new one replaces opened rows.
  private lastSpecData: SpecData | undefined;
  private rev = 0;
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
  private branch: string | undefined;
  // Runs merged since the panel last rendered them.
  private stale = false;
  // Why the last render failed; cleared by the next one that works.
  private renderError: string | undefined;
  private rendering: Promise<void> = Promise.resolve();
  private renderBusy = 0;
  private actions: PanelActions | undefined;
  private change: ChangeScope | undefined;
  private runsScope: RunsScope | undefined;
  // What the webview shows and asked for.
  private tab: Tab = "now";
  private query = "";
  private readonly filters = new Set<FilterStatus>();
  private readonly sent = new Set<Tab>();
  private readonly poster: Poster;
  // Runs that ended since the last render: their counts stay on show until
  // the render that includes them.
  private ended: RunProgress[] = [];
  private runningTimer: ReturnType<typeof setInterval> | undefined;
  // Folders whose run records were asked for a flaky hint.
  private readonly flakyAsked = new Set<string>();
  // Per source on show ("" for the workspace's runs), each package's
  // freshness as the CLI last judged it.
  private readonly freshness = new Map<string, Map<string, Freshness>>();
  private readonly freshPending = new Set<string>();
  private freshTimer: ReturnType<typeof setTimeout> | undefined;
  private readonly freshChecks = new Map<
    string,
    { child?: ManagedChild; gen: number }
  >();
  // Each folder's document as the last render drew it.
  private folderDocs = new Map<string, SpecData>();

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
    // The window's runs in progress, and its CLI sessions.
    private readonly live: LiveRuns = liveRuns,
    private readonly sessionList: Sessions = windowSessions,
  ) {
    this.persisted = new JsonStore(storageDir, RUNS_FILE, RUNS_VERSION);
    this.poster = new Poster(
      {
        visible: () => this.isVisible(),
        tab: () => this.tab,
        post: (m) => this.post(m),
      },
      (ch) => this.build(ch),
    );
    this.disposables.push(live.onDidChange((run) => this.liveChanged(run)));
    this.disposables.push(
      this.sessionList.onDidChange(() => this.sessionsChanged()),
    );
    if (cache) {
      this.disposables.push(
        cache.onDidUpdate(() => {
          this.pruneUndiscovered();
          if (this.unranFolders().length > 0) this.stale = true;
          if (this.stale && this.isVisible() && this.autoRefresh())
            void this.render();
          else this.markNow();
        }),
      );
    }
  }

  setChangeScope(change: ChangeScope): void {
    this.change = change;
  }

  // changeChanged tells the Change tab, and the badge, what the scope holds.
  changeChanged(): void {
    this.poster.mark("change", "badges");
    this.busy("change", !!this.change?.view().busy);
  }

  setRunsScope(runs: RunsScope): void {
    this.runsScope = runs;
  }

  // runsChanged tells the Runs tab; Change words its evidence from the runs
  // and Now its flaky hints, so they are told too.
  runsChanged(): void {
    this.poster.mark("runs", "change", "now");
    const f = this.runsScope?.folder;
    this.busy("runs", !!f && this.runsScope!.view().busy);
  }

  // ensureScope has the tab on show fetch what it lacks.
  private ensureScope(): void {
    if (this.tab === "change") this.change?.ensure();
    else if (this.tab === "runs") this.runsScope?.ensure();
  }

  private isVisible(): boolean {
    return !!this.panel && this.panel.visible !== false;
  }

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
      for (const pkg of known) {
        const declared = declaredTests(pkg);
        if (declared) removed += this.runs.pruneTests(pkg.importPath, declared);
      }
    }
    if (removed === 0) return;
    this.persist();
    this.stale = true;
    if (this.isVisible() && this.autoRefresh()) void this.render();
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

  setExtensionUri(uri: vscode.Uri): void {
    this.extensionUri = uri;
  }

  // setActions connects Run and Debug to the Test Explorer.
  setActions(actions: PanelActions): void {
    this.actions = actions;
  }

  // show reveals the panel, on the given tab.
  async show(scope?: Scope): Promise<void> {
    if (scope) this.tab = toTab(scope);
    if (!this.lastSpecData && this.unranFolders().length > 0) this.stale = true;
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.Beside);
      if (scope) {
        this.post({ type: "tab", tab: this.tab });
        this.tabShown();
      }
      this.ensureScope();
      if (this.stale) await this.render();
      return;
    }
    this.createPanel();
    await this.resolveModulePaths();
    this.ensureScope();
    if (this.stale) await this.render();
    else this.markNow();
  }

  showChange(): Promise<void> {
    return this.show("change");
  }

  showRuns(): Promise<void> {
    return this.show("runs");
  }

  async restorePanel(
    panel: vscode.WebviewPanel,
    state: unknown,
  ): Promise<void> {
    this.panel = panel;
    this.setIcon();
    this.wirePanel();
    const stored = parseStoredState(state);
    if (stored.tab) this.tab = stored.tab;
    this.query = stored.search;
    for (const f of stored.filters) this.filters.add(f);
    if (stored.folder) this.change?.select(stored.folder);
    if (stored.runsFolder) this.runsScope?.select(stored.runsFolder);
    this.setHtml();
    await this.resolveModulePaths();
    if (!this.lastSpecData && this.unranFolders().length > 0) this.stale = true;
    this.ensureScope();
    if (this.stale && this.autoRefresh()) await this.render();
  }

  private setIcon(): void {
    if (!this.panel || !this.extensionUri) return;
    this.panel.iconPath = vscode.Uri.joinPath(
      this.extensionUri,
      "static",
      "spec-icon.svg",
    );
  }

  private createPanel(): void {
    const localResourceRoots: vscode.Uri[] = [];
    if (this.extensionUri)
      localResourceRoots.push(vscode.Uri.joinPath(this.extensionUri, "static"));
    this.panel = vscode.window.createWebviewPanel(
      "gotestPanel",
      "Go Test",
      vscode.ViewColumn.Beside,
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots,
      },
    );
    this.setIcon();
    this.wirePanel();
    this.setHtml();
  }

  // setHtml writes the webview's document: once per panel.
  private setHtml(): void {
    if (!this.panel) return;
    this.sent.clear();
    this.panel.webview.html = buildShell({
      nonce: getNonce(),
      cspSource: this.panel.webview.cspSource,
      tab: this.tab,
      has: { change: !!this.change, runs: !!this.runsScope },
    });
  }

  private wirePanel(): void {
    this.panel!.webview.onDidReceiveMessage(
      (msg) => this.handleMessage(msg),
      null,
      this.disposables,
    );
    // A hidden panel is posted nothing; it catches up when it is seen.
    this.panel!.onDidChangeViewState?.(
      (e) => {
        if (!e.webviewPanel.visible) return;
        this.ensureScope();
        this.checkFreshSoon([], 0);
        if (this.stale && this.autoRefresh()) void this.render();
        this.sessionsChanged();
        this.poster.seen();
      },
      null,
      this.disposables,
    );
    this.panel!.onDidDispose(
      () => {
        this.panel = undefined;
        this.stopRunningTick();
      },
      null,
      this.disposables,
    );
  }

  private post(msg: HostMessage): void {
    if (!this.panel || !this.isVisible()) return;
    if (msg.type === "now" || msg.type === "change" || msg.type === "runs")
      this.sent.add(msg.type);
    void this.panel.webview.postMessage?.(msg);
  }

  private busy(tab: Tab, busy: boolean): void {
    this.post({ type: "busy", tab, busy });
  }

  // build is a channel's message, built when it is posted.
  private build(ch: Channel): HostMessage | undefined {
    switch (ch) {
      case "now":
        return { type: "now", model: this.nowModel() };
      case "change":
        return this.change
          ? { type: "change", model: buildChangeTab(this.change.view()) }
          : undefined;
      case "runs":
        return this.runsScope
          ? { type: "runs", model: buildRunsTab(this.runsScope.view()) }
          : undefined;
      case "badges":
        return {
          type: "badges",
          now: this.failedCount(),
          change: this.change ? this.openDecisions() : 0,
        };
      case "running":
        return { type: "running", items: this.runningItems() };
    }
  }

  private openDecisions(): number {
    const v = this.change!.view();
    return v.state.kind === "doc" ? buildChangeTab(v).openCount : 0;
  }

  // failedCount is the Now badge: the document's failures and those a run
  // in progress reported since.
  private failedCount(): number {
    const s = this.lastSpecData?.stats;
    let n = s ? s.failed + (s.failedPackages ?? 0) : 0;
    for (const run of this.liveShown())
      for (const lp of run.packages.values()) n += liveFailures(lp).length;
    return n;
  }

  private liveShown(): RunProgress[] {
    if (this.selected) return [];
    return [...this.live.active(), ...this.ended];
  }

  private markNow(): void {
    this.poster.mark("now", "badges");
  }

  // tabShown answers the webview switching to a tab: it shows what it kept,
  // and gets the tab's model when the host has news or never sent one.
  private tabShown(): void {
    if (!this.sent.has(this.tab)) this.poster.mark(this.tab);
    this.ensureScope();
    if (this.tab === "now") {
      if (this.stale && this.autoRefresh()) void this.render();
      else this.checkFreshSoon([], 0);
    }
    if (this.tab === "runs") this.sessionsChanged();
    this.poster.seen();
  }

  nowModel(): NowModel {
    const source = this.selected ?? "";
    const unran = new Set(this.unranFolders());
    const doc = this.lastSpecData;
    const shownRun = doc?.runs?.[0] ?? doc?.run;
    const eventsFile = shownRun?.args
      ?.find((a) => a.startsWith("--events-file="))
      ?.slice("--events-file=".length);
    return buildNowModel({
      rev: this.rev,
      doc,
      error: this.renderError,
      modulePaths: this.modulePaths,
      dirOf: (p) => this.cache?.getPackage(p)?.dir,
      freshnessOf: (p) => this.freshness.get(source)?.get(p),
      ranAtOf: (p) => this.currentRuns().ranAtOf(p),
      neverRan: (p) => {
        if (this.selected || this.runs.folderOf(p)) return false;
        const dir = this.cache?.getWorkspaceDir(p);
        return dir === undefined || unran.has(dir);
      },
      now: Date.now(),
      branch: this.branch,
      sources: this.sourceOptions(),
      imported: this.selected
        ? this.imports.get(this.selected)?.label
        : undefined,
      live: this.liveShown().map((r) => ({
        line: r.active
          ? `Running ${r.label} · ${elapsedText(Date.now() - r.startedAt)} · ${tallyText(r.tally)}`
          : `Ran ${r.label} · ${tallyText(r.tally)} · rendering`,
        packages: r.packages,
      })),
      filters: this.filters,
      flakyOf: (pkg, test, hash) => this.flakyOf(pkg, test, hash, eventsFile),
    });
  }

  // flakyOf reads the hint from the folder's run records, asking for them
  // once when the Runs tab has not listed them yet.
  private flakyOf(
    pkg: string,
    test: string,
    hash: string | undefined,
    eventsFile: string | undefined,
  ): string | undefined {
    if (!this.runsScope || !hash || this.selected) return undefined;
    const folder = this.runs.folderOf(pkg) ?? this.cache?.getWorkspaceDir(pkg);
    if (!folder) return undefined;
    const doc = this.runsScope.doc(folder);
    if (!doc && !this.flakyAsked.has(folder)) {
      this.flakyAsked.add(folder);
      void this.runsScope.listed(folder);
    }
    return flakyHint(
      doc,
      pkg,
      test,
      hash,
      (r) => !!eventsFile && r.eventsFile === eventsFile,
    );
  }

  private sourceOptions(): SourceOption[] {
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

  // sendTree posts one package's rows as the chips leave them.
  private sendTree(pkg: string): void {
    const tree = packageTree(this.lastSpecData, pkg, {
      dirOf: (p) => this.cache?.getPackage(p)?.dir,
      query: "",
      filters: this.filters,
      stale: this.freshness.get(this.selected ?? "")?.get(pkg) === "stale",
    });
    this.post({
      type: "package",
      pkg,
      rev: this.rev,
      nodes: tree?.nodes ?? [],
      failure: tree?.failure,
    });
  }

  // sendFound answers a search with the rows it finds, bounded, in the
  // order the tab lists the packages.
  private sendFound(): void {
    const query = this.query.trim();
    const order = packagesIn(this.nowModel().tree);
    const stale = this.freshness.get(this.selected ?? "");
    const found = searchFound(this.lastSpecData, query, order, {
      dirOf: (p) => this.cache?.getPackage(p)?.dir,
      filters: this.filters,
      staleOf: (p) => stale?.get(p) === "stale",
    });
    this.post({ type: "found", query, rev: this.rev, ...found });
  }

  // The item ids above each row a button acted on: a behavior's name may
  // hold slashes, so its ancestors are never found by cutting its id.
  private readonly ancestry = new Map<string, string[]>();

  private noteAncestors(raw: Record<string, string[]>): void {
    for (const [id, list] of Object.entries(raw)) this.ancestry.set(id, list);
  }

  // A row to reveal once the webview is ready for it.
  private revealing: { pkg: string; pos: number[] } | undefined;

  // reveal shows a Test Explorer item's row in the Now tab: its package and
  // the rows above it opened, the row in view and focused.
  async reveal(itemId: string): Promise<boolean> {
    const existed = !!this.panel;
    await this.show("now");
    // An item the list does not hold shows as the nearest one it does.
    let at: { pkg: string; pos: number[] } | undefined;
    for (
      let id = itemId;
      id && !at;
      id = id.slice(0, Math.max(0, id.lastIndexOf("/")))
    )
      at = rowOfItem(this.lastSpecData, id);
    if (!at) {
      this.say(`${itemId} is not in the list yet: run it first.`);
      return false;
    }
    const pos = at.pos.join(".");
    // A panel just made gets it once its page is ready.
    if (existed) this.post({ type: "reveal", pkg: at.pkg, pos });
    else this.revealing = at;
    return true;
  }

  // ancestorsOf lists the item ids above a row the panel last acted on.
  ancestorsOf(id: string): string[] | undefined {
    return this.ancestry.get(id);
  }

  private posOf(raw: unknown): number[] | undefined {
    if (typeof raw !== "string") return undefined;
    if (raw === "") return [];
    const pos = raw.split(".").map(Number);
    return pos.every((i) => Number.isInteger(i) && i >= 0) ? pos : undefined;
  }

  // actOn names the item a row's button acts on, noting the ids above it.
  private actOn(msg: {
    id?: unknown;
    pkg?: unknown;
    pos?: unknown;
  }): string | undefined {
    const id = typeof msg.id === "string" ? msg.id : undefined;
    if (!id) return undefined;
    const pos = this.posOf(msg.pos);
    if (typeof msg.pkg === "string" && pos) {
      const at = nodeAt(this.lastSpecData, msg.pkg, pos);
      if (at && at.id === id) this.noteAncestors({ [id]: at.ancestors });
    }
    return id;
  }

  // The webview posts plain objects; each branch checks what it reads.
  private handleMessage(msg: any): void {
    if (!msg || typeof msg !== "object") return;
    switch (msg.type) {
      case "ready": {
        if (isTab(msg.tab)) this.tab = msg.tab;
        this.query = typeof msg.search === "string" ? msg.search : "";
        this.setFilters(msg.filters);
        this.sent.clear();
        this.poster.mark(this.tab, "badges");
        this.tabShown();
        if (this.revealing)
          this.post({
            type: "reveal",
            pkg: this.revealing.pkg,
            pos: this.revealing.pos.join("."),
          });
        this.revealing = undefined;
        return;
      }
      case "tab":
        if (!isTab(msg.tab)) return;
        this.tab = msg.tab;
        this.tabShown();
        return;
      case "openPackage":
        if (typeof msg.pkg !== "string") return;
        this.sendTree(msg.pkg);
        return;
      case "search":
        this.query = typeof msg.text === "string" ? msg.text : "";
        if (this.query.trim()) this.sendFound();
        return;
      case "filters":
        this.setFilters(msg.filters);
        this.markNow();
        return;
      case "run": {
        // A directory runs its packages; any other row its own item.
        const ids = Array.isArray(msg.ids)
          ? msg.ids.filter((i: unknown): i is string => typeof i === "string")
          : [this.actOn(msg)].filter((i): i is string => !!i);
        if (ids.length > 0 && this.actions) void this.actions.run(ids);
        return;
      }
      case "debug": {
        const id = this.actOn(msg);
        if (id && this.actions) void this.actions.debug(id);
        return;
      }
      case "rerunFailed": {
        const f = failedIds(this.lastSpecData);
        this.noteAncestors(f.ancestors);
        if (f.ids.length > 0 && this.actions) void this.actions.run(f.ids);
        return;
      }
      case "output": {
        const pos = this.posOf(msg.pos ?? "");
        if (typeof msg.pkg !== "string" || typeof msg.key !== "string" || !pos)
          return;
        const out = fullOutput(
          this.lastSpecData,
          msg.pkg,
          pos,
          (p) => this.cache?.getPackage(p)?.dir,
        );
        this.post({
          type: "output",
          key: msg.key,
          lines: out?.lines ?? [],
          more: out?.more,
        });
        return;
      }
      case "selectSource":
        this.select(typeof msg.id === "string" ? msg.id : undefined);
        return;
      case "runTests":
        void vscode.commands.executeCommand("testing.runAll");
        return;
      case "runChanged":
        void vscode.commands.executeCommand("gotest.runChanged");
        return;
      case "refresh":
        this.stale = true;
        void this.render();
        return;
      case "copySpec":
        void this.copy(this.query);
        return;
      case "copy": {
        const pkgs = Array.isArray(msg.pkgs)
          ? msg.pkgs.filter((p: unknown): p is string => typeof p === "string")
          : [];
        const pos = this.posOf(msg.pos);
        const label = typeof msg.label === "string" ? msg.label : "";
        if (typeof msg.pkg === "string" && pos && pos.length > 0)
          void this.copyScope(label, [msg.pkg], { pkg: msg.pkg, pos });
        else if (pkgs.length > 0) void this.copyScope(label, pkgs);
        return;
      }
      case "change":
        if (
          msg.act === "changePackage" &&
          typeof msg.key === "string" &&
          this.change
        ) {
          this.post({
            type: "changePackage",
            pkg: msg.key,
            rows: changePackageRows(this.change.view(), msg.key),
          });
          return;
        }
        void this.change?.act(msg);
        return;
      case "runs":
        if (msg.act === "open") this.tab = "now";
        void this.runsScope?.act(msg);
        return;
      case "stopSession":
        if (typeof msg.id === "number") this.sessionList.stop(msg.id);
        return;
      case "clearResults":
        this.clear();
        return;
      case "goToLocation":
        if (
          typeof msg.file === "string" &&
          typeof msg.line === "number" &&
          msg.line > 0
        )
          this.goTo(msg.file, msg.line);
        return;
    }
  }

  private setFilters(raw: unknown): void {
    this.filters.clear();
    for (const f of Array.isArray(raw) ? raw : [])
      if (FILTER_STATUSES.includes(f as FilterStatus))
        this.filters.add(f as FilterStatus);
  }

  private goTo(file: string, lineNo: number): void {
    const uri = vscode.Uri.file(file);
    const line = Math.max(0, lineNo - 1);
    const own = this.panel?.viewColumn;
    const other = vscode.window.visibleTextEditors.find(
      (e) => e.viewColumn !== undefined && e.viewColumn !== own,
    );
    const viewColumn =
      other?.viewColumn ??
      (own === vscode.ViewColumn.One
        ? vscode.ViewColumn.Beside
        : vscode.ViewColumn.One);
    vscode.window
      .showTextDocument(uri, {
        viewColumn,
        selection: new vscode.Range(line, 0, line, 0),
      })
      .then(undefined, (err: unknown) => {
        this.outputChannel.error(`[panel] showTextDocument failed: ${err}`);
      });
  }

  // clear drops the kept runs and empties the Now tab. Called both by the
  // panel's own Clear and when the Test Explorer's results are cleared, so
  // the two surfaces cannot disagree about what has been run.
  clear(): void {
    this.lastSpecData = undefined;
    this.rev++;
    this.runs.clear();
    this.imports.clear();
    this.freshness.clear();
    this.selected = undefined;
    this.stale = false;
    this.renderError = undefined;
    this.ended = [];
    this.persist();
    if (this.unranFolders().length > 0) this.stale = true;
    this.markNow();
    if (this.stale && this.isVisible()) void this.render();
  }

  // refresh folds finished runs into what the panel keeps and renders the
  // result: every package seen, each test with its newest verdict.
  async refresh(pieces: RunPiece | RunPiece[]): Promise<void> {
    const list = Array.isArray(pieces) ? pieces : [pieces];
    // What a run just judged is the tree's until the CLI says otherwise.
    const own = this.freshness.get("");
    for (const piece of list)
      for (const pkg of this.runs.merge(piece)) own?.delete(pkg);
    this.change?.runFinished(
      list.map((p) => p.folder),
      this.isVisible() && this.tab === "change",
    );
    this.runsScope?.invalidate(
      list.map((p) => p.folder),
      this.isVisible() && this.tab === "runs",
    );
    this.persist();
    // The newest change is what the tab shows.
    this.selected = undefined;
    this.stale = true;
    if (!this.isVisible() || !this.autoRefresh()) return;
    await this.render();
  }

  // openStream shows a saved event stream as a source of its own, beside the
  // workspace's runs and never merged into them: it is evidence about
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
    await this.show(scope ?? "now");
  }

  // sourceChanged asks again, once a burst of edits settles, whether the
  // verdicts on show still belong to the folder's tree.
  sourceChanged(folder: string): void {
    this.runsScope?.invalidate([folder], false);
    if (!this.renderFolders().includes(folder)) return;
    this.checkFreshSoon([folder], FRESH_DEBOUNCE_MS);
  }

  // checkFreshSoon gathers folders and checks them after delay, while the
  // Now tab is on show; otherwise when it is seen again.
  private checkFreshSoon(folders: string[], delay: number): void {
    for (const f of folders) this.freshPending.add(f);
    if (this.freshPending.size === 0) return;
    clearTimeout(this.freshTimer);
    this.freshTimer = setTimeout(() => {
      this.freshTimer = undefined;
      if (!this.isVisible() || this.tab !== "now") return;
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
      onFinished: (line) => this.outputChannel.info(`[fresh] ${line}`),
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
    this.markNow();
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

  private autoRefresh(): boolean {
    return (
      vscode.workspace
        .getConfiguration("gotest")
        .get<boolean>("panel.autoRefresh") ?? true
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
    this.renderBusy++;
    this.busy("now", true);
    const ended = this.ended.length;
    try {
      const data = await this.renderRuns();
      this.lastSpecData = data;
      this.rev++;
      this.renderError = undefined;
      // The render holds what the runs that ended before it streamed.
      this.ended = this.ended.slice(ended);
      await this.resolveModulePaths();
      const folder = this.renderFolders()[0];
      if (folder) this.branch = await currentBranch(folder);
      this.markNow();
      this.checkFreshSoon(this.renderFolders(), 0);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[panel] ${message}`);
      // The last picture stays, under a notice; the next reveal retries.
      this.renderError = message;
      this.stale = true;
      this.markNow();
    } finally {
      this.renderBusy--;
      if (this.renderBusy === 0) this.busy("now", false);
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
    return docs.size > 0
      ? combineSpecData(Array.from(docs.values()))
      : undefined;
  }

  // copy puts on the clipboard what the tab shows, as the CLI writes it in
  // Markdown: the same streams, rendered with the same flags. A search keeps
  // the rows the search keeps and the chips hide what they leave out; a node
  // is rendered alone, by the CLI of the folder its package belongs to.
  private async copy(
    search = "",
    node?: { pkg: string; position: number[] },
  ): Promise<void> {
    const runs = this.currentRuns();
    const parts: string[] = [];
    const chosen = HIDEABLE.filter((s) => this.filters.has(s));
    const hide =
      node || chosen.length === 0
        ? []
        : HIDEABLE.filter((s) => !this.filters.has(s));
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
        for (const folder of this.renderFolders())
          parts.push(
            await this.runSpec(
              runs.compose(folder),
              folder,
              "md",
              extra,
              undefined,
              hide,
            ),
          );
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[panel] copy: ${message}`);
      return;
    }
    if (parts.length === 0) return;
    const text = parts.join("\n");
    await vscode.env.clipboard.writeText(text);
    const tokens = Math.round(text.length / 4);
    this.say(
      `Copied the whole list as Markdown: ${text.length} characters (about ${tokens} tokens)${this.narrowed() ? ". Filters apply" : ""}.` +
        (text.length > GITHUB_COMMENT_LIMIT
          ? " It is above GitHub's 65,536-character comment limit."
          : ""),
    );
  }

  private narrowed(): boolean {
    return this.filters.size > 0 || this.query.trim() !== "";
  }

  // say shows a short line in the panel, which goes by itself.
  private say(text: string): void {
    this.post({ type: "toast", text });
  }

  // copyScope copies packages, or one row of a package, as the CLI writes
  // them in Markdown: the stream holds those packages only, the source read
  // under it is theirs, and the chips and the search narrow it as they
  // narrow the whole list. A row is found again by its names in what the
  // CLI shows under them, since --only counts the shown rows.
  private async copyScope(
    label: string,
    pkgs: string[],
    node?: { pkg: string; pos: number[] },
  ): Promise<void> {
    const runs = this.currentRuns();
    const byFolder = new Map<string, string[]>();
    for (const p of pkgs) {
      const f = runs.folderOf(p);
      if (f !== undefined) byFolder.set(f, [...(byFolder.get(f) ?? []), p]);
    }
    const chosen = HIDEABLE.filter((s) => this.filters.has(s));
    const hide =
      chosen.length === 0 ? [] : HIDEABLE.filter((s) => !this.filters.has(s));
    const match = this.query.trim();
    const extra = match ? [`--match=${match}`] : [];
    const names = node
      ? namesAt(this.lastSpecData, node.pkg, node.pos)
      : undefined;
    const parts: string[] = [];
    let behaviors = 0;
    try {
      for (const [folder, only] of byFolder) {
        const stream = runs.compose(folder, new Set(only));
        const shown = JSON.parse(
          await this.runSpec(
            stream,
            folder,
            "json",
            extra,
            undefined,
            hide,
            only,
          ),
        ) as SpecData;
        if (node) {
          const at = names && findByNames(shown, node.pkg, names);
          if (!at) continue;
          behaviors += leavesUnder(at.node);
          parts.push(
            await this.runSpec(
              stream,
              folder,
              "md",
              [...extra, `--only=${node.pkg}:${at.pos.join(".")}`],
              undefined,
              hide,
              only,
            ),
          );
        } else {
          for (const p of shown.packages)
            behaviors += p.nodes.reduce((n, x) => n + leavesUnder(x), 0);
          parts.push(
            await this.runSpec(
              stream,
              folder,
              "md",
              extra,
              undefined,
              hide,
              only,
            ),
          );
        }
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.error(`[panel] copy: ${message}`);
      this.say(`Could not copy ${label}: ${message.split("\n")[0]}`);
      return;
    }
    if (parts.length === 0) {
      this.say(`Nothing of ${label} to copy: the filters leave none of it.`);
      return;
    }
    const text = parts.join("\n");
    await vscode.env.clipboard.writeText(text);
    this.say(
      `Copied ${label} as Markdown: ${countNoun(behaviors, "behavior")}${this.narrowed() ? ". Filters apply" : ""}.` +
        (text.length > GITHUB_COMMENT_LIMIT
          ? " It is above GitHub's 65,536-character comment limit."
          : ""),
    );
  }

  // specArgs are the CLI's flags for a folder's render: the declared
  // specification under the workspace's runs, never under an imported
  // stream. --hide is always given, so every package carries the CLI's
  // counts.
  private async specArgs(
    folder: string,
    format: string,
    extra: string[] = [],
    hide: string[] = [],
    // The packages a render is about; the source read is theirs then.
    only?: string[],
  ): Promise<string[]> {
    const args = [
      "spec",
      "--input=-",
      `--format=${format}`,
      // A renderer, not a CI gate: without it the exit code also reports
      // whether the tests passed, which says nothing about the rendering.
      "--render-only",
      `--hide=${[...hide].sort().join(",")}`,
    ];
    const patterns = this.selected
      ? undefined
      : await this.patternsFor?.(folder);
    // --declared takes the rest of the line, so the flags come before it.
    args.push(...extra);
    if (patterns && patterns.length > 0)
      args.push("--declared", ...(only ?? patterns));
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
    this.poster.dispose();
    this.stopRunningTick();
    clearTimeout(this.freshTimer);
    for (const c of this.freshChecks.values())
      void c.child?.terminate("prompt");
    this.change?.dispose();
    this.panel?.dispose();
    for (const d of this.disposables) d.dispose();
    this.disposables = [];
  }

  // runSpec has the folder's CLI render a composed stream in format.
  private async runSpec(
    jsonInput: string,
    folder: string | undefined,
    format: string,
    extra: string[] = [],
    // Told of the child, so a superseded render can be stopped.
    track?: Set<ManagedChild>,
    hide: string[] = [],
    only?: string[],
  ): Promise<string> {
    const workspaceDir =
      folder ?? vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    let cmd;
    try {
      cmd = await buildCliCommand(
        await this.specArgs(workspaceDir ?? "", format, extra, hide, only),
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
      onFinished: (line) => this.outputChannel.info(`[spec] ${line}`),
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
    // Rendering is a read: it has no fixtures to tear down, so it gets a
    // budget and the prompt escalation.
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
      if (timedOut)
        throw new Error(`spec render timed out after ${budgetSeconds}s`);
      const outcome = interpretSpecExit(code, stdout, stderr);
      if (!outcome.ok) throw new Error(outcome.message);
      return outcome.stdout;
    } finally {
      clearTimeout(budget);
      track?.delete(mc);
      mc.dispose();
    }
  }

  // liveChanged follows a run in progress: the Now tab shows what its events
  // said so far, coalesced; the CLI renders it once, when it ends.
  private liveChanged(run: RunProgress): void {
    if (!run.active && !this.selected && !this.ended.includes(run))
      this.ended.push(run);
    this.markNow();
  }

  // --- the Running block ---

  private runningItems(): RunningItem[] {
    const now = Date.now();
    return this.sessionList.list().map((s) => ({
      id: s.id,
      label: `${s.kind}: ${s.target}`,
      elapsed: elapsedText(now - s.startedAt),
      counts: s.progress ? tallyText(s.progress.tally) : undefined,
      stopping: s.stopping,
    }));
  }

  private sessionsChanged(): void {
    this.poster.mark("running");
    const any = this.sessionList.list().length > 0;
    if (any && this.isVisible()) this.startRunningTick();
    else if (!any) this.stopRunningTick();
  }

  private startRunningTick(): void {
    if (this.runningTimer) return;
    this.runningTimer = setInterval(() => {
      if (this.sessionList.list().length === 0 || !this.panel) {
        this.stopRunningTick();
        this.poster.mark("running");
        return;
      }
      this.poster.mark("running");
      if (this.live.active().length > 0) this.markNow();
    }, RUNNING_TICK_MS);
    this.runningTimer.unref?.();
  }

  private stopRunningTick(): void {
    clearInterval(this.runningTimer);
    this.runningTimer = undefined;
  }
}
