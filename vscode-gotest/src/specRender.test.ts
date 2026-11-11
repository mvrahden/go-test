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

const { clipboard, notes } = vi.hoisted(() => ({
  clipboard: { text: "" },
  notes: { info: [] as string[], warn: [] as string[] },
}));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "/ws" } }],
    getConfiguration: () => ({ get: () => undefined }),
  },
  env: {
    clipboard: {
      writeText: async (t: string) => {
        clipboard.text = t;
      },
    },
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  window: {
    showInformationMessage: async (m: string) => {
      notes.info.push(m);
    },
    showWarningMessage: async (m: string) => {
      notes.warn.push(m);
    },
  },
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

// runSpec is private; the behaviour under test is its process handling.
function render(panel: SpecViewPanel, input: string): Promise<string> {
  return (
    panel as unknown as {
      runSpec(s: string, folder: undefined, format: string): Promise<string>;
    }
  ).runSpec(input, undefined, "json");
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

describe("what the panel asks the CLI for", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    script.once = [];
    script.always = undefined;
    script.calls = [];
  });

  type Internals = {
    runs: { merge(p: { stream: string; folder: string }): void };
    imports: Map<string, unknown>;
    selected: string | undefined;
    handleMessage(msg: unknown): void;
    renderRuns(): Promise<unknown>;
    copy(): Promise<void>;
  };
  // copies runs the panel's copy as the webview's message asks for it.
  const copies = async (panel: SpecViewPanel, msg: unknown) => {
    internalsOf(panel).handleMessage(msg);
    await vi.waitFor(() => expect(notes.info.length).toBeGreaterThan(0));
  };
  const internalsOf = (panel: SpecViewPanel) => panel as unknown as Internals;
  const argsOfCall = (i: number) => buildCli.mock.calls[i][0];

  it("lays the workspace's runs over what the folder's source declares, hiding what the toolbar hides", async () => {
    script.always = { stdout: [doc("example.com/a", 1, 0)], code: 0 };
    const patternsFor = vi.fn(async () => ["./...", "./examples/..."]);
    const panel = new SpecViewPanel(
      channel,
      undefined as never,
      undefined,
      patternsFor,
    );
    const internals = internalsOf(panel);
    internals.runs.merge({ stream: runOf("example.com/a"), folder: "/ws/a" });
    internals.handleMessage({ type: "toggleHide", status: "pass" });
    internals.handleMessage({ type: "toggleHide", status: "none" });
    internals.handleMessage({ type: "toggleHide", status: "pass" });
    internals.handleMessage({ type: "toggleHide", status: "skip" });

    await internals.renderRuns();

    expect(patternsFor).toHaveBeenCalledWith("/ws/a");
    expect(argsOfCall(buildCli.mock.calls.length - 1)).toEqual([
      "spec",
      "--input=-",
      "--format=json",
      "--render-only",
      "--hide=none,skip",
      "--declared",
      "./...",
      "./examples/...",
    ]);
  });

  it("renders an imported stream as it is, without the source under it", async () => {
    script.always = { stdout: [doc("example.com/a", 1, 0)], code: 0 };
    const panel = new SpecViewPanel(
      channel,
      undefined as never,
      undefined,
      async () => ["./..."],
    );
    const internals = internalsOf(panel);
    const imported = new (await import("./specStore.js")).SpecStore();
    imported.merge({ stream: runOf("example.com/a"), folder: "/ws" });
    internals.imports.set("ci", { label: "ci.json", runs: imported });
    internals.selected = "ci";

    await internals.renderRuns();

    expect(argsOfCall(0)).toEqual([
      "spec",
      "--input=-",
      "--format=json",
      "--render-only",
      "--hide=",
    ]);
  });

  it("copies what it shows as the CLI's Markdown, with the same flags", async () => {
    script.always = { stdout: ["# Behavior Specification\n"], code: 0 };
    const panel = new SpecViewPanel(
      channel,
      undefined as never,
      undefined,
      async () => ["./..."],
    );
    const internals = internalsOf(panel);
    internals.runs.merge({ stream: runOf("example.com/a"), folder: "/ws/a" });
    internals.handleMessage({ type: "toggleHide", status: "pass" });
    await vi.waitFor(() => expect(buildCli).toHaveBeenCalled());
    buildCli.mockClear();

    await internals.copy();

    expect(argsOfCall(0)).toEqual([
      "spec",
      "--input=-",
      "--format=md",
      "--render-only",
      "--hide=pass",
      "--declared",
      "./...",
    ]);
    expect(clipboard.text).toBe("# Behavior Specification\n");
  });

  async function loaded(md: string): Promise<SpecViewPanel> {
    notes.info = [];
    notes.warn = [];
    script.always = { stdout: [md], code: 0 };
    const panel = new SpecViewPanel(channel, undefined as never);
    internalsOf(panel).runs.merge({
      stream: runOf("example.com/pkg"),
      folder: "/ws/a",
    });
    internalsOf(panel).runs.merge({
      stream: runOf("example.com/other"),
      folder: "/ws/b",
    });
    buildCli.mockClear();
    return panel;
  }

  it("copies what the search leaves, as the CLI's match", async () => {
    const panel = await loaded("# spec\n");
    await copies(panel, { type: "copySpec", search: "  email " });
    expect(buildCli.mock.calls.map((c) => c[0])).toEqual([
      expect.arrayContaining(["--format=md", "--match=email"]),
      expect.arrayContaining(["--format=md", "--match=email"]),
    ]);
  });

  it("asks for no match when the search is empty", async () => {
    const panel = await loaded("# spec\n");
    await copies(panel, { type: "copySpec", search: "" });
    expect(argsOfCall(0).some((a) => a.startsWith("--match"))).toBe(false);
  });

  it("copies one node from the folder of the row's package", async () => {
    const panel = await loaded("## node\n");
    await copies(panel, {
      type: "copyNode",
      pkg: "example.com/pkg",
      position: [0, 1],
    });
    expect(buildCli.mock.calls).toHaveLength(1);
    expect(argsOfCall(0)).toEqual(
      expect.arrayContaining(["--format=md", "--only=example.com/pkg:0.1"]),
    );
    expect(buildCli.mock.calls[0][1]).toBe("/ws/a");
    expect(clipboard.text).toBe("## node\n");
  });

  it("copies a whole package for an empty position", async () => {
    const panel = await loaded("## pkg\n");
    await copies(panel, {
      type: "copyNode",
      pkg: "example.com/other",
      position: [],
    });
    expect(argsOfCall(0)).toContain("--only=example.com/other");
    expect(buildCli.mock.calls[0][1]).toBe("/ws/b");
  });

  it("says how big the copy is", async () => {
    const panel = await loaded("x".repeat(400));
    await copies(panel, { type: "copySpec", search: "" });
    expect(notes.info).toEqual(["Copied 801 characters (about 200 tokens)."]);
    expect(notes.warn).toEqual([]);
  });

  it("warns when the copy is above GitHub's comment limit", async () => {
    const panel = await loaded("x".repeat(35_000));
    await copies(panel, { type: "copySpec", search: "" });
    await vi.waitFor(() => expect(notes.warn).toHaveLength(1));
    expect(notes.warn[0]).toContain(
      "above GitHub's 65,536-character comment limit.",
    );
  });
});
