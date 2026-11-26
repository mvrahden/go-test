import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: {
    joinPath: (...args: string[]) => ({ toString: () => args.join("/") }),
  },
  workspace: {
    getConfiguration: () => ({ get: () => true }),
  },
  window: {},
  ViewColumn: { Beside: 2 },
  commands: {},
}));

import {
  addStats,
  sourceOf,
  statsText,
  type SpecNode,
  type SpecPackage,
  type SpecStats,
  interpretSpecExit,
  encodeStateForScript,
  itemIdOf,
  nearestItemId,
  SpecViewPanel,
  declaredTests,
  formatRanAt,
  combineSpecData,
  provenanceText,
  groupText,
} from "./specView.js";
import { parseStoredState } from "./panel/host.js";
import type { DiscoverPackage } from "./types.js";
import { mkdirSync, mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

type Node = SpecNode;

function node(
  name: string,
  status: string,
  children: Node[] = [],
  extra: Partial<Node> = {},
): Node {
  return {
    name,
    display: name,
    kind: children.length > 0 ? "suite" : "block",
    status,
    duration: 0.002,
    focused: false,
    excluded: false,
    external: false,
    output: [],
    children,
    ...extra,
  };
}

function stats(over: Partial<SpecStats> = {}): SpecStats {
  return {
    suites: 0,
    behaviors: 0,
    tests: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    ...over,
  };
}

describe("statsText", () => {
  it("words the CLI's counts as its trailer does, not run included", () => {
    expect(
      statsText(stats({ passed: 3, failed: 1, skipped: 2, notRun: 4 })),
    ).toBe("3 passed, 1 failed, 2 skipped, 4 not run");
    expect(statsText(stats({ failedPackages: 1 }))).toBe("1 failed package");
    expect(statsText(undefined)).toBe("");
  });
});

describe("addStats", () => {
  it("adds the packages' counts for a module's row", () => {
    const sum = addStats([
      stats({ passed: 2, notRun: 1 }),
      undefined,
      stats({ passed: 1, failed: 1, failedPackages: 1 }),
    ]);
    expect(sum).toMatchObject({
      passed: 3,
      failed: 1,
      notRun: 1,
      failedPackages: 1,
    });
    expect(addStats([undefined])).toBeUndefined();
  });
});

describe("sourceOf", () => {
  it("joins the node's file to its package's directory, at its line", () => {
    expect(
      sourceOf(node("x", "pass", [], { file: "a_test.go", line: 7 }), "/p"),
    ).toEqual({ file: path.join("/p", "a_test.go"), line: 7 });
  });

  it("names no source for a node only a run revealed", () => {
    expect(sourceOf(node("x", "pass"), "/p")).toBeUndefined();
    expect(
      sourceOf(
        node("x", "pass", [], { file: "a_test.go", line: 7 }),
        undefined,
      ),
    ).toBeUndefined();
  });
});

describe("interpretSpecExit", () => {
  const SPEC_JSON = '{"packages":[],"stats":{}}';

  it("keeps the rendered spec when the stream carried failures", () => {
    // `gotest spec --input=-` exits 1 to report "this tree has failures".
    // That is a verdict about the tests, not a failure to render.
    const outcome = interpretSpecExit(1, SPEC_JSON, "");
    expect(outcome).toEqual({ ok: true, stdout: SPEC_JSON });
  });

  it("keeps the rendered spec when go run appends its exit status epilogue", () => {
    // The extension resolves the CLI through `go run` by default, which
    // prints "exit status 1" on stderr whenever the child exits non-zero.
    const outcome = interpretSpecExit(1, SPEC_JSON, "exit status 1\n");
    expect(outcome).toEqual({ ok: true, stdout: SPEC_JSON });
  });

  it("keeps the rendered spec on a clean run", () => {
    const outcome = interpretSpecExit(0, SPEC_JSON, "");
    expect(outcome).toEqual({ ok: true, stdout: SPEC_JSON });
  });

  it("reports an operational failure that produced no spec", () => {
    const outcome = interpretSpecExit(
      2,
      "",
      "FAIL: parsing test events: unexpected EOF\n",
    );
    expect(outcome).toEqual({
      ok: false,
      message:
        "gotest spec exited with code 2: FAIL: parsing test events: unexpected EOF",
    });
  });

  it("reports a go run build failure instead of swallowing it as a verdict", () => {
    // `go run` also exits 1 when the module fails to build. There is no spec
    // on stdout, so this must not be mistaken for a failing test tree.
    const outcome = interpretSpecExit(1, "", "spec.go:12:2: undefined: Foo\n");
    expect(outcome).toEqual({
      ok: false,
      message: "gotest spec exited with code 1: spec.go:12:2: undefined: Foo",
    });
  });

  it("strips the go run epilogue from a reported failure", () => {
    const outcome = interpretSpecExit(
      2,
      "",
      "FAIL: opening input file: no such file\nexit status 2\n",
    );
    expect(outcome).toEqual({
      ok: false,
      message:
        "gotest spec exited with code 2: FAIL: opening input file: no such file",
    });
  });

  it("reports a signal death that left no spec behind", () => {
    const outcome = interpretSpecExit(null, "", "");
    expect(outcome).toEqual({
      ok: false,
      message: "gotest spec exited with code null",
    });
  });
});

describe("encodeStateForScript", () => {
  // Regression: the spec is embedded inside a <script> block, and a test whose
  // failure output contained "</script>" closed that block early, spilling the
  // rest of the document into the page as live markup. Found by running a
  // suite whose assertion message carried markup.
  it("does not let test output close the script block", () => {
    const encoded = encodeStateForScript({
      message: "</script><script>alert('xss')</script>",
    });
    expect(encoded).not.toContain("</script>");
    expect(encoded).not.toContain("<script>");
  });

  it("round-trips to the identical value", () => {
    const data = {
      message: "</script><img src=x onerror=alert(1)>",
      nested: { markup: "<b>&amp;</b>", unicode: "日本語 🧪" },
    };
    expect(JSON.parse(encodeStateForScript(data))).toEqual(data);
  });

  it("leaves ordinary payloads parseable", () => {
    expect(
      JSON.parse(encodeStateForScript({ packages: [], stats: {} })),
    ).toEqual({ packages: [], stats: {} });
  });
});

describe("itemIdOf", () => {
  const leaf = (name: string, extra: Partial<Node> = {}) =>
    node(name, "pass", [], extra);

  it("names a node by the id the Test Explorer gives it", () => {
    const suite = itemIdOf("example.com/cart", null, leaf("TestCartTestSuite"));
    expect(suite).toBe("example.com/cart/CartTestSuite");
    const method = itemIdOf("example.com/cart", suite, leaf("TestAdd"));
    expect(itemIdOf("example.com/cart", method, leaf("when_empty"))).toBe(
      "example.com/cart/CartTestSuite/TestAdd/when_empty",
    );
    expect(itemIdOf("example.com/cart", null, leaf("TestPlain"))).toBe(
      "example.com/cart/Plain",
    );
  });

  it("names no item for a fixture node or a row beneath one", () => {
    expect(
      itemIdOf("example.com/cart", null, leaf("Test_DBFixture")),
    ).toBeUndefined();
    expect(
      itemIdOf("example.com/cart", undefined, leaf("TestAdd")),
    ).toBeUndefined();
  });

  it("marks only an external twin's run, not a lone external suite", () => {
    expect(
      itemIdOf(
        "p",
        null,
        leaf("TestTwinsTestSuite", { external: true, variant: 2 }),
      ),
    ).toBe("p/TwinsTestSuite~external");
    expect(
      itemIdOf("p", null, leaf("TestLoneTestSuite", { external: true })),
    ).toBe("p/LoneTestSuite");
  });
});

describe("nearestItemId", () => {
  it("falls back to the closest ancestor the tree holds", () => {
    const tree = new Set(["p/S", "p/S/TestM"]);
    const has = (id: string) => tree.has(id);
    expect(nearestItemId("p/S/TestM", has)).toBe("p/S/TestM");
    expect(nearestItemId("p/S/TestM/row_101", has)).toBe("p/S/TestM");
    expect(nearestItemId("q/T", has)).toBeUndefined();
  });

  it("falls to the package item when neither the node nor its suite is an item", () => {
    const has = (id: string) => id === "p";
    expect(
      nearestItemId("p/S/TestM/gets_./", has, ["p", "p/S", "p/S/TestM"]),
    ).toBe("p");
  });

  it("takes the nearest ancestor from the list, not from cutting at a slash", () => {
    const tree = new Set(["p/S", "p/S/TestM", "p/S/TestM/gets_."]);
    const has = (id: string) => tree.has(id);
    const id = "p/S/TestM/gets_./";
    expect(nearestItemId(id, has, ["p/S", "p/S/TestM"])).toBe("p/S/TestM");
    expect(nearestItemId(id, has)).toBe("p/S/TestM/gets_.");
  });
});

describe("the kept runs on disk", () => {
  const channel = {
    info: () => {},
    warn: () => {},
    error: () => {},
    debug: () => {},
  } as never;
  // A record as this build writes it, under the envelope version given.
  const file = (version: number) =>
    JSON.stringify({
      version,
      data: [
        {
          path: "example.com/cart",
          folder: "/ws",
          pkgLines: ['{"Action":"pass","Package":"example.com/cart"}'],
          tests: [
            [
              "1\u0000TestCartTestSuite",
              [
                {
                  seq: 0,
                  line: '{"Action":"pass","Package":"example.com/cart","Test":"TestCartTestSuite"}',
                },
              ],
            ],
          ],
          runLevel: false,
        },
      ],
    });
  const keptBy = (panel: SpecViewPanel) =>
    (panel as unknown as { runs: { size: number } }).runs.size;

  it("restore drops a store of the previous version", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-specruns-"));
    try {
      writeFileSync(path.join(dir, "specRuns.json"), file(1));
      const old = new SpecViewPanel(channel, undefined, dir);
      await old.load();
      expect(keptBy(old)).toBe(0);

      writeFileSync(path.join(dir, "specRuns.json"), file(2));
      const current = new SpecViewPanel(channel, undefined, dir);
      await current.load();
      expect(keptBy(current)).toBe(1);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("declaredTests", () => {
  const method = (name: string, testName?: string) => ({
    name,
    ...(testName ? { testName } : {}),
    parallel: false,
    focused: false,
    excluded: false,
    file: "cart_test.go",
    line: 1,
    col: 1,
  });
  const suite = (
    name: string,
    methods: ReturnType<typeof method>[],
    fuzzers: ReturnType<typeof method>[] = [],
    external = false,
  ) => ({
    name,
    ...(external ? { external: true } : {}),
    parallel: false,
    focused: false,
    excluded: false,
    guarded: false,
    file: "cart_test.go",
    line: 1,
    col: 1,
    lifecycle: [],
    fixtures: [],
    methods,
    benchmarks: [],
    fuzzers,
  });
  const pkg: DiscoverPackage = {
    importPath: "example.com/cart",
    dir: "/ws/cart",
    suites: [
      suite(
        "CartTestSuite",
        [method("TestAdd"), method("TestTotal")],
        [method("FuzzParse")],
      ),
      suite("CartTestSuite", [method("TestAdd", "TestAdd#01")], [], true),
      suite("Order_FuzzyTestSuite", [], [method("FuzzIds")]),
    ],
  };
  const declared = declaredTests(pkg)!;

  it.each([
    ["a suite's own line", 1, "TestCartTestSuite", true],
    ["a declared method", 1, "TestCartTestSuite/TestAdd", true],
    [
      "a behavior under a declared method",
      1,
      "TestCartTestSuite/TestAdd/when_empty/it_adds",
      true,
    ],
    [
      "an external twin's method by its reported name",
      2,
      "TestCartTestSuite/TestAdd#01",
      true,
    ],
    [
      "a repeat's method under -count (the internal twin's second run)",
      3,
      "TestCartTestSuite/TestTotal#01",
      true,
    ],
    ["a method that is gone", 1, "TestCartTestSuite/TestRemoved", false],
    [
      "a repeat of a method that is gone (the internal twin's second run)",
      3,
      "TestCartTestSuite/TestRemoved#01",
      false,
    ],
    [
      "a behavior under a method that is gone",
      1,
      "TestCartTestSuite/TestRemoved/when_x",
      false,
    ],
    ["a suite that is gone", 1, "TestOldTestSuite/TestX", false],
    ["a plain test function", 1, "TestParseFlags", true],
    ["a plain test function's subtest", 1, "TestParseFlags/empty", true],
    ["a declared fuzz target", 1, "FuzzCartTestSuite_FuzzParse", true],
    ["a fuzz target's seed row", 1, "FuzzCartTestSuite_FuzzParse/seed#0", true],
    [
      "a fuzz target of a suite whose name holds _Fuzz",
      1,
      "FuzzOrder_FuzzyTestSuite_FuzzIds",
      true,
    ],
    ["a fuzz target that is gone", 1, "FuzzCartTestSuite_FuzzGone", false],
    ["a plain fuzz function", 1, "FuzzDecode", true],
    ["a benchmark", 1, "BenchmarkCartTestSuite/BenchmarkAdd", true],
  ])("%s", (_name, instance, testPath, want) => {
    expect(declared(instance as number, testPath as string)).toBe(want);
  });

  it("says nothing about a package that does not load", () => {
    expect(declaredTests({ ...pkg, broken: true })).toBeUndefined();
  });

  it("matches a twin's row against its own twin's methods", () => {
    // The external twin's TestAdd is deleted; the internal twin keeps its own.
    const gone = declaredTests({
      ...pkg,
      suites: [
        suite("CartTestSuite", [method("TestAdd"), method("TestTotal")]),
        suite("CartTestSuite", [method("TestTotal", "TestTotal#01")], [], true),
      ],
    })!;
    expect(gone(2, "TestCartTestSuite/TestAdd#01")).toBe(false);
    expect(gone(1, "TestCartTestSuite/TestAdd")).toBe(true);
    expect(gone(2, "TestCartTestSuite/TestTotal#01")).toBe(true);
  });
});

describe("formatRanAt", () => {
  const now = new Date(2026, 10, 11, 16, 5).getTime();
  it("gives the time of a run of today", () => {
    expect(formatRanAt(new Date(2026, 10, 11, 9, 7).getTime(), now)).toBe(
      "09:07",
    );
  });
  it("adds the date to a run of another day", () => {
    expect(formatRanAt(new Date(2026, 10, 10, 23, 59).getTime(), now)).toBe(
      "2026-11-10 23:59",
    );
  });
});

describe("the document's run and groups", () => {
  it("states the run's commit, dirty tree, agent and exit code", () => {
    expect(
      provenanceText({
        commit: "abc1234",
        dirty: true,
        agent: "claude-code",
        exitCode: 1,
      }),
    ).toBe("abc1234, uncommitted changes · agent claude-code · exit 1");
    expect(provenanceText({ commit: "abc1234" })).toBe(
      "abc1234 · run did not finish",
    );
  });

  it("says a --changed run was narrowed and how many packages it left out", () => {
    expect(
      provenanceText({ exitCode: 0, changed: { ran: 1, unchanged: 3 } }),
    ).toBe(
      "exit 0 · narrowed by --changed: ran 1 package, left out 3 unchanged",
    );
    expect(
      provenanceText({
        exitCode: 0,
        changed: { ran: 4, unchanged: 0, noRecord: true },
      }),
    ).toBe("exit 0 · --changed found no record and ran everything");
  });

  it("words a group as the CLI's summary does", () => {
    expect(groupText({ name: "e2e", suites: 1, wall: 72 })).toBe(
      "group e2e: 1 suite, 72.0s, run apart",
    );
    expect(groupText({ name: "db", suites: 2, wall: 0.5 })).toBe(
      "group db: 2 suites, 500ms, run apart",
    );
  });

  it("keeps each folder's run and groups when it combines documents", () => {
    const one = {
      packages: [],
      stats: stats(),
      run: { commit: "a" },
      groups: [{ name: "x", suites: 1, wall: 1 }],
    };
    const two = {
      packages: [],
      stats: stats({ vacuous: 1 }),
      run: { commit: "b" },
    };
    const joined = combineSpecData([one, two]);
    expect(joined.runs?.map((r) => r.commit)).toEqual(["a", "b"]);
    expect(joined.groups).toHaveLength(1);
    expect(joined.stats.vacuous).toBe(1);
  });
});

describe("restoring a panel", () => {
  it("restores the tab, its opened packages, search and chips, and the folders", () => {
    expect(
      parseStoredState({
        tab: "change",
        open: ["m/a", 3],
        search: "cart",
        filters: ["fail", "bogus"],
        folder: "/ws",
        runsFolder: "/r",
      }),
    ).toEqual({
      tab: "change",
      open: ["m/a"],
      search: "cart",
      filters: ["fail"],
      folder: "/ws",
      runsFolder: "/r",
    });
  });

  it("reads an older panel's scope, Spec as Now, and drops its spec", () => {
    expect(
      parseStoredState({ scope: "spec", spec: { packages: [] } }).tab,
    ).toBe("now");
    expect(parseStoredState({ scope: "runs" }).tab).toBe("runs");
    expect(parseStoredState({ scope: "history" }).tab).toBeUndefined();
    expect(parseStoredState(null)).toEqual({
      open: [],
      search: "",
      filters: [],
    });
  });
});
