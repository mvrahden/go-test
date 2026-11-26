// The host sets the webview's document once and from then on posts small,
// coalesced messages: a hidden panel gets nothing, a tab not on show only its
// badge, and a run in progress updates the Now tab from its events without
// rendering through the CLI until it ends.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { SpawnScript, FakeChild } from "../scriptedSpawn.test-support.js";

const { script, children, panels } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  children: [] as FakeChild[],
  panels: [] as unknown[],
}));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [{ uri: { fsPath: "/ws" } }],
    getConfiguration: () => ({ get: () => undefined }),
  },
  Uri: { file: (p: string) => ({ fsPath: p }), joinPath: () => ({}) },
  ViewColumn: { One: 1, Beside: 2 },
  window: {
    createWebviewPanel: () => panels.shift(),
    visibleTextEditors: [],
  },
  commands: { executeCommand: vi.fn() },
  env: { clipboard: { writeText: vi.fn() } },
}));

vi.mock("../cli.js", () => ({
  buildCliCommand: vi.fn(async () => ({ bin: "gotest", args: ["spec"] })),
  hasRuns: async () => false,
  offerCliUpgrade: () => false,
  stripGoRunExitEcho: (s: string) => s,
}));

vi.mock("../gomod.js", () => ({ readModulePath: async () => undefined }));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("../scriptedSpawn.test-support.js");
  const spawn = createScriptedSpawn(script);
  return {
    spawn: (bin: string, args: string[], opts?: Record<string, unknown>) => {
      const child = spawn(bin, args, opts);
      children.push(child);
      return child;
    },
    // git, for the branch: no checkout here.
    execFile: (_b: string, _a: string[], _o: unknown, cb: (e: Error) => void) =>
      cb(new Error("no git")),
  };
});

import { SpecViewPanel } from "./host.js";
import { COALESCE_MS } from "./poster.js";
import { LiveRuns } from "../liveProgress.js";
import { Sessions } from "../sessions.js";
import { MESSAGE_BUDGET, type HostMessage } from "./protocol.js";

const channel = {
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
  debug: vi.fn(),
} as never;

const specDoc = (pkg: string, failed = 0) =>
  JSON.stringify({
    packages: [
      {
        path: pkg,
        status: failed ? "fail" : "pass",
        duration: 1,
        stats: {
          suites: 1,
          behaviors: 2,
          tests: 0,
          passed: 2 - failed,
          failed,
          skipped: 0,
        },
        nodes: [
          {
            name: "TestSTestSuite",
            display: "S",
            kind: "suite",
            status: failed ? "fail" : "pass",
            duration: 1,
            focused: false,
            excluded: false,
            external: false,
            output: [],
            children: [
              {
                name: "a",
                display: "a",
                kind: "block",
                status: failed ? "fail" : "pass",
                duration: 0,
                focused: false,
                excluded: false,
                external: false,
                output: ["    s_test.go:3: boom\n"],
                children: [],
              },
              {
                name: "b",
                display: "b",
                kind: "block",
                status: "pass",
                duration: 0,
                focused: false,
                excluded: false,
                external: false,
                output: [],
                children: [],
              },
            ],
          },
        ],
      },
    ],
    stats: {
      suites: 1,
      behaviors: 2,
      tests: 0,
      passed: 2 - failed,
      failed,
      skipped: 0,
    },
  });

const line = (e: object) =>
  JSON.stringify({ Package: "m/a", Time: "2026-11-25T10:00:00Z", ...e });

interface FakePanel {
  visible: boolean;
  htmlSets: number;
  webview: {
    html: string;
    cspSource: string;
    postMessage: ReturnType<typeof vi.fn>;
    onDidReceiveMessage: (fn: (m: unknown) => void) => void;
    asWebviewUri: (u: unknown) => unknown;
  };
  send?: (m: unknown) => void;
  seen?: (e: { webviewPanel: { visible: boolean } }) => void;
  onDidChangeViewState: (
    fn: (e: { webviewPanel: { visible: boolean } }) => void,
  ) => void;
  onDidDispose: () => void;
  reveal: () => void;
  dispose: () => void;
  viewColumn: number;
}

function fakePanel(visible = true): FakePanel {
  let html = "";
  const p: FakePanel = {
    visible,
    htmlSets: 0,
    webview: {
      get html() {
        return html;
      },
      set html(v: string) {
        html = v;
        p.htmlSets++;
      },
      cspSource: "vscode-resource:",
      postMessage: vi.fn(async () => true),
      onDidReceiveMessage: (fn) => {
        p.send = fn;
      },
      asWebviewUri: (u) => u,
    },
    onDidChangeViewState: (fn) => {
      p.seen = fn;
    },
    onDidDispose: () => {},
    reveal: () => {},
    dispose: () => {},
    viewColumn: 2,
  };
  return p;
}

const posts = (p: FakePanel): HostMessage[] =>
  p.webview.postMessage.mock.calls.map(([m]) => m as HostMessage);
const ofType = (p: FakePanel, t: HostMessage["type"]) =>
  posts(p).filter((m) => m.type === t);

async function setup(visible = true) {
  const live = new LiveRuns();
  const sessions = new Sessions();
  const fake = fakePanel(visible);
  panels.push(fake);
  const panel = new SpecViewPanel(
    channel,
    undefined,
    undefined,
    undefined,
    undefined,
    live,
    sessions,
  );
  await panel.show();
  fake.send!({ type: "ready", tab: "now", open: [], search: "", filters: [] });
  return { panel, fake, live, sessions };
}

describe("the panel's host", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    children.length = 0;
    panels.length = 0;
    script.once = [];
    script.always = { stdout: [specDoc("m/a", 1)], code: 1 };
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("sets the webview's document once, across tab switches, refreshes and a run", async () => {
    const { panel, fake, live } = await setup();
    await panel.refresh({
      stream: line({ Action: "pass" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    for (const tab of ["change", "runs", "now", "runs", "now"])
      fake.send!({ type: "tab", tab });
    fake.send!({ type: "refresh" });
    const run = live.start("run: m/a");
    run.note(JSON.parse(line({ Action: "run", Test: "TestSTestSuite" })));
    await vi.advanceTimersByTimeAsync(2000);
    run.end();
    await panel.refresh({
      stream: line({ Action: "fail" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    await panel.show("runs");
    panel.clear();
    await vi.advanceTimersByTimeAsync(1000);
    expect(fake.htmlSets).toBe(1);
    expect(fake.webview.html).toContain("Content-Security-Policy");
    expect(ofType(fake, "now").length).toBeGreaterThan(0);
  });

  it("restores a panel with one document, on the tab and with the packages the webview kept", async () => {
    const fake = fakePanel();
    const panel = new SpecViewPanel(
      channel,
      undefined,
      undefined,
      undefined,
      undefined,
      new LiveRuns(),
      new Sessions(),
    );
    await panel.restorePanel(fake as never, {
      tab: "runs",
      open: ["m/a"],
      search: "x",
    });
    expect(fake.htmlSets).toBe(1);
    expect(fake.webview.html).toContain('"tab":"runs"');
  });

  it("posts the Now summary at most once per interval while news keeps coming", async () => {
    const { fake, live } = await setup();
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    const before = ofType(fake, "now").length;
    const run = live.start("run: m/a");
    for (let i = 0; i < 100; i++) {
      run.note(
        JSON.parse(line({ Action: "run", Test: `TestSTestSuite/T${i}` })),
      );
      await vi.advanceTimersByTimeAsync(10);
    }
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    const sent = ofType(fake, "now").length - before;
    // 1s of events: at most one post per interval, and the last state lands.
    expect(sent).toBeGreaterThanOrEqual(2);
    // A quarter-second interval: about four posts in a second.
    expect(sent).toBeLessThanOrEqual(5);
    const last = ofType(fake, "now").at(-1) as Extract<
      HostMessage,
      { type: "now" }
    >;
    expect(last.model.packages[0].counts).toContain("running");
    run.end();
  });

  it("posts nothing to a hidden panel, and catches up when it is seen", async () => {
    const { fake, live } = await setup(false);
    const run = live.start("run: m/a");
    run.note(JSON.parse(line({ Action: "run", Test: "TestSTestSuite" })));
    await vi.advanceTimersByTimeAsync(5 * COALESCE_MS);
    expect(fake.webview.postMessage).not.toHaveBeenCalled();
    fake.visible = true;
    fake.seen!({ webviewPanel: { visible: true } });
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    expect(ofType(fake, "now").length).toBe(1);
    run.end();
  });

  it("tells a tab not on show only its badge", async () => {
    const { fake, live } = await setup();
    fake.send!({ type: "tab", tab: "runs" });
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    fake.webview.postMessage.mockClear();
    const run = live.start("run: m/a");
    run.note(JSON.parse(line({ Action: "run", Test: "TestSTestSuite/a" })));
    run.note(JSON.parse(line({ Action: "fail", Test: "TestSTestSuite/a" })));
    await vi.advanceTimersByTimeAsync(5 * COALESCE_MS);
    expect(ofType(fake, "now")).toEqual([]);
    expect(ofType(fake, "badges").at(-1)).toEqual({
      type: "badges",
      now: 1,
      change: 0,
    });
    fake.send!({ type: "tab", tab: "now" });
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    expect(ofType(fake, "now").length).toBe(1);
    run.end();
  });

  it("shows a run's failure at once without the CLI, and renders through the CLI once when it ends", async () => {
    script.always = { stdout: [specDoc("m/a", 0)], code: 0 };
    const { panel, fake, live } = await setup();
    await panel.refresh({
      stream: line({ Action: "pass" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    const spawned = children.length;
    const run = live.start("run: m/a");
    for (const e of [
      { Action: "run", Test: "TestSTestSuite" },
      { Action: "run", Test: "TestSTestSuite/a" },
      {
        Action: "output",
        Test: "TestSTestSuite/a",
        Output: "    s_test.go:3: boom\n",
      },
      { Action: "fail", Test: "TestSTestSuite/a" },
    ])
      run.note(JSON.parse(line(e)));
    await vi.advanceTimersByTimeAsync(10_000);
    expect(children.length).toBe(spawned);
    const mid = ofType(fake, "now").at(-1) as Extract<
      HostMessage,
      { type: "now" }
    >;
    expect(mid.model.cards.map((c) => [c.name, c.live])).toEqual([["a", true]]);
    expect(mid.model.context.live?.[0]).toMatch(
      /^Running run: m\/a · 0:\d\d · 2 started · 0 passed · 1 failed$/,
    );
    run.end();
    script.always = { stdout: [specDoc("m/a", 1)], code: 1 };
    await panel.refresh({
      stream: line({ Action: "fail" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    expect(children.length).toBe(spawned + 1);
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    const after = ofType(fake, "now").at(-1) as Extract<
      HostMessage,
      { type: "now" }
    >;
    expect(after.model.cards.map((c) => [c.name, c.live ?? false])).toEqual([
      ["a", false],
    ]);
    expect(after.model.context.live).toBeUndefined();
  });

  it("sends a package's rows only when it is opened, again on a new render, and a row's output on request", async () => {
    const { panel, fake } = await setup();
    await panel.refresh({
      stream: line({ Action: "fail" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    expect(ofType(fake, "package")).toEqual([]);
    fake.send!({ type: "openPackage", pkg: "m/a" });
    const [pkgMsg] = ofType(fake, "package") as Extract<
      HostMessage,
      { type: "package" }
    >[];
    expect(pkgMsg.nodes[0].c!.map((n) => n.d)).toEqual(["a", "b"]);
    fake.send!({ type: "refresh" });
    await vi.waitFor(() => expect(ofType(fake, "package")).toHaveLength(2));
    fake.send!({ type: "output", key: "k", pkg: "m/a", pos: "0.0" });
    expect(ofType(fake, "output").at(-1)).toMatchObject({
      type: "output",
      key: "k",
      lines: [["    s_test.go:3: boom"]],
    });
    for (const m of posts(fake))
      if (m.type !== "package")
        expect(JSON.stringify(m).length).toBeLessThan(MESSAGE_BUDGET);
  });

  it("narrows the summary and the opened rows to a search and the chips", async () => {
    const { panel, fake } = await setup();
    await panel.refresh({
      stream: line({ Action: "fail" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    fake.send!({ type: "openPackage", pkg: "m/a" });
    fake.send!({ type: "filters", filters: ["fail", "bogus"] });
    const last = ofType(fake, "package").at(-1) as Extract<
      HostMessage,
      { type: "package" }
    >;
    expect(last.nodes[0].c!.map((n) => n.d)).toEqual(["a"]);
    fake.send!({ type: "search", text: "b" });
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    const now = ofType(fake, "now").at(-1) as Extract<
      HostMessage,
      { type: "now" }
    >;
    expect(now.model.packages[0].matches).toBe(0);
  });

  it("runs a row by its item and notes the ids above it; Rerun failed runs the narrowest failed rows", async () => {
    const { panel, fake } = await setup();
    const run = vi.fn(async () => {});
    panel.setActions({ run, debug: vi.fn(async () => {}) });
    await panel.refresh({
      stream: line({ Action: "fail" }) + "\n",
      folder: "/ws",
      at: Date.now(),
    });
    fake.send!({ type: "run", id: "m/a/STestSuite/a", pkg: "m/a", pos: "0.0" });
    expect(run).toHaveBeenLastCalledWith(["m/a/STestSuite/a"]);
    expect(panel.ancestorsOf("m/a/STestSuite/a")).toEqual([
      "m/a",
      "m/a/STestSuite",
    ]);
    fake.send!({ type: "rerunFailed" });
    expect(run).toHaveBeenLastCalledWith(["m/a/STestSuite/a"]);
  });

  it("stops a session from the Running block, and redraws the block while one runs", async () => {
    const { fake, sessions } = await setup();
    fake.send!({ type: "tab", tab: "runs" });
    const stop = vi.fn();
    sessions.add({ kind: "fuzz", folder: "/ws", target: "m/a FuzzX", stop });
    await vi.advanceTimersByTimeAsync(3000 + COALESCE_MS);
    const running = ofType(fake, "running") as Extract<
      HostMessage,
      { type: "running" }
    >[];
    expect(running.length).toBeGreaterThanOrEqual(3);
    expect(running.at(-1)!.items[0]).toMatchObject({
      label: "fuzz: m/a FuzzX",
      elapsed: "0:03",
      stopping: false,
    });
    fake.send!({ type: "stopSession", id: running.at(-1)!.items[0].id });
    expect(stop).toHaveBeenCalledOnce();
    const tally = { started: 12, passed: 10, failed: 1 };
    sessions.add({
      kind: "run",
      folder: "/ws",
      target: "3 items",
      stop: () => {},
      progress: { tally },
    });
    await vi.advanceTimersByTimeAsync(COALESCE_MS);
    const last = ofType(fake, "running").at(-1) as Extract<
      HostMessage,
      { type: "running" }
    >;
    expect(last.items.map((i) => [i.label, i.counts, i.stopping])).toEqual([
      ["fuzz: m/a FuzzX", undefined, true],
      ["run: 3 items", "12 started · 10 passed · 1 failed", false],
    ]);
  });
});
