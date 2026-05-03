import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class TestTag {
    constructor(public readonly id: string) {}
  }
  return { TestTag };
});

import {
  getItemDepth,
  getRootItem,
  groupByPackage,
  buildRunFilter,
  getPackageDepth,
  getPackageItem,
  expandToPackages,
} from "./runnerUtils.js";

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

const mockCache = {
  getPackage: (importPath: string) => {
    if (importPath === "example.com/pkg") {
      return {
        importPath: "example.com/pkg",
        dir: "/abs/pkg",
        suites: [
          {
            name: "MySuite",
            fixtures: [],
            methods: [],
            lifecycle: [],
            parallel: false,
            focused: false,
            excluded: false,
            file: "t.go",
            line: 1,
            col: 1,
          },
          {
            name: "FixtureSuite",
            fixtures: ["SomeFixture"],
            methods: [],
            lifecycle: [],
            parallel: false,
            focused: false,
            excluded: false,
            file: "t.go",
            line: 10,
            col: 1,
          },
        ],
      };
    }
    return undefined;
  },
} as any;

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
    expect(
      buildRunFilter([pkg as any], "example.com/pkg", mockCache),
    ).toBeUndefined();
  });

  it("returns suite filter for depth-1 items", () => {
    const { suite } = makeTree();
    expect(buildRunFilter([suite as any], "example.com/pkg", mockCache)).toBe(
      "^TestMySuite$",
    );
  });

  it("returns suite/method filter for depth-2 items", () => {
    const { method } = makeTree();
    expect(buildRunFilter([method as any], "example.com/pkg", mockCache)).toBe(
      "^TestMySuite$/^TestFoo$",
    );
  });

  it("returns suite/method/subtest filter for depth-3 items", () => {
    const { dynamic } = makeTree();
    expect(buildRunFilter([dynamic as any], "example.com/pkg", mockCache)).toBe(
      "^TestMySuite$/^TestFoo$/^sub1$",
    );
  });

  it("returns undefined for suite with fixtures", () => {
    const pkg = createItem("example.com/pkg", "example.com/pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem(
      "example.com/pkg/FixtureSuite",
      "FixtureSuite",
      pkg,
    );
    expect(
      buildRunFilter([suite as any], "example.com/pkg", mockCache),
    ).toBeUndefined();
  });

  it("joins multiple methods with alternation", () => {
    const pkg = createItem("example.com/pkg", "example.com/pkg", undefined, [
      { id: "package" },
    ]);
    const suite = createItem("example.com/pkg/MySuite", "MySuite", pkg);
    const m1 = createItem("example.com/pkg/MySuite/TestA", "TestA", suite);
    const m2 = createItem("example.com/pkg/MySuite/TestB", "TestB", suite);
    expect(
      buildRunFilter([m1 as any, m2 as any], "example.com/pkg", mockCache),
    ).toBe("^TestMySuite$/^(TestA|TestB)$");
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
