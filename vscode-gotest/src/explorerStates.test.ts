// What the Test Explorer shows and the result store keeps when a run is cut
// off, and for containers no event of their own reaches. The tree, the
// controller, the store and the applier are the real ones; only VS Code's
// run is a recorder.

import { describe, it, expect, vi } from "vitest";
import { readFileSync } from "node:fs";
import * as path from "node:path";

const runs = vi.hoisted(() => [] as Array<Map<string, string>>);

vi.mock("vscode", () => {
  const collection = () => {
    const map = new Map<string, any>();
    return {
      get: (id: string) => map.get(id),
      add: (item: any) => map.set(item.id, item),
      delete: (id: string) => map.delete(id),
      forEach: (fn: (i: any) => void) => map.forEach(fn),
      get size() {
        return map.size;
      },
    };
  };
  return {
    workspace: {
      workspaceFolders: [],
      getWorkspaceFolder: () => ({ name: "ws" }),
      getConfiguration: () => ({ get: () => undefined }),
    },
    Uri: { file: (p: string) => ({ fsPath: p }) },
    window: {},
    tests: {
      createTestController: () => ({
        items: collection(),
        createTestItem: (id: string, label: string, uri?: unknown) => {
          const children = collection();
          const item: any = { id, label, uri, tags: [] };
          // VS Code sets parent when an item is added under another.
          item.children = {
            ...children,
            add: (child: any) => {
              child.parent = item;
              children.add(child);
            },
          };
          return item;
        },
        createRunProfile: () => ({}),
        createTestRun: () => {
          const shown = new Map<string, string>();
          runs.push(shown);
          const say = (what: string) => (item: { id: string }) =>
            shown.set(item.id, what);
          return {
            started: say("started"),
            enqueued: say("enqueued"),
            passed: say("passed"),
            failed: say("failed"),
            errored: say("errored"),
            skipped: say("skipped"),
            appendOutput: () => {},
            end: () => {},
          };
        },
        dispose() {},
      }),
    },
    TestRunRequest: class {},
    TestMessage: class {
      constructor(public message: string) {}
    },
    Location: class {},
    TestRunProfileKind: { Run: 1, Debug: 2, Coverage: 3 },
    TestTag: class {
      constructor(public id: string) {}
    },
    Range: class {},
    Position: class {},
    EventEmitter: class {
      event = () => ({ dispose() {} });
      fire() {}
      dispose() {}
    },
  };
});

import { GoTestController } from "./testController.js";
import { DiscoveryCache } from "./discovery.js";
import { TestResultStore } from "./testResultStore.js";
import {
  StreamApplier,
  enqueueDescendants,
  restoreStoredResults,
  startAncestors,
} from "./runnerUtils.js";
import type { TestEvent } from "./outputParser.js";
import type { DiscoverPackage } from "./types.js";

const MOD = "github.com/mvrahden/go-test";
const LIFECYCLE = `${MOD}/tests/conformance/lifecycle`;
const UNIDIFF = `${MOD}/internal/unidiff`;
const ABOUT = `${MOD}/internal/about`;
const MIGRATE = `${MOD}/tests/conformance/migrate`;

// A real run a reload cut off, trimmed to package events, suites, methods
// and the run's own events.
const sample = readFileSync(
  path.join(__dirname, "..", "testdata", "interrupted", "run.jsonl"),
  "utf-8",
)
  .split("\n")
  .filter(Boolean)
  .map((l) => JSON.parse(l) as TestEvent);

const isInterruptMarker = (e: TestEvent) =>
  e.RunLevel === true && e.Package === "interrupted";

function suite(name: string, extra: object = {}) {
  return {
    name,
    parallel: false,
    focused: false,
    excluded: false,
    guarded: false,
    file: "x_test.go",
    line: 1,
    col: 1,
    lifecycle: [],
    fixtures: [],
    methods: [],
    ...extra,
  };
}

function pkg(importPath: string, suites: object[]): DiscoverPackage {
  return {
    importPath,
    dir: `/ws/${importPath.slice(MOD.length + 1)}`,
    modulePath: MOD,
    suites,
  } as unknown as DiscoverPackage;
}

// The sample's packages, with a suite per top-level test it ran.
function samplePackages(): DiscoverPackage[] {
  const suites = new Map<string, Set<string>>();
  for (const e of sample) {
    if (!e.Test || e.Test.includes("/")) continue;
    const names = suites.get(e.Package) ?? new Set();
    names.add(e.Test.replace(/^Test/, ""));
    suites.set(e.Package, names);
  }
  // Siblings the run does not touch, so both packages sit in folders.
  return [LIFECYCLE, UNIDIFF, ABOUT, MIGRATE].map((ip) =>
    pkg(
      ip,
      [...(suites.get(ip) ?? ["OtherTestSuite"])].map((n) => suite(n)),
    ),
  );
}

function setup(
  packages: DiscoverPackage[],
  store = new TestResultStore(undefined),
) {
  const cache = new DiscoveryCache();
  cache.update(packages, true, "/ws");
  const noop = async () => {};
  const controller = new GoTestController(
    cache,
    store,
    {} as never,
    noop,
    noop,
    noop,
    noop,
  );
  controller.rebuild();
  return { controller, store, cache };
}

// A full run's start: every package cleared, started, its tree enqueued.
function startRun(controller: GoTestController, importPaths: string[]) {
  const run = controller.createTestRun({} as never, "run");
  const shown = runs[runs.length - 1];
  const items = importPaths.map((ip) => controller.findItem(ip)!);
  for (const item of items) {
    controller.clearResults(item);
    run.started(item);
    enqueueDescendants(run, item);
  }
  startAncestors(run, items);
  return { run, shown };
}

function applier(
  controller: GoTestController,
  run: ReturnType<GoTestController["createTestRun"]>,
  cache: DiscoveryCache,
  stopped = () => false,
) {
  return new StreamApplier(
    controller,
    run,
    (ip) => cache.getPackage(ip)?.dir,
    "/ws",
    undefined,
    undefined,
    stopped,
  );
}

function folderIds(controller: GoTestController): string[] {
  const ids: string[] = [];
  const walk = (c: any) =>
    c.forEach((i: any) => {
      if (/^(dir|module|wsFolder):/.test(i.id)) ids.push(i.id);
      walk(i.children);
    });
  walk(controller.testController.items);
  return ids;
}

describe("a run an interrupt cut off", () => {
  function expectNoFailure(
    controller: GoTestController,
    store: TestResultStore,
    shown: Map<string, string>,
  ) {
    const folders = folderIds(controller);
    expect(folders.length).toBeGreaterThan(0);
    for (const id of [LIFECYCLE, UNIDIFF, ...folders]) {
      expect(shown.get(id), id).not.toBe("failed");
    }
    const kept: string[] = [];
    store.forEach((r, id) => {
      if (r.status === "fail" || r.status === "errored") kept.push(id);
      if (/^(dir|module|wsFolder):/.test(id)) kept.push(id);
    });
    expect(kept).toEqual([]);
    expect(store.get(LIFECYCLE)).toBeUndefined();
    expect(store.get(UNIDIFF)).toBeUndefined();
  }

  it("takes back the package fails when the interrupt is said after them", () => {
    const { controller, store, cache } = setup(samplePackages());
    const { run, shown } = startRun(controller, [LIFECYCLE, UNIDIFF]);
    const a = applier(controller, run, cache);
    const marker = sample.findIndex(isInterruptMarker);
    a.apply(sample.slice(0, marker));
    // Before the marker the stream reads as failures.
    expect(shown.get(UNIDIFF)).toBe("failed");
    a.apply(sample.slice(marker));

    expect(a.interrupted).toBe(true);
    expect(a.failed).toBe(false);
    expectNoFailure(controller, store, shown);
    expect(shown.get(UNIDIFF)).toBe("skipped");
    // What did pass is kept.
    expect(store.get(`${LIFECYCLE}/EachRowsTestSuite`)?.status).toBe("pass");
  });

  it("records none of them once a stop was asked, before any marker", () => {
    const { controller, store, cache } = setup(samplePackages());
    const { run, shown } = startRun(controller, [LIFECYCLE, UNIDIFF]);
    const a = applier(controller, run, cache, () => true);
    a.apply(sample.filter((e) => !e.RunLevel));

    expectNoFailure(controller, store, shown);
    expect(a.reported.has(UNIDIFF)).toBe(false);
  });

  it("keeps a package a test of failed, and its folders' failure", () => {
    const { controller, store, cache } = setup(samplePackages());
    const { run, shown } = startRun(controller, [LIFECYCLE, UNIDIFF]);
    const a = applier(controller, run, cache);
    const marker = sample.findIndex(isInterruptMarker);
    const failing: TestEvent = {
      Time: "",
      Action: "fail",
      Package: UNIDIFF,
      Test: "TestUnidiffTestSuite",
      Gotest: { Subtest: "TestUnidiffTestSuite" },
    };
    const unidiffFail = sample.findIndex(
      (e) => e.Package === UNIDIFF && e.Action === "fail" && !e.Test,
    );
    a.apply([
      ...sample.slice(0, unidiffFail),
      failing,
      ...sample.slice(unidiffFail, marker),
    ]);
    a.apply(sample.slice(marker));

    expect(store.get(UNIDIFF)?.status).toBe("fail");
    expect(shown.get(UNIDIFF)).toBe("failed");
    expect(shown.get(`dir:internal`)).toBe("failed");
    expect(store.get(LIFECYCLE)).toBeUndefined();
    expect(a.failed).toBe(true);
  });
});

describe("restoring a store an interrupt left behind", () => {
  it("drops package fails that took no time over no failed test, and every folder's own result", () => {
    const store = new TestResultStore(undefined);
    store.record(UNIDIFF, "fail", 0);
    store.record("dir:internal", "fail");
    store.record("dir:tests/conformance", "fail");
    store.record(LIFECYCLE, "fail", 0);
    store.record(`${LIFECYCLE}/EachRowsTestSuite`, "fail", 10);
    store.record(ABOUT, "fail", 0);
    const packages = samplePackages().map((p) =>
      p.importPath === ABOUT ? { ...p, broken: true } : p,
    );
    const { controller } = setup(packages, store);

    restoreStoredResults(controller, store);
    const shown = runs[runs.length - 1];

    expect(store.get(UNIDIFF)).toBeUndefined();
    expect(shown.get(UNIDIFF)).toBeUndefined();
    expect(store.get("dir:internal")).toBeUndefined();
    expect(store.get("dir:tests/conformance")).toBeUndefined();
    // A package fail with a failed test below it is a verdict.
    expect(store.get(LIFECYCLE)?.status).toBe("fail");
    expect(shown.get(LIFECYCLE)).toBe("failed");
    expect(shown.get("dir:tests/conformance")).toBe("failed");
    // So is one that does not build, and its folder fails by it.
    expect(store.get(ABOUT)?.status).toBe("fail");
    expect(shown.get("dir:internal")).toBe("failed");
  });
});

describe("a package's verdict", () => {
  it("resolves a suite that holds only fuzz targets once they passed", () => {
    const fuzzer = {
      name: "FuzzName",
      focused: false,
      excluded: false,
      parallel: false,
      file: "x_test.go",
      line: 2,
      col: 1,
    };
    const p = pkg(`${MOD}/pkg/gotest`, [
      suite("FNameTestSuite", { fuzzers: [fuzzer] }),
      suite("PlainTestSuite", {
        methods: [{ ...fuzzer, name: "TestNeverRan" }],
      }),
    ]);
    const { controller, cache } = setup([p]);
    const { run, shown } = startRun(controller, [p.importPath]);
    const a = applier(controller, run, cache);
    const at = (Action: TestEvent["Action"], Test?: string): TestEvent => ({
      Time: "",
      Action,
      Package: p.importPath,
      Test,
      Gotest: Test ? { Subtest: Test } : undefined,
    });
    a.apply([
      at("start"),
      at("run", "FuzzFNameTestSuite_FuzzName"),
      at("pass", "FuzzFNameTestSuite_FuzzName"),
    ]);
    const suiteId = `${p.importPath}/FNameTestSuite`;
    expect(shown.get(`${suiteId}/FuzzName`)).toBe("passed");
    expect(shown.get(suiteId)).toBe("enqueued");

    a.apply([at("pass")]);
    expect(shown.get(suiteId)).toBe("passed");
    expect(shown.get(`${p.importPath}/PlainTestSuite/TestNeverRan`)).toBe(
      "skipped",
    );
    expect(shown.get(`${p.importPath}/PlainTestSuite`)).toBe("skipped");
  });
});

describe("a narrowed run's folders", () => {
  const GOTEST = `${MOD}/pkg/gotest`;
  const LINT_A = `${MOD}/pkg/lint/a`;
  const LINT_B = `${MOD}/pkg/lint/b`;
  const at = (
    Package: string,
    Action: TestEvent["Action"],
    Test?: string,
  ): TestEvent => ({
    Time: "",
    Action,
    Package,
    Test,
    Gotest: Test ? { Subtest: Test } : undefined,
  });

  function narrowed(listed: string[]) {
    const store = new TestResultStore(undefined);
    // What earlier runs left: verdicts below, and a folder's own stale fail.
    store.record(LINT_A, "pass", 5);
    store.record(LINT_B, "pass", 5);
    store.record("dir:lint", "fail");
    store.record("dir:pkg", "fail");
    const { controller, cache } = setup(
      [GOTEST, LINT_A, LINT_B].map((ip) => pkg(ip, [suite("STestSuite")])),
      store,
    );
    expect(controller.findItem("dir:lint")).toBeDefined();
    const { run, shown } = startRun(controller, listed);
    return { a: applier(controller, run, cache), shown };
  }

  it("derive from what their children show, not from a stored result of their own", () => {
    const { a, shown } = narrowed([GOTEST]);
    a.apply([
      at(GOTEST, "run", "TestSTestSuite"),
      at(GOTEST, "pass", "TestSTestSuite"),
      at(GOTEST, "pass"),
    ]);
    expect(shown.get("dir:pkg")).toBe("passed");
  });

  it("stay running while a listed package below them has not finished", () => {
    const { a, shown } = narrowed([GOTEST, LINT_A]);
    a.apply([at(GOTEST, "pass")]);
    expect(shown.get("dir:pkg")).toBe("started");
    expect(shown.get("dir:lint")).toBe("started");

    a.apply([at(LINT_A, "pass")]);
    expect(shown.get("dir:lint")).toBe("passed");
    expect(shown.get("dir:pkg")).toBe("passed");
  });
});
