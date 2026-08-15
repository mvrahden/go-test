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
  // partial list as the whole truth — but it must not promise VS Code a
  // resolve handler either, since the missing children cannot be fetched
  // without running the test.
  it("says so when a method's behaviors are only knowable at run time", () => {
    const partial = controller.findItem(
      `${pkg("runtimebehaviors")}/RuntimeBehaviorsTestSuite/TestConditional`,
    );
    expect(partial?.description).toBe("+ behaviors known only at run time");
    expect(partial?.canResolveChildren).toBe(false);

    // Only the behavior that is written unconditionally is declared.
    const declared: string[] = [];
    partial!.children.forEach((when) =>
      when.children.forEach((b) => declared.push(b.label)),
    );
    expect(declared).toEqual(["always states this one"]);
  });

  it("leaves a fully declared method unannotated", () => {
    const complete = controller.findItem(
      `${pkg("table")}/TableTestSuite/TestClassify`,
    );
    expect(complete?.description).toBeUndefined();
    expect(complete?.canResolveChildren).toBe(false);
  });

  // The conditional behavior appears once it has actually run, on the same
  // canonical id, alongside the declared one.
  it("adds a run-time-only behavior to the declared tree when it appears", async () => {
    const method = `${pkg("runtimebehaviors")}/RuntimeBehaviorsTestSuite/TestConditional`;
    const run = new FakeTestRun();
    await executeBatch({
      pkgInfos: [
        {
          importPath: pkg("runtimebehaviors"),
          items: [controller.findItem(pkg("runtimebehaviors"))!] as never,
          dir: path.join(fixturesDir, "runtimebehaviors"),
        },
      ],
      filter: undefined,
      workspaceDir: fixturesDir,
      testFlags: [],
      run: run as never,
      token: token as never,
      controller,
      outputChannel: createRecordingChannel().channel as never,
      label: "runtime",
    });

    const passed = run.verdictsMatching("passed");
    expect(
      passed.some((id) =>
        id.startsWith(`${method}/the_feature_flag_decides_what_is_specified/`),
      ),
    ).toBe(true);
    expect(
      passed.filter((id) => id.startsWith(`${method}/`)).length,
    ).toBeGreaterThanOrEqual(3);
  }, 300_000);
});

// Two names a run produces that the source does not spell out. Getting either
// wrong is invisible until a run happens, and then it shows up as a second tree
// node beside the declared one.
describe("declared names match the ones go test invents", () => {
  it("numbers a description that repeats among its siblings", () => {
    expect(
      idsUnder(
        `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions`,
      ),
    ).toEqual([
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions/the_same_words`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions/the_same_words#01`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions/the_same_words#01/names_the_second`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions/the_same_words#02`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions/the_same_words#02/names_the_third`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestRepeatedDescriptions/the_same_words/names_the_first`,
    ]);
  });

  it("makes a slash a level, and lets two descriptions share it", () => {
    expect(
      idsUnder(`${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping`),
    ).toEqual([
      `${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping/a`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping/a/b_grouping`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping/a/b_grouping/shares_the_first_level`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping/a/c_grouping`,
      `${pkg("duplicates")}/DuplicatesTestSuite/TestSlashGrouping/a/c_grouping/shares_it_too`,
    ]);
  });

  // A run of slashes is not a separator: "https:// URIs" is one subtest whose
  // name contains slashes, and go test reports it as one level.
  it("keeps a run of slashes inside a single level", () => {
    const id =
      `${pkg("metachars")}/MetacharsTestSuite/TestPunctuation/a_description_has_{braces}/handles_https:%2F%2F_URIs`.replace(
        /%2F/g,
        "/",
      );
    expect(controller.findItem(id)?.label).toBe("handles https:// URIs");
  });

  // The failure this guards against is silent: an observed result can build a
  // second item whose id string equals the declared one but whose parent does
  // not, so comparing ids alone sees nothing wrong. Counting the tree does.
  it.each(["duplicates", "metachars"])(
    "gains no tree items when %s actually runs",
    async (name) => {
      const before = idsUnder(pkg(name));
      const run = new FakeTestRun();
      await executeBatch({
        pkgInfos: [
          {
            importPath: pkg(name),
            items: [controller.findItem(pkg(name))!] as never,
            dir: path.join(fixturesDir, name),
          },
        ],
        filter: undefined,
        workspaceDir: fixturesDir,
        testFlags: [],
        run: run as never,
        token: token as never,
        controller,
        outputChannel: createRecordingChannel().channel as never,
        label: name,
      });

      expect(idsUnder(pkg(name))).toEqual(before);

      // Every declared leaf reported, so the run landed on the declared items
      // rather than on look-alikes built beside them.
      const leaves = before.filter(
        (id) => !before.some((other) => other.startsWith(id + "/")),
      );
      expect(run.verdictsMatching("passed")).toEqual(
        expect.arrayContaining(leaves),
      );
    },
    300_000,
  );
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

describe("results survive a reload", () => {
  // Restoring a stored result does `findItem(id)` and drops anything it cannot
  // resolve. Behavior ids used to exist only during a run, so every behavior
  // result was persisted, aged, and then silently discarded on load. Declaring
  // behaviors at discovery is what makes the lookup succeed.
  it("resolves a stored behavior id against the declared tree", () => {
    const behaviorId = `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/negative`;

    // Exactly the operation restoreResults performs on each stored key.
    expect(controller.findItem(behaviorId)).toBeDefined();

    controller.recordResult(behaviorId, "pass", 12);
    expect(controller.getResult(behaviorId)).toMatchObject({ status: "pass" });
  });

  it("persists and reloads behavior results under the current store version", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-restore-"));
    const behaviorId = `${pkg("table")}/TableTestSuite/TestClassify/classifying_a_number/zero`;

    const store = new TestResultStore({ fsPath: dir });
    store.record(behaviorId, "fail", 7);
    // record only mutates memory; save schedules the write and flush forces it.
    store.save();
    await store.flush();

    const reloaded = new TestResultStore({ fsPath: dir });
    await reloaded.load();

    expect(reloaded.get(behaviorId)).toMatchObject({ status: "fail" });
  });
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
