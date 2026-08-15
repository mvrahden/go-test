// Behaviors are part of the tree before anything runs.
//
// The Test Explorer used to learn a suite's behaviors only by executing it, so
// the run counter's denominator grew mid-flight and a behavior could not be
// addressed until it had already been observed. Discovery now reads the
// specification from source, and this suite pins the properties that depend on
// it — above all that a declared behavior and the observed one are the same
// tree node rather than two.

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
import { enqueueDescendants, buildRunFilter } from "../../src/runnerUtils.js";
import { createRecordingChannel } from "./vscodeStub.js";
import { FakeTestRun, FakeTestController } from "./vscodeTestApi.js";

const extensionDir = path.resolve(__dirname, "..", "..");
const repoRoot = path.resolve(extensionDir, "..");
const fixturesDir = path.join(extensionDir, "testdata", "fixtures");
const pkg = (name: string) => `gotest.fixtures/${name}`;

let cache: DiscoveryCache;
let controller: GoTestController;
let fake: FakeTestController;
let savedGoWork: string | undefined;

const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
};

function idsUnder(prefix: string): string[] {
  return fake
    .allItems()
    .map((i) => i.id)
    .filter((id) => id === prefix || id.startsWith(prefix + "/"))
    .sort();
}

beforeAll(async () => {
  const tmp = mkdtempSync(path.join(tmpdir(), "gotest-behaviors-"));
  const workFile = path.join(tmp, "corpus.work");
  writeFileSync(
    workFile,
    `go 1.24.0\n\nuse (\n\t${repoRoot}\n\t${path.join(repoRoot, "examples")}\n\t${fixturesDir}\n)\n`,
    "utf-8",
  );
  savedGoWork = process.env.GOWORK;
  process.env.GOWORK = workFile;

  state.workspaceDir = fixturesDir;
  state.cliPath = "";
  state.config = {};

  const recorder = createRecordingChannel();
  cache = new DiscoveryCache();
  await new DiscoveryService(cache, recorder.channel as never).discover(
    fixturesDir,
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
  fake = state.controllers[0] as FakeTestController;
}, 300_000);

afterAll(() => {
  if (savedGoWork === undefined) delete process.env.GOWORK;
  else process.env.GOWORK = savedGoWork;
  controller?.dispose();
});

describe("the specification is in the tree before anything runs", () => {
  it("builds a test item for every declared behavior", () => {
    expect(idsUnder(pkg("table"))).toEqual([
      pkg("table"),
      `${pkg("table")}/TableTestSuite`,
      `${pkg("table")}/TableTestSuite/TestClassify`,
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number`,
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/negative`,
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/positive`,
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/zero`,
    ]);
  });

  it("nests behaviors as deeply as they are written", () => {
    expect(idsUnder(pkg("nested"))).toContain(
      `${pkg("nested")}/NestedTestSuite/TestCheckout/a_cart_has_items/and_the_customer_is_a_member/and_a_coupon_applies/stacks_both_discounts`,
    );
  });

  it("labels a behavior with the text the developer wrote, not the subtest name", () => {
    const item = controller.findItem(
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number`,
    );
    expect(item?.label).toBe("classifying a number");
  });

  it("gives each behavior a source position, so it can be decorated in the gutter", () => {
    const item = controller.findItem(
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/negative`,
    );
    expect(item?.range).toBeDefined();
    expect(item?.uri).toBeDefined();
  });

  // A method whose behaviors depend on runtime values must not present its
  // partial list as the whole truth.
  it("marks a method with runtime-determined behaviors as resolvable", () => {
    const complete = controller.findItem(
      `${pkg("table")}/TableTestSuite/TestClassify`,
    );
    expect(complete?.canResolveChildren).toBe(false);
  });
});

describe("the run counter has its total up front", () => {
  it("enqueues every behavior at run start and gains no items during the run", async () => {
    const before = idsUnder(pkg("table"));
    const run = new FakeTestRun();
    const item = controller.findItem(pkg("table"))!;
    enqueueDescendants(run as never, item);
    const enqueued = run.verdicts.size;

    await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("table"),
          items: [item] as never,
          dir: path.join(fixturesDir, "table"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "behaviors",
    });

    const after = idsUnder(pkg("table"));

    // The denominator VS Code can show is what was enqueued: every item under
    // the package except the package itself.
    expect(enqueued).toBe(before.length - 1);
    // Nothing appeared mid-run, which is what used to make the total climb.
    expect(after).toEqual(before);
  }, 300_000);

  // The whole scheme rests on this: a declared behavior and the observed one
  // must be the same id, or the tree would carry both.
  it("reports results onto the declared items rather than creating new ones", async () => {
    const run = new FakeTestRun();
    const item = controller.findItem(pkg("table"))!;

    await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("table"),
          items: [item] as never,
          dir: path.join(fixturesDir, "table"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "behaviors",
    });

    expect(run.verdictsMatching("passed")).toContain(
      `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/negative`,
    );
    // No id carries the old structural marker any more.
    expect(idsUnder(pkg("table")).some((id) => id.includes("/dynamic/"))).toBe(
      false,
    );
  }, 300_000);
});

describe("a single behavior can be run on its own", () => {
  // The payoff of declaring behaviors up front: a developer can run one row of
  // a table without running the other nineteen. The filter has to speak go
  // test's language — the rewritten subtest name, not the prose label.
  it("filters the run down to exactly the selected behavior", async () => {
    const behaviorId = `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/negative`;
    const behavior = controller.findItem(behaviorId)!;
    expect(behavior.label).toBe("negative");

    const filter = buildRunFilter([behavior]);
    expect(filter).toBe(
      "^TestTableTestSuite$/^TestClassify$/^classifying_a_number/negative$",
    );

    const run = new FakeTestRun();
    await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("table"),
          items: [behavior] as never,
          dir: path.join(fixturesDir, "table"),
        },
      ],
      filter,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "one-behavior",
    });

    const passed = run.verdictsMatching("passed");
    expect(passed).toContain(behaviorId);
    // The siblings were not run, which is the entire point of the filter.
    expect(passed.some((id) => id.endsWith("/zero"))).toBe(false);
    expect(passed.some((id) => id.endsWith("/positive"))).toBe(false);
  }, 300_000);

  // Descriptions are prose, and go test's -run is a regular expression. Every
  // metacharacter in Go's QuoteMeta set appears in the metachars fixture, so
  // each of these addresses a behavior whose name would otherwise be read as a
  // pattern. Without escaping the filter matches nothing at all.
  it("addresses behaviors whose descriptions contain regex metacharacters", async () => {
    const whenId = `${pkg("metachars")}/MetacharsTestSuite/TestPunctuation/a_description_has_{braces}`;
    const whenItem = controller.findItem(whenId);
    expect(
      whenItem,
      "the When block should be declared statically",
    ).toBeDefined();

    const behaviors: string[] = [];
    whenItem!.children.forEach((child) => behaviors.push(child.id));
    expect(behaviors.length).toBeGreaterThanOrEqual(6);

    for (const id of behaviors) {
      const item = controller.findItem(id)!;
      const filter = buildRunFilter([item]);
      const run = new FakeTestRun();

      await executeBatch({
        pkgInfos: [
          {
            importPath: pkg("metachars"),
            items: [item] as never,
            dir: path.join(fixturesDir, "metachars"),
          },
        ],
        filter,
        workspaceDir: fixturesDir,
        testFlags: [],
        run: run as never,
        token: token as never,
        controller,
        outputChannel: createRecordingChannel().channel as never,
        label: "metachars",
      });

      const passedLeaves = run
        .verdictsMatching("passed")
        .filter((passedId) => behaviors.includes(passedId));
      expect(passedLeaves, `filter for ${id} was ${filter}`).toEqual([id]);
    }
  }, 600_000);
});
