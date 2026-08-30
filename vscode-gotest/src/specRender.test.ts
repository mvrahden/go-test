// The spec render used to have no budget, no kill path and no cancellation: a
// wedged CLI left the Spec View waiting forever with nothing able to end it.

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SpawnScript } from "./scriptedSpawn.test-support.js";

const { script, mockKill } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  mockKill: vi.fn(),
}));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "/ws" } }],
    getConfiguration: () => ({ get: () => undefined }),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  window: {},
  EventEmitter: class {
    event = () => ({ dispose: () => {} });
    fire = () => {};
    dispose = () => {};
  },
}));

vi.mock("./cli.js", () => ({
  buildCliCommand: async () => ({ bin: "go", args: ["run", "."] }),
  stripGoRunExitEcho: (s: string) => s,
}));

vi.mock("./gomod.js", () => ({ readModulePath: async () => undefined }));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  return { spawn: createScriptedSpawn(script, mockKill) };
});

import { SpecViewPanel } from "./specView.js";

// runSpecFromInput is private; the behaviour under test is its process handling.
function render(panel: SpecViewPanel, input: string): Promise<string> {
  return (
    panel as unknown as { runSpecFromInput(s: string): Promise<string> }
  ).runSpecFromInput(input);
}

const channel = {
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
  debug: vi.fn(),
  appendLine: vi.fn(),
  show: vi.fn(),
} as never;

describe("spec render process handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    script.once = [];
    script.always = undefined;
    script.calls = [];
  });

  it("gives up on a CLI that never answers, and ends it", async () => {
    script.always = { neverExits: true };
    const panel = new SpecViewPanel(channel, undefined as never);

    const p = render(panel, "{}");
    const settled = expect(p).rejects.toThrow("spec render timed out");
    await vi.advanceTimersByTimeAsync(120_000);
    expect(mockKill).toHaveBeenCalledWith("SIGTERM");
    await settled;
  });

  it("escalates to SIGKILL when the CLI ignores SIGTERM", async () => {
    script.always = { neverExits: true, ignoresSigterm: true };
    const panel = new SpecViewPanel(channel, undefined as never);

    const p = render(panel, "{}");
    const settled = expect(p).rejects.toThrow("spec render timed out");
    await vi.advanceTimersByTimeAsync(120_000);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(mockKill).toHaveBeenCalledWith("SIGKILL");
    await settled;
  });

  it("does not signal a render that answered in time", async () => {
    script.always = { stdout: ['{"tree":[]}'], code: 0 };
    const panel = new SpecViewPanel(channel, undefined as never);

    await expect(render(panel, "{}")).resolves.toContain("tree");
    await vi.advanceTimersByTimeAsync(200_000);
    expect(mockKill).not.toHaveBeenCalled();
  });
});
