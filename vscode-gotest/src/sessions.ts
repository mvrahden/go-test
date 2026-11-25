// The CLI sessions this window has running: test runs, watchers, bench and
// fuzz sessions, audits and debug preparations. Each owner adds its session
// with the stop it already has (a run's cancel, a watcher's stop), so a
// stop from the Sessions view takes the same path as any other stop: the
// child's own graceful stop, its teardown grace and the force after it.
import * as vscode from "vscode";
import * as path from "node:path";
import { elapsedText, tallyText, type Tally } from "./liveProgress.js";

// How often a row that counts a run's events is redrawn.
export const PROGRESS_TICK_MS = 1000;

export interface SessionInfo {
  // What it is: run, coverage, watch, bench, fuzz, audit, prepare.
  kind: string;
  // The workspace folder it runs in.
  folder: string;
  // What it runs: packages, a scope, a target.
  target: string;
  // Asks the session to stop, as its owner stops it.
  stop: () => void;
  // A test run's events so far: its row shows them and its elapsed time.
  progress?: { tally: Tally };
}

export interface Session extends SessionInfo {
  id: number;
  startedAt: number;
  stopping: boolean;
}

export class Sessions {
  private readonly live = new Map<number, Session>();
  private next = 0;
  // Plain listeners, so a module importing this needs no vscode at load.
  private readonly listeners = new Set<() => void>();
  readonly onDidChange: vscode.Event<void> = (listener, thisArg) => {
    const fn = () => listener.call(thisArg, undefined);
    this.listeners.add(fn);
    return { dispose: () => this.listeners.delete(fn) };
  };

  private tick: ReturnType<typeof setInterval> | undefined;

  private fire(): void {
    for (const fn of Array.from(this.listeners)) fn();
  }

  // ticking redraws the rows each PROGRESS_TICK_MS while one counts a run.
  private ticking(): void {
    const counting = Array.from(this.live.values()).some((s) => s.progress);
    if (counting && !this.tick) {
      this.tick = setInterval(() => this.fire(), PROGRESS_TICK_MS);
      this.tick.unref?.();
    } else if (!counting && this.tick) {
      clearInterval(this.tick);
      this.tick = undefined;
    }
  }

  // add lists a session until the returned handle is disposed.
  add(info: SessionInfo): vscode.Disposable {
    const id = ++this.next;
    this.live.set(id, {
      ...info,
      id,
      startedAt: Date.now(),
      stopping: false,
    });
    this.fire();
    this.ticking();
    return {
      dispose: () => {
        if (!this.live.delete(id)) return;
        this.ticking();
        this.fire();
      },
    };
  }

  list(): Session[] {
    return Array.from(this.live.values());
  }

  // stop asks one session to stop; asking again does nothing more.
  stop(id: number): boolean {
    const s = this.live.get(id);
    if (!s || s.stopping) return false;
    s.stopping = true;
    this.fire();
    s.stop();
    return true;
  }
}

// The window's sessions, which every owner adds to.
export const sessions = new Sessions();

// linkedSession lists a session stopped by cancelling a token of its own,
// which the given token cancels too.
export function linkedSession(
  token: vscode.CancellationToken | undefined,
  info: Omit<SessionInfo, "stop">,
): { token: vscode.CancellationToken; dispose: () => void } {
  const cts = new vscode.CancellationTokenSource();
  const sub = token?.onCancellationRequested(() => cts.cancel());
  if (token?.isCancellationRequested) cts.cancel();
  const handle = sessions.add({ ...info, stop: () => cts.cancel() });
  return {
    token: cts.token,
    dispose: () => {
      handle.dispose();
      sub?.dispose();
      cts.dispose();
    },
  };
}

// A row of the Sessions view: a folder holding sessions, or a session.
export type SessionNode =
  { type: "folder"; folder: string } | { type: "session"; session: Session };

function clock(at: number): string {
  const d = new Date(at);
  const two = (v: number) => String(v).padStart(2, "0");
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`;
}

// sessionItem is how a session's row reads: a test run's with its elapsed
// time and the events it applied so far.
export function sessionItem(
  s: Session,
  now = Date.now(),
): {
  label: string;
  description: string;
  tooltip: string;
  contextValue: string;
} {
  const counts = s.progress
    ? ` · ${elapsedText(now - s.startedAt)} · ${tallyText(s.progress.tally)}`
    : "";
  return {
    label: `${s.kind}: ${s.target}`,
    description:
      (s.stopping
        ? `stopping… (started ${clock(s.startedAt)})`
        : `started ${clock(s.startedAt)}`) + counts,
    tooltip: `${s.kind} in ${s.folder}\n${s.target}\nstarted ${new Date(s.startedAt).toLocaleString()}`,
    contextValue: s.stopping ? "gotestSessionStopping" : "gotestSession",
  };
}

// SessionsTree shows the live sessions, under their folder when the
// window has several.
export class SessionsTree implements vscode.TreeDataProvider<SessionNode> {
  readonly onDidChangeTreeData: vscode.Event<void>;

  constructor(private readonly all: Sessions = sessions) {
    this.onDidChangeTreeData = all.onDidChange;
  }

  getChildren(node?: SessionNode): SessionNode[] {
    const list = this.all.list();
    if (node?.type === "session") return [];
    if (node?.type === "folder")
      return list
        .filter((s) => s.folder === node.folder)
        .map((session) => ({ type: "session", session }));
    const folders = Array.from(new Set(list.map((s) => s.folder)));
    if (folders.length > 1)
      return folders.map((folder) => ({ type: "folder", folder }));
    return list.map((session) => ({ type: "session", session }));
  }

  getTreeItem(node: SessionNode): vscode.TreeItem {
    if (node.type === "folder") {
      const item = new vscode.TreeItem(
        path.basename(node.folder),
        vscode.TreeItemCollapsibleState.Expanded,
      );
      item.tooltip = node.folder;
      return item;
    }
    const v = sessionItem(node.session);
    const item = new vscode.TreeItem(
      v.label,
      vscode.TreeItemCollapsibleState.None,
    );
    item.description = v.description;
    item.tooltip = v.tooltip;
    item.contextValue = v.contextValue;
    item.id = `session-${node.session.id}`;
    return item;
  }
}
