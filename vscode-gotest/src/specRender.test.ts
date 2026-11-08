// The spec render used to have no budget, no kill path and no cancellation: a
// wedged CLI left the Spec View waiting forever with nothing able to end it.

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SpawnScript } from "./scriptedSpawn.test-support.js";

const { script, mockKill, buildCli } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  mockKill: vi.fn(),
  buildCli: vi.fn(async (_args: string[], _dir?: string) => ({
    bin: "go",
    args: ["run", "."],
  })),
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
  buildCliCommand: buildCli,
  offerCliUpgrade: () => false,
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

// A spec document as the CLI writes it for one package.
function doc(pkg: string, passed: number, failed: number): string {
  return JSON.stringify({
    version: "v1.36.0",
    packages: [
      { path: pkg, status: "pass", duration: 0, nodes: [], output: [] },
    ],
    stats: {
      suites: 1,
      behaviors: passed + failed,
      tests: 0,
      fuzzers: 0,
      passed,
      failed,
      skipped: 0,
    },
  });
}

const runOf = (pkg: string) =>
  JSON.stringify({ Action: "pass", Package: pkg, Elapsed: 0.1 }) + "\n";

describe("which CLI renders a package", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    script.once = [];
    script.always = undefined;
    script.calls = [];
  });

  it("is the one of the folder that ran it, each folder rendering its own", async () => {
    script.once = [
      { stdout: [doc("example.com/a", 2, 0)], code: 0 },
      { stdout: [doc("example.com/b", 1, 1)], code: 0 },
    ];
    const panel = new SpecViewPanel(channel, undefined as never);
    const internals = panel as unknown as {
      runs: { merge(p: { stream: string; folder: string }): void };
      renderRuns(): Promise<{
        packages: { path: string }[];
        stats: { passed: number; failed: number; suites: number };
      }>;
    };
    internals.runs.merge({ stream: runOf("example.com/a"), folder: "/ws/a" });
    internals.runs.merge({ stream: runOf("example.com/b"), folder: "/ws/b" });

    const data = await internals.renderRuns();

    expect(buildCli.mock.calls.map((c) => c[1])).toEqual(["/ws/a", "/ws/b"]);
    expect(script.calls?.map((c) => c.opts?.cwd)).toEqual(["/ws/a", "/ws/b"]);
    expect(data.packages.map((p) => p.path)).toEqual([
      "example.com/a",
      "example.com/b",
    ]);
    expect(data.stats).toMatchObject({ suites: 2, passed: 3, failed: 1 });
  });
});

describe("the runs the panel keeps across a reload", () => {
  const runs = (panel: SpecViewPanel) =>
    (panel as unknown as { runs: { packages: string[] } }).runs.packages;

  it("come back from storage, and never over a run merged first", async () => {
    vi.useRealTimers();
    const { mkdtempSync, rmSync } = await import("node:fs");
    const { tmpdir } = await import("node:os");
    const path = await import("node:path");
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-specruns-"));
    try {
      const before = new SpecViewPanel(channel, undefined as never, dir);
      await before.refresh({ stream: runOf("example.com/a"), folder: "/ws" });
      await before.flush();

      const after = new SpecViewPanel(channel, undefined as never, dir);
      await after.load();
      expect(runs(after)).toEqual(["example.com/a"]);

      const raced = new SpecViewPanel(channel, undefined as never, dir);
      await raced.refresh({ stream: runOf("example.com/b"), folder: "/ws" });
      await raced.load();
      expect(runs(raced)).toEqual(["example.com/b"]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("a kept-runs file of the wrong shape", () => {
  it("loads as nothing kept, without throwing, and later runs still land", async () => {
    vi.useRealTimers();
    const { mkdtempSync, rmSync, writeFileSync } = await import("node:fs");
    const { tmpdir } = await import("node:os");
    const path = await import("node:path");
    const shapes = [
      null,
      { not: "an array" },
      [
        {
          path: "example.com/a",
          folder: "/ws",
          pkgLines: [],
          tests: 7,
          runLevel: false,
        },
      ],
      [
        {
          path: "example.com/a",
          folder: "/ws",
          pkgLines: [],
          tests: [["T", null]],
          runLevel: false,
        },
      ],
    ];
    for (const data of shapes) {
      const dir = mkdtempSync(path.join(tmpdir(), "gotest-specruns-bad-"));
      try {
        writeFileSync(
          path.join(dir, "specRuns.json"),
          JSON.stringify({ version: 1, data }),
        );
        const panel = new SpecViewPanel(channel, undefined as never, dir);
        await expect(panel.load()).resolves.toBeUndefined();
        const runs = (panel as unknown as { runs: { packages: string[] } })
          .runs;
        expect(runs.packages).toEqual([]);
        await panel.refresh({ stream: runOf("example.com/b"), folder: "/ws" });
        expect(runs.packages).toEqual(["example.com/b"]);
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    }
  });
});
