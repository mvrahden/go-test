import { describe, it, expect, vi, afterEach } from "vitest";

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
  };
});

import * as vscode from "vscode";
import {
  PROGRESS_TICK_MS,
  Sessions,
  linkedSession,
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

describe("a test run's session", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("is redrawn each tick while a run counts, and not once none does", () => {
    vi.useFakeTimers();
    const all = new Sessions();
    let told = 0;
    all.onDidChange(() => told++);
    const watch = all.add({
      kind: "watch",
      folder: "/w",
      target: "./...",
      stop: () => {},
    });
    told = 0;
    vi.advanceTimersByTime(5 * PROGRESS_TICK_MS);
    expect(told, "a watcher counts nothing").toBe(0);
    const run = all.add({
      kind: "run",
      folder: "/w",
      target: "m/a",
      stop: () => {},
      progress: { tally: { started: 0, passed: 0, failed: 0 } },
    });
    told = 0;
    vi.advanceTimersByTime(3 * PROGRESS_TICK_MS);
    expect(told).toBe(3);
    run.dispose();
    told = 0;
    vi.advanceTimersByTime(5 * PROGRESS_TICK_MS);
    expect(told).toBe(0);
    watch.dispose();
  });
});
