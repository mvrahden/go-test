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
  buildSpecBody,
  sourceOf,
  statsText,
  type SpecNode,
  type SpecPackage,
  type SpecStats,
  interpretSpecExit,
  encodeStateForScript,
  itemIdOf,
  nearestItemId,
  linkSources,
  SpecViewPanel,
  renderErrorHtml,
  declaredTests,
  formatRanAt,
  buildScopeTabs,
  combineSpecData,
  parseStoredState,
  provenanceText,
  groupText,
} from "./specView.js";
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

function body(
  packages: SpecPackage[],
  modulePaths: string[] = [],
  hidden: Set<string> = new Set(),
  dirOf: (p: string) => string | undefined = () => undefined,
): string {
  return buildSpecBody(
    { packages, stats: stats() },
    modulePaths,
    { dirOf },
    [],
    hidden,
  );
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

describe("buildSpecBody freshness", () => {
  const pkg = (p: string): SpecPackage => ({
    path: p,
    status: "pass",
    duration: 0,
    nodes: [node("TestATestSuite", "pass", [node("x", "pass")])],
  });
  const render = (
    pkgs: SpecPackage[],
    fresh: Record<string, "current" | "stale" | "unknown">,
  ) =>
    buildSpecBody({ packages: pkgs, stats: stats() }, ["m"], {
      dirOf: () => undefined,
      freshnessOf: (p) => fresh[p],
    });

  it("dims a stale package's rows under its label and counts it in the header", () => {
    const html = render([pkg("m/a"), pkg("m/b")], {
      "m/a": "stale",
      "m/b": "current",
    });
    expect(html).toMatch(/<details class="pkg-section stale" open>/);
    expect(html).toMatch(
      /<summary class="pkg-header">a[^]*stale: inputs changed since the run<\/span><\/summary>/,
    );
    expect(html).toMatch(
      /<details class="pkg-section" open>\s*<summary class="pkg-header">b/,
    );
    expect(html).toContain(
      '<div class="stale-headline" role="status">1 of 2 packages stale: their inputs changed since the run</div>',
    );
  });

  it("shows an unknown package's freshness only in a tooltip", () => {
    const html = render([pkg("m/a"), pkg("m/b")], { "m/a": "unknown" });
    expect(html).not.toContain("stale");
    expect(html).toMatch(/class="fresh-unknown" title="Freshness unknown/);
  });

  it("marks a lone package's tree stale", () => {
    const html = buildSpecBody({ packages: [pkg("m/a")], stats: stats() }, [], {
      dirOf: () => undefined,
      freshnessOf: () => "stale",
    });
    expect(html).toContain('<div class="pkg-body stale">');
    expect(html).toContain("stale: inputs changed since the run");
  });
});

describe("buildSpecBody", () => {
  it("shows a branch with the status the CLI gave it, deriving none from its children", () => {
    const html = body([
      {
        path: "example.com/p",
        status: "fail",
        duration: 0,
        nodes: [
          node("TestAsGivenTestSuite", "pass", [node("fails", "fail")]),
          node("TestFailedTestSuite", "fail", [node("fails", "fail")]),
        ],
      },
    ]);
    expect(html).toMatch(
      /<details class="branch pass" data-display="TestAsGivenTestSuite">/,
    );
    expect(html).toMatch(
      /<details class="branch fail" data-display="TestFailedTestSuite" open>/,
    );
  });

  it("gives each branch its package and its place among the shown rows, with a copy button", () => {
    const html = body([
      {
        path: "example.com/p",
        status: "pass",
        duration: 0,
        nodes: [
          node("TestAATestSuite", "pass", [node("x", "pass")]),
          node("TestBBTestSuite", "pass", [
            node("TestM", "pass", [node("gets_./", "pass")]),
          ]),
        ],
      },
    ]);
    const summaries = html.match(/<summary class="node[^>]*>/g) ?? [];
    expect(summaries).toHaveLength(3);
    expect(summaries[0]).toContain('data-pkg="example.com/p"');
    expect(summaries[0]).toContain('data-position="0"');
    expect(summaries[1]).toContain('data-position="1"');
    expect(summaries[2]).toContain('data-position="1.0"');
    expect(html).toContain('data-act="copyNode"');
  });

  it("lists the item ids above a row, nearest last, so a slash in a name cannot mislead", () => {
    const html = body([
      {
        path: "example.com/p",
        status: "pass",
        duration: 0,
        nodes: [
          node("TestSTestSuite", "pass", [
            node("TestM", "pass", [node("gets_./", "pass")]),
          ]),
        ],
      },
    ]);
    const leaf = html.match(
      /<div class="leaf[^>]*data-display="gets_\.\/"[^>]*>/,
    );
    expect(leaf?.[0]).toContain(
      'data-ancestors="[&quot;example.com/p&quot;,&quot;example.com/p/STestSuite&quot;,&quot;example.com/p/STestSuite/TestM&quot;]"',
    );
  });

  it("marks a node the run did not report NOT RUN, with no duration", () => {
    const html = body([
      {
        path: "example.com/p",
        status: "none",
        duration: 0,
        nodes: [node("TestLeftTestSuite", "none", [node("waits", "none")])],
      },
    ]);
    expect(html).toMatch(
      /class="leaf block none"[^>]*>\s*<span class="icon none">/,
    );
    expect(html).toContain('<span class="tag none">NOT RUN</span>');
    expect(html).not.toContain("<1ms");
  });

  it("heads each package with the CLI's counts and each module with their sum", () => {
    const pkg = (p: string, s: SpecStats): SpecPackage => ({
      path: p,
      status: "pass",
      duration: 0,
      nodes: [node("TestOneTestSuite", "pass", [node("ok", "pass")])],
      stats: s,
    });
    const html = body(
      [
        pkg("example.com/a/x", stats({ passed: 2, notRun: 1 })),
        pkg("example.com/a/y", stats({ passed: 1, failed: 1 })),
        pkg("example.com/b", stats({ skipped: 1 })),
      ],
      ["example.com/a", "example.com/b"],
    );
    expect(html).toContain(
      'example.com/a <span class="row-stats">3 passed, 1 failed, 1 not run</span>',
    );
    expect(html).toContain(
      'x <span class="row-stats">2 passed, 1 not run</span>',
    );
    expect(html).toContain(
      'y <span class="row-stats">1 passed, 1 failed</span>',
    );
  });

  it("links a node to the source the CLI names, and no node without a position", () => {
    const html = body(
      [
        {
          path: "example.com/p",
          dir: "/src/p",
          status: "pass",
          duration: 0,
          nodes: [
            node(
              "TestPlacedTestSuite",
              "pass",
              [
                node("declared", "pass", [], { file: "p_test.go", line: 12 }),
                node("revealed", "pass"),
              ],
              { file: "p_test.go", line: 3 },
            ),
          ],
        },
      ],
      [],
      new Set(),
      () => "/elsewhere",
    );
    const file = path.join("/src/p", "p_test.go");
    expect(html).toContain(`data-loc-file="${file}" data-loc-line="3"`);
    expect(html).toContain(`data-loc-file="${file}" data-loc-line="12"`);
    expect(html.match(/data-loc-file=/g)).toHaveLength(2);
  });

  it("shows a package's own failure when the CLI counts it, and only then", () => {
    const failing = (failedPackages: number): SpecPackage => ({
      path: "example.com/p",
      status: "fail",
      duration: 0,
      nodes: [],
      output: ["# example.com/p", "p.go:3:1: syntax error"],
      stats: stats({ failedPackages }),
    });
    expect(body([failing(1)])).toContain("<strong>FAIL</strong>");
    expect(body([failing(1)])).toContain("syntax error");
    expect(body([failing(0)])).not.toContain("<strong>FAIL</strong>");
  });

  it("shows no package failure when the CLI gives it no output, as under --hide=fail", () => {
    const hidden: SpecPackage = {
      path: "example.com/p",
      status: "fail",
      duration: 0,
      nodes: [],
      output: [],
      stats: stats({ failedPackages: 1 }),
    };
    expect(body([hidden])).not.toContain("<strong>FAIL</strong>");
  });

  it("gives a package whose rows the CLI left out no row, and still adds its counts to its module", () => {
    const pkg = (p: string, s: SpecStats, nodes: Node[]): SpecPackage => ({
      path: p,
      status: "pass",
      duration: 0,
      nodes,
      stats: s,
    });
    const html = body(
      [
        pkg("example.com/a/green", stats({ passed: 2 }), []),
        pkg("example.com/a/red", stats({ passed: 1, failed: 1 }), [
          node("TestRedTestSuite", "fail", [node("breaks", "fail")]),
        ]),
        pkg("example.com/b", stats({ passed: 1 }), []),
      ],
      ["example.com/a", "example.com/b"],
    );
    expect(html).toContain(
      'example.com/a <span class="row-stats">3 passed, 1 failed</span>',
    );
    expect(html).toContain(
      'red <span class="row-stats">1 passed, 1 failed</span>',
    );
    expect(html).not.toContain(">green ");
    expect(html).not.toContain("example.com/b");
  });

  it("offers a toggle per status the CLI hides, not run included, off for the hidden", () => {
    const html = body([], [], new Set(["pass", "none"]));
    expect(html).toContain('class="filter-btn pass" data-filter="pass"');
    expect(html).toContain('class="filter-btn fail active" data-filter="fail"');
    expect(html).toContain('class="filter-btn skip active" data-filter="skip"');
    expect(html).toContain(
      'class="filter-btn none" data-filter="none" title="Show or hide not run"',
    );
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

describe("an external twin's rows", () => {
  const pkg: SpecPackage = {
    path: "example.com/twins",
    status: "pass",
    duration: 0,
    nodes: [
      node("TestTwinsTestSuite", "pass", [
        node("TestCreate", "pass", [], { kind: "method" }),
      ]),
      node(
        "TestTwinsTestSuite",
        "pass",
        [
          node("TestCreate", "pass", [node("when_empty", "pass")], {
            kind: "method",
            reported: "TestCreate#01",
          }),
        ],
        { external: true, variant: 2 },
      ),
    ],
  };
  const twin = "example.com/twins/TwinsTestSuite~external";
  const rows = () =>
    [
      ...body([pkg]).matchAll(
        /data-id="([^"]+)"(?: data-ancestors="([^"]+)")?/g,
      ),
    ].map((m) => ({
      id: m[1],
      ancestors: m[2]
        ? (JSON.parse(m[2].replace(/&quot;/g, '"')) as string[])
        : [],
    }));

  it("name the twin's items, its method by the name go test reported", () => {
    expect(rows().map((r) => r.id)).toEqual([
      "example.com/twins/TwinsTestSuite",
      "example.com/twins/TwinsTestSuite/TestCreate",
      twin,
      `${twin}/TestCreate#01`,
      `${twin}/TestCreate#01/when_empty`,
    ]);
  });

  it("resolve to the twin's nearest item", () => {
    const tree = new Set(["example.com/twins", twin, `${twin}/TestCreate#01`]);
    const behavior = rows().find((r) => r.id.endsWith("when_empty"))!;
    expect(
      nearestItemId(behavior.id, (id) => tree.has(id), behavior.ancestors),
    ).toBe(`${twin}/TestCreate#01`);
  });
});

describe("repeated runs of a suite", () => {
  const suiteNode = (extra: Partial<Node>) =>
    node("TestCartTestSuite", "pass", [node("TestAdd", "pass")], extra);
  const shown = (nodes: Node[]) => {
    const html = body([
      { path: "example.com/cart", status: "pass", duration: 0, nodes },
    ]);
    return [
      ...html.matchAll(
        /<summary class="node suite" data-id="([^"]+)"[^>]*>(.*?)<\/summary>/g,
      ),
    ].map((m) => ({
      id: m[1],
      external: m[2].includes("EXT."),
      run: /RUN (\d+ OF \d+)/.exec(m[2])?.[1],
    }));
  };
  const suite = "example.com/cart/CartTestSuite";
  const twin = `${suite}~external`;

  it("shows a second run as run 2 of 2 of the same item", () => {
    expect(
      shown([
        suiteNode({ run: 1, runs: 2 }),
        suiteNode({ variant: 2, run: 2, runs: 2 }),
      ]),
    ).toEqual([
      { id: suite, external: false, run: "1 OF 2" },
      { id: suite, external: false, run: "2 OF 2" },
    ]);
  });

  it("shows both twins with their runs under -count=2", () => {
    expect(
      shown([
        suiteNode({ run: 1, runs: 2 }),
        suiteNode({ external: true, variant: 2, run: 1, runs: 2 }),
        suiteNode({ variant: 3, run: 2, runs: 2 }),
        suiteNode({ external: true, variant: 4, run: 2, runs: 2 }),
      ]),
    ).toEqual([
      { id: suite, external: false, run: "1 OF 2" },
      { id: twin, external: true, run: "1 OF 2" },
      { id: suite, external: false, run: "2 OF 2" },
      { id: twin, external: true, run: "2 OF 2" },
    ]);
  });

  it("keeps a lone external suite's second run on its own item", () => {
    // --declared marks it external from the source; it has no twin.
    expect(
      itemIdOf("p", null, {
        name: "TestLoneTestSuite",
        external: true,
        variant: 2,
        run: 2,
      }),
    ).toBe("p/LoneTestSuite");
  });

  it("reads an older CLI's document, without run or runs, as before", () => {
    expect(
      shown([suiteNode({}), suiteNode({ external: true, variant: 2 })]),
    ).toEqual([
      { id: suite, external: false, run: undefined },
      { id: twin, external: true, run: undefined },
    ]);
  });
});

describe("durations, as the CLI's spec shows them", () => {
  // The row of the node displayed as name: its own line, not its children.
  const rowOf = (html: string, name: string) => {
    const row = new RegExp(
      `data-display="${name}"[\\s\\S]*?(?:</summary>|</div>)`,
    ).exec(html);
    expect(row).not.toBeNull();
    return row![0];
  };
  const durationOf = (html: string, name: string) =>
    /class="dur">\(([^)]+)\)/.exec(rowOf(html, name))?.[1];
  const tree = (seconds: number, status = "pass") => {
    const pkg: SpecPackage = {
      path: "example.com/cart",
      status: "pass",
      duration: 0,
      nodes: [
        node(
          "TestCartTestSuite",
          status,
          [
            node(
              "TestAdd",
              status,
              [
                // The CLI gives every When and It the kind "block".
                node("when_empty", status, [node("it_adds", status)], {
                  kind: "block",
                  duration: seconds,
                }),
                node("it_counts", status, [], { duration: seconds }),
              ],
              { kind: "method", duration: seconds },
            ),
            node("TestTotal", status, [], {
              kind: "method",
              duration: seconds,
            }),
          ],
          { duration: seconds },
        ),
        node("TestPlain", status, [], { kind: "test", duration: seconds }),
        node("FuzzParse", status, [node("seed#0", status)], {
          kind: "fuzz",
          duration: seconds,
        }),
      ],
    };
    return body([pkg]);
  };

  it("shows a suite's, a method's, a test's and a fuzz target's time at any length", () => {
    for (const [seconds, shown] of [
      [0.128, "128ms"],
      [1.2, "1.2s"],
    ] as const) {
      const html = tree(seconds);
      for (const name of [
        "TestCartTestSuite",
        "TestAdd",
        "TestTotal",
        "TestPlain",
        "FuzzParse",
      ]) {
        expect([name, durationOf(html, name)]).toEqual([name, shown]);
      }
    }
  });

  it("shows a When's and a behavior's time from one second up", () => {
    const slow = tree(1.2);
    expect(durationOf(slow, "when_empty")).toBe("1.2s");
    expect(durationOf(slow, "it_counts")).toBe("1.2s");
    const fast = tree(0.128);
    expect(durationOf(fast, "when_empty")).toBeUndefined();
    expect(durationOf(fast, "it_counts")).toBeUndefined();
  });

  it("shows no time on a row without a verdict, which says NOT RUN", () => {
    const html = tree(1.2, "none");
    for (const name of ["TestTotal", "it_counts", "TestAdd", "when_empty"]) {
      expect([name, durationOf(html, name)]).toEqual([name, undefined]);
    }
    expect(rowOf(html, "it_counts")).toContain("NOT RUN");
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

describe("linkSources", () => {
  it("links the file:line tokens of failure output to files that are there", () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-links-"));
    try {
      writeFileSync(path.join(dir, "store_test.go"), "package store\n");
      const file = path.join(dir, "store_test.go");

      // As go test names it: relative to the package.
      expect(linkSources("    store_test.go:40: boom", dir)).toBe(
        `    <a class="src-link" data-loc-file="${file}" data-loc-line="40">store_test.go:40</a>: boom`,
      );
      // As the compiler names it: relative to where it ran.
      expect(
        linkSources("store/store_test.go:13:22: undefined: x", dir),
      ).toContain(`data-loc-file="${file}" data-loc-line="13"`);
      // A stack frame names the file absolutely.
      expect(linkSources(`\t${file}:7 +0x1d`, undefined)).toContain(
        `data-loc-line="7"`,
      );
      // Windows names a temp dir by a short name, tilde included.
      const short = path.join(dir, "RUNNER~1");
      mkdirSync(short);
      writeFileSync(path.join(short, "short_test.go"), "package short\n");
      expect(
        linkSources(
          `\t${path.join(short, "short_test.go")}:5 +0x1d`,
          undefined,
        ),
      ).toContain(
        `data-loc-file="${path.join(short, "short_test.go")}" data-loc-line="5"`,
      );
      // Nothing to open, nothing to click.
      expect(linkSources("    gone_test.go:3: boom", dir)).toBe(
        "    gone_test.go:3: boom",
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
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

describe("renderErrorHtml", () => {
  it("is empty when the last render worked", () => {
    expect(renderErrorHtml(undefined)).toBe("");
  });
  it("says the view is out of date, with the reason's first line escaped and a retry button", () => {
    const html = renderErrorHtml("spec render timed out after 30s\n<stack>");
    expect(html).toContain("This view is out of date: the last render failed.");
    expect(html).toContain("spec render timed out after 30s");
    expect(html).not.toContain("<stack>");
    expect(html).toContain('id="retry-render-btn"');
    expect(renderErrorHtml("a <b> c")).toContain("a &lt;b&gt; c");
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

describe("when each package last ran", () => {
  const pkgOf = (p: string): SpecPackage => ({
    path: p,
    status: "pass",
    duration: 0.1,
    nodes: [node("TestCartTestSuite", "pass", [node("adds", "pass")])],
  });
  const twoPackages = {
    packages: [pkgOf("example.com/cart"), pkgOf("example.com/order")],
    stats: stats(),
  };
  const onePackage = { packages: [pkgOf("example.com/cart")], stats: stats() };

  it("shows on each package when it last ran, and nothing where it is not known", () => {
    const now = new Date(2026, 10, 11, 16, 5).getTime();
    const at = new Date(2026, 10, 11, 14, 32).getTime();
    const html = buildSpecBody(twoPackages, [], {
      dirOf: () => undefined,
      ranAtOf: (p) => (p === "example.com/cart" ? at : undefined),
      now,
    });
    expect(html.match(/class="ran-at"/g)).toHaveLength(1);
    expect(html).toContain(">last run 14:32</span>");
  });

  it("shows it above the rows when one package is all there is", () => {
    const at = new Date(2026, 10, 11, 14, 32).getTime();
    const html = buildSpecBody(onePackage, [], {
      dirOf: () => undefined,
      ranAtOf: () => at,
      now: new Date(2026, 10, 11, 16, 5).getTime(),
    });
    expect(html).toContain('<div class="ran-at-line">last run 14:32</div>');
  });

  it("shows no time when none is known", () => {
    const html = buildSpecBody(twoPackages, [], { dirOf: () => undefined });
    expect(html).not.toContain("last run");
  });
});

describe("the facts a node carries", () => {
  const pkg = (nodes: Node[]): SpecPackage => ({
    path: "example.com/p",
    status: "pass",
    duration: 0,
    nodes,
  });

  it("marks a vacuous leaf, and one that asserts by other means", () => {
    const html = body([
      pkg([
        node("S", "pass", [
          node("a", "pass", [], { vacuous: true, assertions: 0 }),
          node("b", "pass", [], { assertsBy: "golden file" }),
          node("c", "pass", [], { assertions: 3 }),
        ]),
      ]),
    ]);
    expect(html).toContain("VACUOUS: passed without an assertion");
    expect(html).toContain("ASSERTS BY: golden file");
    expect(html).toContain("3 assertions");
    expect(html).not.toContain("0 assertions");
  });

  it("gives a skip its reason", () => {
    const html = body([
      pkg([
        node(
          "S",
          "skip",
          [node("a", "skip", [], { skipReason: "needs docker" })],
          { skipReason: "guarded" },
        ),
      ]),
    ]);
    expect(html).toContain("— SKIPPED: needs docker");
    expect(html).toContain("SKIPPED: guarded</span>");
  });

  it("marks a row whose children are a floor", () => {
    const html = body([
      pkg([node("S", "none", [node("M", "none", [], { incomplete: true })])]),
    ]);
    expect(html).toContain("INCOMPLETE: behaviors known only at run time");
  });

  it("counts vacuous and other-means leaves as the CLI's trailer does", () => {
    expect(statsText(stats({ passed: 2, vacuous: 1, assertsBy: 1 }))).toBe(
      "2 passed, 1 vacuous, 1 asserting by other means",
    );
    const html = buildSpecBody(
      { packages: [], stats: stats({ vacuous: 2, excluded: 1 }) },
      [],
      { dirOf: () => undefined },
    );
    expect(html).toContain("2 vacuous");
    expect(html).toContain("1 excluded");
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
    const html = buildSpecBody(
      { packages: [], stats: stats(), run: { commit: "abc1234", exitCode: 0 } },
      [],
      { dirOf: () => undefined },
    );
    expect(html).toContain("Run: abc1234 · exit 0");
  });

  it("words a group as the CLI's summary does", () => {
    expect(groupText({ name: "e2e", suites: 1, wall: 72 })).toBe(
      "group e2e: 1 suite, 72.0s, run apart",
    );
    const html = buildSpecBody(
      {
        packages: [],
        stats: stats(),
        groups: [{ name: "db", suites: 2, wall: 0.5 }],
      },
      [],
      { dirOf: () => undefined },
    );
    expect(html).toContain("group db: 2 suites, 500ms, run apart");
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

  it("says a package of a folder nothing ran in is declared, not run yet", () => {
    const html = buildSpecBody(
      {
        packages: [
          {
            path: "a",
            status: "none",
            duration: 0,
            nodes: [node("S", "none")],
          },
          {
            path: "b",
            status: "none",
            duration: 0,
            nodes: [node("T", "none")],
          },
        ],
        stats: stats(),
      },
      [],
      { dirOf: () => undefined, neverRan: (p) => p === "a" },
    );
    expect(html.match(/declared, not run yet/g)).toHaveLength(1);
  });
});

describe("scopes", () => {
  it("switches between Spec and Change, the current one selected", () => {
    const html = buildScopeTabs("change");
    expect(html).toContain('data-scope="spec" aria-selected="false"');
    expect(html).toContain('data-scope="change" aria-selected="true"');
  });

  it("restores the scope, its folder and the spec, and an older panel's spec", () => {
    const spec = { packages: [], stats: stats() };
    expect(parseStoredState(spec)).toEqual({ spec });
    expect(
      parseStoredState({ scope: "change", folder: "/ws", spec: null }),
    ).toEqual({ scope: "change", folder: "/ws", spec: undefined });
    expect(parseStoredState({ scope: "runs" }).scope).toBeUndefined();
    expect(parseStoredState(null)).toEqual({});
  });
});
