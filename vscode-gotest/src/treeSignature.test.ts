import { describe, it, expect } from "vitest";
import { treeSignature } from "./treeSignature.js";
import type { DiscoverBehavior, DiscoverPackage } from "./types.js";

function behavior(name: string, children?: DiscoverBehavior[]) {
  return {
    name,
    display: name,
    kind: "it",
    line: 1,
    children,
  } as DiscoverBehavior;
}

function tree(behaviors: DiscoverBehavior[]): DiscoverPackage[] {
  return [
    {
      importPath: "example.com/m/a",
      dir: "/ws/a",
      suites: [
        {
          name: "ThingTestSuite",
          parallel: false,
          focused: false,
          excluded: false,
          guarded: false,
          file: "a_test.go",
          line: 1,
          col: 1,
          lifecycle: [],
          fixtures: [],
          methods: [
            {
              name: "TestIt",
              parallel: false,
              focused: false,
              excluded: false,
              file: "a_test.go",
              line: 2,
              col: 1,
              behaviors,
            },
          ],
        },
      ],
    } as DiscoverPackage,
  ];
}

describe("treeSignature", () => {
  it("is stable for an unchanged tree", () => {
    expect(treeSignature(tree([behavior("when_a")]))).toBe(
      treeSignature(tree([behavior("when_a")])),
    );
  });

  // The whole point of the signature is to say whether stored results have to
  // be applied again, and result ids go all the way down to behaviors. A
  // signature blind to them asserts an invariant it does not hold.
  it("changes when a behavior is renamed", () => {
    expect(treeSignature(tree([behavior("when_a")]))).not.toBe(
      treeSignature(tree([behavior("when_b")])),
    );
  });

  it("changes when a behavior is added", () => {
    expect(treeSignature(tree([behavior("when_a")]))).not.toBe(
      treeSignature(tree([behavior("when_a"), behavior("when_b")])),
    );
  });

  it("changes when a nested behavior is renamed", () => {
    const before = tree([behavior("when_a", [behavior("it_x")])]);
    const after = tree([behavior("when_a", [behavior("it_y")])]);
    expect(treeSignature(before)).not.toBe(treeSignature(after));
  });

  it("does not depend on package order", () => {
    const a = tree([behavior("when_a")]);
    const b = [
      { ...a[0], importPath: "example.com/m/b", dir: "/ws/b" },
      a[0],
    ] as DiscoverPackage[];
    expect(treeSignature(b)).toBe(treeSignature([b[1], b[0]]));
  });
});
