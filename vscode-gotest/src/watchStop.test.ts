// gotest watch holds fixtures for as long as it runs, so it is stopped the
// way a run is: by a line on its stdin, which Windows honours too.

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SpawnScript, FakeChild } from "./scriptedSpawn.test-support.js";

const h = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  children: [] as FakeChild[],
}));

vi.mock("vscode", () => ({
  Uri: { file: (p: string) => ({ fsPath: p }) },
  StatusBarAlignment: { Left: 1 },
  TestRunRequest: class {},
  TestMessage: class {
    constructor(public readonly message: string) {}
  },
  window: {
    createStatusBarItem: () => ({ show() {}, hide() {}, dispose() {} }),
    showErrorMessage: vi.fn(),
    showWarningMessage: vi.fn(),
  },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  const spawn = createScriptedSpawn(h.script);
  return {
    spawn: (bin: string, args: string[], opts?: Record<string, unknown>) => {
      const child = spawn(bin, args, opts);
      const kill = child.kill;
      child.kill = vi.fn((signal?: NodeJS.Signals) => kill(signal));
      h.children.push(child);
      return child;
    },
  };
});

vi.mock("./cli.js", () => ({
  buildCliCommand: vi.fn(async (args: string[]) => ({
    bin: "go",
    args: ["tool", "gotest", ...args],
  })),
  clearBinaryCache: vi.fn(),
  formatCliCommand: (cmd: { bin: string; args: string[] }) =>
    `${cmd.bin} ${cmd.args.join(" ")}`,
  offerCliUpgrade: vi.fn(() => false),
}));

import { WatchManager } from "./watch.js";

const log = { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() };
const registry = {
  register: () => ({ id: "r" }),
  cancel: vi.fn(),
  crash: vi.fn(),
};

function manager(): WatchManager {
  return new WatchManager(
    {} as never,
    {} as never,
    log as never,
    () => {},
    registry as never,
  );
}

describe("WatchManager", () => {
  beforeEach(() => {
    h.script.once = [];
    h.script.always = { neverExits: true };
    h.script.calls = [];
    h.children = [];
  });

  it("starts gotest watch with --control=stdin, before the go test flags", async () => {
    await manager().start("./pkg/...", "/ws");
    expect(h.script.calls?.[0].args).toEqual([
      "tool",
      "gotest",
      "watch",
      "--control=stdin",
      "--",
      "-json",
      "./pkg/...",
    ]);
  });

  it("stops a watch by its stdin, never a signal", async () => {
    const m = manager();
    await m.start("./pkg/...", "/ws");
    m.stop("./pkg/...");
    expect(h.children[0].written).toEqual(["stop\n"]);
    expect(h.children[0].kill).not.toHaveBeenCalled();
  });

  it("stops every watch by its stdin when the extension shuts down", async () => {
    const m = manager();
    await m.start("./a/...", "/ws");
    await m.start("./b/...", "/ws");
    m.dispose();
    for (const child of h.children) {
      expect(child.written).toEqual(["stop\n"]);
      expect(child.kill).not.toHaveBeenCalled();
    }
  });

  it.each([
    [1, "exit code 1"],
    [0, "the watch process exited with exit code 0 during the cycle"],
  ])(
    "ends a cycle the watch process died in with exit code %i, erroring the packages it reported with that code",
    async (code, said) => {
      const pkg = {
        id: "example.com/pkg",
        label: "example.com/pkg",
        children: new Map(),
        tags: [{ id: "package" }],
      };
      const results = new Map<string, { status: string }>();
      const run = {
        started: vi.fn(),
        passed: vi.fn(),
        failed: vi.fn(),
        skipped: vi.fn(),
        errored: vi.fn(),
        appendOutput: vi.fn(),
        end: vi.fn(),
      };
      const controller = {
        createTestRun: () => run,
        findItem: (id: string) => (id === pkg.id ? pkg : undefined),
        recordResult: (id: string, status: string) =>
          results.set(id, { status }),
        getResult: (id: string) => results.get(id),
        noteStart: () => {},
        notePaused: () => {},
        saveResults: () => {},
        testController: {
          items: { forEach: (cb: (i: unknown) => void) => cb(pkg) },
        },
      };
      const line = (e: Record<string, unknown>) =>
        JSON.stringify({ Package: "example.com/pkg", ...e }) + "\n";
      h.script.once = [
        {
          stdout: [
            JSON.stringify({ Action: "watch-start" }) +
              "\n" +
              line({ Action: "run", Test: "TestMySuite" }) +
              line({ Action: "pass", Test: "TestMySuite", Elapsed: 0.01 }) +
              line({ Action: "pass", Elapsed: 0.02 }),
          ],
          code,
        },
      ];
      const m = new WatchManager(
        controller as never,
        { resolveImportPath: () => "/ws/pkg" } as never,
        log as never,
        () => {},
        registry as never,
      );
      await m.start("./...", "/ws");
      try {
        await vi.waitFor(() => expect(run.end).toHaveBeenCalled());
        expect(run.errored).toHaveBeenCalledTimes(1);
        expect(run.errored.mock.calls[0][0]).toBe(pkg);
        expect(run.errored.mock.calls[0][1].message).toBe(said);
        expect(run.end).toHaveBeenCalledTimes(1);
      } finally {
        m.dispose();
      }
    },
  );
});
