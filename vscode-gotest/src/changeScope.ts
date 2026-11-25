// The Change scope of the Spec View panel: per workspace folder, the audit
import { sessions } from "./sessions.js";
// of the working tree against a base, as `gotest audit --format=json`
// prints it. The CLI computes the document; this runs it, keeps it, and
// wires its rows to the editor.
import * as vscode from "vscode";
import * as path from "node:path";
import { existsSync } from "node:fs";
import {
  AUDIT_CLI_VERSION,
  buildCliCommand,
  cliPackagePath,
  offerCliUpgrade,
  pinnedBelow,
  queryCliCapabilities,
  stripGoRunExitEcho,
  type CliBelowFloorError,
} from "./cli.js";
import { auditTimeoutSeconds } from "./config.js";
import { ManagedChild } from "./managedChild.js";
import {
  AUDIT_SCHEMA,
  buildChangeModel,
  parseAuditDocument,
  runsOf,
  targetsOf,
  type AuditDocument,
  type AuditLocation,
  type RunTarget,
} from "./auditModel.js";
import type { ChangeState, ChangeView } from "./changeView.js";
import { AcceptanceStore, type Memento } from "./acceptance.js";
import type { EvidenceSelection } from "./evidence.js";
import {
  BASE_SCHEME,
  baseQuery,
  defaultBranch,
  gitToplevel,
  listRefs,
  mergeBase,
} from "./git.js";

const BASE_KEY = "gotest.audit.base";

// A base as the user chose it: a ref, or the merge-base of HEAD with one.
export interface BaseChoice {
  ref: string;
  mergeBase: boolean;
}

export interface ChangeScopeDeps {
  log: vscode.LogOutputChannel;
  memento: Memento;
  // The folders that use gotest; the scope audits one of them.
  folders: () => string[];
  patternsFor?: (folder: string) => Promise<string[] | undefined>;
  // Whether the Test Explorer holds a package, and running a -run pattern
  // in it through the extension's runner.
  hasPackage?: (pkg: string) => boolean;
  runTest?: (pkg: string, run: string) => Promise<void>;
  // The runs chosen in the Runs scope; their events files are the audit's
  // --evidence.
  evidence?: EvidenceSelection;
  // Called whenever what the scope shows changed.
  changed: () => void;
}

export type AuditOutcome =
  | { ok: true; doc: AuditDocument; exitCode: number | null }
  | { ok: false; message: string };

// interpretAuditExit reads a finished audit. A document on stdout stands
// whatever the exit code: 2 still writes one for a package it could not
// read, 1 for a policy. Without one, stderr says why.
export function interpretAuditExit(
  code: number | null,
  stdout: string,
  stderr: string,
): AuditOutcome {
  const doc = parseAuditDocument(stdout);
  if (doc) return { ok: true, doc, exitCode: code };
  const detail = stripGoRunExitEcho(stderr);
  return {
    ok: false,
    message: detail || `gotest audit exited with code ${code}`,
  };
}

// One side of a target placed on disk: the file in the working tree, its
// path in the checkout for the base's copy, and its lines.
interface PlacedSide {
  abs: string;
  rel: string;
  loc: AuditLocation;
}

interface Placed {
  base?: PlacedSide;
  head?: PlacedSide;
}

// An audit's kind: from source alone, or with a run of the change.
export type AuditMode = "static" | "run";

interface FolderState {
  state: ChangeState;
  busy: false | AuditMode;
  // An audit asked for while one runs; a run outweighs a plain one.
  again?: AuditMode;
  // A run in the folder finished since the last audit.
  stale: boolean;
  child?: ManagedChild;
  cancelled: boolean;
  notice?: string;
  doc?: AuditDocument;
  exitCode?: number | null;
  baseRef?: string;
  baseLabel?: string;
  toplevel?: string;
  targets: Map<string, Placed>;
  runs: Map<string, RunTarget>;
  cliOk: boolean;
  upgrade?: CliBelowFloorError;
  // Waiting for the folder's audit slot.
  waiters: (() => void)[];
}

// How much of a run audit's stderr is kept for its complaint: the run's
// own output goes there too.
const STDERR_KEEP = 64 * 1024;

export class ChangeScope implements vscode.Disposable {
  private selected: string | undefined;
  private readonly folders = new Map<string, FolderState>();
  private readonly acceptance: AcceptanceStore;
  private disposed = false;

  constructor(private readonly deps: ChangeScopeDeps) {
    this.acceptance = new AcceptanceStore(deps.memento);
  }

  // folder is the one the scope shows: the one chosen, while it still uses
  // gotest, else the first that does.
  get folder(): string | undefined {
    const all = this.deps.folders();
    if (this.selected && all.includes(this.selected)) return this.selected;
    return all[0];
  }

  select(folder: string | undefined): void {
    if (folder) this.selected = folder;
  }

  private stateOf(folder: string): FolderState {
    let fs = this.folders.get(folder);
    if (!fs) {
      fs = {
        state: { kind: "idle" },
        busy: false,
        stale: false,
        cancelled: false,
        targets: new Map(),
        runs: new Map(),
        cliOk: false,
        waiters: [],
      };
      this.folders.set(folder, fs);
    }
    return fs;
  }

  // ensure audits the shown folder when it has nothing yet, or a run
  // finished in it since.
  ensure(): void {
    const folder = this.folder;
    if (!folder) return;
    const fs = this.stateOf(folder);
    if ((fs.state.kind === "idle" || fs.stale) && !fs.busy) {
      void this.audit(folder);
    }
  }

  // runFinished audits again a folder whose tests just ran, when the scope
  // is on show; otherwise the next look at it does.
  runFinished(folders: string[], showing: boolean): void {
    for (const folder of new Set(folders)) {
      const fs = this.folders.get(folder);
      if (!fs) continue;
      if (showing && folder === this.folder) void this.audit(folder);
      else fs.stale = true;
    }
  }

  view(): ChangeView {
    const folder = this.folder;
    const all = this.deps.folders();
    const folders = all.map((f) => ({
      path: f,
      label: path.basename(f),
      selected: f === folder,
    }));
    const none: ChangeView = {
      state: { kind: "idle" },
      busy: false,
      folders,
      evidence: [],
      canRun: () => false,
      canOpen: () => false,
    };
    if (!folder) return none;
    const fs = this.stateOf(folder);
    let state = fs.state;
    if (state.kind === "doc" && fs.doc) {
      const accepted = this.acceptance.accepted(
        folder,
        fs.doc.head.commit ?? "",
      );
      state = {
        kind: "doc",
        model: buildChangeModel(fs.doc, (k) => accepted.has(k)),
        exitCode: fs.exitCode ?? null,
      };
    }
    return {
      state,
      busy: fs.busy,
      folders,
      evidence: (this.deps.evidence?.list(folder) ?? []).map((p) => ({
        id: p.id,
        label: p.label,
        missing: !existsSync(p.file),
      })),
      baseLabel: fs.baseLabel,
      notice: fs.notice,
      canOpen: (key) => fs.targets.has(key),
      canRun: (key) => {
        const r = fs.runs.get(key);
        return !!r && !!this.deps.hasPackage?.(r.package);
      },
    };
  }

  // audit runs the folder's audit, one at a time: asked again while one
  // runs, it runs once more after it, with a run if either asked for one.
  async audit(folder: string, mode: AuditMode = "static"): Promise<void> {
    if (this.disposed) return;
    const fs = this.stateOf(folder);
    if (fs.busy) {
      fs.again = fs.again === "run" ? "run" : mode;
      return;
    }
    fs.busy = mode;
    fs.stale = false;
    fs.cancelled = false;
    fs.notice = undefined;
    this.deps.changed();
    const session = sessions.add({
      kind: mode === "run" ? "audit --run" : "audit",
      folder,
      target: fs.baseLabel ? `against ${fs.baseLabel}` : "the working tree",
      stop: () => this.cancel(folder),
    });
    try {
      await this.auditNow(folder, fs, mode);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      if (fs.cancelled) {
        fs.notice = fs.doc
          ? "The audit was stopped; this is the last one that finished."
          : "The audit was stopped.";
        this.deps.log.info(`[change] stopped: ${message}`);
      } else {
        this.deps.log.error(`[change] ${message}`);
        fs.state = { kind: "error", message };
      }
    } finally {
      session.dispose();
      fs.busy = false;
      fs.child = undefined;
      if (!this.disposed) this.deps.changed();
    }
    const again = fs.again;
    fs.again = undefined;
    if (again && !this.disposed && !fs.cancelled) {
      await this.audit(folder, again);
      return;
    }
    for (const w of fs.waiters.splice(0)) w();
  }

  // idle resolves once no audit of the folder runs.
  private idle(folder: string): Promise<void> {
    const fs = this.stateOf(folder);
    if (!fs.busy) return Promise.resolve();
    return new Promise((resolve) => fs.waiters.push(resolve));
  }

  // auditMarkdown is the folder's audit as the CLI writes it in Markdown,
  // for a language model tool. It takes the folder's one audit slot, as
  // the scope's own audits do, and the same base, patterns and evidence;
  // base names another ref. What the CLI cannot do is said in plain words.
  async auditMarkdown(
    folder: string,
    opts: { base?: string; run?: boolean },
  ): Promise<string> {
    const mode: AuditMode = opts.run ? "run" : "static";
    let fs = this.stateOf(folder);
    while (fs.busy) {
      await this.idle(folder);
      fs = this.stateOf(folder);
    }
    fs.busy = mode;
    fs.cancelled = false;
    this.deps.changed();
    const session = sessions.add({
      kind: opts.run ? "audit --run" : "audit",
      folder,
      target: "for a language model tool",
      stop: () => this.cancel(folder),
    });
    try {
      if (!(await gitToplevel(folder)))
        return "This folder is not in a git checkout, so there is no base to audit against.";
      const ref = opts.base ?? (await this.resolveBase(folder))?.ref;
      if (!ref)
        return "No base to audit against: the checkout has no remote default branch. Name a base ref.";
      if (!fs.cliOk && !(await this.checkCli(folder, fs))) {
        const st = fs.state as ChangeState;
        if (st.kind === "oldCli")
          return `This folder's gotest CLI${st.version ? ` (${st.version})` : ""} has no audit command; it needs ${st.floor}. ${st.advice}`;
        if (st.kind === "newSchema")
          return `This folder's CLI writes audit schema ${st.schema}; this extension reads ${st.supported}. Update the extension.`;
        return "This folder's gotest CLI cannot audit.";
      }
      const patterns = (await this.deps.patternsFor?.(folder)) ?? [];
      const args = ["audit", `--base=${ref}`, "--format=md"];
      if (opts.run) args.push("--run");
      args.push(...this.evidenceArgs(folder), ...patterns);
      let cmd;
      try {
        cmd = await buildCliCommand(args, folder, this.deps.log);
      } catch (err: unknown) {
        offerCliUpgrade(err);
        return err instanceof Error ? err.message : String(err);
      }
      this.deps.log.info(
        `[change] ${cmd.bin} ${cmd.args.join(" ")} (cwd ${folder}, tool)`,
      );
      const out = await this.spawnAudit(folder, fs, cmd.bin, cmd.args, mode);
      if (out.stdout.trim()) return out.stdout;
      return (
        stripGoRunExitEcho(out.stderr) ||
        `gotest audit exited with code ${out.code}`
      );
    } catch (err: unknown) {
      return err instanceof Error ? err.message : String(err);
    } finally {
      session.dispose();
      fs.busy = false;
      fs.child = undefined;
      // The scope's document may now be older than the tree's.
      if (opts.run) fs.stale = true;
      this.release(folder, fs);
    }
  }

  // release hands the slot to an audit asked for meanwhile, or to whoever
  // waits for the folder to be idle.
  private release(folder: string, fs: FolderState): void {
    if (this.disposed) return;
    this.deps.changed();
    const again = fs.again;
    fs.again = undefined;
    if (again && !fs.cancelled) {
      void this.audit(folder, again);
      return;
    }
    for (const w of fs.waiters.splice(0)) w();
  }

  // cancel stops the folder's audit, and the run it started.
  cancel(folder: string): void {
    const fs = this.folders.get(folder);
    if (!fs?.busy) return;
    fs.cancelled = true;
    fs.again = undefined;
    void fs.child?.terminate(fs.busy === "run" ? "teardown" : "prompt");
  }

  private async auditNow(
    folder: string,
    fs: FolderState,
    mode: AuditMode,
  ): Promise<void> {
    fs.toplevel = await gitToplevel(folder);
    if (!fs.toplevel) {
      fs.state = { kind: "notGit" };
      return;
    }
    const base = await this.resolveBase(folder);
    if (!base) {
      fs.baseLabel = undefined;
      fs.state = { kind: "noBase" };
      return;
    }
    fs.baseRef = base.ref;
    fs.baseLabel = base.label;

    if (!fs.cliOk && !(await this.checkCli(folder, fs))) return;

    const patterns = (await this.deps.patternsFor?.(folder)) ?? [];
    const args = ["audit", `--base=${base.ref}`, "--format=json"];
    if (mode === "run") args.push("--run");
    args.push(...this.evidenceArgs(folder));
    let cmd;
    try {
      cmd = await buildCliCommand(
        [...args, ...patterns],
        folder,
        this.deps.log,
      );
    } catch (err: unknown) {
      offerCliUpgrade(err);
      throw err;
    }
    this.deps.log.info(
      `[change] ${cmd.bin} ${cmd.args.join(" ")} (cwd ${folder})`,
    );
    const outcome = await this.runChild(folder, fs, cmd.bin, cmd.args, mode);
    if (!outcome.ok) {
      fs.state = { kind: "error", message: outcome.message };
      return;
    }
    fs.doc = outcome.doc;
    fs.exitCode = outcome.exitCode;
    const model = buildChangeModel(outcome.doc);
    fs.targets = this.place(folder, fs.toplevel, outcome.doc, model);
    fs.runs = runsOf(model);
    fs.state = { kind: "doc", model, exitCode: outcome.exitCode };
  }

  // evidenceArgs are --evidence for each chosen run whose file is there.
  private evidenceArgs(folder: string): string[] {
    return (this.deps.evidence?.list(folder) ?? [])
      .filter((p) => existsSync(p.file))
      .map((p) => `--evidence=${p.file}`);
  }

  // checkCli asks the folder's CLI what it can do before the first audit:
  // one without audit would read the word as a package to test. A CLI that
  // predates `version --json` answers with its version line.
  private async checkCli(folder: string, fs: FolderState): Promise<boolean> {
    let caps;
    try {
      caps = await queryCliCapabilities(folder, this.deps.log);
    } catch (err: unknown) {
      offerCliUpgrade(err);
      throw err;
    }
    const schema = caps?.schemas.audit;
    if (caps?.json && caps.commands.includes("audit") && schema !== undefined) {
      if (schema > AUDIT_SCHEMA) {
        fs.state = {
          kind: "newSchema",
          version: caps.version,
          schema,
          supported: AUDIT_SCHEMA,
        };
        return false;
      }
      fs.cliOk = true;
      return true;
    }
    fs.upgrade = await pinnedBelow(folder, AUDIT_CLI_VERSION, this.deps.log);
    fs.state = {
      kind: "oldCli",
      version: caps?.version,
      floor: AUDIT_CLI_VERSION,
      advice: `Run: go get -tool ${cliPackagePath(folder)}@${AUDIT_CLI_VERSION}`,
      canUpgrade: !!fs.upgrade,
    };
    return false;
  }

  // runChild runs one audit. A plain one has a budget; one with a run
  // runs tests, so it runs until it ends or is cancelled, and a stop gives
  // its fixtures their teardown.
  private async runChild(
    folder: string,
    fs: FolderState,
    bin: string,
    args: string[],
    mode: AuditMode,
  ): Promise<AuditOutcome> {
    const { code, stdout, stderr } = await this.spawnAudit(
      folder,
      fs,
      bin,
      args,
      mode,
    );
    return interpretAuditExit(code, stdout, stderr);
  }

  private async spawnAudit(
    folder: string,
    fs: FolderState,
    bin: string,
    args: string[],
    mode: AuditMode,
  ): Promise<{ code: number | null; stdout: string; stderr: string }> {
    if (fs.cancelled) throw new Error("audit stopped");
    const budgetSeconds = auditTimeoutSeconds(folder);
    const mc = new ManagedChild(bin, args, {
      cwd: folder,
      kind: mode === "run" ? "audit-run" : "audit",
    });
    fs.child = mc;
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let spawnError: Error | undefined;
    mc.child.stdout?.on("data", (chunk: string) => (stdout += chunk));
    mc.child.stderr?.on("data", (chunk: string) => {
      stderr += chunk;
      if (stderr.length > 2 * STDERR_KEEP) stderr = stderr.slice(-STDERR_KEEP);
    });
    mc.child.on("error", (err: Error) => (spawnError = err));
    mc.child.stdin?.on("error", () => {});
    mc.child.stdin?.end();
    const budget =
      mode === "static"
        ? setTimeout(() => {
            timedOut = true;
            void mc.terminate("prompt");
          }, budgetSeconds * 1000)
        : undefined;
    try {
      const { code } = await mc.exited;
      if (spawnError) throw spawnError;
      if (timedOut) throw new Error(`audit timed out after ${budgetSeconds}s`);
      if (this.disposed || fs.cancelled) throw new Error("audit stopped");
      return { code, stdout, stderr };
    } finally {
      clearTimeout(budget);
      mc.dispose();
    }
  }

  // place puts each side of each row's file on disk, under its module's
  // directory as the document gives it.
  private place(
    folder: string,
    toplevel: string,
    doc: AuditDocument,
    model: ReturnType<typeof buildChangeModel>,
  ): Map<string, Placed> {
    const dirs = new Map(doc.moduleDirs.map((m) => [m.path, m.dir]));
    const out = new Map<string, Placed>();
    for (const [key, t] of targetsOf(doc, model)) {
      const dir = t.module !== undefined ? dirs.get(t.module) : undefined;
      if (dir === undefined) continue;
      const side = (loc: AuditLocation | undefined): PlacedSide | undefined => {
        if (!loc?.file) return undefined;
        const abs = path.join(
          folder,
          ...dir.split("/"),
          ...loc.file.split("/"),
        );
        const rel = path.relative(toplevel, abs).split(path.sep).join("/");
        return { abs, rel, loc };
      };
      const placed = { base: side(t.base), head: side(t.head) };
      if (placed.base || placed.head) out.set(key, placed);
    }
    return out;
  }

  private choices(): Record<string, BaseChoice> {
    const v = this.deps.memento.get<Record<string, BaseChoice>>(BASE_KEY);
    return v && typeof v === "object" ? v : {};
  }

  // resolveBase is the folder's base: the one chosen, else the merge-base of
  // HEAD with the remote default branch. The CLI is given the ref.
  async resolveBase(
    folder: string,
  ): Promise<{ ref: string; label: string } | undefined> {
    const chosen = this.choices()[folder];
    const choice: BaseChoice | undefined =
      chosen ??
      (await defaultBranch(folder).then((ref) =>
        ref ? { ref, mergeBase: true } : undefined,
      ));
    if (!choice) return undefined;
    if (!choice.mergeBase) return { ref: choice.ref, label: choice.ref };
    const mb = await mergeBase(folder, choice.ref);
    if (!mb) return { ref: choice.ref, label: choice.ref };
    return {
      ref: mb,
      label: `merge-base with ${choice.ref} (${mb})`,
    };
  }

  async setBase(folder: string, choice: BaseChoice | undefined): Promise<void> {
    const all = this.choices();
    if (choice) all[folder] = choice;
    else delete all[folder];
    await this.deps.memento.update(BASE_KEY, all);
  }

  // chooseBase lets the user pick the folder's base: the merge-base with the
  // default branch, a branch or tag, or any ref typed in.
  async chooseBase(folder: string): Promise<void> {
    if (!(await gitToplevel(folder))) return;
    const def = await defaultBranch(folder);
    type Item = vscode.QuickPickItem & { choice?: BaseChoice };
    const fixed: Item[] = [];
    if (def) {
      fixed.push({
        label: `$(git-merge) merge-base with ${def}`,
        description: "default",
        choice: { ref: def, mergeBase: true },
      });
    }
    const refs: Item[] = (await listRefs(folder)).map((ref) => ({
      label: ref,
      choice: { ref, mergeBase: false },
    }));
    const pick = vscode.window.createQuickPick<Item>();
    pick.title = "Audit against which base?";
    pick.placeholder = "A branch, tag or commit; type any ref";
    pick.items = [...fixed, ...refs];
    pick.onDidChangeValue((value) => {
      const typed = value.trim();
      const exact = refs.some((r) => r.label === typed);
      pick.items =
        typed && !exact
          ? [
              {
                label: typed,
                description: "use this ref",
                choice: { ref: typed, mergeBase: false },
              },
              ...fixed,
              ...refs,
            ]
          : [...fixed, ...refs];
    });
    const choice = await new Promise<BaseChoice | undefined>((resolve) => {
      pick.onDidAccept(() => {
        resolve(pick.selectedItems[0]?.choice);
        pick.hide();
      });
      pick.onDidHide(() => resolve(undefined));
      pick.show();
    });
    pick.dispose();
    if (!choice) return;
    await this.setBase(folder, choice);
    await this.audit(folder);
  }

  // act answers a click on a row or a button of the scope.
  async act(msg: {
    act?: unknown;
    key?: unknown;
    folder?: unknown;
  }): Promise<void> {
    const folder = this.folder;
    if (msg.act === "folder" && typeof msg.folder === "string") {
      this.select(msg.folder);
      this.deps.changed();
      this.ensure();
      return;
    }
    if (!folder) return;
    const fs = this.stateOf(folder);
    const key = typeof msg.key === "string" ? msg.key : "";
    switch (msg.act) {
      case "refresh":
        if (fs.state.kind === "oldCli" || fs.state.kind === "newSchema")
          fs.cliOk = false;
        await this.audit(folder);
        return;
      case "auditRun":
        await this.audit(folder, "run");
        return;
      case "cancel":
        this.cancel(folder);
        return;
      case "chooseBase":
        await this.chooseBase(folder);
        return;
      case "upgrade":
        if (fs.upgrade) offerCliUpgrade(fs.upgrade);
        return;
      case "dropEvidence":
        if (key) await this.deps.evidence?.remove(folder, key);
        return;
      case "accept":
        if (!fs.doc || !key) return;
        await this.acceptance.toggle(folder, fs.doc.head.commit ?? "", key);
        this.deps.changed();
        return;
      case "run": {
        const r = fs.runs.get(key);
        if (r) await this.deps.runTest?.(r.package, r.run);
        return;
      }
      case "openDiff":
        await this.openDiff(fs, key);
        return;
      case "openSource":
        await this.openSource(fs, key);
        return;
    }
  }

  private baseUri(fs: FolderState, side: PlacedSide): vscode.Uri {
    const commit = fs.doc?.base.commit || fs.baseRef || "";
    return vscode.Uri.file(side.abs).with({
      scheme: BASE_SCHEME,
      query: baseQuery({ toplevel: fs.toplevel ?? "", commit, path: side.rel }),
    });
  }

  private emptyUri(side: PlacedSide): vscode.Uri {
    return vscode.Uri.file(side.abs).with({
      scheme: BASE_SCHEME,
      query: baseQuery({ toplevel: "", commit: "", path: side.rel }),
    });
  }

  private range(loc: AuditLocation): vscode.Range | undefined {
    if (loc.line <= 0) return undefined;
    return new vscode.Range(
      loc.line - 1,
      0,
      Math.max(loc.endLine, loc.line) - 1,
      0,
    );
  }

  // openDiff opens the file as the base had it beside the working tree's:
  // the base's lines revealed on the left, the head's selected on the right.
  // A side the row lacks shows its file as the other side names it.
  private async openDiff(fs: FolderState, key: string): Promise<void> {
    const t = fs.targets.get(key);
    if (!t) return;
    const leftSide = (t.base ?? t.head)!;
    const rightSide = (t.head ?? t.base)!;
    const left = this.baseUri(fs, leftSide);
    const right = existsSync(rightSide.abs)
      ? vscode.Uri.file(rightSide.abs)
      : this.emptyUri(rightSide);
    const base = fs.doc?.base.commit || fs.baseRef?.slice(0, 8) || "base";
    const selection = t.head ? this.range(t.head.loc) : undefined;
    await vscode.commands.executeCommand(
      "vscode.diff",
      left,
      right,
      `${path.basename(rightSide.abs)} (${base} ↔ working tree)`,
      { preview: true, selection },
    );
    const lines = t.base ? this.range(t.base.loc) : undefined;
    if (!lines) return;
    const editor = vscode.window.visibleTextEditors.find(
      (e) => e.document.uri.toString() === left.toString(),
    );
    if (!editor) return;
    editor.revealRange(
      lines,
      vscode.TextEditorRevealType.InCenterIfOutsideViewport,
    );
    if (!t.head)
      editor.selection = new vscode.Selection(lines.start, lines.end);
  }

  // openSource opens a row's lines: the working tree's, or the base's copy
  // of what the head no longer has.
  private async openSource(fs: FolderState, key: string): Promise<void> {
    const t = fs.targets.get(key);
    if (!t) return;
    const side = (t.head ?? t.base)!;
    const uri = t.head ? vscode.Uri.file(side.abs) : this.baseUri(fs, side);
    const lines = this.range(side.loc);
    await vscode.window.showTextDocument(uri, {
      preview: true,
      selection: lines,
    });
  }

  dispose(): void {
    this.disposed = true;
    for (const fs of this.folders.values())
      void fs.child?.terminate(fs.busy === "run" ? "teardown" : "prompt");
  }
}
