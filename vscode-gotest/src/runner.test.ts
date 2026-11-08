import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("vscode", () => {
  class EventEmitter {
    private listeners: ((v: unknown) => void)[] = [];
    event = (fn: (v: unknown) => void) => {
      this.listeners.push(fn);
      return { dispose: () => {} };
    };
    fire(v: unknown) {
      for (const fn of this.listeners) fn(v);
    }
    dispose() {
      this.listeners = [];
    }
  }
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
    EventEmitter,
    CancellationTokenSource,
    TestMessage: class {
      constructor(public message: string) {}
    },
    window: {
      showInformationMessage: vi.fn(),
      showWarningMessage: vi.fn(),
    },
  };
});

const h = vi.hoisted(() => ({
  config: {} as Record<string, unknown>,
  stdout: "" as string,
}));

vi.mock("./cli.js", () => ({
  scopedConfig: vi.fn(() => ({ get: (k: string) => h.config[k] })),
}));

// The grouping and filter logic has its own tests; here each fake item names
// its package and filter directly so the runner's orchestration stands alone.
// Errored items go through the real helper, which also records the verdict.
vi.mock("./runnerUtils.js", async (importOriginal) => ({
  erroredItem: (await importOriginal<typeof import("./runnerUtils.js")>())
    .erroredItem,
  collectItems: (_c: unknown, req: any) => [...(req.include ?? [])],
  groupByPackage: (items: any[]) => {
    const groups = new Map<string, any[]>();
    for (const item of items) {
      groups.set(item.pkg, [...(groups.get(item.pkg) ?? []), item]);
    }
    return groups;
  },
  buildRunFilter: (items: any[]) => items[0].filter,
  enqueueDescendants: vi.fn(),
  startAncestors: vi.fn(),
  skipUnresolved: vi.fn(),
  resolveAncestorItems: vi.fn(),
}));

vi.mock("./batchRunner.js", () => ({
  executeBatch: vi.fn(async () => result(h.stdout)),
}));

import * as vscode from "vscode";
import { TestRunner } from "./runner.js";
import { executeBatch } from "./batchRunner.js";
import type { RunPiece } from "./specStore.js";

const batch = vi.mocked(executeBatch);

// result is a batch's outcome: its stdout and the piece the Spec View keeps.
function result(stdout: string, folder = "/ws") {
  return { stdout, piece: { stream: stdout, folder } };
}

function item(
  id: string,
  pkg: string,
  opts: { filter?: string; tags?: string[] } = {},
) {
  return {
    id,
    pkg,
    filter: opts.filter,
    tags: (opts.tags ?? []).map((t) => ({ id: t })),
  } as any;
}

function makeRun() {
  return {
    started: vi.fn(),
    errored: vi.fn(),
    skipped: vi.fn(),
    addCoverage: vi.fn(),
    end: vi.fn(),
  };
}

interface Pkg {
  importPath: string;
  dir: string;
  workspaceDir?: string;
}

function setup(pkgs: Pkg[], opts: { coverageStore?: any } = {}) {
  const runs: ReturnType<typeof makeRun>[] = [];
  const controller = {
    createTestRun: vi.fn(() => {
      const r = makeRun();
      runs.push(r);
      return r;
    }),
    clearResults: vi.fn(),
    saveResults: vi.fn(),
    recordResult: vi.fn(),
  };
  const cache = {
    packages: pkgs,
    getPackage: (ip: string) => pkgs.find((p) => p.importPath === ip),
    getWorkspaceDir: (ip: string) =>
      pkgs.find((p) => p.importPath === ip)?.workspaceDir,
  };
  const log = { info: vi.fn(), error: vi.fn() };
  let next = 0;
  const registry = {
    register: vi.fn(() => ({ id: `rec${++next}` })),
    complete: vi.fn(),
    cancel: vi.fn(),
  };
  const runner = new TestRunner(
    controller as any,
    cache as any,
    log as any,
    registry as any,
    opts.coverageStore,
  );
  return { runner, controller, registry, runs };
}

function token() {
  return new vscode.CancellationTokenSource();
}

const A: Pkg = { importPath: "m/a", dir: "/ws/a", workspaceDir: "/ws" };
const B: Pkg = { importPath: "m/b", dir: "/ws/b", workspaceDir: "/ws" };
const C: Pkg = { importPath: "m/c", dir: "/ws2/c", workspaceDir: "/ws2" };

describe("TestRunner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    batch.mockImplementation(async () => result(h.stdout));
    h.config = {};
    h.stdout = "";
  });

  it("runs nothing for a selection of benchmarks alone, and says where they run", async () => {
    const { runner, registry } = setup([A]);
    await runner.run(
      { include: [item("b1", "m/a", { tags: ["benchmark"] })] } as any,
      token().token,
    );
    expect(batch).not.toHaveBeenCalled();
    expect(registry.register).not.toHaveBeenCalled();
    expect(vscode.window.showInformationMessage).toHaveBeenCalledWith(
      expect.stringContaining("Bench profile"),
    );
  });

  it("says nothing for a request with no items", async () => {
    const { runner } = setup([A]);
    await runner.run({ include: [] } as any, token().token);
    expect(batch).not.toHaveBeenCalled();
    expect(vscode.window.showInformationMessage).not.toHaveBeenCalled();
  });

  it("errors items of a package the cache does not know", async () => {
    const { runner, runs, controller } = setup([A]);
    const lost = item("x1", "m/gone");
    await runner.run({ include: [lost] } as any, token().token);
    expect(runs[0].errored).toHaveBeenCalledWith(lost, {
      message: "Package not found: m/gone",
    });
    expect(controller.recordResult).toHaveBeenCalledWith("x1", "errored");
    expect(batch).not.toHaveBeenCalled();
  });

  it("errors items whose package has no workspace folder", async () => {
    const { runner, runs, controller } = setup([
      { importPath: "m/o", dir: "/o" },
    ]);
    const orphan = item("o1", "m/o");
    await runner.run({ include: [orphan] } as any, token().token);
    expect(runs[0].errored).toHaveBeenCalledWith(orphan, {
      message: expect.stringContaining("Workspace folder not found"),
    });
    expect(controller.recordResult).toHaveBeenCalledWith("o1", "errored");
    expect(batch).not.toHaveBeenCalled();
  });

  it("batches unfiltered packages per workspace and gives each filter its own call", async () => {
    h.config = { testFlags: ["-race"] };
    const { runner } = setup([
      A,
      B,
      C,
      { ...B, importPath: "m/f", dir: "/ws/f" },
    ]);
    await runner.run(
      {
        include: [
          item("a", "m/a"),
          item("b", "m/b"),
          item("c", "m/c"),
          item("f", "m/f", { filter: "^TestF$" }),
        ],
      } as any,
      token().token,
    );

    const calls = batch.mock.calls.map(([cfg]) => ({
      ws: cfg.workspaceDir,
      pkgs: cfg.pkgInfos.map((p) => p.importPath),
      filter: cfg.filter,
      flags: cfg.testFlags,
      env: cfg.env,
    }));
    expect(calls).toEqual([
      {
        ws: "/ws",
        pkgs: ["m/a", "m/b"],
        filter: undefined,
        flags: ["-race"],
        env: undefined,
      },
      {
        ws: "/ws",
        pkgs: ["m/f"],
        filter: "^TestF$",
        flags: ["-race"],
        env: undefined,
      },
      {
        ws: "/ws2",
        pkgs: ["m/c"],
        filter: undefined,
        flags: ["-race"],
        env: undefined,
      },
    ]);
  });

  it("sets GOTEST_UPDATE_SNAPSHOTS when asked to update snapshots", async () => {
    const { runner } = setup([A]);
    await runner.run({ include: [item("a", "m/a")] } as any, token().token, {
      updateSnapshots: true,
    });
    expect(batch.mock.calls[0][0].env).toEqual({
      GOTEST_UPDATE_SNAPSHOTS: "1",
    });
  });

  it("records the run in the registry and completes it", async () => {
    const { runner, registry } = setup([A]);
    await runner.run(
      { include: [item("a1", "m/a"), item("a2", "m/a")] } as any,
      token().token,
    );
    expect(registry.register).toHaveBeenCalledWith({
      kind: "test",
      packages: ["a1", "a2"],
    });
    expect(registry.complete).toHaveBeenCalledWith("rec1");
    expect(registry.cancel).not.toHaveBeenCalled();
  });

  it("skips later batches after a stop and cancels the record", async () => {
    const outer = token();
    batch.mockImplementationOnce(async () => {
      outer.cancel();
      return result("");
    });
    const { runner, registry, runs } = setup([A, B, C]);
    const filtered = item("b", "m/b", { filter: "^TestB$" });
    const other = item("c", "m/c");
    await runner.run(
      { include: [item("a", "m/a"), filtered, other] } as any,
      outer.token,
    );

    expect(batch).toHaveBeenCalledTimes(1);
    expect(runs[0].skipped).toHaveBeenCalledWith(filtered);
    expect(runs[0].skipped).toHaveBeenCalledWith(other);
    expect(registry.cancel).toHaveBeenCalledWith("rec1");
    expect(registry.complete).not.toHaveBeenCalled();
    expect(runs[0].end).toHaveBeenCalledTimes(1);
  });

  it("cancels an active run before starting the next, and waits for it", async () => {
    const { runner, registry, runs } = setup([A]);
    const order: string[] = [];
    // The first batch takes a moment to wind down after the stop, as a
    // process tearing down fixtures does.
    batch.mockImplementationOnce(
      (cfg) =>
        new Promise((resolve) => {
          order.push("first batch started");
          cfg.token.onCancellationRequested(() =>
            setTimeout(() => {
              order.push("first batch returned");
              resolve(result(""));
            }, 5),
          );
        }),
    );
    batch.mockImplementationOnce(async () => {
      order.push(
        runs[0].end.mock.calls.length > 0
          ? "second batch started after the first run ended"
          : "second batch started early",
      );
      return result("");
    });

    const first = runner.run(
      { include: [item("a", "m/a")] } as any,
      token().token,
    );
    await vi.waitFor(() => expect(batch).toHaveBeenCalledTimes(1));
    const firstToken = batch.mock.calls[0][0].token;

    const second = runner.run(
      { include: [item("a", "m/a")] } as any,
      token().token,
    );
    await Promise.all([first, second]);

    expect(firstToken.isCancellationRequested).toBe(true);
    expect(registry.cancel).toHaveBeenCalledWith("rec1");
    expect(registry.complete).toHaveBeenCalledWith("rec2");
    expect(order).toEqual([
      "first batch started",
      "first batch returned",
      "second batch started after the first run ended",
    ]);
    expect(runs[1].end).toHaveBeenCalledTimes(1);
  });

  it("publishes the piece of every batch once the run ends", async () => {
    const one = result("one\n");
    const two = result("two\n", "/ws2");
    batch.mockResolvedValueOnce(one).mockResolvedValueOnce(two);
    const { runner } = setup([A, C]);
    const seen: RunPiece[][] = [];
    runner.onDidComplete((pieces) => seen.push(pieces));
    await runner.run(
      { include: [item("a", "m/a"), item("c", "m/c")] } as any,
      token().token,
    );
    expect(seen).toEqual([[one.piece, two.piece]]);
  });

  it("publishes nothing when no batch ran", async () => {
    const { runner } = setup([A]);
    const seen: RunPiece[][] = [];
    runner.onDidComplete((pieces) => seen.push(pieces));
    await runner.run({ include: [item("x", "m/gone")] } as any, token().token);
    expect(seen).toEqual([]);
  });

  it("adds the store's coverage to the run and saves it", async () => {
    const store = {
      buildFileCoverages: vi.fn(() => ({ coverages: ["fc1", "fc2"] })),
      save: vi.fn(async () => {}),
    };
    const { runner, runs } = setup([A], { coverageStore: store });
    await runner.run({ include: [item("a", "m/a")] } as any, token().token);

    expect(batch.mock.calls[0][0].coverage).toEqual({ store });
    expect(store.buildFileCoverages).toHaveBeenCalledTimes(1);
    expect(runs[0].addCoverage.mock.calls).toEqual([["fc1"], ["fc2"]]);
    expect(store.save).toHaveBeenCalledTimes(1);
  });

  it("leaves coverage alone when coverOnRun is off", async () => {
    h.config = { coverOnRun: false };
    const store = {
      buildFileCoverages: vi.fn(() => ({ coverages: ["fc1"] })),
      save: vi.fn(async () => {}),
    };
    const { runner, runs } = setup([A], { coverageStore: store });
    await runner.run({ include: [item("a", "m/a")] } as any, token().token);

    expect(batch.mock.calls[0][0].coverage).toBeUndefined();
    expect(store.buildFileCoverages).not.toHaveBeenCalled();
    expect(store.save).not.toHaveBeenCalled();
    expect(runs[0].addCoverage).not.toHaveBeenCalled();
  });

  it("asks for no coverage without a store", async () => {
    const { runner, runs } = setup([A]);
    await runner.run({ include: [item("a", "m/a")] } as any, token().token);
    expect(batch.mock.calls[0][0].coverage).toBeUndefined();
    expect(runs[0].addCoverage).not.toHaveBeenCalled();
  });

  it("offers to update snapshots on a mismatch and reruns with updates on", async () => {
    vi.mocked(vscode.window.showWarningMessage).mockResolvedValueOnce(
      "Update Snapshots" as any,
    );
    batch.mockResolvedValueOnce(result("MatchSnapshot: snapshot mismatch"));
    const { runner } = setup([A]);
    await runner.run({ include: [item("a", "m/a")] } as any, token().token);

    expect(vscode.window.showWarningMessage).toHaveBeenCalledWith(
      expect.any(String),
      "Update Snapshots",
    );
    await vi.waitFor(() => expect(batch).toHaveBeenCalledTimes(2));
    expect(batch.mock.calls[1][0].env).toEqual({
      GOTEST_UPDATE_SNAPSHOTS: "1",
    });
    expect(vscode.window.showWarningMessage).toHaveBeenCalledTimes(1);
  });

  it("does not rerun when the prompt is dismissed", async () => {
    vi.mocked(vscode.window.showWarningMessage).mockResolvedValueOnce(
      undefined as any,
    );
    batch.mockResolvedValueOnce(result("MatchSnapshot: snapshot mismatch"));
    const { runner } = setup([A]);
    await runner.run({ include: [item("a", "m/a")] } as any, token().token);
    await new Promise((r) => setTimeout(r, 0));
    expect(vscode.window.showWarningMessage).toHaveBeenCalledTimes(1);
    expect(batch).toHaveBeenCalledTimes(1);
  });

  it("does not prompt when the run was already updating snapshots", async () => {
    batch.mockResolvedValueOnce(result("MatchSnapshot: snapshot mismatch"));
    const { runner } = setup([A]);
    await runner.run({ include: [item("a", "m/a")] } as any, token().token, {
      updateSnapshots: true,
    });
    expect(vscode.window.showWarningMessage).not.toHaveBeenCalled();
  });

  it("ends the run when a batch throws", async () => {
    batch.mockRejectedValueOnce(new Error("spawn failed"));
    const { runner, runs, registry } = setup([A]);
    await expect(
      runner.run({ include: [item("a", "m/a")] } as any, token().token),
    ).rejects.toThrow("spawn failed");
    expect(runs[0].end).toHaveBeenCalledTimes(1);
    expect(registry.complete).toHaveBeenCalledWith("rec1");
  });

  it("ends the run on the early returns too", async () => {
    const { runner, runs } = setup([A]);
    await runner.run({ include: [] } as any, token().token);
    expect(runs[0].end).toHaveBeenCalledTimes(1);
  });
});
