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
} from "./specView.js";
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
  it("names a node by the id the Test Explorer gives it", () => {
    expect(itemIdOf("example.com/cart", "TestCartTestSuite")).toBe(
      "example.com/cart/CartTestSuite",
    );
    expect(
      itemIdOf("example.com/cart", "TestCartTestSuite/TestAdd/when_empty"),
    ).toBe("example.com/cart/CartTestSuite/TestAdd/when_empty");
    expect(itemIdOf("example.com/cart", "TestPlain")).toBe(
      "example.com/cart/Plain",
    );
  });

  it("names no item for a fixture node", () => {
    expect(itemIdOf("example.com/cart", "Test_DBFixture")).toBeUndefined();
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
