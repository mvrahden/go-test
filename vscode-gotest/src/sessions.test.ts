import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class CancellationTokenSource {
    private listeners: (() => void)[] = [];
    token = {
      isCancellationRequested: false,
      onCancellationRequested: (fn: () => void) => {
        this.listeners.push(fn);
        return { dispose: () => {} };
      },
    };
    cancel() {
      if (this.token.isCancellationRequested) return;
      this.token.isCancellationRequested = true;
      for (const fn of this.listeners) fn();
    }
    dispose() {}
  }
  return {
    CancellationTokenSource,
    TreeItemCollapsibleState: { None: 0, Expanded: 2 },
    TreeItem: class {
      description?: string;
      tooltip?: string;
      contextValue?: string;
      id?: string;
      constructor(
        public label: string,
        public collapsibleState: number,
      ) {}
    },
  };
});

import * as vscode from "vscode";
import {
  Sessions,
  SessionsTree,
  linkedSession,
  sessionItem,
  sessions,
} from "./sessions.js";

describe("Sessions", () => {
  it("lists a session until its owner ends it, and tells each change", () => {
    const all = new Sessions();
    let told = 0;
    all.onDidChange(() => told++);
    const h = all.add({
      kind: "run",
      folder: "/w",
      target: "m/a",
      stop: () => {},
    });
    expect(all.list().map((s) => s.target)).toEqual(["m/a"]);
    h.dispose();
    h.dispose();
    expect(all.list()).toEqual([]);
    expect(told).toBe(2);
  });

  it("stops one session by its owner's stop, once", () => {
    const all = new Sessions();
    const stopped: string[] = [];
    all.add({
      kind: "watch",
      folder: "/w",
      target: "./a/...",
      stop: () => stopped.push("a"),
    });
    const b = all.add({
      kind: "watch",
      folder: "/w",
      target: "./b/...",
      stop: () => stopped.push("b"),
    });
    const [first] = all.list();
    expect(all.stop(first.id)).toBe(true);
    expect(all.stop(first.id)).toBe(false);
    expect(stopped).toEqual(["a"]);
    expect(all.list().find((s) => s.id === first.id)?.stopping).toBe(true);
    expect(sessionItem(all.list()[0]).contextValue).toBe(
      "gotestSessionStopping",
    );
    b.dispose();
  });
});

describe("linkedSession", () => {
  it("cancels its own token on a stop, and on the owner's cancel", () => {
    const outer = new vscode.CancellationTokenSource();
    const a = linkedSession(outer.token, {
      kind: "bench",
      folder: "/w",
      target: "x",
    });
    const row = sessions.list().at(-1)!;
    sessions.stop(row.id);
    expect(a.token.isCancellationRequested).toBe(true);
    expect(outer.token.isCancellationRequested).toBe(false);
    a.dispose();
    expect(sessions.list().some((s) => s.id === row.id)).toBe(false);

    const b = linkedSession(outer.token, {
      kind: "fuzz",
      folder: "/w",
      target: "y",
    });
    outer.cancel();
    expect(b.token.isCancellationRequested).toBe(true);
    b.dispose();
  });
});

describe("SessionsTree", () => {
  it("lists sessions flat in one folder, under their folders in several", () => {
    const all = new Sessions();
    const tree = new SessionsTree(all);
    all.add({ kind: "run", folder: "/w/a", target: "m/a", stop: () => {} });
    expect(tree.getChildren().map((n) => n.type)).toEqual(["session"]);
    all.add({
      kind: "audit",
      folder: "/w/b",
      target: "against main",
      stop: () => {},
    });
    const top = tree.getChildren();
    expect(top).toEqual([
      { type: "folder", folder: "/w/a" },
      { type: "folder", folder: "/w/b" },
    ]);
    const under = tree.getChildren(top[1]);
    expect(under).toHaveLength(1);
    const item = tree.getTreeItem(under[0]);
    expect(item.label).toBe("audit: against main");
    expect(item.contextValue).toBe("gotestSession");
    expect(String(item.description)).toMatch(/^started \d\d:\d\d:\d\d$/);
  });
});
