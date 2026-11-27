// What the Test Explorer shows while a run is in progress: with live results
// each test's verdict lands as the test ends, not when its suite's process
// does. Both suites of the fixture pass a first method and then hold until
// the test releases them, so a first pass applied before the release was
// streamed live.
//
// Driven against the real CLI, with only the editor stubbed.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import { writeFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

const state = vi.hoisted(() => ({
  workspaceDir: "",
  cliPath: "",
  autoRefresh: true,
  panels: [] as { webview: { html: string; cspSource: string } }[],
  controllers: [] as unknown[],
  config: {} as Record<string, unknown>,
}));

vi.mock("vscode", async () => {
  const { buildVscodeStub } = await import("./vscodeStub.js");
  return buildVscodeStub(state as never);
});

import { DiscoveryCache, DiscoveryService } from "../../src/discovery.js";
import { GoTestController } from "../../src/testController.js";
import { TestResultStore } from "../../src/testResultStore.js";
import { executeBatch } from "../../src/batchRunner.js";
import { RunProgress } from "../../src/liveProgress.js";
import { createRecordingChannel } from "./vscodeStub.js";
import { FakeTestRun } from "./vscodeTestApi.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixtureDir = path.join(extensionDir, "testdata", "liveholds");
const importPath = "gotest.liveholds";
const firstPasses = [
  `${importPath}/FirstTestSuite/TestAPasses`,
  `${importPath}/SecondTestSuite/TestAPasses`,
];
const suites = [
  `${importPath}/FirstTestSuite`,
  `${importPath}/SecondTestSuite`,
];
// How long a run that streams nothing live is held before it is released.
const FALLBACK_MS = 8_000;

const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
};

let cache: DiscoveryCache;
let controller: GoTestController;
let recorder: ReturnType<typeof createRecordingChannel>;
let savedGoWork: string | undefined;
let tmp = "";

beforeAll(async () => {
  tmp = mkdtempSync(path.join(tmpdir(), "gotest-liveholds-"));
  const workFile = path.join(tmp, "liveholds.work");
  writeFileSync(
    workFile,
    `go 1.26.0\n\nuse (\n\t${repoRoot}\n\t${fixtureDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;

  state.workspaceDir = fixtureDir;
  state.cliPath = "";
  state.config = {};

  recorder = createRecordingChannel();
  cache = new DiscoveryCache();
  await new DiscoveryService(cache, recorder.channel as never).discover(
    fixtureDir,
  );
  controller = new GoTestController(
    cache,
    new TestResultStore(undefined),
    recorder.channel as never,
    async () => {},
    async () => {},
    async () => {},
    async () => {},
  );
  controller.rebuild();
  // Built once, holding nothing, so no held run waits on a compile.
  await runHeld(false);
}, 300_000);

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
  controller?.dispose();
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

// runHeld runs the package with its suites held, releasing them once both
// first passes are on the Test Explorer, or after FALLBACK_MS. It says what
// the tree held when the suites were released.
async function runHeld(hold = true) {
  const release = mkdtempSync(path.join(tmp, "release-"));
  const run = new FakeTestRun();
  let releasedBy: "passes" | "fallback" | undefined;
  let suitesEndedFirst = false;
  const free = (by: "passes" | "fallback") => {
    if (releasedBy) return;
    releasedBy = by;
    suitesEndedFirst = suites.some((s) => run.verdicts.get(s) === "passed");
    for (const name of ["first", "second"])
      writeFileSync(path.join(release, name), "");
  };
  const passed = run.passed.bind(run);
  run.passed = (item: { id: string }) => {
    passed(item as never);
    if (firstPasses.every((id) => run.verdicts.get(id) === "passed"))
      free("passes");
  };
  const fallback = setTimeout(() => free("fallback"), FALLBACK_MS);
  const progress = new RunProgress("run: liveholds", () => {});
  const item = controller.findItem(importPath);
  expect(item, `no test item for ${importPath}`).toBeDefined();
  try {
    await executeBatch({
      pkgInfos: [{ importPath, items: [item!] as never, dir: fixtureDir }],
      filter: undefined,
      workspaceDir: fixtureDir,
      testFlags: [],
      run: run as never,
      token,
      controller,
      outputChannel: recorder.channel as never,
      label: "liveholds",
      env: hold ? { LIVE_RELEASE_DIR: release } : undefined,
      progress,
    });
  } finally {
    clearTimeout(fallback);
  }
  return { run, releasedBy, suitesEndedFirst, progress };
}

describe("a run with live results", () => {
  it("shows each suite's first pass while both suites still run", async () => {
    state.config = {};
    const { run, releasedBy, suitesEndedFirst, progress } = await runHeld();
    expect(releasedBy).toBe("passes");
    expect(suitesEndedFirst).toBe(false);
    for (const id of [...firstPasses, ...suites]) {
      expect(run.verdicts.get(id), id).toBe("passed");
    }
    expect(progress.tally).toEqual({ started: 6, passed: 6, failed: 0 });
  }, 120_000);

  it("shows them only when the suites end where the folder turned it off", async () => {
    state.config = { liveResults: false };
    try {
      const { run, releasedBy } = await runHeld();
      expect(releasedBy).toBe("fallback");
      for (const id of [...firstPasses, ...suites]) {
        expect(run.verdicts.get(id), id).toBe("passed");
      }
    } finally {
      state.config = {};
    }
  }, 120_000);
});
