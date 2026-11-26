import type * as vscode from "vscode";

// What a run last showed on an item: started and enqueued are both running.
export type Shown = "running" | "passed" | "failed" | "skipped";

const SHOWN: Record<string, Shown> = {
  started: "running",
  enqueued: "running",
  passed: "passed",
  failed: "failed",
  errored: "failed",
  skipped: "skipped",
};

const states = new WeakMap<object, Map<string, Shown>>();

// tracked wraps a run so shownIn can say what it showed on each item. The
// TestRun API has no read side, and a folder's state is derived from what its
// children show now, not from what was stored of them.
export function tracked(run: vscode.TestRun): vscode.TestRun {
  const shown = new Map<string, Shown>();
  const proxy = new Proxy(run, {
    get(target, key, receiver) {
      const value = Reflect.get(target, key, receiver);
      if (typeof value !== "function") return value;
      const state = typeof key === "string" ? SHOWN[key] : undefined;
      if (!state) return value.bind(target);
      return (item: vscode.TestItem, ...rest: unknown[]) => {
        shown.set(item.id, state);
        return value.call(target, item, ...rest);
      };
    },
  });
  states.set(proxy, shown);
  return proxy;
}

// shownIn is what the run last showed on an item; undefined for an item the
// run never touched, and for every item of a run that is not tracked.
export function shownIn(run: vscode.TestRun, id: string): Shown | undefined {
  return states.get(run)?.get(id);
}
