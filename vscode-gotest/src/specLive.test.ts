// While a test run is in progress the Spec scope shows a banner with what
// the run applied so far, and has the CLI render the rows the run reached:
// spaced, one render at a time, never for a hidden panel, and stopped when a
// newer picture supersedes it.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { SpawnScript, FakeChild } from "./scriptedSpawn.test-support.js";

const { script, mockKill, children } = vi.hoisted(() => ({
  script: { once: [], always: undefined, calls: [] } as SpawnScript,
  mockKill: vi.fn(),
  children: [] as FakeChild[],
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
  buildCliCommand: vi.fn(async () => ({ bin: "gotest", args: ["spec"] })),
  hasRuns: async () => false,
  offerCliUpgrade: () => false,
  stripGoRunExitEcho: (s: string) => s,
}));

vi.mock("./gomod.js", () => ({ readModulePath: async () => undefined }));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  const spawn = createScriptedSpawn(script, mockKill);
  return {
    spawn: (bin: string, args: string[], opts?: Record<string, unknown>) => {
      const child = spawn(bin, args, opts);
      children.push(child);
      return child;
    },
  };
});

import {
  LIVE_BANNER_MS,
  LIVE_RENDER_MS,
  SpecViewPanel,
  liveBannerHtml,
} from "./specView.js";
import { LiveRuns, type RunProgress } from "./liveProgress.js";

const channel = {
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
  debug: vi.fn(),
} as never;

function doc(pkg: string, passed: number): string {
  return JSON.stringify({
    packages: [{ path: pkg, status: "none", duration: 0, nodes: [] }],
    stats: {
      suites: 1,
      behaviors: passed,
      tests: 0,
      passed,
      failed: 0,
      skipped: 0,
    },
  });
}

const line = (e: object) =>
  JSON.stringify({ Package: "m/a", Time: "2026-11-25T10:00:00Z", ...e });

interface Fake {
  visible: boolean;
  webview: {
    html: string;
    cspSource: string;
    postMessage: ReturnType<typeof vi.fn>;
  };
}

// setup is a panel on show with a kept run of m/kept, and a run in
// progress in /ws streaming into m/a.
function setup(visible = true) {
  const live = new LiveRuns();
  const panel = new SpecViewPanel(
    channel,
    undefined,
    undefined,
    undefined,
    undefined,
    live,
  );
  const fake: Fake = {
    visible,
    webview: { html: "", cspSource: "", postMessage: vi.fn(async () => true) },
  };
  const internals = panel as unknown as {
    panel: Fake;
    runs: { merge(p: { stream: string; folder: string }): void };
  };
  internals.panel = fake;
  internals.runs.merge({
    stream: line({ Package: "m/kept", Action: "pass", Elapsed: 0.1 }) + "\n",
    folder: "/ws",
  });
  let stream = "";
  const run = live.start("run: 3 items", Date.now());
  run.streaming(() => ({ stream, folder: "/ws" }));
  const emit = (e: object) => {
    stream += line(e) + "\n";
    run.note(JSON.parse(line(e)));
  };
  return { panel, fake, run, emit, live };
}

const renders = () => children.length;

// The packages of the picture on show.
const shownPackages = (panel: SpecViewPanel) =>
  (
    panel as unknown as { lastSpecData?: { packages: { path: string }[] } }
  ).lastSpecData?.packages.map((p) => p.path) ?? [];

describe("a run in progress in the Spec scope", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    children.length = 0;
    script.once = [];
    script.always = { stdout: [doc("m/a", 1)], code: 0 };
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders the rows it reached through the CLI, over the kept runs", async () => {
    const { fake, emit, run, panel } = setup();
    emit({ Action: "run", Test: "TestS" });
    emit({ Action: "run", Test: "TestS/TestA" });
    emit({ Action: "pass", Test: "TestS/TestA", Elapsed: 0 });
    await vi.advanceTimersByTimeAsync(LIVE_RENDER_MS - 1);
    expect(renders()).toBe(0);
    await vi.advanceTimersByTimeAsync(1);
    await vi.waitFor(() => expect(renders()).toBe(1));
    const input = children[0].written.join("");
    expect(input).toContain('"Package":"m/kept"');
    expect(input).toContain('"Test":"TestS/TestA"');
    await vi.waitFor(() =>
      expect(fake.webview.html).toContain('id="live-banner"'),
    );
    expect(fake.webview.html).toContain(
      "Running run: 3 items · 0:03 · 2 started · 1 passed · 0 failed",
    );
    expect(shownPackages(panel)).toEqual(["m/a"]);
    run.end();
  });

  it("renders at most once each LIVE_RENDER_MS while events keep coming", async () => {
    const { emit, run } = setup();
    for (let i = 0; i < 100; i++) {
      emit({ Action: "run", Test: `TestS/T${i}` });
      await vi.advanceTimersByTimeAsync(100);
    }
    // 10s of events: a render 3s after the first, then 3s after each ended.
    expect(renders()).toBeGreaterThanOrEqual(2);
    expect(renders()).toBeLessThanOrEqual(10_000 / LIVE_RENDER_MS);
    run.end();
  });

  it("renders nothing while the panel is hidden, and catches up when seen", async () => {
    const { fake, emit, run, panel } = setup(false);
    emit({ Action: "run", Test: "TestS" });
    await vi.advanceTimersByTimeAsync(4 * LIVE_RENDER_MS);
    expect(renders()).toBe(0);
    expect(fake.webview.postMessage).not.toHaveBeenCalled();
    fake.visible = true;
    (panel as unknown as { liveSoon(): void }).liveSoon();
    await vi.advanceTimersByTimeAsync(LIVE_RENDER_MS);
    await vi.waitFor(() => expect(renders()).toBe(1));
    run.end();
  });

  it("runs one render at a time", async () => {
    script.always = { neverExits: true };
    const { emit, run } = setup();
    emit({ Action: "run", Test: "TestS" });
    await vi.advanceTimersByTimeAsync(LIVE_RENDER_MS);
    await vi.waitFor(() => expect(renders()).toBe(1));
    for (let i = 0; i < 20; i++) {
      emit({ Action: "run", Test: `TestS/T${i}` });
      await vi.advanceTimersByTimeAsync(LIVE_RENDER_MS);
    }
    expect(renders()).toBe(1);
    run.end();
  });

  it("stops a render the run's end superseded, and draws nothing of it", async () => {
    // It answers once it is stopped: too late to be drawn.
    script.always = { stdout: [doc("m/a", 1)], neverExits: true };
    const { fake, emit, run, panel } = setup();
    emit({ Action: "run", Test: "TestS" });
    await vi.advanceTimersByTimeAsync(LIVE_RENDER_MS);
    await vi.waitFor(() => expect(renders()).toBe(1));
    run.end();
    expect(mockKill).toHaveBeenCalledWith("SIGTERM");
    await vi.advanceTimersByTimeAsync(10_000);
    expect(shownPackages(panel)).toEqual([]);
    expect(fake.webview.html).toContain(
      'id="live-banner" class="live-banner" role="status" hidden',
    );
  });

  it("redraws the banner in place each LIVE_BANNER_MS, empty once the run ended", async () => {
    const { fake, emit, run } = setup();
    emit({ Action: "run", Test: "TestS" });
    emit({ Action: "fail", Test: "TestS" });
    await vi.advanceTimersByTimeAsync(2 * LIVE_BANNER_MS);
    const banners = fake.webview.postMessage.mock.calls.map(
      ([m]) => (m as { html: string }).html,
    );
    expect(banners.length).toBeGreaterThanOrEqual(2);
    expect(banners.at(-1)).toContain("1 started · 0 passed · 1 failed");
    run.end();
    expect(fake.webview.postMessage.mock.calls.at(-1)![0]).toEqual({
      type: "liveBanner",
      html: "",
    });
    const after = fake.webview.postMessage.mock.calls.length;
    await vi.advanceTimersByTimeAsync(5 * LIVE_BANNER_MS);
    expect(fake.webview.postMessage.mock.calls.length).toBe(after);
  });
});

describe("liveBannerHtml", () => {
  it("is hidden without a run, and names each run with its counts", () => {
    expect(liveBannerHtml([], 0)).toBe(
      '<div id="live-banner" class="live-banner" role="status" hidden></div>',
    );
    const run = {
      label: "coverage: <m/a>",
      startedAt: 0,
      tally: { started: 4, passed: 3, failed: 1 },
    } as RunProgress;
    expect(liveBannerHtml([run], 65_000)).toContain(
      "Running coverage: &lt;m/a&gt; · 1:05 · 4 started · 3 passed · 1 failed",
    );
  });
});
