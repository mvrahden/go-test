// The panel's Runs tab: per workspace folder, the run records `gotest runs
// --format=json` lists, with what each says of the tree now. The CLI keeps
// and judges the records; this lists them, opens a run's events file, and
// picks runs as the Change tab's evidence.
import * as vscode from "vscode";
import { execFile } from "node:child_process";
import * as path from "node:path";
import { existsSync } from "node:fs";
import {
  buildCliCommand,
  cliPackagePath,
  folderCapabilities,
  offerCliUpgrade,
  pinnedBelow,
  RUNS_CLI_VERSION,
  RUNS_COMMAND,
  stripGoRunExitEcho,
  type CliBelowFloorError,
} from "./cli.js";
import { specRenderTimeoutSeconds } from "./config.js";
import { ManagedChild } from "./managedChild.js";
import {
  evidenceLabel,
  newestCurrent,
  parseRunsDocument,
  type EvidencePick,
  type RunsDocument,
} from "./runsModel.js";
import type { RunsState, RunsView } from "./runsView.js";
import type { EvidenceSelection } from "./evidence.js";

export interface RunsScopeDeps {
  log: vscode.LogOutputChannel;
  folders: () => string[];
  evidence: EvidenceSelection;
  // Shows a run's saved stream in the Now tab.
  openStream: (
    id: string,
    label: string,
    folder: string,
    file: string,
  ) => Promise<void>;
  // Runs the folder's changed packages (--changed).
  runChanged?: (folder: string) => Promise<void>;
  changed: () => void;
}

interface FolderState {
  state: RunsState;
  doc?: RunsDocument;
  busy: boolean;
  again: boolean;
  stale: boolean;
  notice?: string;
  upgrade?: CliBelowFloorError;
  child?: ManagedChild;
  head?: string;
  // Who waits for the listing running now.
  waiters: (() => void)[];
}

// headCommit is the checkout's HEAD as run records name it.
function headCommit(dir: string): Promise<string | undefined> {
  return new Promise((resolve) =>
    execFile(
      "git",
      ["rev-parse", "--short=8", "HEAD"],
      { cwd: dir, timeout: 15_000 },
      (err, stdout) => resolve(err ? undefined : stdout.trim() || undefined),
    ),
  );
}

export type CliOutcome =
  { ok: true; stdout: string; stderr: string } | { ok: false; message: string };

// runCli runs one budgeted read of the folder's CLI.
export async function runCli(
  folder: string,
  args: string[],
  log: vscode.LogOutputChannel,
  kind: string,
  onChild?: (mc: ManagedChild) => void,
): Promise<CliOutcome> {
  let cmd;
  try {
    cmd = await buildCliCommand(args, folder, log);
  } catch (err: unknown) {
    offerCliUpgrade(err);
    return {
      ok: false,
      message: err instanceof Error ? err.message : String(err),
    };
  }
  log.info(`[runs] ${cmd.bin} ${cmd.args.join(" ")} (cwd ${folder})`);
  const mc = new ManagedChild(cmd.bin, cmd.args, {
    cwd: folder,
    kind,
    onFinished: (line) => log.info(`[${kind}] ${line}`),
  });
  onChild?.(mc);
  let stdout = "";
  let stderr = "";
  let spawnError: Error | undefined;
  mc.child.stdout?.on("data", (c: string) => (stdout += c));
  mc.child.stderr?.on("data", (c: string) => (stderr += c));
  mc.child.on("error", (err: Error) => (spawnError = err));
  mc.child.stdin?.on("error", () => {});
  mc.child.stdin?.end();
  const seconds = specRenderTimeoutSeconds(folder);
  let timedOut = false;
  const budget = setTimeout(() => {
    timedOut = true;
    void mc.terminate("prompt");
  }, seconds * 1000);
  try {
    const { code } = await mc.exited;
    if (spawnError) return { ok: false, message: spawnError.message };
    if (timedOut)
      return { ok: false, message: `gotest timed out after ${seconds}s` };
    if (code !== 0)
      return {
        ok: false,
        message:
          stripGoRunExitEcho(stderr) || `gotest exited with code ${code}`,
      };
    return { ok: true, stdout, stderr };
  } finally {
    clearTimeout(budget);
    mc.dispose();
  }
}

export class RunsScope implements vscode.Disposable {
  private selected: string | undefined;
  private readonly folders = new Map<string, FolderState>();
  private disposed = false;

  constructor(private readonly deps: RunsScopeDeps) {
    deps.evidence.onChange(() => deps.changed());
  }

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
        stale: true,
        waiters: [],
      };
      this.folders.set(folder, fs);
    }
    return fs;
  }

  // ensure lists the shown folder's runs when it has none yet, or a run or
  // an edit since may have changed what they say.
  ensure(): void {
    const folder = this.folder;
    if (!folder) return;
    const fs = this.stateOf(folder);
    if (fs.stale && !fs.busy) void this.refresh(folder);
  }

  // invalidate marks folders' lists out of date; the shown one is listed
  // again when on show.
  invalidate(folders: string[], showing: boolean): void {
    for (const folder of new Set(folders)) {
      const fs = this.folders.get(folder);
      if (!fs) continue;
      fs.stale = true;
      if (showing && folder === this.folder) void this.refresh(folder);
    }
  }

  view(): RunsView {
    const folder = this.folder;
    const folders = this.deps.folders().map((f) => ({
      path: f,
      label: path.basename(f),
      selected: f === folder,
    }));
    const now = new Date();
    const base = { now, exists: existsSync, folders };
    if (!folder)
      return {
        ...base,
        state: { kind: "idle" },
        busy: false,
        chosen: () => false,
      };
    const fs = this.stateOf(folder);
    const chosen = this.deps.evidence.list(folder);
    return {
      ...base,
      state: fs.state,
      busy: fs.busy,
      notice: fs.notice,
      head: fs.head,
      chosen: (id) => chosen.some((p) => p.id === id),
      auto:
        chosen.length === 0
          ? newestCurrent(fs.doc, existsSync, now)?.id
          : undefined,
    };
  }

  // doc is the folder's listing as last read.
  doc(folder: string): RunsDocument | undefined {
    return this.folders.get(folder)?.doc;
  }

  // listed resolves with the folder's listing, read again when stale.
  async listed(folder: string): Promise<RunsDocument | undefined> {
    const fs = this.stateOf(folder);
    if (fs.busy) await new Promise<void>((r) => fs.waiters.push(r));
    else if (fs.stale || !fs.doc) await this.refresh(folder);
    return fs.doc;
  }

  // newestCurrent is the evidence an audit takes when no run is chosen.
  async newestCurrent(folder: string): Promise<EvidencePick | undefined> {
    return newestCurrent(await this.listed(folder), existsSync, new Date());
  }

  // refresh lists the folder's records, one listing at a time.
  async refresh(folder: string): Promise<void> {
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
      fs.head = await headCommit(folder);
      if (await this.checkCli(folder, fs)) {
        const out = await runCli(
          folder,
          ["runs", "--format=json"],
          this.deps.log,
          "runs",
          (mc) => (fs.child = mc),
        );
        const doc = out.ok ? parseRunsDocument(out.stdout) : undefined;
        if (doc) {
          fs.doc = doc;
          fs.state = { kind: "doc", doc };
        } else {
          fs.state = {
            kind: "error",
            message: out.ok ? "gotest runs printed no document" : out.message,
          };
        }
      }
    } finally {
      fs.busy = false;
      fs.child = undefined;
      if (!this.disposed) this.deps.changed();
    }
    if (fs.again && !this.disposed) {
      fs.again = false;
      await this.refresh(folder);
      return;
    }
    for (const w of fs.waiters.splice(0)) w();
  }

  private async checkCli(folder: string, fs: FolderState): Promise<boolean> {
    let caps;
    try {
      caps = await folderCapabilities(folder, this.deps.log);
    } catch (err: unknown) {
      offerCliUpgrade(err);
      fs.state = {
        kind: "error",
        message: err instanceof Error ? err.message : String(err),
      };
      return false;
    }
    if (caps?.json && caps.commands.includes(RUNS_COMMAND)) return true;
    fs.upgrade = await pinnedBelow(folder, RUNS_CLI_VERSION, this.deps.log);
    fs.state = {
      kind: "oldCli",
      version: caps?.version,
      floor: RUNS_CLI_VERSION,
      advice: `Run: go get -tool ${cliPackagePath(folder)}@${RUNS_CLI_VERSION}`,
      canUpgrade: !!fs.upgrade,
    };
    return false;
  }

  // importRun keeps a saved stream's run among the folder's records.
  async importRun(folder: string, file?: string): Promise<boolean> {
    const picked =
      file ??
      (
        await vscode.window.showOpenDialog({
          canSelectMany: false,
          openLabel: "Import Run",
          filters: {
            "go test -json streams": ["jsonl", "json", "ndjson", "log", "txt"],
            "All files": ["*"],
          },
        })
      )?.[0]?.fsPath;
    if (!picked) return false;
    const fs = this.stateOf(folder);
    const out = await runCli(
      folder,
      ["runs", `--import=${picked}`],
      this.deps.log,
      "runs",
    );
    fs.notice = out.ok
      ? `Imported the run of ${path.basename(picked)}.`
      : `The import failed: ${out.message}`;
    await this.refresh(folder);
    return out.ok;
  }

  // act answers a click in the scope.
  async act(msg: {
    act?: unknown;
    key?: unknown;
    folder?: unknown;
  }): Promise<void> {
    if (msg.act === "folder" && typeof msg.folder === "string") {
      this.select(msg.folder);
      this.deps.changed();
      this.ensure();
      return;
    }
    const folder = this.folder;
    if (!folder) return;
    const fs = this.stateOf(folder);
    const key = typeof msg.key === "string" ? msg.key : "";
    const row = fs.doc?.runs.find((r) => r.id === key);
    switch (msg.act) {
      case "refresh":
        fs.notice = undefined;
        await this.refresh(folder);
        return;
      case "import":
        await this.importRun(folder);
        return;
      case "upgrade":
        if (fs.upgrade) offerCliUpgrade(fs.upgrade);
        return;
      case "runChanged":
        await this.deps.runChanged?.(folder);
        return;
      case "open": {
        if (!row?.eventsFile || !existsSync(row.eventsFile)) return;
        const label = this.labelOf(key);
        await this.deps.openStream(
          `run:${row.id}`,
          label,
          folder,
          row.eventsFile,
        );
        return;
      }
      case "evidence": {
        if (!row?.eventsFile) return;
        await this.deps.evidence.toggle(folder, {
          id: row.id,
          file: row.eventsFile,
          label: this.labelOf(key),
        });
        return;
      }
    }
  }

  private labelOf(id: string): string {
    const folder = this.folder;
    const r = folder
      ? this.doc(folder)?.runs.find((x) => x.id === id)
      : undefined;
    return r ? evidenceLabel(r, new Date()) : id;
  }

  dispose(): void {
    this.disposed = true;
    for (const fs of this.folders.values()) void fs.child?.terminate("prompt");
  }
}
