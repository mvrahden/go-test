// Tier 1 + Tier 2 — the real seam.
//
// Everything here spawns the actual `gotest` CLI. `node:child_process` is NOT
// mocked; only the editor is stubbed. The regression this suite exists for was
// invisible to every mocked test in the repo, and invisible again to any
// integration test that used a prebuilt binary or merged stdout with stderr —
// so resolution mode is an explicit axis and the streams are kept apart.

import { describe, it, expect, beforeAll, beforeEach, vi } from "vitest";
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdtempSync, chmodSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as {
    webview: { html: string; cspSource: string };
  }[],
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { SpecViewPanel } from "../../src/specView.js";
import { createRecordingChannel } from "./vscodeStub.js";
import { clearBinaryCache } from "../../src/cli.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const streamDir = path.join(extensionDir, "testdata", "streams");

const stream = (name: string) =>
  readFileSync(path.join(streamDir, `${name}.jsonl`), "utf-8");

let realBinary = "";
let prebuiltBinary = "";
let brokenBinary = "";

beforeAll(() => {
  const dir = mkdtempSync(path.join(tmpdir(), "gotest-integration-"));

  realBinary = path.join(dir, "gotest-real");
  const built = spawnSync("go", ["build", "-o", realBinary, "./cmd/gotest"], {
    cwd: repoRoot,
    encoding: "utf-8",
  });
  expect(built.status, `go build failed: ${built.stderr}`).toBe(0);

  // A build from a working tree reports a pseudo-version below MIN_CLI_VERSION,
  // which the extension rejects — correctly, but that would silently turn the
  // cliPath axis into a second `go run` axis. The wrapper reports a releasable
  // version and delegates everything else to the real binary, so the axis tests
  // cliPath resolution rather than this tree's version string.
  prebuiltBinary = path.join(dir, "gotest");
  writeFileSync(
    prebuiltBinary,
    [
      "#!/bin/sh",
      'if [ "$1" = "version" ]; then echo "gotest v99.0.0 integration"; exit 0; fi',
      `exec ${realBinary} "$@"`,
      "",
    ].join("\n"),
    "utf-8",
  );
  chmodSync(prebuiltBinary, 0o755);

  // A binary that passes the extension's version probe but fails to produce a
  // spec. This is the shape of a toolchain or build failure, and it must stay
  // distinguishable from "the tests failed".
  brokenBinary = path.join(dir, "broken-gotest");
  writeFileSync(
    brokenBinary,
    [
      "#!/bin/sh",
      'if [ "$1" = "version" ]; then echo "gotest v1.99.0 stub"; exit 0; fi',
      'echo "simulated toolchain failure" >&2',
      "exit 1",
      "",
    ].join("\n"),
    "utf-8",
  );
  chmodSync(brokenBinary, 0o755);
});

beforeEach(() => {
  state.workspaceDir = repoRoot;
  state.cliPath = "";
  state.autoRefresh = true;
  state.panels.length = 0;
  clearBinaryCache();
});

function newPanel() {
  const recorder = createRecordingChannel();
  const panel = new SpecViewPanel(recorder.channel as never);
  return { panel, recorder };
}

function renderedHtml(): string {
  expect(state.panels.length).toBeGreaterThan(0);
  return state.panels[state.panels.length - 1].webview.html;
}

// Both resolution modes the extension actually uses. `go run` is the default
// and the only one that exposes the exit-status epilogue; a suite that tested
// solely the prebuilt path would have shipped the regression untouched.
const POSIX = process.platform !== "win32";

const MODES = [
  { name: "go run (extension default)", cliPath: () => "" },
  // The prebuilt axis routes through a /bin/sh wrapper, so it is POSIX-only.
  ...(POSIX
    ? [{ name: "prebuilt cliPath", cliPath: () => prebuiltBinary }]
    : []),
];

describe.each(MODES)("spec sync via $name", (mode) => {
  beforeEach(() => {
    state.cliPath = mode.cliPath();
  });

  it("renders a run whose tests all passed", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(stream("all-pass"), "run");

    const html = renderedHtml();
    expect(html).toContain("adds an item");
    expect(html).toContain("totals the basket");
    expect(html).toContain("2 passed");
    expect(recorder.errors).toEqual([]);
  });

  // The regression, stated as user-visible behaviour: a run containing a
  // failure must still reach the panel.
  it("renders a run that contained a failure", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(stream("mixed"), "run");

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("totals the basket");
    expect(html).toContain("expected 300, got 250");
    expect(html).toContain("1 passed");
    expect(html).toContain("1 failed");
  });

  it("renders a package that failed to build, which has no failing behaviour to speak for it", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(stream("package-failure"), "run");

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("FAIL");
    expect(html).toContain("undefined: helper");
    expect(html).toContain("1 failed packages");
  });

  it("renders an empty stream as an empty spec, not as an error", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    await panel.refresh(stream("empty"), "run");

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    // No package and no rendered node — asserted on content rather than on the
    // static template, which carries status glyphs of its own.
    expect(html).not.toContain("example.com");
    expect(html).not.toContain('class="leaf');
  });
});

describe("CLI resolution is the mode the test intends", () => {
  // Guards the trap this suite was designed around: if the prebuilt path
  // silently fell back to `go run` (or vice versa), the matrix above would be
  // testing one mode twice and quietly lose its only failing-path coverage.
  it("uses go run when no cliPath is configured", async () => {
    const debug: string[] = [];
    const recorder = createRecordingChannel();
    const channel = {
      ...recorder.channel,
      debug: (m: string) => debug.push(m),
    };
    const panel = new SpecViewPanel(channel as never);
    state.cliPath = "";
    await panel.show();
    await panel.refresh(stream("mixed"), "run");

    expect(debug.some((m) => m.includes("go run"))).toBe(true);
    expect(debug.some((m) => m.includes("cliPath override"))).toBe(false);
  });

  it.skipIf(!POSIX)(
    "uses the configured binary when cliPath is set",
    async () => {
      const debug: string[] = [];
      const recorder = createRecordingChannel();
      const channel = {
        ...recorder.channel,
        debug: (m: string) => debug.push(m),
      };
      const panel = new SpecViewPanel(channel as never);
      state.cliPath = prebuiltBinary;
      await panel.show();
      await panel.refresh(stream("mixed"), "run");

      expect(debug.some((m) => m.includes("cliPath override"))).toBe(true);
    },
  );

  // The extension now sends --render-only, which older CLIs would reject. The
  // version floor is what keeps that from reaching them, so its enforcement is
  // asserted against a real binary rather than only against mocked probes.
  it.skipIf(!POSIX)(
    "refuses a cliPath binary below the version floor and falls back to go run",
    async () => {
      const debug: string[] = [];
      const warnings: string[] = [];
      const channel = {
        info: () => {},
        error: () => {},
        warn: (m: string) => warnings.push(m),
        debug: (m: string) => debug.push(m),
      };
      const panel = new SpecViewPanel(channel as never);
      state.cliPath = realBinary; // reports a working-tree pseudo-version
      await panel.show();
      await panel.refresh(stream("mixed"), "run");

      expect(warnings.some((m) => m.includes("requires >="))).toBe(true);
      expect(debug.some((m) => m.includes("cliPath override"))).toBe(false);
      expect(debug.some((m) => m.includes("go run"))).toBe(true);
    },
  );
});

describe("a CLI that produced no spec is reported, not swallowed", () => {
  // The stand-in binaries are /bin/sh scripts, so this case is POSIX-only.
  // Everything above runs on every platform; only the fabricated binaries here
  // and the cliPath wrapper depend on a shell.
  it.skipIf(process.platform === "win32")(
    "surfaces the real diagnostic when the binary fails to render",
    async () => {
      const { panel, recorder } = newPanel();
      state.cliPath = brokenBinary;
      await panel.show();
      await panel.refresh(stream("mixed"), "run");

      expect(recorder.errors.length).toBe(1);
      expect(recorder.errors[0]).toContain("simulated toolchain failure");
      // The failure must read as a failure, not as an unhelpful parse error
      // downstream of a silently discarded exit code.
      expect(recorder.errors[0]).not.toContain("JSON");
      expect(renderedHtml()).not.toContain("totals the basket");
    },
  );
});

// Tier 2 — refresh() is stateful. jsonLayers accumulates run / coverage /
// watch layers and re-renders their concatenation every time, so a failing
// layer that never clears can freeze the panel long after the run that
// produced it. No exit-code assertion reaches this.
describe("layer accumulation across refreshes", () => {
  it("replaces a failing run with a later passing run under the same tag", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(stream("mixed"), "run");
    expect(renderedHtml()).toContain("1 failed");

    await panel.refresh(stream("all-pass"), "run");
    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("2 passed");
    expect(html).not.toContain("1 failed");
  });

  it("keeps rendering a passing run while a stale failing layer is still held", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(stream("other-package-fail"), "coverage");
    await panel.refresh(stream("all-pass"), "run");

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    // Both layers are present: the panel is live, not frozen on the failure.
    expect(html).toContain("rejects an expired card");
    expect(html).toContain("adds an item");
    expect(html).toContain("2 passed");
    expect(html).toContain("1 failed");
  });

  it("renders every layer's package, not only the most recent one", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();

    await panel.refresh(stream("all-pass"), "run");
    await panel.refresh(stream("other-package-fail"), "watch:./...@/tmp");

    const html = renderedHtml();
    expect(recorder.errors).toEqual([]);
    expect(html).toContain("example.com/cart");
    expect(html).toContain("example.com/checkout");
  });
});

describe("autoRefresh honours the user's setting", () => {
  it("does not repaint the panel when autoRefresh is off", async () => {
    const { panel, recorder } = newPanel();
    await panel.show();
    const before = renderedHtml();

    state.autoRefresh = false;
    await panel.refresh(stream("mixed"), "run");

    expect(recorder.errors).toEqual([]);
    expect(renderedHtml()).toBe(before);
  });
});
