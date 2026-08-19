// The other half of the extension: the Test Explorer tree, coverage, and watch.
//
// The spec-view tests prove the rendered panel is right. Nothing there touches
// the path that turns events into TestItem verdicts, reads a coverage profile,
// or drives a watch process. This file runs those three entry points against
// the real CLI and the real fixture corpus, with only the editor stubbed.

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
import { CoverageStore } from "../../src/coverageStore.js";
import { RunRegistry } from "../../src/runRegistry.js";
import { CoverageRunner } from "../../src/coverage.js";
import { WatchManager } from "../../src/watch.js";
import { executeBatch } from "../../src/batchRunner.js";
import { createRecordingChannel } from "./vscodeStub.js";
import { FakeTestRun, FakeTestController } from "./vscodeTestApi.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixturesDir = path.join(extensionDir, "testdata", "fixtures");

const pkg = (name: string) => `gotest.fixtures/${name}`;

let cache: DiscoveryCache;
let controller: GoTestController;
let registry: RunRegistry;
let savedGoWork: string | undefined;

const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
};

function newRun(): FakeTestRun {
  return new FakeTestRun();
}

function itemsFor(importPath: string) {
  const item = controller.findItem(importPath);
  expect(item, `no test item for ${importPath}`).toBeDefined();
  return [item!];
}

beforeAll(async () => {
  const tmp = mkdtempSync(path.join(tmpdir(), "gotest-explorer-"));
  const workFile = path.join(tmp, "corpus.work");
  // A superset of the repository's own workspace, so anything spawned during
  // these tests resolves the fixture module and the repo alike.
  writeFileSync(
    workFile,
    `go 1.25.0\n\nuse (\n\t${repoRoot}\n\t${path.join(repoRoot, "examples")}\n\t${fixturesDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;

  state.workspaceDir = fixturesDir;
  state.cliPath = "";
  state.config = {};

  const recorder = createRecordingChannel();
  cache = new DiscoveryCache();
  const discovery = new DiscoveryService(cache, recorder.channel as never);
  await discovery.discover(fixturesDir);

  registry = new RunRegistry(tmp);
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

describe("discovery builds the Test Explorer tree", () => {
  it("found the fixture packages", () => {
    const paths = cache.packages.map((p) => p.importPath);
    expect(paths).toContain(pkg("passing"));
    expect(paths).toContain(pkg("failing"));
    expect(paths).toContain(pkg("nested"));
  });

  it("created a test item per package, suite, and method", () => {
    expect(controller.findItem(pkg("passing"))).toBeDefined();
    expect(
      controller.findItem(`${pkg("passing")}/PassingTestSuite`),
    ).toBeDefined();
    expect(
      controller.findItem(`${pkg("passing")}/PassingTestSuite/TestArithmetic`),
    ).toBeDefined();
  });

  it("marks a package that could not be loaded rather than dropping it", () => {
    // A broken package's suites are unknowable, not absent. Losing the item
    // would make the failure invisible in the tree.
    expect(controller.findItem(pkg("broken"))).toBeDefined();
  });
});

describe("executeBatch maps events onto test items", () => {
  it("passes every item of a passing package", async () => {
    const run = newRun();
    const items = itemsFor(pkg("passing"));
    const result = await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("passing"),
          items: items as never,
          dir: path.join(fixturesDir, "passing"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "test",
    });

    expect(result.stdout.length).toBeGreaterThan(0);
    expect(run.verdictsMatching("failed")).toEqual([]);
    expect(run.verdictsMatching("passed").length).toBeGreaterThan(0);
    // Behaviors are discovered at run time and hang off the method item, so
    // assert on the behavior rather than on the exact synthesised id.
    const passed = run.verdictsMatching("passed");
    expect(passed).toContain(pkg("passing"));
    expect(passed.some((id) => id.endsWith("returns_their_sum"))).toBe(true);
    expect(passed.some((id) => id.endsWith("is_commutative"))).toBe(true);
  });

  it("fails only the behaviors that failed, leaving siblings passing", async () => {
    const run = newRun();
    const result = await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("failing"),
          items: itemsFor(pkg("failing")) as never,
          dir: path.join(fixturesDir, "failing"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "test",
    });

    expect(result.stdout.length).toBeGreaterThan(0);
    const failed = run.verdictsMatching("failed");
    const passed = run.verdictsMatching("passed");
    expect(failed.some((id) => id.includes("reports_the_expected_total"))).toBe(
      true,
    );
    expect(
      passed.some((id) => id.includes("still_runs_the_sibling_behavior")),
    ).toBe(true);
  });

  it("attaches the failure message to the item that failed", async () => {
    const run = newRun();
    await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("failing"),
          items: itemsFor(pkg("failing")) as never,
          dir: path.join(fixturesDir, "failing"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "test",
    });

    const failedId = run
      .verdictsMatching("failed")
      .find((id) => id.includes("reports_the_expected_total"))!;
    const messages = (run.messages.get(failedId) ?? []).join("\n");
    expect(messages).toContain("300");
  });

  it("fails a package that does not compile, in the compiler's words", async () => {
    const run = newRun();
    const result = await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("broken"),
          items: itemsFor(pkg("broken")) as never,
          dir: path.join(fixturesDir, "broken"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "test",
    });

    // The CLI books a load failure as a package verdict and keeps going, so the
    // package item carries the failure rather than being errored out of band.
    expect(run.verdictsMatching("failed")).toContain(pkg("broken"));
    const reported =
      [...run.messages.values()].flat().join("\n") + result.stdout;
    expect(reported).toContain("undefinedHelper");
  });

  it("streams output through to the run", async () => {
    const run = newRun();
    await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("failing"),
          items: itemsFor(pkg("failing")) as never,
          dir: path.join(fixturesDir, "failing"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "test",
    });

    expect(run.output.join("")).toContain("FAIL");
  });

  it("honours a -run filter instead of running the whole package", async () => {
    // Space-separated `-run <filter>` is the form the extension sends; a value
    // carrying "/" must pair with its flag rather than read as a package.
    const run = newRun();
    const result = await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("nested"),
          items: itemsFor(pkg("nested")) as never,
          dir: path.join(fixturesDir, "nested"),
        },
      ],
      filter: "^TestNestedTestSuite$/^TestCheckout$",
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "test",
    });

    expect(result.stdout).toContain("TestCheckout");
    expect(run.verdictsMatching("passed").length).toBeGreaterThan(0);
  });
});

describe("the coverage entry point", () => {
  it("produces a profile, stores it, and hands the stream on", async () => {
    const store = new CoverageStore(undefined);
    let handedOn = "";
    const runner = new CoverageRunner(
      controller,
      cache,
      store,
      createRecordingChannel().channel as never,
      (json) => {
        handedOn = json;
      },
      registry,
    );

    // `table` is the fixture with real non-test source; a package of only test
    // files would yield an empty profile and prove nothing.
    await runner.run(
      { include: itemsFor(pkg("table")) } as never,
      token as never,
    );

    // The stream reaches the Spec View through the same callback the extension
    // wires up, so a coverage run keeps the panel current too.
    expect(handedOn).toContain(pkg("table"));

    const { coverages } = store.buildFileCoverages(cache);
    expect(coverages.length).toBeGreaterThan(0);
    const classify = coverages.find((c) =>
      String((c as { uri?: { fsPath?: string } }).uri?.fsPath ?? "").endsWith(
        "classify.go",
      ),
    );
    expect(classify, "no coverage for the covered source file").toBeDefined();
  }, 300_000);
});

describe("the watch entry point", () => {
  it("drives a live test run from its first cycle, then stops cleanly", async () => {
    // onCycleComplete flushes the previous cycle, so it fires on the *second*
    // cycle or on an unexpected exit — never after a single initial pass. What
    // is observable after one cycle is the run it drives, which is exactly the
    // integration that matters: the Test Explorer updating while watching.
    const manager = new WatchManager(
      controller,
      cache,
      createRecordingChannel().channel as never,
      () => {},
      registry,
    );
    const fake = state.controllers[0] as FakeTestController;
    const runsBefore = fake.runs.length;

    try {
      await manager.start("./passing/...", fixturesDir);

      const run = await waitFor(
        () => fake.runs.slice(runsBefore).find((r) => r.verdicts.size > 0),
        120_000,
      );

      const passed = run.verdictsMatching("passed");
      expect(passed.some((id) => id.endsWith("returns_their_sum"))).toBe(true);
      expect(run.ended).toBe(false);
    } finally {
      manager.stopAll();
      manager.dispose();
    }

    const run = fake.runs.slice(runsBefore).find((r) => r.verdicts.size > 0)!;
    expect(run.ended, "stopping the watch must end its run").toBe(true);
  }, 300_000);
});

// Polls rather than sleeping a fixed interval, so a slow first compile does not
// turn into a flaky failure and a fast one does not waste the budget.
async function waitFor<T>(probe: () => T | undefined, ms: number): Promise<T> {
  const deadline = Date.now() + ms;
  for (;;) {
    const value = probe();
    if (value !== undefined) return value;
    if (Date.now() > deadline) throw new Error("timed out waiting for watch");
    await new Promise((r) => setTimeout(r, 250));
  }
}
