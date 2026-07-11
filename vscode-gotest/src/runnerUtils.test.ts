import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class TestTag {
    constructor(public readonly id: string) {}
  }
  class Position {
    constructor(
      public readonly line: number,
      public readonly character: number,
    ) {}
  }
  class Uri {
    static file(path: string) {
      return { fsPath: path, toString: () => path };
    }
  }
  class TestMessage {
    location?: unknown;
    expectedOutput?: string;
    actualOutput?: string;
    constructor(public readonly message: string) {}
    static diff(message: string, expected: string, actual: string) {
      const msg = new TestMessage(message);
      msg.expectedOutput = expected;
      msg.actualOutput = actual;
      return msg;
    }
  }
  class Location {
    constructor(
      public readonly uri: unknown,
      public readonly range: unknown,
    ) {}
  }
  return { TestTag, Position, Uri, TestMessage, Location };
});

import {
  getItemDepth,
  getRootItem,
  groupByPackage,
  buildRunFilter,
  getPackageDepth,
  getPackageItem,
  expandToPackages,
  computeWildcard,
  resolveRunPatterns,
  applyResults,
  applyEvent,
  enqueueDescendants,
  startAncestors,
  resolveAncestorItems,
  resolveAncestorsOf,
  skipUnresolved,
} from "./runnerUtils.js";
import { buildPathTrie, collapsePathTrie, type PathNode } from "./pathTrie.js";

interface MockTestItem {
  id: string;
  label: string;
  parent: MockTestItem | undefined;
  children: Map<string, MockTestItem>;
  tags: { id: string }[];
  uri?: unknown;
  range?: unknown;
}

function createItem(
  id: string,
  label: string,
  parent?: MockTestItem,
  tags: { id: string }[] = [],
): MockTestItem {
  const item: MockTestItem = {
    id,
    label,
    parent,
    children: new Map(),
    tags,
  };
  if (parent) {
    parent.children.set(id, item);
  }
  return item;
}

function makeTree() {
  const pkg = createItem("example.com/pkg", "example.com/pkg", undefined, [
    { id: "package" },
  ]);
  const suite = createItem("example.com/pkg/MySuite", "MySuite", pkg);
  const method = createItem(
    "example.com/pkg/MySuite/TestFoo",
    "TestFoo",
    suite,
  );
  const dynamic = createItem(
    "example.com/pkg/MySuite/TestFoo/dynamic/sub1",
    "sub1",
    method,
  );
  return { pkg, suite, method, dynamic };
}

describe("getItemDepth", () => {
  it("returns 0 for a root item", () => {
    const { pkg } = makeTree();
    expect(getItemDepth(pkg as any)).toBe(0);
  });

  it("returns 1 for a suite", () => {
    const { suite } = makeTree();
    expect(getItemDepth(suite as any)).toBe(1);
  });

  it("returns 2 for a method", () => {
    const { method } = makeTree();
    expect(getItemDepth(method as any)).toBe(2);
  });

  it("returns 3 for a dynamic subtest", () => {
    const { dynamic } = makeTree();
    expect(getItemDepth(dynamic as any)).toBe(3);
  });
});

describe("getRootItem", () => {
  it("returns itself for a root item", () => {
    const { pkg } = makeTree();
    expect(getRootItem(pkg as any).id).toBe("example.com/pkg");
  });

  it("returns the package for a method", () => {
    const { method } = makeTree();
    expect(getRootItem(method as any).id).toBe("example.com/pkg");
  });

  it("returns the package for a dynamic subtest", () => {
    const { dynamic } = makeTree();
    expect(getRootItem(dynamic as any).id).toBe("example.com/pkg");
  });
});

describe("groupByPackage", () => {
  it("groups items by their root package", () => {
    const { suite, method } = makeTree();
    const pkg2 = createItem("example.com/other", "example.com/other");
    const suite2 = createItem(
      "example.com/other/OtherSuite",
      "OtherSuite",
      pkg2,
    );

    const groups = groupByPackage([suite as any, method as any, suite2 as any]);
    expect(groups.size).toBe(2);
    expect(groups.get("example.com/pkg")).toHaveLength(2);
    expect(groups.get("example.com/other")).toHaveLength(1);
  });

  it("returns empty map for empty input", () => {
    const groups = groupByPackage([]);
    expect(groups.size).toBe(0);
  });
});

describe("buildRunFilter", () => {
  it("returns undefined for package-level items (depth 0)", () => {
    const { pkg } = makeTree();
    expect(buildRunFilter([pkg as any])).toBeUndefined();
  });

  it("returns suite filter for depth-1 items", () => {
    const { suite } = makeTree();
    expect(buildRunFilter([suite as any])).toBe("^TestMySuite$");
  });

  it("returns suite/method filter for depth-2 items", () => {
    const { method } = makeTree();
    expect(buildRunFilter([method as any])).toBe("^TestMySuite$/^TestFoo$");
  });

  it("returns suite/method/subtest filter for depth-3 items", () => {
    const { dynamic } = makeTree();
    expect(buildRunFilter([dynamic as any])).toBe(
      "^TestMySuite$/^TestFoo$/^sub1$",
    );
  });

  it("returns suite filter for suite with fixtures", () => {
    const pkg = createItem("example.com/pkg", "example.com/pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem(
      "example.com/pkg/FixtureSuite",
      "FixtureSuite",
      pkg,
    );
    expect(buildRunFilter([suite as any])).toBe("^TestFixtureSuite$");
  });

  it("joins multiple methods with alternation", () => {
    const pkg = createItem("example.com/pkg", "example.com/pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/pkg/MySuite", "MySuite", pkg);
    const m1 = createItem("example.com/pkg/MySuite/TestA", "TestA", suite);
    const m2 = createItem("example.com/pkg/MySuite/TestB", "TestB", suite);
    expect(buildRunFilter([m1 as any, m2 as any])).toBe(
      "^TestMySuite$/^(TestA|TestB)$",
    );
  });
});

function makeTreeWithDirs() {
  const dir = createItem("dir:pkg", "pkg", undefined);
  const pkg = createItem("example.com/pkg", "pkg/auth", dir, [
    { id: "package" },
  ]);
  const suite = createItem("example.com/pkg/MySuite", "MySuite", pkg);
  const method = createItem(
    "example.com/pkg/MySuite/TestFoo",
    "TestFoo",
    suite,
  );
  const dynamic = createItem(
    "example.com/pkg/MySuite/TestFoo/dynamic/sub1",
    "sub1",
    method,
  );
  return { dir, pkg, suite, method, dynamic };
}

describe("getPackageItem", () => {
  it("returns the package item from a method", () => {
    const { pkg, method } = makeTreeWithDirs();
    expect(getPackageItem(method as any).id).toBe("example.com/pkg");
  });

  it("returns the package item from the package itself", () => {
    const { pkg } = makeTreeWithDirs();
    expect(getPackageItem(pkg as any).id).toBe("example.com/pkg");
  });

  it("returns the item itself if no package tag found (directory node)", () => {
    const { dir } = makeTreeWithDirs();
    expect(getPackageItem(dir as any).id).toBe("dir:pkg");
  });

  it("works with flat tree (package at root, backward compat)", () => {
    const pkg = createItem("example.com/flat", "example.com/flat", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/flat/S", "S", pkg);
    expect(getPackageItem(suite as any).id).toBe("example.com/flat");
  });
});

describe("getPackageDepth", () => {
  it("returns 0 for a package item", () => {
    const { pkg } = makeTreeWithDirs();
    expect(getPackageDepth(pkg as any)).toBe(0);
  });

  it("returns 1 for a suite", () => {
    const { suite } = makeTreeWithDirs();
    expect(getPackageDepth(suite as any)).toBe(1);
  });

  it("returns 2 for a method", () => {
    const { method } = makeTreeWithDirs();
    expect(getPackageDepth(method as any)).toBe(2);
  });

  it("returns 3 for a dynamic subtest", () => {
    const { dynamic } = makeTreeWithDirs();
    expect(getPackageDepth(dynamic as any)).toBe(3);
  });

  it("returns -1 for a directory node", () => {
    const { dir } = makeTreeWithDirs();
    expect(getPackageDepth(dir as any)).toBe(-1);
  });

  it("matches getItemDepth when tree is flat (backward compat)", () => {
    const pkg = createItem("example.com/flat", "example.com/flat", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/flat/S", "S", pkg);
    const method = createItem("example.com/flat/S/M", "M", suite);
    expect(getPackageDepth(pkg as any)).toBe(0);
    expect(getPackageDepth(suite as any)).toBe(1);
    expect(getPackageDepth(method as any)).toBe(2);
  });
});

describe("expandToPackages", () => {
  it("returns package items as-is", () => {
    const pkg = createItem("example.com/pkg", "pkg", undefined, [
      { id: "package" },
    ]);
    const result = expandToPackages([pkg as any]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("example.com/pkg");
  });

  it("expands a directory node to its package descendants", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkg1 = createItem("example.com/pkg/a", "a", dir, [{ id: "package" }]);
    const pkg2 = createItem("example.com/pkg/b", "b", dir, [{ id: "package" }]);

    const result = expandToPackages([dir as any]);
    expect(result).toHaveLength(2);
    const ids = result.map((r) => r.id).sort();
    expect(ids).toEqual(["example.com/pkg/a", "example.com/pkg/b"]);
  });

  it("expands nested directory nodes", () => {
    const root = createItem("dir:root", "root", undefined);
    const sub = createItem("dir:root/sub", "sub", root);
    const pkg = createItem("example.com/pkg", "pkg", sub, [{ id: "package" }]);

    const result = expandToPackages([root as any]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("example.com/pkg");
  });

  it("passes through suite/method items unchanged", () => {
    const pkg = createItem("example.com/pkg", "pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/pkg/S", "S", pkg);
    const result = expandToPackages([suite as any]);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("example.com/pkg/S");
  });
});

describe("buildPathTrie", () => {
  it("builds a trie from workspace-relative paths", () => {
    const entries = [
      { relativePath: "pkg/auth", importPath: "example.com/pkg/auth" },
      { relativePath: "pkg/store", importPath: "example.com/pkg/store" },
      { relativePath: "internal/svc", importPath: "example.com/internal/svc" },
    ];
    const root = buildPathTrie(entries);
    expect(root.children.size).toBe(2);
    expect(root.children.has("pkg")).toBe(true);
    expect(root.children.has("internal")).toBe(true);

    const pkg = root.children.get("pkg")!;
    expect(pkg.children.size).toBe(2);
    expect(pkg.children.has("auth")).toBe(true);
    expect(pkg.children.has("store")).toBe(true);
    expect(pkg.children.get("auth")!.importPath).toBe("example.com/pkg/auth");
  });

  it("handles a single package at workspace root", () => {
    const entries = [{ relativePath: ".", importPath: "example.com/root" }];
    const root = buildPathTrie(entries);
    expect(root.importPath).toBe("example.com/root");
  });
});

describe("collapsePathTrie", () => {
  it("collapses single-child directory chains", () => {
    const entries = [
      { relativePath: "internal/svc", importPath: "example.com/internal/svc" },
    ];
    const root = buildPathTrie(entries);
    collapsePathTrie(root);
    expect(root.children.size).toBe(1);
    const collapsed = root.children.get("internal/svc")!;
    expect(collapsed.importPath).toBe("example.com/internal/svc");
  });

  it("does not collapse nodes with multiple children", () => {
    const entries = [
      { relativePath: "pkg/auth", importPath: "example.com/pkg/auth" },
      { relativePath: "pkg/store", importPath: "example.com/pkg/store" },
    ];
    const root = buildPathTrie(entries);
    collapsePathTrie(root);
    expect(root.children.has("pkg")).toBe(true);
    const pkg = root.children.get("pkg")!;
    expect(pkg.children.size).toBe(2);
  });

  it("does not collapse a node that is itself a package", () => {
    const entries = [
      { relativePath: "pkg", importPath: "example.com/pkg" },
      { relativePath: "pkg/sub", importPath: "example.com/pkg/sub" },
    ];
    const root = buildPathTrie(entries);
    collapsePathTrie(root);
    expect(root.children.has("pkg")).toBe(true);
    const pkg = root.children.get("pkg")!;
    expect(pkg.importPath).toBe("example.com/pkg");
    expect(pkg.children.size).toBe(1);
  });
});

describe("enqueueDescendants", () => {
  it("enqueues all descendants recursively", () => {
    const pkg = createItem("example.com/pkg", "pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/pkg/SuiteA", "SuiteA", pkg);
    const method1 = createItem("example.com/pkg/SuiteA/Test1", "Test1", suite);
    const method2 = createItem("example.com/pkg/SuiteA/Test2", "Test2", suite);

    const run = { enqueued: vi.fn() };

    enqueueDescendants(run as any, pkg as any);

    expect(run.enqueued).toHaveBeenCalledTimes(3);
    expect(run.enqueued).toHaveBeenCalledWith(suite);
    expect(run.enqueued).toHaveBeenCalledWith(method1);
    expect(run.enqueued).toHaveBeenCalledWith(method2);
  });

  it("does nothing for leaf items", () => {
    const method = createItem("example.com/pkg/Suite/Test1", "Test1");
    const run = { enqueued: vi.fn() };

    enqueueDescendants(run as any, method as any);

    expect(run.enqueued).not.toHaveBeenCalled();
  });
});

describe("applyResults", () => {
  function makeApplyResultsFixture() {
    const suiteItem = createItem("example.com/pkg/MySuite", "MySuite");
    const passItem = createItem(
      "example.com/pkg/MySuite/TestPass",
      "TestPass",
      suiteItem,
    );
    const failItem = createItem(
      "example.com/pkg/MySuite/TestFail",
      "TestFail",
      suiteItem,
    );
    const skipItem = createItem(
      "example.com/pkg/MySuite/TestSkip",
      "TestSkip",
      suiteItem,
    );

    const itemMap = new Map<string, MockTestItem>([
      ["example.com/pkg/MySuite", suiteItem],
      ["example.com/pkg/MySuite/TestPass", passItem],
      ["example.com/pkg/MySuite/TestFail", failItem],
      ["example.com/pkg/MySuite/TestSkip", skipItem],
    ]);

    const controller = {
      findItem: vi.fn((id: string) => itemMap.get(id) ?? undefined),
      recordResult: vi.fn(),
      createDynamicSubtest: vi.fn(),
    };

    const run = {
      passed: vi.fn(),
      failed: vi.fn(),
      skipped: vi.fn(),
      started: vi.fn(),
      appendOutput: vi.fn(),
    };

    return { controller, run, passItem, failItem, skipItem };
  }

  it("returns AppliedResult[] and records results to controller", () => {
    const { controller, run, passItem, failItem, skipItem } =
      makeApplyResultsFixture();

    const events = [
      {
        Action: "run" as const,
        Test: "MySuite/TestPass",
        Package: "example.com/pkg",
      },
      {
        Action: "run" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
      },
      {
        Action: "run" as const,
        Test: "MySuite/TestSkip",
        Package: "example.com/pkg",
      },
      {
        Action: "pass" as const,
        Test: "MySuite/TestPass",
        Package: "example.com/pkg",
        Elapsed: 0.1,
      },
      {
        Action: "fail" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Elapsed: 0.2,
      },
      {
        Action: "skip" as const,
        Test: "MySuite/TestSkip",
        Package: "example.com/pkg",
      },
    ];

    const applied = applyResults(
      controller as any,
      run as any,
      events as any,
      "example.com/pkg",
      "/some/dir",
    );

    // Returns 3 test-level + 0 package-level entries (no pkg event in fixture)
    expect(applied).toHaveLength(3);

    const passResult = applied.find((r) => r.itemId === passItem.id);
    expect(passResult).toEqual({
      itemId: passItem.id,
      status: "pass",
      duration: 100,
    });

    const failResult = applied.find((r) => r.itemId === failItem.id);
    expect(failResult).toEqual({
      itemId: failItem.id,
      status: "fail",
      duration: 200,
    });

    const skipResult = applied.find((r) => r.itemId === skipItem.id);
    expect(skipResult).toEqual({
      itemId: skipItem.id,
      status: "skip",
      duration: undefined,
    });

    // Records each test result to the controller
    expect(controller.recordResult).toHaveBeenCalledWith(
      passItem.id,
      "pass",
      100,
    );
    expect(controller.recordResult).toHaveBeenCalledWith(
      failItem.id,
      "fail",
      200,
    );
    expect(controller.recordResult).toHaveBeenCalledWith(
      skipItem.id,
      "skip",
      undefined,
    );

    // Does call run methods
    expect(run.passed).toHaveBeenCalledWith(passItem, 100);
    expect(run.failed).toHaveBeenCalledWith(failItem, expect.any(Array), 200);
    expect(run.skipped).toHaveBeenCalledWith(skipItem);
  });

  it("captures package-level pass event with Elapsed", () => {
    const { controller, run } = makeApplyResultsFixture();

    const events = [
      {
        Action: "pass" as const,
        Test: "MySuite/TestPass",
        Package: "example.com/pkg",
        Elapsed: 0.1,
      },
      { Action: "pass" as const, Package: "example.com/pkg", Elapsed: 1.5 },
    ];

    const applied = applyResults(
      controller as any,
      run as any,
      events as any,
      "example.com/pkg",
      "/some/dir",
    );

    const pkgResult = applied.find((r) => r.itemId === "example.com/pkg");
    expect(pkgResult).toEqual({
      itemId: "example.com/pkg",
      status: "pass",
      duration: 1500,
    });
  });

  it("captures package-level fail event with Elapsed", () => {
    const { controller, run } = makeApplyResultsFixture();

    const events = [
      {
        Action: "fail" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Elapsed: 0.2,
      },
      { Action: "fail" as const, Package: "example.com/pkg", Elapsed: 2.3 },
    ];

    const applied = applyResults(
      controller as any,
      run as any,
      events as any,
      "example.com/pkg",
      "/some/dir",
    );

    const pkgResult = applied.find((r) => r.itemId === "example.com/pkg");
    expect(pkgResult).toEqual({
      itemId: "example.com/pkg",
      status: "fail",
      duration: 2300,
    });
  });

  it("creates TestMessage.diff with expected/actual from assertion output", () => {
    const { controller, run, failItem } = makeApplyResultsFixture();

    const events = [
      {
        Action: "output" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Output: "=== RUN   MySuite/TestFail\n",
      },
      {
        Action: "output" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Output: "    fail_test.go:14: Equal failed:\n",
      },
      {
        Action: "output" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Output: "          expected: 720000000000\n",
      },
      {
        Action: "output" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Output: "          actual:   120000000000\n",
      },
      {
        Action: "output" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Output: "--- FAIL: MySuite/TestFail (0.00s)\n",
      },
      {
        Action: "fail" as const,
        Test: "MySuite/TestFail",
        Package: "example.com/pkg",
        Elapsed: 0,
      },
    ];

    applyResults(
      controller as any,
      run as any,
      events as any,
      "example.com/pkg",
      "/some/dir",
    );

    expect(run.failed).toHaveBeenCalledTimes(1);
    const messages = run.failed.mock.calls[0][1];
    expect(messages).toHaveLength(1);
    expect(messages[0].expectedOutput).toBe("720000000000");
    expect(messages[0].actualOutput).toBe("120000000000");
    expect(messages[0].message).toBe(
      "Equal failed: expected 720000000000, actual 120000000000",
    );
  });
});

describe("applyEvent", () => {
  function makeApplyEventFixture() {
    const suiteItem = createItem("example.com/pkg/MySuite", "MySuite");
    const passItem = createItem(
      "example.com/pkg/MySuite/TestPass",
      "TestPass",
      suiteItem,
    );
    const failItem = createItem(
      "example.com/pkg/MySuite/TestFail",
      "TestFail",
      suiteItem,
    );
    const skipItem = createItem(
      "example.com/pkg/MySuite/TestSkip",
      "TestSkip",
      suiteItem,
    );
    const pkgItem = createItem("example.com/pkg", "pkg", undefined, [
      { id: "package" },
    ]);

    const itemMap = new Map<string, MockTestItem>([
      ["example.com/pkg", pkgItem],
      ["example.com/pkg/MySuite", suiteItem],
      ["example.com/pkg/MySuite/TestPass", passItem],
      ["example.com/pkg/MySuite/TestFail", failItem],
      ["example.com/pkg/MySuite/TestSkip", skipItem],
    ]);

    const results = new Map<string, { status: string; duration?: number }>();

    const controller = {
      findItem: vi.fn((id: string) => itemMap.get(id) ?? undefined),
      recordResult: vi.fn((id: string, status: string, duration?: number) => {
        results.set(id, { status, duration });
      }),
      getResult: vi.fn((id: string) => results.get(id)),
      createDynamicSubtest: vi.fn(),
    };

    const run = {
      passed: vi.fn(),
      failed: vi.fn(),
      skipped: vi.fn(),
      started: vi.fn(),
      appendOutput: vi.fn(),
    };

    return {
      controller,
      run,
      pkgItem,
      suiteItem,
      passItem,
      failItem,
      skipItem,
    };
  }

  it("processes 'run' event — calls run.started, returns undefined", () => {
    const { controller, run, passItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const result = applyEvent(
      controller as any,
      run as any,
      {
        Action: "run",
        Test: "TestMySuite/TestPass",
        Package: "example.com/pkg",
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toBeUndefined();
    expect(run.started).toHaveBeenCalledWith(passItem);
  });

  it("processes 'pass' event — calls run.passed, records result, returns AppliedResult", () => {
    const { controller, run, passItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const result = applyEvent(
      controller as any,
      run as any,
      {
        Action: "pass",
        Test: "TestMySuite/TestPass",
        Package: "example.com/pkg",
        Elapsed: 0.1,
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toEqual({
      itemId: passItem.id,
      status: "pass",
      duration: 100,
    });
    expect(run.passed).toHaveBeenCalledWith(passItem, 100);
    expect(controller.recordResult).toHaveBeenCalledWith(
      passItem.id,
      "pass",
      100,
    );
  });

  it("processes 'skip' event — calls run.skipped, records result, returns AppliedResult", () => {
    const { controller, run, skipItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const result = applyEvent(
      controller as any,
      run as any,
      {
        Action: "skip",
        Test: "TestMySuite/TestSkip",
        Package: "example.com/pkg",
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toEqual({
      itemId: skipItem.id,
      status: "skip",
      duration: undefined,
    });
    expect(run.skipped).toHaveBeenCalledWith(skipItem);
    expect(controller.recordResult).toHaveBeenCalledWith(
      skipItem.id,
      "skip",
      undefined,
    );
  });

  it("processes 'output' event — accumulates in outputMap, appends to run", () => {
    const { controller, run } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const result = applyEvent(
      controller as any,
      run as any,
      {
        Action: "output",
        Test: "TestMySuite/TestFail",
        Package: "example.com/pkg",
        Output: "    fail_test.go:14: boom\n",
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toBeUndefined();
    expect(outputMap.get("TestMySuite/TestFail")).toBe(
      "    fail_test.go:14: boom\n",
    );
    expect(run.appendOutput).toHaveBeenCalled();
  });

  it("processes 'fail' event — uses accumulated output for diagnostics", () => {
    const { controller, run, failItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();
    outputMap.set(
      "TestMySuite/TestFail",
      "    fail_test.go:14: Equal failed:\n          expected: 1\n          actual:   2\n",
    );

    const result = applyEvent(
      controller as any,
      run as any,
      {
        Action: "fail",
        Test: "TestMySuite/TestFail",
        Package: "example.com/pkg",
        Elapsed: 0.2,
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toEqual({
      itemId: failItem.id,
      status: "fail",
      duration: 200,
    });
    expect(run.failed).toHaveBeenCalledWith(failItem, expect.any(Array), 200);
    expect(controller.recordResult).toHaveBeenCalledWith(
      failItem.id,
      "fail",
      200,
    );
  });

  it("uses diagnostic location as fallback for panic stack traces", () => {
    const { controller, run, failItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();
    outputMap.set(
      "TestMySuite/TestFail",
      [
        "panic: runtime error: index out of range [0] with length 0",
        "",
        "goroutine 7 [running]:",
        "testing.tRunner.func1.2({0x5836c0, 0x1})",
        "\t/usr/local/go/src/testing/testing.go:1974 +0x232",
        "example.com/pkg.(*Suite).BeforeEach(...)",
        "\t/some/dir/suite_test.go:56",
      ].join("\n"),
    );

    applyEvent(
      controller as any,
      run as any,
      {
        Action: "fail",
        Test: "TestMySuite/TestFail",
        Package: "example.com/pkg",
        Elapsed: 0.1,
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(run.failed).toHaveBeenCalledTimes(1);
    const messages = run.failed.mock.calls[0][1];
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toContain("panic: runtime error");
    expect(messages[0].location).toBeDefined();
    expect(messages[0].location.uri.fsPath).toBe("/some/dir/suite_test.go");
    expect(messages[0].location.range.line).toBe(55);
  });

  it("processes package terminal event — resolves package and ancestors", () => {
    const { controller, run, pkgItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const result = applyEvent(
      controller as any,
      run as any,
      { Action: "pass", Package: "example.com/pkg", Elapsed: 1.5 } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toEqual({
      itemId: "example.com/pkg",
      status: "pass",
      duration: 1500,
    });
    expect(run.passed).toHaveBeenCalledWith(pkgItem, 1500);
    expect(controller.recordResult).toHaveBeenCalledWith(
      "example.com/pkg",
      "pass",
      1500,
    );
  });

  it("returns undefined for unknown test path", () => {
    const { controller, run } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const result = applyEvent(
      controller as any,
      run as any,
      {
        Action: "pass",
        Test: "TestUnknown/Method",
        Package: "example.com/pkg",
        Elapsed: 0.1,
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toBeUndefined();
    expect(run.passed).not.toHaveBeenCalled();
  });

  it("filters 'exit status' output lines", () => {
    const { controller, run } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    applyEvent(
      controller as any,
      run as any,
      {
        Action: "output",
        Package: "example.com/pkg",
        Output: "exit status 1\n",
      } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(run.appendOutput).not.toHaveBeenCalled();
  });

  it("accumulates package-level output and attaches as message on fail", () => {
    const { controller, run, pkgItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    const outputLines = [
      "WARNING: DATA RACE\n",
      "Read at 0x00c00001c0f0 by goroutine 7:\n",
      "  example.com/pkg.(*Foo).Bar()\n",
      "      /some/dir/foo.go:42 +0x1a4\n",
    ];
    for (const line of outputLines) {
      applyEvent(
        controller as any,
        run as any,
        {
          Action: "output",
          Package: "example.com/pkg",
          Output: line,
        } as any,
        outputMap,
        "example.com/pkg",
        "/some/dir",
      );
    }

    const result = applyEvent(
      controller as any,
      run as any,
      { Action: "fail", Package: "example.com/pkg", Elapsed: 1.2 } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(result).toEqual({
      itemId: "example.com/pkg",
      status: "fail",
      duration: 1200,
    });
    expect(run.failed).toHaveBeenCalledTimes(1);
    const messages = run.failed.mock.calls[0][1];
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toContain("WARNING: DATA RACE");
    expect(messages[0].message).toContain("Read at 0x00c00001c0f0");
    expect(messages[0].location).toBeDefined();
    expect(messages[0].location.uri.fsPath).toBe("/some/dir/foo.go");
    expect(messages[0].location.range.line).toBe(41);
  });

  it("shows fallback message on package fail with no output", () => {
    const { controller, run, pkgItem } = makeApplyEventFixture();
    const outputMap = new Map<string, string>();

    applyEvent(
      controller as any,
      run as any,
      { Action: "fail", Package: "example.com/pkg", Elapsed: 0.5 } as any,
      outputMap,
      "example.com/pkg",
      "/some/dir",
    );

    expect(run.failed).toHaveBeenCalledTimes(1);
    const messages = run.failed.mock.calls[0][1];
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toBe("Package failed");
  });
});

describe("computeWildcard", () => {
  it("returns undefined for a single path", () => {
    expect(computeWildcard(["example.com/pkg/a"])).toBeUndefined();
  });

  it("returns undefined for empty array", () => {
    expect(computeWildcard([])).toBeUndefined();
  });

  it("returns wildcard for two paths with common prefix", () => {
    expect(computeWildcard(["example.com/pkg/a", "example.com/pkg/b"])).toEqual(
      ["example.com/pkg/..."],
    );
  });

  it("finds deep common prefix", () => {
    expect(
      computeWildcard([
        "example.com/pkg/platform/billing",
        "example.com/pkg/platform/cluster",
        "example.com/pkg/platform/auth",
      ]),
    ).toEqual(["example.com/pkg/platform/..."]);
  });

  it("stops at segment boundary", () => {
    expect(
      computeWildcard(["example.com/foo/bar", "example.com/foo/baz"]),
    ).toEqual(["example.com/foo/..."]);
  });

  it("handles divergence at top level", () => {
    expect(
      computeWildcard(["example.com/pkg/a", "example.com/internal/b"]),
    ).toEqual(["example.com/..."]);
  });

  it("returns undefined when all paths are identical", () => {
    expect(
      computeWildcard(["example.com/pkg", "example.com/pkg"]),
    ).toBeUndefined();
  });

  it("groups by sub-directory when prefix equals module root", () => {
    expect(
      computeWildcard(
        ["example.com/pkg/a", "example.com/pkg/b", "example.com/internal/c"],
        "example.com",
      ),
    ).toEqual(["example.com/pkg/...", "example.com/internal/c"]);
  });

  it("returns undefined when sub-grouping does not reduce count", () => {
    expect(
      computeWildcard(
        ["example.com/pkg/a", "example.com/internal/b"],
        "example.com",
      ),
    ).toBeUndefined();
  });

  it("includes module-root package alongside sub-directory wildcards", () => {
    expect(
      computeWildcard(
        [
          "example.com",
          "example.com/pkg/a",
          "example.com/pkg/b",
          "example.com/internal/c",
        ],
        "example.com",
      ),
    ).toEqual(["example.com", "example.com/pkg/...", "example.com/internal/c"]);
  });

  it("allows wildcard deeper than module root", () => {
    expect(
      computeWildcard(
        ["example.com/pkg/a", "example.com/pkg/b"],
        "example.com",
      ),
    ).toEqual(["example.com/pkg/..."]);
  });
});

describe("resolveAncestorItems", () => {
  function makeAncestorFixture() {
    const dir = createItem("dir:internal", "internal", undefined);
    const pkg = createItem("example.com/internal/auth", "auth", dir, [
      { id: "package" },
    ]);
    const suite = createItem(
      "example.com/internal/auth/AuthSuite",
      "AuthSuite",
      pkg,
    );
    const method = createItem(
      "example.com/internal/auth/AuthSuite/TestLogin",
      "TestLogin",
      suite,
    );

    const run = {
      started: vi.fn(),
      passed: vi.fn(),
      failed: vi.fn(),
    };

    const controller = {
      getResult: vi.fn((_id: string) => undefined as any),
      recordResult: vi.fn(),
      testController: {
        items: new Map<string, MockTestItem>([["dir:internal", dir]]),
      },
    };

    return { dir, pkg, suite, method, run, controller };
  }

  it("marks package and dir passed when suite passed", () => {
    const { dir, pkg, run, controller } = makeAncestorFixture();
    controller.getResult.mockImplementation((id: string) => {
      if (id === "example.com/internal/auth/AuthSuite")
        return { status: "pass" as const, duration: 500 };
      return undefined;
    });

    resolveAncestorItems(run as any, controller as any);

    expect(run.passed).toHaveBeenCalledTimes(2);
    expect(run.passed).toHaveBeenCalledWith(pkg);
    expect(run.passed).toHaveBeenCalledWith(dir);
    expect(run.failed).not.toHaveBeenCalled();
  });

  it("marks package and dir failed when suite failed", () => {
    const { dir, pkg, run, controller } = makeAncestorFixture();
    controller.getResult.mockImplementation((id: string) => {
      if (id === "example.com/internal/auth/AuthSuite")
        return { status: "fail" as const, duration: 500 };
      return undefined;
    });

    resolveAncestorItems(run as any, controller as any);

    expect(run.failed).toHaveBeenCalledTimes(2);
    expect(run.failed).toHaveBeenCalledWith(pkg, []);
    expect(run.failed).toHaveBeenCalledWith(dir, []);
  });

  it("marks dir failed when one of multiple packages failed", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkgA = createItem("example.com/pkg/a", "a", dir, [{ id: "package" }]);
    const suiteA = createItem("example.com/pkg/a/SuiteA", "SuiteA", pkgA);
    const pkgB = createItem("example.com/pkg/b", "b", dir, [{ id: "package" }]);
    const suiteB = createItem("example.com/pkg/b/SuiteB", "SuiteB", pkgB);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/pkg/a/SuiteA")
          return { status: "pass" as const, duration: 100 };
        if (id === "example.com/pkg/b/SuiteB")
          return { status: "fail" as const, duration: 200 };
        return undefined;
      }),
      recordResult: vi.fn(),
      testController: {
        items: new Map<string, MockTestItem>([["dir:pkg", dir]]),
      },
    };

    resolveAncestorItems(run as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(pkgA);
    expect(run.failed).toHaveBeenCalledWith(pkgB, []);
    expect(run.failed).toHaveBeenCalledWith(dir, []);
  });

  it("aggregates from children, ignoring overwritten package result", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkg = createItem("example.com/pkg", "pkg", dir, [{ id: "package" }]);
    const suiteA = createItem("example.com/pkg/SuiteA", "SuiteA", pkg);
    const suiteB = createItem("example.com/pkg/SuiteB", "SuiteB", pkg);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/pkg")
          return { status: "pass" as const, duration: 500 };
        if (id === "example.com/pkg/SuiteA")
          return { status: "fail" as const, duration: 100 };
        if (id === "example.com/pkg/SuiteB")
          return { status: "pass" as const, duration: 200 };
        return undefined;
      }),
      recordResult: vi.fn(),
      testController: {
        items: new Map<string, MockTestItem>([["dir:pkg", dir]]),
      },
    };

    resolveAncestorItems(run as any, controller as any);

    expect(run.failed).toHaveBeenCalledWith(pkg, []);
    expect(run.failed).toHaveBeenCalledWith(dir, []);
    expect(run.passed).not.toHaveBeenCalled();
  });

  it("propagates through nested directory levels", () => {
    const root = createItem("dir:src", "src", undefined);
    const sub = createItem("dir:src/internal", "internal", root);
    const pkg = createItem("example.com/src/internal/svc", "svc", sub, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/src/internal/svc/Svc", "Svc", pkg);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/src/internal/svc/Svc")
          return { status: "fail" as const, duration: 300 };
        return undefined;
      }),
      recordResult: vi.fn(),
      testController: {
        items: new Map<string, MockTestItem>([["dir:src", root]]),
      },
    };

    resolveAncestorItems(run as any, controller as any);

    expect(run.failed).toHaveBeenCalledTimes(3);
    expect(run.failed).toHaveBeenCalledWith(pkg, []);
    expect(run.failed).toHaveBeenCalledWith(root, []);
    expect(run.failed).toHaveBeenCalledWith(sub, []);
  });

  it("propagates through wsFolder items", () => {
    const wsFolder = createItem("wsFolder:myproject", "myproject", undefined);
    const pkg = createItem("example.com/root", "root", wsFolder, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/root/Suite", "Suite", pkg);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/root/Suite")
          return { status: "pass" as const, duration: 100 };
        return undefined;
      }),
      recordResult: vi.fn(),
      testController: {
        items: new Map<string, MockTestItem>([
          ["wsFolder:myproject", wsFolder],
        ]),
      },
    };

    resolveAncestorItems(run as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(pkg);
    expect(run.passed).toHaveBeenCalledWith(wsFolder);
  });

  it("does not set state when no descendants have results", () => {
    const { dir, run, controller } = makeAncestorFixture();

    resolveAncestorItems(run as any, controller as any);

    expect(run.passed).not.toHaveBeenCalled();
    expect(run.failed).not.toHaveBeenCalled();
  });

  it("derives suite, package, and dir state from method results", () => {
    const { dir, pkg, suite, run, controller } = makeAncestorFixture();
    controller.getResult.mockImplementation((id: string) => {
      if (id === "example.com/internal/auth/AuthSuite/TestLogin")
        return { status: "fail" as const, duration: 50 };
      return undefined;
    });

    resolveAncestorItems(run as any, controller as any);

    expect(run.failed).toHaveBeenCalledTimes(3);
    expect(run.failed).toHaveBeenCalledWith(suite, []);
    expect(run.failed).toHaveBeenCalledWith(pkg, []);
    expect(run.failed).toHaveBeenCalledWith(dir, []);
    expect(run.passed).not.toHaveBeenCalled();
  });

  it("re-aggregates dir node instead of using stale stored result", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkgA = createItem("example.com/pkg/a", "a", dir, [{ id: "package" }]);
    const suiteA = createItem("example.com/pkg/a/SuiteA", "SuiteA", pkgA);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "dir:pkg")
          return { status: "fail" as const, duration: undefined };
        if (id === "example.com/pkg/a/SuiteA")
          return { status: "pass" as const, duration: 100 };
        return undefined;
      }),
      recordResult: vi.fn(),
      testController: {
        items: new Map([["dir:pkg", dir]]),
      },
    };

    resolveAncestorItems(run as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(pkgA);
    expect(run.passed).toHaveBeenCalledWith(dir);
    expect(run.failed).not.toHaveBeenCalled();
  });

  it("re-aggregates wsFolder node instead of using stale stored result", () => {
    const wsFolder = createItem("wsFolder:myproject", "myproject", undefined);
    const pkg = createItem("example.com/root", "root", wsFolder, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/root/Suite", "Suite", pkg);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "wsFolder:myproject")
          return { status: "fail" as const, duration: undefined };
        if (id === "example.com/root/Suite")
          return { status: "pass" as const, duration: 100 };
        return undefined;
      }),
      recordResult: vi.fn(),
      testController: {
        items: new Map([["wsFolder:myproject", wsFolder]]),
      },
    };

    resolveAncestorItems(run as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(pkg);
    expect(run.passed).toHaveBeenCalledWith(wsFolder);
    expect(run.failed).not.toHaveBeenCalled();
  });

  it("cascades through all ancestor levels when resolveAncestorsOf is followed by resolveAncestorItems", () => {
    // Models user scenario: re-run one package, siblings passed in prior runs,
    // structural nodes have no stored results (first run with new extension).
    //
    // Tree:
    //   wsFolder:root → dir:pkg → dir:platform → [api (pkg), billing (pkg)]
    //                           → dir:core     → [auth (pkg)]
    const root = createItem("wsFolder:root", "root", undefined);
    const dirPkg = createItem("dir:pkg", "pkg", root);
    const dirPlatform = createItem("dir:platform", "platform", dirPkg);
    const dirCore = createItem("dir:core", "core", dirPkg);

    const apiPkg = createItem("example.com/platform/api", "api", dirPlatform, [
      { id: "package" },
    ]);
    const apiSuite = createItem("example.com/platform/api/Api", "Api", apiPkg);

    const billingPkg = createItem(
      "example.com/platform/billing",
      "billing",
      dirPlatform,
      [{ id: "package" }],
    );
    const billingSuite = createItem(
      "example.com/platform/billing/Billing",
      "Billing",
      billingPkg,
    );

    const authPkg = createItem("example.com/core/auth", "auth", dirCore, [
      { id: "package" },
    ]);
    const authSuite = createItem("example.com/core/auth/Auth", "Auth", authPkg);

    // Dynamic mock: recordResult feeds back into getResult
    const results = new Map<string, { status: string; duration?: number }>();
    // Package + suite results from current run and prior runs
    results.set("example.com/platform/api", { status: "pass", duration: 100 });
    results.set("example.com/platform/api/Api", {
      status: "pass",
      duration: 50,
    });
    results.set("example.com/platform/billing", {
      status: "pass",
      duration: 200,
    });
    results.set("example.com/platform/billing/Billing", {
      status: "pass",
      duration: 100,
    });
    results.set("example.com/core/auth", { status: "pass", duration: 150 });
    results.set("example.com/core/auth/Auth", {
      status: "pass",
      duration: 75,
    });
    // NO structural node results — simulates first run after installing fix

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => results.get(id)),
      recordResult: vi.fn((id: string, status: string, duration?: number) => {
        results.set(id, { status, duration });
      }),
      testController: {
        items: new Map([["wsFolder:root", root]]),
      },
    };

    // Step 1: resolveAncestorsOf from just-ran package (streaming, during run).
    // dir:core has no stored result but its package descendants do, so
    // resolveAncestorsOf resolves it from descendants and cascades all the way up.
    resolveAncestorsOf(run as any, apiPkg as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(dirPlatform);
    expect(run.passed).toHaveBeenCalledWith(dirCore);
    expect(run.passed).toHaveBeenCalledWith(dirPkg);
    expect(run.passed).toHaveBeenCalledWith(root);
    expect(run.failed).not.toHaveBeenCalled();
  });
});

describe("startAncestors", () => {
  it("starts all ancestors of given items", () => {
    const root = createItem("dir:pkg", "pkg", undefined);
    const sub = createItem("dir:pkg/sub", "sub", root);
    const pkg = createItem("example.com/pkg/sub/a", "a", sub, [
      { id: "package" },
    ]);

    const run = { started: vi.fn() };
    startAncestors(run as any, [pkg as any]);

    expect(run.started).toHaveBeenCalledTimes(2);
    expect(run.started).toHaveBeenCalledWith(sub);
    expect(run.started).toHaveBeenCalledWith(root);
  });

  it("deduplicates shared ancestors", () => {
    const root = createItem("dir:pkg", "pkg", undefined);
    const pkgA = createItem("example.com/pkg/a", "a", root, [
      { id: "package" },
    ]);
    const pkgB = createItem("example.com/pkg/b", "b", root, [
      { id: "package" },
    ]);

    const run = { started: vi.fn() };
    startAncestors(run as any, [pkgA as any, pkgB as any]);

    expect(run.started).toHaveBeenCalledTimes(1);
    expect(run.started).toHaveBeenCalledWith(root);
  });
});

describe("resolveAncestorsOf", () => {
  it("propagates passed to parent when all siblings resolved", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkgA = createItem("example.com/pkg/a", "a", dir, [{ id: "package" }]);
    const pkgB = createItem("example.com/pkg/b", "b", dir, [{ id: "package" }]);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/pkg/a")
          return { status: "pass" as const, duration: 100 };
        if (id === "example.com/pkg/b")
          return { status: "pass" as const, duration: 200 };
        return undefined;
      }),
      recordResult: vi.fn(),
    };

    resolveAncestorsOf(run as any, pkgB as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(dir);
    expect(controller.recordResult).toHaveBeenCalledWith(
      "dir:pkg",
      "pass",
      undefined,
    );
  });

  it("propagates failed when any sibling failed", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkgA = createItem("example.com/pkg/a", "a", dir, [{ id: "package" }]);
    const pkgB = createItem("example.com/pkg/b", "b", dir, [{ id: "package" }]);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/pkg/a")
          return { status: "fail" as const, duration: 100 };
        if (id === "example.com/pkg/b")
          return { status: "pass" as const, duration: 200 };
        return undefined;
      }),
      recordResult: vi.fn(),
    };

    resolveAncestorsOf(run as any, pkgB as any, controller as any);

    expect(run.failed).toHaveBeenCalledWith(dir, []);
    expect(controller.recordResult).toHaveBeenCalledWith(
      "dir:pkg",
      "fail",
      undefined,
    );
  });

  it("stops propagation when a sibling has no result", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkgA = createItem("example.com/pkg/a", "a", dir, [{ id: "package" }]);
    const pkgB = createItem("example.com/pkg/b", "b", dir, [{ id: "package" }]);

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/pkg/a")
          return { status: "pass" as const, duration: 100 };
        return undefined;
      }),
      recordResult: vi.fn(),
    };

    resolveAncestorsOf(run as any, pkgA as any, controller as any);

    expect(run.passed).not.toHaveBeenCalled();
    expect(run.failed).not.toHaveBeenCalled();
    expect(controller.recordResult).not.toHaveBeenCalled();
  });

  it("propagates through multiple ancestor levels", () => {
    const root = createItem("wsFolder:myproject", "myproject", undefined);
    const dir = createItem("dir:internal", "internal", root);
    const pkg = createItem("example.com/internal/svc", "svc", dir, [
      { id: "package" },
    ]);

    const results = new Map<string, { status: string; duration?: number }>();
    results.set("example.com/internal/svc", { status: "pass", duration: 100 });

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => results.get(id)),
      recordResult: vi.fn((id: string, status: string, duration?: number) => {
        results.set(id, { status, duration });
      }),
    };

    resolveAncestorsOf(run as any, pkg as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(dir);
    expect(run.passed).toHaveBeenCalledWith(root);
    expect(controller.recordResult).toHaveBeenCalledWith(
      "dir:internal",
      "pass",
      undefined,
    );
    expect(controller.recordResult).toHaveBeenCalledWith(
      "wsFolder:myproject",
      "pass",
      undefined,
    );
  });

  it("resolves structural siblings from descendants when they have no stored result", () => {
    // dir:pkg has two structural children: dir:platform (has package result)
    // and dir:core (NO stored result, but its package descendant has a result).
    // resolveAncestorsOf should resolve dir:core from its descendants and
    // cascade to dir:pkg.
    const root = createItem("wsFolder:root", "root", undefined);
    const dirPkg = createItem("dir:pkg", "pkg", root);
    const dirPlatform = createItem("dir:platform", "platform", dirPkg);
    const dirCore = createItem("dir:core", "core", dirPkg);

    const apiPkg = createItem("example.com/platform/api", "api", dirPlatform, [
      { id: "package" },
    ]);
    createItem("example.com/platform/api/Api", "Api", apiPkg);

    const authPkg = createItem("example.com/core/auth", "auth", dirCore, [
      { id: "package" },
    ]);
    createItem("example.com/core/auth/Auth", "Auth", authPkg);

    const results = new Map<string, { status: string; duration?: number }>();
    results.set("example.com/platform/api", { status: "pass", duration: 100 });
    results.set("example.com/platform/api/Api", {
      status: "pass",
      duration: 50,
    });
    results.set("example.com/core/auth", { status: "pass", duration: 150 });
    results.set("example.com/core/auth/Auth", {
      status: "pass",
      duration: 75,
    });

    const run = { started: vi.fn(), passed: vi.fn(), failed: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => results.get(id)),
      recordResult: vi.fn((id: string, status: string, duration?: number) => {
        results.set(id, { status, duration });
      }),
    };

    resolveAncestorsOf(run as any, apiPkg as any, controller as any);

    expect(run.passed).toHaveBeenCalledWith(dirPlatform);
    expect(run.passed).toHaveBeenCalledWith(dirCore);
    expect(run.passed).toHaveBeenCalledWith(dirPkg);
    expect(run.passed).toHaveBeenCalledWith(root);
    expect(run.failed).not.toHaveBeenCalled();
  });
});

describe("resolveRunPatterns", () => {
  it("uses workspace patterns when prefix equals module path", () => {
    const result = resolveRunPatterns(
      [
        "example.com/proj/pkg/a",
        "example.com/proj/pkg/b",
        "example.com/proj/examples/auth",
      ],
      "example.com/proj",
      ["./...", "./examples/..."],
    );
    expect(result).toEqual(["./...", "./examples/..."]);
  });

  it("falls back to computeWildcard when prefix is deeper than module", () => {
    const result = resolveRunPatterns(
      ["example.com/proj/pkg/a", "example.com/proj/pkg/b"],
      "example.com/proj",
      ["./...", "./examples/..."],
    );
    expect(result).toEqual(["example.com/proj/pkg/..."]);
  });

  it("falls back to computeWildcard when no workspace patterns", () => {
    const result = resolveRunPatterns(
      ["example.com/pkg/a", "example.com/pkg/b"],
      "example.com",
      undefined,
    );
    expect(result).toEqual(["example.com/pkg/..."]);
  });

  it("returns undefined for single package", () => {
    expect(
      resolveRunPatterns(["example.com/proj/pkg/a"], "example.com/proj"),
    ).toBeUndefined();
  });

  it("returns undefined for empty array", () => {
    expect(resolveRunPatterns([], "example.com/proj")).toBeUndefined();
  });
});

describe("applyResults records before resolving ancestors", () => {
  it("records package result so resolveAncestorsOf can cascade", () => {
    const dir = createItem("dir:pkg", "pkg", undefined);
    const pkg = createItem("example.com/pkg", "pkg", dir, [{ id: "package" }]);
    const suite = createItem("example.com/pkg/MySuite", "MySuite", pkg);
    const method = createItem(
      "example.com/pkg/MySuite/TestFoo",
      "TestFoo",
      suite,
    );

    const results = new Map<string, { status: string; duration?: number }>();
    const run = {
      passed: vi.fn(),
      failed: vi.fn(),
      skipped: vi.fn(),
      started: vi.fn(),
      appendOutput: vi.fn(),
    };
    const controller = {
      findItem: vi.fn((id: string) => {
        if (id === "example.com/pkg") return pkg;
        if (id === "example.com/pkg/MySuite") return suite;
        if (id === "example.com/pkg/MySuite/TestFoo") return method;
        return undefined;
      }),
      getResult: vi.fn((id: string) => results.get(id)),
      recordResult: vi.fn((id: string, status: string, duration?: number) => {
        results.set(id, { status, duration });
      }),
      createDynamicSubtest: vi.fn(),
    };

    const events = [
      {
        Package: "example.com/pkg",
        Test: "MySuite/TestFoo",
        Action: "pass" as const,
        Elapsed: 0.1,
      },
      {
        Package: "example.com/pkg",
        Test: "MySuite",
        Action: "pass" as const,
        Elapsed: 0.2,
      },
      { Package: "example.com/pkg", Action: "pass" as const, Elapsed: 0.3 },
    ];

    applyResults(
      controller as any,
      run as any,
      events as any,
      "example.com/pkg",
      "/fake/dir",
    );

    // The package result must have been recorded
    expect(controller.recordResult).toHaveBeenCalledWith(
      "example.com/pkg",
      "pass",
      300,
    );
    // resolveAncestorsOf should have resolved dir:pkg because the pkg result was in the store
    expect(run.passed).toHaveBeenCalledWith(dir);
  });
});

describe("skipUnresolved", () => {
  it("marks descendants without results as skipped", () => {
    const pkg = createItem("example.com/pkg", "pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/pkg/SuiteA", "SuiteA", pkg);
    const method1 = createItem("example.com/pkg/SuiteA/Test1", "Test1", suite);
    const method2 = createItem("example.com/pkg/SuiteA/Test2", "Test2", suite);

    const run = { skipped: vi.fn() };
    const controller = {
      getResult: vi.fn(() => undefined),
    };

    skipUnresolved(run as any, pkg as any, controller as any);

    expect(run.skipped).toHaveBeenCalledTimes(3);
    expect(run.skipped).toHaveBeenCalledWith(suite);
    expect(run.skipped).toHaveBeenCalledWith(method1);
    expect(run.skipped).toHaveBeenCalledWith(method2);
  });

  it("does not touch items that have results", () => {
    const pkg = createItem("example.com/pkg", "pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/pkg/SuiteA", "SuiteA", pkg);
    const method1 = createItem("example.com/pkg/SuiteA/Test1", "Test1", suite);
    const method2 = createItem("example.com/pkg/SuiteA/Test2", "Test2", suite);

    const run = { skipped: vi.fn() };
    const controller = {
      getResult: vi.fn((id: string) => {
        if (id === "example.com/pkg/SuiteA/Test1")
          return { status: "pass" as const, duration: 100 };
        return undefined;
      }),
    };

    skipUnresolved(run as any, pkg as any, controller as any);

    expect(run.skipped).toHaveBeenCalledTimes(2);
    expect(run.skipped).toHaveBeenCalledWith(suite);
    expect(run.skipped).toHaveBeenCalledWith(method2);
    expect(run.skipped).not.toHaveBeenCalledWith(method1);
  });

  it("is a no-op for leaf items with no children", () => {
    const method = createItem("example.com/pkg/SuiteA/Test1", "Test1");
    const run = { skipped: vi.fn() };
    const controller = { getResult: vi.fn(() => undefined) };

    skipUnresolved(run as any, method as any, controller as any);

    expect(run.skipped).not.toHaveBeenCalled();
  });

  it("recurses through dynamic subtests", () => {
    const method = createItem("example.com/pkg/Suite/Test1", "Test1");
    const dynamic1 = createItem(
      "example.com/pkg/Suite/Test1/dynamic/sub1",
      "sub1",
      method,
    );
    const dynamic2 = createItem(
      "example.com/pkg/Suite/Test1/dynamic/sub2",
      "sub2",
      method,
    );

    const run = { skipped: vi.fn() };
    const controller = { getResult: vi.fn(() => undefined) };

    skipUnresolved(run as any, method as any, controller as any);

    expect(run.skipped).toHaveBeenCalledTimes(2);
    expect(run.skipped).toHaveBeenCalledWith(dynamic1);
    expect(run.skipped).toHaveBeenCalledWith(dynamic2);
  });
});
