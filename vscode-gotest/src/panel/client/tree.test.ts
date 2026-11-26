import { describe, it, expect } from "vitest";
import { treeLogic, type TreeState, type VRow } from "./tree.js";
import type { TreeEntry, TreeNode } from "../protocol.js";

const T = treeLogic();

const pkg = (p: string, name = p): TreeEntry => ({
  kind: "pkg",
  pkg: p,
  name,
  counts: "1 passed",
  tone: "pass",
  time: "1s",
});

const dir = (key: string, children: TreeEntry[]): TreeEntry => ({
  kind: "dir",
  key,
  name: key,
  packages: 0,
  counts: "",
  tone: "pass",
  hot: false,
  children,
});

// m/a holds a suite with a method, a When and two Its, and a second suite.
const SLICE: TreeNode[] = [
  {
    d: "Cart",
    k: "suite",
    s: "pass",
    pos: "0",
    id: "m/a/CartTestSuite",
    c: [
      {
        d: "Adds",
        k: "method",
        s: "pass",
        pos: "0.0",
        id: "m/a/CartTestSuite/TestAdds",
        c: [
          {
            d: "when empty",
            k: "block",
            s: "pass",
            pos: "0.0.0",
            c: [
              {
                d: "adds one",
                k: "block",
                s: "pass",
                pos: "0.0.0.0",
                id: "x",
                loc: ["/a_test.go", 3],
              },
              { d: "counts", k: "block", s: "pass", pos: "0.0.0.1" },
            ],
          },
        ],
      },
    ],
  },
  {
    d: "Stock",
    k: "suite",
    s: "pass",
    pos: "1",
    c: [{ d: "y", k: "block", s: "pass", pos: "1.0" }],
  },
];

const TREE: TreeEntry[] = [
  dir("pkg", [pkg("m/a", "a"), pkg("m/b", "b")]),
  pkg("m/c", "c"),
];

const state = (over: Partial<TreeState> = {}): TreeState => ({
  open: {},
  deep: {},
  level: "packages",
  ...over,
});

const keys = (rows: VRow[]) => T.flat(rows).map((r) => r.key);
const view = (
  st: TreeState,
  slices: Record<string, { nodes: TreeNode[] } | undefined> = {
    "m/a": { nodes: SLICE },
  },
) => T.rows(TREE, slices, st);

describe("the behavior tree's rows", () => {
  it("open directories and close packages by default", () => {
    expect(keys(view(state()))).toEqual(["d:pkg", "p:m/a", "p:m/b", "p:m/c"]);
  });

  it("open a package to its suites, a suite to its whole outline, a When as a heading", () => {
    const st = state();
    T.toggle(st, T.find(view(st), "p:m/a")!);
    expect(keys(view(st))).toEqual([
      "d:pkg",
      "p:m/a",
      "n:m/a:0",
      "n:m/a:1",
      "p:m/b",
      "p:m/c",
    ]);
    T.toggle(st, T.find(view(st), "n:m/a:0")!);
    const rows = view(st);
    expect(keys(rows).slice(2, 7)).toEqual([
      "n:m/a:0",
      "n:m/a:0.0",
      "n:m/a:0.0.0",
      "n:m/a:0.0.0.0",
      "n:m/a:0.0.0.1",
    ]);
    const when = T.find(rows, "n:m/a:0.0.0")!;
    expect([when.kind, when.expandable]).toEqual(["when", false]);
    expect(T.find(rows, "n:m/a:0.0")!.kind).toBe("method");
  });

  it("show that a package's rows load until its slice comes", () => {
    const st = state({ open: { "p:m/b": true } });
    const b = T.find(view(st), "p:m/b")!;
    expect(b.children.map((c) => c.kind)).toEqual(["loading"]);
  });

  it("open everything below a directory, and close it to its own rows", () => {
    const st = state();
    T.openBelow(st, T.find(view(st), "d:pkg")!);
    expect(st.deep).toEqual({ "m/a": true, "m/b": true });
    let rows = view(st);
    expect(keys(rows)).toContain("n:m/a:0.0.0.1");
    expect(T.allOpen(T.find(rows, "p:m/a")!)).toBe(true);
    // m/b has no slice yet: everything below is not open until it comes.
    expect(T.allOpen(T.find(rows, "d:pkg")!)).toBe(false);
    T.closeBelow(st, T.find(rows, "d:pkg")!);
    rows = view(st);
    expect(keys(rows)).toEqual(["d:pkg", "p:m/a", "p:m/b", "p:m/c"]);
    expect(T.find(rows, "d:pkg")!.open).toBe(true);
  });

  it("close everything below a suite and leave the suite open at its methods", () => {
    const st = state({ open: { "p:m/a": true, "n:m/a:0": true } });
    T.closeBelow(st, T.find(view(st), "n:m/a:0")!);
    const rows = view(st);
    expect(T.find(rows, "n:m/a:0")!.open).toBe(true);
    expect(T.find(rows, "n:m/a:0.0")!.open).toBe(false);
    expect(keys(rows)).not.toContain("n:m/a:0.0.0");
  });

  it("open the whole list to a level once, toggles after it staying free", () => {
    const st = state({ open: { "p:m/c": true } });
    T.setLevel(st, "suites");
    expect(st.open).toEqual({});
    let rows = view(st);
    expect(T.find(rows, "p:m/a")!.open).toBe(true);
    expect(T.find(rows, "n:m/a:0")!.open).toBe(false);
    T.toggle(st, T.find(rows, "p:m/a")!);
    expect(T.find(view(st), "p:m/a")!.open).toBe(false);
    T.setLevel(st, "behaviors");
    rows = view(st);
    expect(T.find(rows, "n:m/a:0")!.open).toBe(true);
    T.setLevel(st, "packages");
    expect(T.find(view(st), "p:m/a")!.open).toBe(false);
  });

  it("reveal opens the directories, the package and the rows above a row, and focuses it", () => {
    const st = state({ open: { "d:pkg": false } });
    T.reveal(st, TREE, "m/a", "0.0.0.1");
    expect(st.focus).toBe("n:m/a:0.0.0.1");
    expect(keys(view(st))).toContain("n:m/a:0.0.0.1");
    T.reveal(st, TREE, "m/c", "");
    expect(st.focus).toBe("p:m/c");
  });
});

describe("the tree's keys", () => {
  const st = state({ open: { "p:m/a": true, "n:m/a:0": true, "p:m/b": true } });
  const rows = view(st);
  const op = (focus: string, key: string, mods: object = {}) =>
    T.keyOp(rows, focus, { key, ...mods });

  it("move over the rows that show, the rows of a loading package left out", () => {
    expect(op("d:pkg", "ArrowDown")).toEqual({ op: "focus", key: "p:m/a" });
    expect(op("n:m/a:1", "ArrowDown")).toEqual({ op: "focus", key: "p:m/b" });
    expect(op("p:m/b", "ArrowDown")).toEqual({ op: "focus", key: "p:m/c" });
    expect(op("p:m/a", "ArrowUp")).toEqual({ op: "focus", key: "d:pkg" });
    expect(op("n:m/a:0.0", "Home")).toEqual({ op: "focus", key: "d:pkg" });
    expect(op("d:pkg", "End")).toEqual({ op: "focus", key: "p:m/c" });
  });

  it("open a closed branch or go to its first row; close an open one or go to the parent", () => {
    expect(op("n:m/a:1", "ArrowRight")).toEqual({
      op: "toggle",
      key: "n:m/a:1",
    });
    expect(op("n:m/a:0", "ArrowRight")).toEqual({
      op: "focus",
      key: "n:m/a:0.0",
    });
    expect(op("n:m/a:0", "ArrowLeft")).toEqual({
      op: "toggle",
      key: "n:m/a:0",
    });
    expect(op("n:m/a:0.0.0.0", "ArrowLeft")).toEqual({
      op: "focus",
      key: "n:m/a:0.0.0",
    });
    expect(op("n:m/a:0.0.0", "ArrowLeft")).toEqual({
      op: "focus",
      key: "n:m/a:0.0",
    });
  });

  it("open and close everything below with Shift, toggle with Enter, open a behavior's source", () => {
    expect(op("p:m/a", "ArrowRight", { shiftKey: true })).toEqual({
      op: "openBelow",
      key: "p:m/a",
    });
    expect(op("d:pkg", "ArrowLeft", { shiftKey: true })).toEqual({
      op: "closeBelow",
      key: "d:pkg",
    });
    expect(
      op("n:m/a:0.0.0.0", "ArrowRight", { shiftKey: true }),
    ).toBeUndefined();
    expect(op("p:m/a", "Enter")).toEqual({ op: "toggle", key: "p:m/a" });
    expect(op("n:m/a:0.0.0.0", "Enter")).toEqual({
      op: "source",
      key: "n:m/a:0.0.0.0",
    });
    expect(op("n:m/a:0.0.0.1", "Enter")).toBeUndefined();
  });

  it("copy a branch with Ctrl or Cmd+C, never a behavior, and open the menu with Shift+F10", () => {
    expect(op("n:m/a:0.0", "c", { ctrlKey: true })).toEqual({
      op: "copy",
      key: "n:m/a:0.0",
    });
    expect(op("d:pkg", "c", { metaKey: true })).toEqual({
      op: "copy",
      key: "d:pkg",
    });
    expect(op("n:m/a:0.0.0.0", "c", { ctrlKey: true })).toBeUndefined();
    expect(op("n:m/a:0", "c")).toBeUndefined();
    expect(op("n:m/a:0", "F10", { shiftKey: true })).toEqual({
      op: "menu",
      key: "n:m/a:0",
    });
    expect(op("n:m/a:0", "F10")).toBeUndefined();
    expect(op("n:m/a:0", "ContextMenu")).toEqual({
      op: "menu",
      key: "n:m/a:0",
    });
  });
});

describe("a row's actions", () => {
  const st = state({ open: { "p:m/a": true, "n:m/a:0": true } });
  const rows = view(st);
  const of = (k: string) => T.find(rows, k)!;

  it("are, in order: open everything below, copy, run, debug, open source, as the row's kind has them", () => {
    expect(T.actions(of("d:pkg"))).toEqual(["openBelow", "copy", "run"]);
    expect(T.actions(of("p:m/a"))).toEqual(["openBelow", "copy", "run"]);
    expect(T.actions(of("n:m/a:0"))).toEqual(["copy", "run", "debug"]);
    expect(T.actions(of("n:m/a:0.0"))).toEqual(["copy", "run", "debug"]);
    expect(T.actions(of("n:m/a:0.0.0.0"))).toEqual(["run", "debug", "source"]);
    expect(T.actions(of("n:m/a:0.0.0"))).toEqual([]);
  });

  it("turn into closing when everything below is open", () => {
    const all = state();
    T.openBelow(all, T.find(view(all), "p:m/a")!);
    expect(T.actions(T.find(view(all), "p:m/a")!)[0]).toBe("closeBelow");
  });

  it("list in the menu: both ways below a directory or package, closing below an open suite, copy, then the rest", () => {
    expect(T.menuItems(of("p:m/a"))).toEqual([
      "openBelow",
      "closeBelow",
      "copy",
      "-",
      "run",
    ]);
    expect(T.menuItems(of("n:m/a:0"))).toEqual([
      "closeBelow",
      "copy",
      "-",
      "run",
      "debug",
    ]);
    expect(T.menuItems(of("n:m/a:1"))).toEqual(["copy"]);
    expect(T.menuItems(of("n:m/a:0.0.0.0"))).toEqual([
      "run",
      "debug",
      "source",
    ]);
  });
});
