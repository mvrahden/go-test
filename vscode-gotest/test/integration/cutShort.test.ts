// What the Test Explorer shows for a test whose process died under it: a
// goroutine panic ends the test binary while the method runs, so the stream
// holds the method's start and no verdict. The tree must name that test,
// errored with what it printed, not leave it skipped.
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
import { createRecordingChannel } from "./vscodeStub.js";
import { FakeTestRun } from "./vscodeTestApi.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixtureDir = path.join(extensionDir, "testdata", "cutshort");
const importPath = "gotest.cutshort";
const suiteId = `${importPath}/CutShortTestSuite`;

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
  tmp = mkdtempSync(path.join(tmpdir(), "gotest-cutshort-"));
  const workFile = path.join(tmp, "cutshort.work");
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
}, 300_000);

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
  controller?.dispose();
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

async function runPackage(env?: Record<string, string>): Promise<FakeTestRun> {
  const run = new FakeTestRun();
  const item = controller.findItem(importPath);
  expect(item, `no test item for ${importPath}`).toBeDefined();
  await executeBatch({
    pkgInfos: [{ importPath, items: [item!] as never, dir: fixtureDir }],
    filter: undefined,
    workspaceDir: fixtureDir,
    testFlags: [],
    run: run as never,
    token,
    controller,
    outputChannel: recorder.channel as never,
    label: "cutshort",
    env,
  });
  return run;
}

describe("a test whose process died under it", () => {
  it("is errored with what it printed, beside its passing sibling", async () => {
    const run = await runPackage();

    const dies = `${suiteId}/TestBDies`;
    expect(run.verdicts.get(dies)).toBe("errored");
    const said = (run.messages.get(dies) ?? []).join("\n");
    expect(said).toMatch(/^No verdict:/);
    expect(said).toContain("boom on a goroutine");
    expect(run.verdicts.get(`${suiteId}/TestAFast`)).toBe("passed");
    expect(run.verdicts.get(importPath)).toBe("failed");
    expect(controller.getResult(dies)?.status).toBe("errored");
  }, 300_000);
});
