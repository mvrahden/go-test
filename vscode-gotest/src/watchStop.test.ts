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
});
