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
  const pkg = createItem("example.com/pkg", "example.com/pkg");
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

    const groups = groupByPackage([
      suite as any,
      method as any,
      suite2 as any,
    ]);
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
    expect(
      buildRunFilter([suite as any], "example.com/pkg", mockCache),
    ).toBe("^TestMySuite$");
  });

  it("returns suite/method filter for depth-2 items", () => {
    const { method } = makeTree();
    expect(
      buildRunFilter([method as any], "example.com/pkg", mockCache),
    ).toBe("^TestMySuite$/^TestFoo$");
  });

  it("returns suite/method/subtest filter for depth-3 items", () => {
    const { dynamic } = makeTree();
    expect(
      buildRunFilter([dynamic as any], "example.com/pkg", mockCache),
    ).toBe("^TestMySuite$/^TestFoo$/^sub1$");
  });

  it("returns undefined for suite with fixtures", () => {
    const pkg = createItem("example.com/pkg", "example.com/pkg");
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
    const pkg = createItem("example.com/pkg", "example.com/pkg");
    const suite = createItem("example.com/pkg/MySuite", "MySuite", pkg);
    const m1 = createItem("example.com/pkg/MySuite/TestA", "TestA", suite);
    const m2 = createItem("example.com/pkg/MySuite/TestB", "TestB", suite);
    expect(
      buildRunFilter([m1 as any, m2 as any], "example.com/pkg", mockCache),
    ).toBe("^TestMySuite$/^(TestA|TestB)$");
  });
});
