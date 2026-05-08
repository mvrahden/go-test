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
    constructor(public readonly message: string) {}
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
  applyResults,
} from "./runnerUtils.js";
import {
  buildPathTrie,
  collapsePathTrie,
  type PathNode,
} from "./testController.js";

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

  it("returns AppliedResult[] and does NOT call controller.recordResult", () => {
    const { controller, run, passItem, failItem, skipItem } =
      makeApplyResultsFixture();

    const events = [
      { Action: "run" as const, Test: "MySuite/TestPass", Package: "example.com/pkg" },
      { Action: "run" as const, Test: "MySuite/TestFail", Package: "example.com/pkg" },
      { Action: "run" as const, Test: "MySuite/TestSkip", Package: "example.com/pkg" },
      { Action: "pass" as const, Test: "MySuite/TestPass", Package: "example.com/pkg", Elapsed: 0.1 },
      { Action: "fail" as const, Test: "MySuite/TestFail", Package: "example.com/pkg", Elapsed: 0.2 },
      { Action: "skip" as const, Test: "MySuite/TestSkip", Package: "example.com/pkg" },
    ];

    const applied = applyResults(
      controller as any,
      run as any,
      events as any,
      "example.com/pkg",
      "/some/dir",
    );

    // Returns 3 AppliedResult entries
    expect(applied).toHaveLength(3);

    const passResult = applied.find((r) => r.itemId === passItem.id);
    expect(passResult).toEqual({ itemId: passItem.id, status: "pass", duration: 100 });

    const failResult = applied.find((r) => r.itemId === failItem.id);
    expect(failResult).toEqual({ itemId: failItem.id, status: "fail", duration: 200 });

    const skipResult = applied.find((r) => r.itemId === skipItem.id);
    expect(skipResult).toEqual({ itemId: skipItem.id, status: "skip", duration: undefined });

    // Does NOT call controller.recordResult
    expect(controller.recordResult).not.toHaveBeenCalled();

    // Does call run methods
    expect(run.passed).toHaveBeenCalledWith(passItem, 100);
    expect(run.failed).toHaveBeenCalledWith(failItem, expect.any(Array), 200);
    expect(run.skipped).toHaveBeenCalledWith(skipItem);
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
    expect(
      computeWildcard(["example.com/pkg/a", "example.com/pkg/b"]),
    ).toBe("example.com/pkg/...");
  });

  it("finds deep common prefix", () => {
    expect(
      computeWildcard([
        "example.com/pkg/platform/billing",
        "example.com/pkg/platform/cluster",
        "example.com/pkg/platform/auth",
      ]),
    ).toBe("example.com/pkg/platform/...");
  });

  it("stops at segment boundary", () => {
    expect(
      computeWildcard(["example.com/foo/bar", "example.com/foo/baz"]),
    ).toBe("example.com/foo/...");
  });

  it("handles divergence at top level", () => {
    expect(
      computeWildcard([
        "example.com/pkg/a",
        "example.com/internal/b",
      ]),
    ).toBe("example.com/...");
  });

  it("returns undefined when all paths are identical", () => {
    expect(
      computeWildcard(["example.com/pkg", "example.com/pkg"]),
    ).toBeUndefined();
  });
});
