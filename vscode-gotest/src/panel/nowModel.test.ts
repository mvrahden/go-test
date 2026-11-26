import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({}));
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import {
  buildNowModel,
  CARD_LIMIT,
  failedIds,
  failureOf,
  fullOutput,
  nodeAt,
  packageTree,
  shortNames,
  shortDuration,
  type NowEnv,
} from "./nowModel.js";
import {
  nearestItemId,
  sourceSegments,
  type SpecData,
  type SpecNode,
  type SpecPackage,
  type SpecStats,
} from "./specData.js";
import {
  MESSAGE_BUDGET,
  type FilterStatus,
  type TreeNode,
} from "./protocol.js";
import { LiveRuns } from "../liveProgress.js";
import { clientHelpers } from "./client/helpers.js";
import { renderNow } from "./client/renderNow.js";

const NOW = renderNow(clientHelpers());

function node(
  name: string,
  status: string,
  children: SpecNode[] = [],
  extra: Partial<SpecNode> = {},
): SpecNode {
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

function pkg(
  p: string,
  nodes: SpecNode[],
  extra: Partial<SpecPackage> = {},
): SpecPackage {
  return { path: p, status: "pass", duration: 0, nodes, ...extra };
}

const docOf = (
  packages: SpecPackage[],
  s: Partial<SpecStats> = {},
): SpecData => ({ packages, stats: stats(s) });

function env(doc: SpecData | undefined, over: Partial<NowEnv> = {}): NowEnv {
  return {
    rev: 1,
    doc,
    modulePaths: [],
    dirOf: () => undefined,
    freshnessOf: () => undefined,
    ranAtOf: () => undefined,
    neverRan: () => false,
    now: new Date(2026, 10, 11, 16, 5).getTime(),
    sources: [],
    live: [],
    query: "",
    filters: new Set(),
    ...over,
  };
}

const treeOf = (
  doc: SpecData,
  p: string,
  filters: FilterStatus[] = [],
  query = "",
  stale = false,
) =>
  packageTree(doc, p, {
    dirOf: () => undefined,
    query,
    filters: new Set(filters),
    stale,
  })!;

const flat = (nodes: TreeNode[]): TreeNode[] =>
  nodes.flatMap((n) => [n, ...flat(n.c ?? [])]);

describe("a package's rows", () => {
  it("keep the status the CLI gave a branch, deriving none from its children, and open a failed one", () => {
    const doc = docOf([
      pkg("example.com/p", [
        node("TestAsGivenTestSuite", "pass", [node("fails", "fail")]),
        node("TestFailedTestSuite", "fail", [node("fails", "fail")]),
      ]),
    ]);
    const t = treeOf(doc, "example.com/p");
    expect(t.nodes.map((n) => n.s)).toEqual(["pass", "fail"]);
    const html = NOW.tree(t.nodes, undefined, {});
    expect(html).toMatch(
      /<details class="branch" data-id="[^"]+" data-pos="0">/,
    );
    expect(html).toMatch(
      /<details class="branch" data-id="[^"]+" data-pos="1" open>/,
    );
  });

  it("give each row its place among the package's rows, and each branch a copy button", () => {
    const doc = docOf([
      pkg("example.com/p", [
        node("TestAATestSuite", "pass", [node("x", "pass")]),
        node("TestBBTestSuite", "pass", [
          node("TestM", "pass", [node("gets_./", "pass")]),
        ]),
      ]),
    ]);
    const rows = flat(treeOf(doc, "example.com/p").nodes);
    expect(rows.map((r) => r.pos)).toEqual(["0", "0.0", "1", "1.0", "1.0.0"]);
    const html = NOW.tree(treeOf(doc, "example.com/p").nodes, undefined, {});
    expect(html.match(/data-act="copyNode"/g)).toHaveLength(3);
  });

  it("name the item ids above a row, the package first and the nearest last, so a slash in a name cannot mislead", () => {
    const doc = docOf([
      pkg("example.com/p", [
        node("TestSTestSuite", "pass", [
          node("TestM", "pass", [node("gets_./", "pass")]),
        ]),
      ]),
    ]);
    const at = nodeAt(doc, "example.com/p", [0, 0, 0])!;
    expect(at.id).toBe("example.com/p/STestSuite/TestM/gets_./");
    expect(at.ancestors).toEqual([
      "example.com/p",
      "example.com/p/STestSuite",
      "example.com/p/STestSuite/TestM",
    ]);
    const tree = new Set([
      "example.com/p",
      "example.com/p/STestSuite/TestM/gets_.",
    ]);
    expect(nearestItemId(at.id!, (i) => tree.has(i), at.ancestors)).toBe(
      "example.com/p",
    );
  });

  it("mark a row the run did not report NOT RUN, with no time", () => {
    const doc = docOf([
      pkg(
        "example.com/p",
        [
          node("TestLeftTestSuite", "none", [
            node("waits", "none", [], { duration: 3 }),
          ]),
        ],
        { status: "none" },
      ),
    ]);
    const [leaf] = treeOf(doc, "example.com/p").nodes[0].c!;
    expect(leaf.tags).toEqual([{ text: "NOT RUN" }]);
    expect(leaf.t).toBeUndefined();
    expect(NOW.tree([leaf], undefined, {})).toContain(
      '<span class="sr">not run</span>',
    );
  });

  it("link a row to the source the CLI names, and no row without a position", () => {
    const doc = docOf([
      pkg(
        "example.com/p",
        [
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
        { dir: "/src/p" },
      ),
    ]);
    const rows = flat(
      packageTree(doc, "example.com/p", {
        dirOf: () => "/elsewhere",
        query: "",
        filters: new Set(),
        stale: false,
      })!.nodes,
    );
    const file = path.join("/src/p", "p_test.go");
    expect(rows.map((r) => r.loc)).toEqual([[file, 3], [file, 12], undefined]);
  });

  it("show a package's own failure when the CLI counts it, and only then", () => {
    const failing = (failedPackages: number, output: string[]) =>
      docOf([
        pkg("example.com/p", [], {
          status: "fail",
          output,
          stats: stats({ failedPackages }),
        }),
      ]);
    const out = ["# example.com/p", "p.go:3:1: syntax error"];
    expect(
      treeOf(failing(1, out), "example.com/p").failure?.flat().join(""),
    ).toContain("syntax error");
    expect(treeOf(failing(0, out), "example.com/p").failure).toBeUndefined();
    // Under --hide=fail the CLI gives it no output.
    expect(treeOf(failing(1, []), "example.com/p").failure).toBeUndefined();
  });

  it("name an external twin's items, its method by the name go test reported", () => {
    const doc = docOf([
      pkg("example.com/twins", [
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
          {
            external: true,
            variant: 2,
          },
        ),
      ]),
    ]);
    const twin = "example.com/twins/TwinsTestSuite~external";
    expect(
      flat(treeOf(doc, "example.com/twins").nodes).map((r) => r.id),
    ).toEqual([
      "example.com/twins/TwinsTestSuite",
      "example.com/twins/TwinsTestSuite/TestCreate",
      twin,
      `${twin}/TestCreate#01`,
      `${twin}/TestCreate#01/when_empty`,
    ]);
    const at = nodeAt(doc, "example.com/twins", [1, 0, 0])!;
    const tree = new Set(["example.com/twins", twin, `${twin}/TestCreate#01`]);
    expect(nearestItemId(at.id!, (i) => tree.has(i), at.ancestors)).toBe(
      `${twin}/TestCreate#01`,
    );
  });

  it("say which run of a repeated suite a row is, and nothing for an older CLI's document", () => {
    const suite = (extra: Partial<SpecNode>) =>
      node("TestCartTestSuite", "pass", [node("TestAdd", "pass")], extra);
    const tags = (nodes: SpecNode[]) =>
      treeOf(
        docOf([pkg("example.com/cart", nodes)]),
        "example.com/cart",
      ).nodes.map((n) => ({
        id: n.id,
        tags: (n.tags ?? []).map((t) => t.text).join(" "),
      }));
    const id = "example.com/cart/CartTestSuite";
    expect(
      tags([
        suite({ run: 1, runs: 2 }),
        suite({ external: true, variant: 2, run: 1, runs: 2 }),
      ]),
    ).toEqual([
      { id, tags: "RUN 1 OF 2" },
      { id: `${id}~external`, tags: "EXT. RUN 1 OF 2" },
    ]);
    expect(tags([suite({}), suite({ external: true, variant: 2 })])).toEqual([
      { id, tags: "" },
      { id: `${id}~external`, tags: "EXT." },
    ]);
  });

  it("show a suite's, method's, test's and fuzz target's time at any length, a block's from one second", () => {
    const tree = (seconds: number, status = "pass") =>
      flat(
        treeOf(
          docOf([
            pkg("example.com/cart", [
              node(
                "TestCartTestSuite",
                status,
                [
                  node(
                    "TestAdd",
                    status,
                    [
                      node("when_empty", status, [node("it_adds", status)], {
                        kind: "block",
                        duration: seconds,
                      }),
                    ],
                    {
                      kind: "method",
                      duration: seconds,
                    },
                  ),
                ],
                { duration: seconds },
              ),
              node("TestPlain", status, [], {
                kind: "test",
                duration: seconds,
              }),
              node("FuzzParse", status, [node("seed#0", status)], {
                kind: "fuzz",
                duration: seconds,
              }),
            ]),
          ]),
          "example.com/cart",
        ).nodes,
      );
    const t = (rows: TreeNode[], d: string) => rows.find((r) => r.d === d)?.t;
    const fast = tree(0.128);
    expect(
      ["TestCartTestSuite", "TestAdd", "TestPlain", "FuzzParse"].map((d) =>
        t(fast, d),
      ),
    ).toEqual(["128ms", "128ms", "128ms", "128ms"]);
    expect(t(fast, "when_empty")).toBeUndefined();
    expect(t(tree(1.2), "when_empty")).toBe("1.2s");
    expect(t(tree(1.2, "none"), "TestAdd")).toBeUndefined();
  });

  it("carry the facts a row states: vacuous, the means it asserts by, assertions, a skip's reason, a floor", () => {
    const doc = docOf([
      pkg("example.com/p", [
        node(
          "S",
          "skip",
          [
            node("a", "pass", [], { vacuous: true, assertions: 0 }),
            node("b", "pass", [], { assertsBy: "golden file" }),
            node("c", "pass", [], { assertions: 3 }),
            node("d", "skip", [], { skipReason: "needs docker" }),
            node("M", "none", [node("e", "none")], { incomplete: true }),
          ],
          { skipReason: "guarded" },
        ),
      ]),
    ]);
    const text = flat(treeOf(doc, "example.com/p").nodes).flatMap((r) =>
      (r.tags ?? []).map((t) => t.text),
    );
    expect(text).toEqual(
      expect.arrayContaining([
        "SKIPPED: guarded",
        "VACUOUS: passed without an assertion",
        "ASSERTS BY: golden file",
        "3 assertions",
        "SKIPPED: needs docker",
        "INCOMPLETE: behaviors known only at run time",
      ]),
    );
    expect(text).not.toContain("0 assertions");
  });

  it("keep only what a search and the chips leave, and the branches above it", () => {
    const doc = docOf([
      pkg("example.com/p", [
        node("TestCartTestSuite", "fail", [
          node("adds an item", "pass"),
          node("removes an item", "fail"),
          node("counts", "pass", [], { vacuous: true }),
        ]),
      ]),
    ]);
    const leaves = (f: FilterStatus[], q = "") =>
      flat(treeOf(doc, "example.com/p", f, q).nodes)
        .filter((r) => !r.c)
        .map((r) => r.d);
    expect(leaves([])).toEqual(["adds an item", "removes an item", "counts"]);
    expect(leaves(["fail"])).toEqual(["removes an item"]);
    expect(leaves(["vacuous", "fail"])).toEqual(["removes an item", "counts"]);
    expect(leaves([], "ITEM")).toEqual(["adds an item", "removes an item"]);
    expect(leaves(["pass"], "remove")).toEqual([]);
    expect(
      treeOf(doc, "example.com/p", ["stale"], "", true).nodes,
    ).toHaveLength(1);
  });

  it("escapes markup from names and output", () => {
    const doc = docOf([
      pkg("example.com/p", [
        node("TestXTestSuite", "fail", [
          node("<b>bold</b>", "fail", [], {
            output: ["    x_test.go:1: <script>alert(1)</script>\n"],
          }),
        ]),
      ]),
    ]);
    const html = NOW.tree(treeOf(doc, "example.com/p").nodes, undefined, {});
    expect(html).toContain("&lt;b&gt;bold&lt;/b&gt;");
    expect(html).toContain("&lt;script&gt;alert(1)&lt;/script&gt;");
    expect(html).not.toContain("<script>");
  });
});

describe("sourceSegments", () => {
  it("links the file:line tokens of output to files that are there", () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-links-"));
    try {
      writeFileSync(path.join(dir, "store_test.go"), "package store\n");
      const file = path.join(dir, "store_test.go");
      // As go test names it: relative to the package.
      expect(sourceSegments("    store_test.go:40: boom", dir)).toEqual([
        "    ",
        ["store_test.go:40", file, 40],
        ": boom",
      ]);
      // As the compiler names it: relative to where it ran.
      expect(
        sourceSegments("store/store_test.go:13:22: undefined: x", dir)[0],
      ).toEqual(["store/store_test.go:13:22", file, 13]);
      // A stack frame names the file absolutely.
      expect(sourceSegments(`\t${file}:7 +0x1d`, undefined)[1]).toEqual([
        `${file}:7`,
        file,
        7,
      ]);
      // Windows names a temp dir by a short name, tilde included.
      const short = path.join(dir, "RUNNER~1");
      mkdirSync(short);
      writeFileSync(path.join(short, "short_test.go"), "package short\n");
      const sf = path.join(short, "short_test.go");
      expect(sourceSegments(`\t${sf}:5 +0x1d`, undefined)[1]).toEqual([
        `${sf}:5`,
        sf,
        5,
      ]);
      // Nothing to open, nothing to click.
      expect(sourceSegments("    gone_test.go:3: boom", dir)).toEqual([
        "    gone_test.go:3: boom",
      ]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("the Now summary", () => {
  const failing = () =>
    docOf(
      [
        pkg(
          "example.com/m/run",
          [
            node(
              "TestLinkTestSuite",
              "fail",
              [
                node(
                  "TestReader",
                  "fail",
                  [
                    node("reads_once", "fail", [], {
                      display: "reads once",
                      file: "l_test.go",
                      line: 9,
                      output: [
                        "=== RUN   TestLinkTestSuite/TestReader/reads_once\n",
                        "    l_test.go:10: Equal failed:\n",
                        "        expected: 1\n",
                        "        actual:   0\n",
                        "        message: goroutine 30 [running]:\n",
                        "--- FAIL: TestLinkTestSuite/TestReader/reads_once (0.00s)\n",
                      ],
                    }),
                    node("closes", "pass"),
                  ],
                  { kind: "method", display: "Reader" },
                ),
              ],
              { display: "Link" },
            ),
          ],
          {
            status: "fail",
            dir: "/src/run",
            inputHash: "h1",
            stats: stats({ behaviors: 2, passed: 1, failed: 1 }),
            duration: 11,
          },
        ),
        pkg(
          "example.com/m/big",
          [node("TestBigTestSuite", "pass", [node("ok", "pass")])],
          { stats: stats({ behaviors: 900, passed: 900 }), duration: 99 },
        ),
        pkg(
          "example.com/m/small",
          [node("TestSmallTestSuite", "pass", [node("ok", "pass")])],
          { stats: stats({ behaviors: 3, passed: 3 }), duration: 0.4 },
        ),
        pkg("example.com/m/nosuites", [], { status: "skip" }),
      ],
      { behaviors: 905, suites: 3, passed: 904, failed: 1 },
    );

  it("opens on a card per failed behavior: where, name, message, the line it names and its source", () => {
    const m = buildNowModel(
      env(failing(), {
        modulePaths: ["example.com/m"],
        flakyOf: (p, t, h) => `${p} ${t} ${h}`,
      }),
    );
    expect(m.cards).toHaveLength(1);
    const [c] = m.cards;
    expect(c).toMatchObject({
      kind: "failed",
      where: "run › Link › Reader",
      name: "reads once",
      loc: { file: path.join("/src/run", "l_test.go"), line: 9 },
      id: "example.com/m/run/LinkTestSuite/TestReader/reads_once",
      pos: [0, 0, 0],
      flaky: "example.com/m/run TestLinkTestSuite/TestReader/reads_once h1",
    });
    expect(c.message!.map((l) => l.join(""))).toEqual([
      "Equal failed:",
      "expected: 1",
      "actual:   0",
    ]);
    expect(c.at!.join("")).toBe("l_test.go:10");
    expect(m.failed).toBe(1);
  });

  it("lists every package failed first, then by behaviors, with the CLI's counts and time", () => {
    const m = buildNowModel(env(failing(), { modulePaths: ["example.com/m"] }));
    expect(m.packages.map((r) => [r.name, r.counts, r.tone, r.time])).toEqual([
      ["run", "1 failed · 1 passed", "fail", "11s"],
      ["big", "900 passed", "pass", "1m 39s"],
      ["small", "3 passed", "pass", "<1s"],
    ]);
    expect(m.noSuites).toEqual({
      text: "1 package has no suites",
      names: ["nosuites"],
    });
    expect(m.total).toBe("905 in 3 suites, 4 packages");
    expect(m.chips.map((c) => [c.label, c.count])).toEqual([
      ["Failed", 1],
      ["Stale", 0],
      ["Vacuous", 0],
      ["Not run", 0],
      ["Passed", 904],
    ]);
  });

  it("marks a stale package's row and gives it a card; an unknown one only a note", () => {
    const fresh: Record<string, "stale" | "current" | "unknown"> = {
      "example.com/m/big": "stale",
      "example.com/m/small": "unknown",
      "example.com/m/run": "current",
    };
    const m = buildNowModel(
      env(failing(), {
        modulePaths: ["example.com/m"],
        freshnessOf: (p) => fresh[p],
      }),
    );
    const big = m.packages.find((r) => r.name === "big")!;
    expect(big).toMatchObject({ stale: true, tone: "warn", note: "stale" });
    expect(m.packages.find((r) => r.name === "small")!.note).toBe(
      "freshness unknown",
    );
    expect(m.cards.map((c) => c.kind)).toEqual(["failed", "stale"]);
    expect(m.context.fresh).toEqual({
      text: "1 of 2 packages stale: their inputs changed since the run",
      tone: "warn",
    });
    expect(
      buildNowModel(env(failing(), { freshnessOf: () => "current" })).context
        .fresh,
    ).toEqual({ text: "verdicts match this tree", tone: "pass" });
  });

  it("says when each package last ran in its row's tooltip, nothing when unknown, and a never-ran folder's packages as declared", () => {
    const at = new Date(2026, 10, 11, 14, 32).getTime();
    const m = buildNowModel(
      env(failing(), {
        ranAtOf: (p) => (p === "example.com/m/big" ? at : undefined),
        neverRan: (p) => p === "example.com/m/small",
      }),
    );
    expect(m.packages.map((r) => r.title).filter(Boolean)).toEqual([
      "last run 14:32",
    ]);
    expect(m.packages.find((r) => r.pkg === "example.com/m/small")!.note).toBe(
      "declared, not run yet",
    );
    expect(m.context.lastRun).toBe("Last run 14:32, from a terminal.");
  });

  it("words the run on show: who started it and how --changed narrowed it", () => {
    const doc = {
      ...failing(),
      run: {
        args: ["--control=stdin", "--changed"],
        exitCode: 1,
        commit: "abc",
        changed: { ran: 95, unchanged: 0, noRecord: true },
      },
    };
    const m = buildNowModel(env(doc));
    expect(m.context.commit).toBe("abc");
    expect(m.context.lastRun).toBe(
      "Last run, from this editor. No earlier record, so all 95 packages ran.",
    );
    const agent = buildNowModel(
      env({
        ...failing(),
        run: {
          agent: "claude-code_2-1_agent",
          changed: { ran: 2, unchanged: 7 },
        },
      }),
    );
    expect(agent.context.lastRun).toBe(
      "Last run, by agent Claude Code. Run changed ran 2 packages and left out 7 unchanged. It did not finish.",
    );
  });

  it("counts a search's and the chips' matches per package", () => {
    const m = buildNowModel(
      env(failing(), { query: "reads", filters: new Set(["fail"]) }),
    );
    expect(m.filtering).toBe(true);
    expect(m.packages.map((r) => r.matches)).toEqual([1, 0, 0]);
  });

  it("counts the cards it leaves out", () => {
    const leaves = Array.from({ length: CARD_LIMIT.vacuous + 3 }, (_, i) =>
      node(`v${i}`, "pass", [], { vacuous: true }),
    );
    const m = buildNowModel(
      env(
        docOf([pkg("p", [node("TestVTestSuite", "pass", leaves)])], {
          vacuous: leaves.length,
        }),
      ),
    );
    expect(m.cards).toHaveLength(CARD_LIMIT.vacuous);
    expect(m.moreCards).toBe(
      "And 3 more vacuous; the chips narrow the rows to them.",
    );
  });

  it("says nothing has run yet when there is no document", () => {
    expect(buildNowModel(env(undefined)).empty?.title).toBe(
      "Nothing has run yet",
    );
  });

  it("shows a run in progress: its packages' counts so far and its failures at once", () => {
    const live = new LiveRuns();
    const run = live.start("run: 3 items");
    const ev = (e: object) =>
      run.note({ Time: "", Package: "example.com/m/run", ...e } as never);
    ev({ Action: "run", Test: "TestLinkTestSuite" });
    ev({ Action: "run", Test: "TestLinkTestSuite/TestReader" });
    ev({ Action: "run", Test: "TestLinkTestSuite/TestReader/reads_once" });
    ev({ Action: "pass", Test: "TestLinkTestSuite/TestReader/reads_once" });
    ev({ Action: "run", Test: "TestLinkTestSuite/TestReader/closes" });
    ev({
      Action: "output",
      Test: "TestLinkTestSuite/TestReader/closes",
      Output: "    l_test.go:20: boom\n",
    });
    ev({ Action: "fail", Test: "TestLinkTestSuite/TestReader/closes" });
    const m = buildNowModel(
      env(failing(), {
        modulePaths: ["example.com/m"],
        live: [{ line: "Running", packages: run.packages }],
      }),
    );
    const row = m.packages.find((r) => r.name === "run")!;
    expect(row).toMatchObject({
      counts: "running · 1 failed · 1 passed",
      tone: "fail",
      live: true,
    });
    // The run passed reads_once again: its old card goes; closes failed now.
    expect(m.cards.map((c) => [c.name, c.live])).toEqual([["closes", true]]);
    expect(m.cards[0].where).toBe("run › Link › Reader");
    expect(m.cards[0].id).toBe(
      "example.com/m/run/LinkTestSuite/TestReader/closes",
    );
    expect(m.context.live).toEqual(["Running"]);
    expect(m.failed).toBe(2);
  });

  it("stays within the message budget on a large document", () => {
    const packages = Array.from({ length: 76 }, (_, p) =>
      pkg(
        `example.com/mod/internal/package${p}`,
        Array.from({ length: 10 }, (_, s) =>
          node(
            `TestSuite${s}TestSuite`,
            "pass",
            Array.from({ length: 15 }, (_, b) =>
              node(
                `behavior_${b}_with_a_reasonably_long_name_like_the_real_ones`,
                b === 0 && p < 40 ? "fail" : "pass",
                [],
                {
                  output:
                    b === 0 && p < 40
                      ? Array.from(
                          { length: 40 },
                          (_, l) =>
                            `    x_test.go:${l}: line ${l} of a long failure\n`,
                        )
                      : [],
                  file: "x_test.go",
                  line: b,
                },
              ),
            ),
          ),
        ),
        {
          stats: stats({ behaviors: 150, passed: 149, failed: 1 }),
          duration: 12,
        },
      ),
    );
    const doc = docOf(packages, { behaviors: 76 * 150, suites: 760 });
    const m = buildNowModel(env(doc, { modulePaths: ["example.com/mod"] }));
    const size = JSON.stringify({ type: "now", model: m }).length;
    expect(size).toBeLessThan(MESSAGE_BUDGET);
    const one = JSON.stringify({
      type: "package",
      ...treeOf(doc, packages[0].path),
    }).length;
    expect(one).toBeLessThan(MESSAGE_BUDGET);
  });
});

describe("what a card or a row asks for", () => {
  const doc = docOf([
    pkg(
      "p",
      [
        node("TestATestSuite", "fail", [
          node("x", "fail", [], {
            output: [
              "=== RUN x\n",
              ...Array.from({ length: 2100 }, (_, i) => `line ${i}\n`),
            ],
          }),
        ]),
      ],
      {
        output: ["pkg out\n"],
      },
    ),
  ]);

  it("gives a row's whole output on request, up to its limit", () => {
    const out = fullOutput(doc, "p", [0, 0], () => undefined)!;
    expect(out.lines[0].join("")).toBe("=== RUN x");
    expect(out.lines).toHaveLength(2000);
    expect(out.more).toBe(101);
    expect(
      fullOutput(doc, "p", [], () => undefined)!.lines.map((l) => l.join("")),
    ).toEqual(["pkg out"]);
    expect(fullOutput(doc, "p", [5], () => undefined)).toBeUndefined();
  });

  it("names the narrowest failed rows for Rerun failed, with the ids above each", () => {
    expect(failedIds(doc)).toEqual({
      ids: ["p/ATestSuite/x"],
      ancestors: { "p/ATestSuite/x": ["p", "p/ATestSuite"] },
    });
  });

  it("reads a failure's message up to its stack", () => {
    const f = failureOf(
      ["x_test.go:3: Equal failed:", "a", "goroutine 4 [running]:", "b"],
      undefined,
    );
    expect(f.message.map((l) => l.join(""))).toEqual(["Equal failed:", "a"]);
    expect(f.more).toBe(true);
    expect(failureOf(["plain"], undefined)).toEqual({
      message: [["plain"]],
      at: undefined,
      more: false,
    });
  });

  it("names packages within their module, and times as a row shows them", () => {
    expect(
      Object.fromEntries(
        shortNames(["m/a", "m/b/c", "m", "m/x/y", "o/z"], ["m", "m/x"]),
      ),
    ).toEqual({
      "m/a": "a",
      "m/b/c": "b/c",
      m: "m",
      "m/x/y": "x/y",
      "o/z": "o/z",
    });
    expect([0, 0.3, 11.4, 99, 600].map(shortDuration)).toEqual([
      "",
      "<1s",
      "11s",
      "1m 39s",
      "10m 00s",
    ]);
  });
});
