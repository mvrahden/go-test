// The Change scope of the Spec View panel: per workspace folder, the audit
// of the working tree against a base, as `gotest audit --format=json`
// prints it. The CLI computes the document; this runs it, keeps it, and
// wires its rows to the editor.
import * as vscode from "vscode";
import * as path from "node:path";
import { existsSync } from "node:fs";
import {
  AUDIT_CLI_VERSION,
  buildCliCommand,
  cliMeets,
  cliPackagePath,
  offerCliUpgrade,
  pinnedBelow,
  queryCliVersion,
  stripGoRunExitEcho,
  type CliBelowFloorError,
} from "./cli.js";
import { auditTimeoutSeconds } from "./config.js";
import { ManagedChild } from "./managedChild.js";
import { readModuleRoots } from "./gomod.js";
import {
  buildChangeModel,
  parseAuditDocument,
  targetsOf,
  type AuditDocument,
  type Target,
} from "./auditModel.js";
import type { ChangeState, ChangeView } from "./changeView.js";
import { AcceptanceStore, type Memento } from "./acceptance.js";
import {
  BASE_SCHEME,
  baseQuery,
  currentBranch,
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
  // Whether a test item is declared in file within the 1-based lines, and
  // running the narrowest such item.
  hasItemAt?: (file: string, line: number, endLine: number) => boolean;
  runAt?: (file: string, line: number, endLine: number) => Promise<void>;
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

// A target placed on disk: the file in the working tree and its path in the
// checkout, for the base's copy.
interface Placed extends Target {
  abs: string;
  rel: string;
}

interface FolderState {
  state: ChangeState;
  busy: boolean;
  again: boolean;
  // A run in the folder finished since the last audit.
  stale: boolean;
  child?: ManagedChild;
  doc?: AuditDocument;
  exitCode?: number | null;
  baseRef?: string;
  baseLabel?: string;
  headBranch?: string;
  toplevel?: string;
  targets: Map<string, Placed>;
  cliOk: boolean;
  upgrade?: CliBelowFloorError;
}

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
        again: false,
        stale: false,
        targets: new Map(),
        cliOk: false,
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
      baseLabel: fs.baseLabel,
      headBranch: fs.headBranch,
      canOpen: (key) => fs.targets.has(key),
      canRun: (key) => {
        const t = fs.targets.get(key);
        return (
          !!t &&
          t.side === "head" &&
          t.line > 0 &&
          !!this.deps.hasItemAt?.(t.abs, t.line, t.endLine)
        );
      },
    };
  }

  // audit runs the folder's audit, one at a time: asked again while one
  // runs, it runs once more after it.
  async audit(folder: string): Promise<void> {
    if (this.disposed) return;
    const fs = this.stateOf(folder);
    if (fs.busy) {
      fs.again = true;
      return;
    }
    fs.busy = true;
    fs.stale = false;
    this.deps.changed();
    try {
      await this.auditNow(folder, fs);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.deps.log.error(`[change] ${message}`);
      fs.state = { kind: "error", message };
    } finally {
      fs.busy = false;
      fs.child = undefined;
      if (!this.disposed) this.deps.changed();
    }
    if (fs.again && !this.disposed) {
      fs.again = false;
      await this.audit(folder);
    }
  }

  private async auditNow(folder: string, fs: FolderState): Promise<void> {
    fs.toplevel = await gitToplevel(folder);
    if (!fs.toplevel) {
      fs.state = { kind: "notGit" };
      return;
    }
    fs.headBranch = await currentBranch(folder);
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
    let cmd;
    try {
      cmd = await buildCliCommand(
        ["audit", `--base=${base.ref}`, "--format=json", ...patterns],
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
    const outcome = await this.runChild(folder, fs, cmd.bin, cmd.args);
    if (!outcome.ok) {
      fs.state = { kind: "error", message: outcome.message };
      return;
    }
    fs.doc = outcome.doc;
    fs.exitCode = outcome.exitCode;
    fs.targets = await this.place(folder, fs.toplevel, outcome.doc);
    fs.state = {
      kind: "doc",
      model: buildChangeModel(outcome.doc),
      exitCode: outcome.exitCode,
    };
  }

  // checkCli asks the folder's CLI its version before the first audit: one
  // that predates audit would read the word as a package to test.
  private async checkCli(folder: string, fs: FolderState): Promise<boolean> {
    let version: string | undefined;
    try {
      version = await queryCliVersion(folder, this.deps.log);
    } catch (err: unknown) {
      offerCliUpgrade(err);
      throw err;
    }
    if (version && cliMeets(version, AUDIT_CLI_VERSION)) {
      fs.cliOk = true;
      return true;
    }
    fs.upgrade = await pinnedBelow(folder, AUDIT_CLI_VERSION, this.deps.log);
    fs.state = {
      kind: "oldCli",
      version,
      floor: AUDIT_CLI_VERSION,
      advice: `Run: go get -tool ${cliPackagePath(folder)}@${AUDIT_CLI_VERSION}`,
      canUpgrade: !!fs.upgrade,
    };
    return false;
  }

  private async runChild(
    folder: string,
    fs: FolderState,
    bin: string,
    args: string[],
  ): Promise<AuditOutcome> {
    const budgetSeconds = auditTimeoutSeconds(folder);
    const mc = new ManagedChild(bin, args, { cwd: folder, kind: "audit" });
    fs.child = mc;
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let spawnError: Error | undefined;
    mc.child.stdout?.on("data", (chunk: string) => (stdout += chunk));
    mc.child.stderr?.on("data", (chunk: string) => (stderr += chunk));
    mc.child.on("error", (err: Error) => (spawnError = err));
    mc.child.stdin?.on("error", () => {});
    mc.child.stdin?.end();
    const budget = setTimeout(() => {
      timedOut = true;
      void mc.terminate("prompt");
    }, budgetSeconds * 1000);
    try {
      const { code } = await mc.exited;
      if (spawnError) throw spawnError;
      if (timedOut) throw new Error(`audit timed out after ${budgetSeconds}s`);
      if (this.disposed) throw new Error("audit stopped");
      return interpretAuditExit(code, stdout, stderr);
    } finally {
      clearTimeout(budget);
      mc.dispose();
    }
  }

  // place puts each row's file on disk: a module's file under the module's
  // directory, a file the document names without a package under the one
  // module that holds it.
  private async place(
    folder: string,
    toplevel: string,
    doc: AuditDocument,
  ): Promise<Map<string, Placed>> {
    const dirs = new Map<string, string>();
    for (const root of await readModuleRoots(folder)) {
      const m = /^\s*module\s+(\S+)/m.exec(root.goMod)?.[1];
      if (m) dirs.set(m, root.dir);
    }
    const out = new Map<string, Placed>();
    for (const [key, t] of targetsOf(buildChangeModel(doc))) {
      let dir = t.module ? dirs.get(t.module) : undefined;
      if (!t.module) {
        const candidates = Array.from(dirs.values());
        dir =
          candidates.length === 1
            ? candidates[0]
            : candidates.find((d) => existsSync(path.join(d, t.file)));
      }
      if (!dir) continue;
      const abs = path.join(dir, ...t.file.split("/"));
      const rel = path.relative(toplevel, abs).split(path.sep).join("/");
      out.set(key, { ...t, abs, rel });
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
        if (fs.state.kind === "oldCli") fs.cliOk = false;
        await this.audit(folder);
        return;
      case "chooseBase":
        await this.chooseBase(folder);
        return;
      case "upgrade":
        if (fs.upgrade) offerCliUpgrade(fs.upgrade);
        return;
      case "accept":
        if (!fs.doc) return;
        await this.acceptance.toggle(folder, fs.doc.head.commit ?? "", key);
        this.deps.changed();
        return;
      case "run": {
        const t = fs.targets.get(key);
        if (t && t.side === "head")
          await this.deps.runAt?.(t.abs, t.line, t.endLine);
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

  private baseUri(fs: FolderState, t: Placed): vscode.Uri {
    const commit = fs.doc?.base.commit || fs.baseRef || "";
    return vscode.Uri.file(t.abs).with({
      scheme: BASE_SCHEME,
      query: baseQuery({ toplevel: fs.toplevel ?? "", commit, path: t.rel }),
    });
  }

  private emptyUri(t: Placed): vscode.Uri {
    return vscode.Uri.file(t.abs).with({
      scheme: BASE_SCHEME,
      query: baseQuery({ toplevel: "", commit: "", path: t.rel }),
    });
  }

  private range(t: Placed): vscode.Range | undefined {
    if (t.line <= 0) return undefined;
    return new vscode.Range(t.line - 1, 0, Math.max(t.endLine, t.line) - 1, 0);
  }

  // openDiff opens the file as the base had it beside the working tree's,
  // at the row's lines when the working tree holds them.
  private async openDiff(fs: FolderState, key: string): Promise<void> {
    const t = fs.targets.get(key);
    if (!t) return;
    const right = existsSync(t.abs) ? vscode.Uri.file(t.abs) : this.emptyUri(t);
    const base = fs.doc?.base.commit || fs.baseRef?.slice(0, 8) || "base";
    const selection = t.side === "head" ? this.range(t) : undefined;
    await vscode.commands.executeCommand(
      "vscode.diff",
      this.baseUri(fs, t),
      right,
      `${path.basename(t.abs)} (${base} ↔ working tree)`,
      { preview: true, selection },
    );
  }

  // openSource opens where a behavior is declared: the working tree's line,
  // or the base's copy of a removed one.
  private async openSource(fs: FolderState, key: string): Promise<void> {
    const t = fs.targets.get(key);
    if (!t) return;
    const uri =
      t.side === "head" ? vscode.Uri.file(t.abs) : this.baseUri(fs, t);
    const selection = this.range(t);
    await vscode.window.showTextDocument(uri, {
      preview: true,
      selection: selection
        ? new vscode.Range(selection.start, selection.start)
        : undefined,
    });
  }

  dispose(): void {
    this.disposed = true;
    for (const fs of this.folders.values()) void fs.child?.terminate("prompt");
  }
}
