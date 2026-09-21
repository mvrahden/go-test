// What the Test Explorer shows when the run fails for a reason no test
// carries: a shared fixture that would not release what it holds. Every suite
// passes, the stream says so, and the CLI still exits non-zero. The tree must
// not read green — that is the one failure this project refuses to ship.
//
// Driven against the real CLI, with only the editor stubbed.

import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import { writeFileSync, mkdtempSync } from "node:fs";
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
const fixtureDir = path.join(extensionDir, "testdata", "teardownfail");
const importPath = "gotest.teardownfail";
const testId = `${importPath}/TeardownFailTestSuite/TestFixtureIsUp`;

const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
};

let cache: DiscoveryCache;
let controller: GoTestController;
let recorder: ReturnType<typeof createRecordingChannel>;
let savedGoWork: string | undefined;

beforeAll(async () => {
  const tmp = mkdtempSync(path.join(tmpdir(), "gotest-verdict-"));
  const workFile = path.join(tmp, "teardownfail.work");
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
    label: "verdict",
    env,
  });
  return run;
}

describe("a run that fails after its last verdict", () => {
  it("marks the package, and leaves the passing test passing", async () => {
    const run = await runPackage({ GOTEST_TEARDOWN_FAIL: "1" });

    expect(run.verdicts.get(testId)).toBe("passed");
    expect(run.verdicts.get(importPath)).toBe("errored");
  }, 300_000);

  it("says on the package what the run failed for", async () => {
    const run = await runPackage({ GOTEST_TEARDOWN_FAIL: "1" });

    const said = (run.messages.get(importPath) ?? []).join("\n");
    expect(said).toContain("shared fixture teardown failed");
    expect(said).toContain("exit code 1");
    // The -json stream is the run's output, not a message to read on an item.
    expect(said).not.toContain('"Action"');
  }, 300_000);

  it("leaves the package alone when the run succeeds", async () => {
    const run = await runPackage();

    expect(run.verdicts.get(testId)).toBe("passed");
    expect(run.verdicts.get(importPath)).not.toBe("errored");
  }, 300_000);
});
