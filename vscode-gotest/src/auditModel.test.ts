import { describe, it, expect } from "vitest";
import {
  buildChangeModel,
  FOLD_ABOVE,
  markOf,
  parseAuditDocument,
  runsOf,
  shortPackage,
  sideText,
  targetsOf,
  valueText,
  type AuditDocument,
  type AuditEvidence,
  type AuditFinding,
  type AuditStream,
} from "./auditModel.js";

function finding(over: Partial<AuditFinding> = {}): AuditFinding {
  const subject = over.subject ?? "Cart / Remove";
  return {
    id: `id:${over.class ?? "removed"}:${subject}`,
    class: "removed",
    weakens: true,
    package: "mod/shop",
    subject,
    base: "test",
    head: "",
    module: "mod",
    location: { file: "shop/cart_test.go", line: 32, endLine: 34 },
    baseLocation: { file: "shop/cart_test.go", line: 32, endLine: 34 },
    test: "TestCartTestSuite/TestRemove",
    run: "^TestCartTestSuite$/^TestRemove$",
    ...over,
  };
}

function ev(over: Partial<AuditEvidence> = {}): AuditEvidence {
  return {
    id: `e:${(over.path ?? ["Cart", "Add"]).join("/")}`,
    package: "mod/shop",
    path: ["Cart", "Add"],
    verdict: "pass",
    assertions: 1,
    stream: 0,
    current: true,
    needsDecision: false,
    source: "run.json",
    commit: "bbbb2222",
    ...over,
  };
}

const current: AuditStream = {
  file: "run.json",
  side: "head",
  run: { commit: "bbbb2222", dirty: false, agent: "claude-code", exitCode: 1 },
  current: true,
};

function doc(over: Partial<AuditDocument> = {}): AuditDocument {
  return {
    version: "v1.42.0",
    schema: 1,
    base: { ref: "main", commit: "aaaa1111" },
    head: { commit: "bbbb2222", dirty: true, branch: "feat/x" },
    modules: ["mod"],
    moduleDirs: [{ path: "mod", dir: "." }],
    summary: {
      weakenings: 0,
      informational: 0,
      added: 0,
      removed: 0,
      reworded: 0,
      moved: 0,
      changed: 0,
      evidence: 0,
      unproven: 0,
      incomplete: 0,
    },
    findings: [],
    behaviors: [],
    streams: [],
    evidence: [],
    unproven: [],
    incomplete: [],
    notes: [],
    ...over,
  };
}

describe("parseAuditDocument", () => {
  it("reads a document and takes the lists the CLI left out as empty", () => {
    const d = parseAuditDocument(
      JSON.stringify({
        version: "dev",
        schema: 1,
        base: {},
        head: {},
        modules: ["m"],
        summary: { weakenings: 0 },
        findings: [],
        behaviors: [],
        evidence: null,
      }),
    );
    expect(d?.evidence).toEqual([]);
    expect(d?.streams).toEqual([]);
    expect(d?.moduleDirs).toEqual([]);
    expect(d?.unproven).toEqual([]);
    expect(d?.coverage).toBeUndefined();
    expect(d?.notes).toEqual([]);
  });

  it("reads no document from a run's verdict line or an error", () => {
    expect(parseAuditDocument("gotest: FAIL exit 2")).toBeUndefined();
    expect(parseAuditDocument('{"version":"dev"}')).toBeUndefined();
    expect(parseAuditDocument("")).toBeUndefined();
  });
});

describe("sideText", () => {
  it("names a side as the CLI's text does", () => {
    expect(sideText({ ref: "main", commit: "aaaa1111" })).toBe("main aaaa1111");
    expect(sideText({ commit: "bbbb2222", dirty: true })).toBe(
      "bbbb2222 with uncommitted changes",
    );
    expect(sideText({})).toBe("(no commit)");
  });
});

describe("shortPackage", () => {
  it("is a package's path in its module, the longest module first", () => {
    const d = { modules: ["mod", "mod/sub"] };
    expect(shortPackage(d, "mod")).toBe(".");
    expect(shortPackage(d, "mod/shop")).toBe("./shop");
    expect(shortPackage(d, "mod/sub/x")).toBe("./x");
    expect(shortPackage(d, "other/x")).toBe("other/x");
  });
});

describe("valueText", () => {
  it("adds the counts each side's run reported, ? for a side without one", () => {
    expect(valueText({ base: "2", head: "1" })).toBe("2 → 1");
    expect(
      valueText({ base: "2", head: "1", runtime: { base: 4, head: null } }),
    ).toBe("2 → 1 (run time 4 → ?)");
  });
});

describe("markOf", () => {
  it("writes the CLI text's marks", () => {
    expect(markOf(undefined)).toBe("");
    expect(markOf(ev({ verdict: "fail" }))).toBe("FAIL");
    expect(markOf(ev({ assertions: 0, vacuous: true }))).toBe("vacuous");
    expect(markOf(ev({ assertions: 3 }))).toBe("pass 3");
    const leaves = { passed: 4, failed: 0, skipped: 0, notRun: 0, vacuous: 0 };
    expect(markOf(ev({ assertions: 4, leaves }))).toBe("pass");
    expect(markOf(ev({ leaves: { ...leaves, notRun: 1 } }))).toBe("partly run");
    expect(markOf(ev({ verdict: "not run", assertions: undefined }))).toBe(
      "not run",
    );
    expect(markOf(ev({ verdict: "skip", assertions: undefined }))).toBe("skip");
    expect(
      markOf(
        ev({
          verdict: "no evidence",
          assertions: undefined,
          stream: undefined,
        }),
      ),
    ).toBe("no evidence");
  });

  it("stars a verdict of a stale stream, not one without a stream", () => {
    expect(markOf(ev({ current: false }))).toBe("pass 1*");
    expect(markOf(ev({ verdict: "fail", current: false }))).toBe("FAIL*");
    expect(
      markOf(
        ev({
          verdict: "no evidence",
          assertions: undefined,
          stream: undefined,
          current: false,
        }),
      ),
    ).toBe("no evidence");
  });
});

describe("buildChangeModel", () => {
  it("lists the weakenings in the document's order, and no information among them", () => {
    const m = buildChangeModel(
      doc({
        findings: [
          finding(),
          finding({
            class: "focus",
            subject: "Cart / Total",
            base: "TestTotal",
            head: "F_TestTotal",
            headLocation: { file: "shop/cart_test.go", line: 39, endLine: 41 },
          }),
          finding({ class: "changed", weakens: false, subject: "Cart / Add" }),
        ],
      }),
    );
    expect(m.open.map((g) => g.tag)).toEqual(["removed", "focus"]);
    expect(m.openCount).toBe(2);
    const focus = m.open[1].rows[0];
    expect(focus.key).toBe("id:focus:Cart / Total");
    expect(focus.fact).toBe("TestTotal → F_TestTotal");
    expect(focus.pkg).toBe("./shop");
    expect(focus.labels).toEqual(["Cart", "Total"]);
    expect(focus.where).toBe("shop/cart_test.go:32");
  });

  it("opens a finding on each side it has, a removed one on the base's alone", () => {
    const moved = finding({
      class: "focus",
      subject: "Cart / Total",
      baseLocation: { file: "shop/old_test.go", line: 40, endLine: 42 },
      headLocation: { file: "shop/cart_test.go", line: 39, endLine: 41 },
    });
    const m = buildChangeModel(doc({ findings: [finding(), moved] }));
    const targets = targetsOf(doc({ findings: [finding(), moved] }), m);
    const removed = targets.get(finding().id);
    expect(removed).toEqual({
      module: "mod",
      base: { file: "shop/cart_test.go", line: 32, endLine: 34 },
      head: undefined,
    });
    expect(targets.get(moved.id)).toMatchObject({
      base: { file: "shop/old_test.go", line: 40 },
      head: { file: "shop/cart_test.go", line: 39 },
    });
  });

  it("runs a row's own -run pattern in its package, never one the head no longer has", () => {
    const focus = finding({
      class: "focus",
      subject: "Cart / Total",
      headLocation: { file: "shop/cart_test.go", line: 39, endLine: 41 },
      run: "^TestCartTestSuite$/^F_TestTotal$",
    });
    const m = buildChangeModel(
      doc({
        findings: [finding(), focus],
        behaviors: [
          {
            change: "added",
            kind: "it",
            package: "mod/shop",
            head: {
              path: ["Cart", "Copies", "the only copy"],
              file: "shop/cart_test.go",
              line: 49,
              endLine: 51,
            },
            run: "^TestCartTestSuite$/^TestCopies$/^the_only_copy$",
          },
          {
            change: "removed",
            kind: "it",
            package: "mod/shop",
            base: {
              path: ["Cart", "Remove"],
              file: "shop/cart_test.go",
              line: 32,
              endLine: 34,
            },
            run: "^TestCartTestSuite$/^TestRemove$",
          },
        ],
      }),
    );
    const runs = runsOf(m);
    expect(runs.get(focus.id)).toEqual({
      package: "mod/shop",
      run: "^TestCartTestSuite$/^F_TestTotal$",
    });
    expect(runs.get("b:0")).toEqual({
      package: "mod/shop",
      run: "^TestCartTestSuite$/^TestCopies$/^the_only_copy$",
    });
    expect(runs.has(finding().id)).toBe(false);
    expect(runs.has("b:1")).toBe(false);
  });

  it("puts the evidence rows that need a decision after the findings, tagged by their mark", () => {
    const m = buildChangeModel(
      doc({
        findings: [finding()],
        streams: [current],
        evidence: [
          ev(),
          ev({
            path: ["Cart", "Copies", "the only copy"],
            assertions: 0,
            vacuous: true,
            needsDecision: true,
            why: ["added"],
          }),
          ev({
            path: ["Stock", "Count"],
            verdict: "fail",
            needsDecision: true,
            why: ["changed", "assertions"],
            run: "^TestStockTestSuite$/^TestCount$",
          }),
        ],
      }),
    );
    expect(m.open.map((g) => g.tag)).toEqual(["removed", "vacuous", "FAIL"]);
    const failing = m.open[2].rows[0];
    expect(failing.key).toBe("e:Stock/Count");
    expect(failing.tone).toBe("fail");
    expect(failing.fact).toBe("changed, assertions");
    expect(failing.runs).toEqual({
      package: "mod/shop",
      run: "^TestStockTestSuite$/^TestCount$",
    });
  });

  it("takes a decision from the document, not from the verdict", () => {
    const m = buildChangeModel(
      doc({
        streams: [current],
        evidence: [
          ev({ verdict: "skip", assertions: undefined, needsDecision: false }),
          ev({ path: ["Cart", "Total"], needsDecision: true }),
        ],
      }),
    );
    expect(m.open.flatMap((g) => g.rows.map((r) => r.key))).toEqual([
      "e:Cart/Total",
    ]);
  });

  it("marks weakenings and behaviors with their evidence, as the CLI's text does", () => {
    const budget = finding({
      class: "budget",
      subject: "Cart",
      base: "Timeout 10s",
      head: "Timeout 1m0s",
      headLocation: { file: "shop/cart_test.go", line: 9, endLine: 9 },
    });
    const m = buildChangeModel(
      doc({
        findings: [finding({ subject: "Cart / Add" }), budget],
        streams: [current, { ...current, file: "old.json", current: false }],
        behaviors: [
          {
            change: "changed",
            kind: "it",
            package: "mod/shop",
            base: { path: ["Cart", "Add"], file: "f", line: 2, endLine: 3 },
            head: { path: ["Cart", "Add"], file: "f", line: 2, endLine: 3 },
          },
        ],
        evidence: [
          ev({ assertions: 2 }),
          ev({
            path: ["Cart"],
            verdict: "fail",
            needsDecision: true,
            others: [{ stream: 1, verdict: "pass", current: false }],
          }),
        ],
      }),
    );
    const [removed, weak] = m.open.flatMap((g) => g.rows);
    // A removed behavior has no head to have run.
    expect(removed.mark).toBeUndefined();
    expect(weak.mark?.text).toBe("FAIL");
    expect(weak.mark?.detail).toContain(
      "judged by run.json of bbbb2222\nold.json: pass (stale)",
    );
    expect(m.behaviors[0].suites[0].rows[0].mark?.text).toBe("pass 2");
  });

  it("names the streams as the CLI's text does, a stale one with why", () => {
    const m = buildChangeModel(
      doc({
        streams: [
          current,
          {
            file: "old.json",
            side: "head",
            run: { commit: "0a1b2c3", exitCode: 0 },
            current: false,
            stale: "another commit",
          },
          {
            file: "bare.json",
            side: "head",
            run: null,
            current: false,
            stale: "the stream has no manifest",
          },
          { side: "head", ran: true, run: { exitCode: 0 }, current: true },
          {
            file: "b.json",
            side: "base",
            run: { commit: "9f8e7d6" },
            current: false,
            stale: "x",
          },
        ],
      }),
    );
    expect(m.streamsHead).toBe(
      "evidence: 5 streams, 2 current; * marks a verdict of a stale one",
    );
    expect(m.streams).toEqual([
      {
        current: true,
        text: "run.json of bbbb2222, exit 1",
        agent: "claude-code",
      },
      {
        current: false,
        text: "old.json of 0a1b2c3, exit 0: another commit",
        agent: undefined,
      },
      {
        current: false,
        text: "bare.json: the stream has no manifest",
        agent: undefined,
      },
      { current: true, text: "this audit's run, exit 0", agent: undefined },
      { current: false, text: "base b.json of 9f8e7d6: x", agent: undefined },
    ]);
    expect(buildChangeModel(doc()).streamsHead).toBeUndefined();
  });

  it("moves accepted rows to the bottom and counts both", () => {
    const keep = finding({ subject: "Cart / Keep" });
    const m = buildChangeModel(
      doc({ findings: [finding(), keep] }),
      (k) => k === keep.id,
    );
    expect(m.openCount).toBe(1);
    expect(m.acceptedCount).toBe(1);
    expect(m.accepted[0].labels).toEqual(["Cart", "Keep"]);
    expect(m.open[0].rows.map((r) => r.labels.at(-1))).toEqual(["Remove"]);
  });

  it("folds a class with many rows behind its count", () => {
    const many = Array.from({ length: FOLD_ABOVE + 1 }, (_, i) =>
      finding({
        class: "asserts-by",
        subject: `Cart / T${i}`,
        base: "no AssertsBy",
        head: "AssertsBy",
      }),
    );
    const few = Array.from({ length: FOLD_ABOVE }, (_, i) =>
      finding({ subject: `Cart / R${i}` }),
    );
    const m = buildChangeModel(doc({ findings: [...few, ...many] }));
    expect(m.open.map((g) => [g.tag, g.rows.length, g.folded])).toEqual([
      ["removed", FOLD_ABOVE, false],
      ["asserts-by", FOLD_ABOVE + 1, true],
    ]);
  });

  it("groups the behaviors by package and suite in the document's order", () => {
    const at = (path: string[], line: number) => ({
      path,
      file: "shop/cart_test.go",
      line,
      endLine: line + 2,
    });
    const d = doc({
      modules: ["mod", "mod/other"],
      moduleDirs: [
        { path: "mod", dir: "." },
        { path: "mod/other", dir: "other" },
      ],
      behaviors: [
        {
          change: "removed",
          kind: "it",
          package: "mod/shop",
          base: at(["Cart", "Copies", "first"], 50),
          behaviors: 1,
        },
        {
          change: "added",
          kind: "when",
          package: "mod/shop",
          head: at(["Stock", "Count", "when empty"], 30),
          behaviors: 3,
        },
        {
          change: "reworded",
          kind: "it",
          package: "mod/shop",
          base: at(["Cart", "Add", "keeps the order"], 21),
          head: at(["Cart", "Add", "ignores the order"], 21),
        },
        {
          change: "moved",
          kind: "it",
          package: "mod/other",
          base: at(["Stock", "full", "counts"], 15),
          head: at(["Stock", "refilled", "counts"], 26),
        },
      ],
    });
    const m = buildChangeModel(d);
    expect(
      m.behaviors.map((p) => [p.pkg, p.suites.map((s) => s.suite)]),
    ).toEqual([
      ["./shop", ["Cart", "Stock"]],
      [".", ["Stock"]],
    ]);
    const cart = m.behaviors[0].suites[0].rows;
    expect(cart.map((r) => [r.change, r.text])).toEqual([
      ["removed", "Copies / first"],
      ["reworded", "Add / keeps the order → ignores the order"],
    ]);
    expect(m.behaviors[0].suites[1].rows[0].where).toBe(
      "shop/cart_test.go:30, 3 behaviors",
    );
    expect(m.behaviors[1].suites[0].rows[0].text).toBe(
      "full / counts → refilled / counts",
    );
    // Each row opens its sides, in the module that holds its package.
    const targets = targetsOf(d, m);
    expect(targets.get("b:0")).toMatchObject({
      module: "mod",
      head: undefined,
    });
    expect(targets.get("b:0")?.base?.line).toBe(50);
    expect(targets.get("b:3")).toMatchObject({ module: "mod/other" });
  });

  it("lists information the behaviors do not show already, as the CLI's text does", () => {
    const m = buildChangeModel(
      doc({
        findings: [
          finding({
            class: "suppression",
            weakens: false,
            package: undefined,
            subject: "shop/plain_test.go",
            base: "//nolint:a",
            head: "//nolint:a // why",
            location: { file: "shop/plain_test.go", line: 5, endLine: 5 },
          }),
          finding({ class: "changed", weakens: false }),
        ],
      }),
    );
    expect(m.information).toEqual([
      "suppression  shop/plain_test.go:5 //nolint:a → //nolint:a // why",
    ]);
  });

  it("lists the unproven lines per file with their ranges, each range a place to open", () => {
    const d = doc({
      summary: {
        ...doc().summary,
        unproven: 3,
        changedLines: 7,
        noCoverage: 1,
      },
      coverage: {
        profile: "shop.cover",
        lines: "the diff shop.diff",
        packages: ["mod/shop"],
      },
      unproven: [
        {
          module: "mod",
          file: "shop/ledger/ledger.go",
          package: "mod/shop/ledger",
          line: 0,
          coverage: false,
          changed: 0,
          unexecuted: 0,
          ranges: [],
        },
        {
          module: "mod",
          file: "shop/shop.go",
          package: "mod/shop",
          line: 10,
          coverage: true,
          changed: 7,
          unexecuted: 3,
          ranges: [
            { line: 10, endLine: 11 },
            { line: 15, endLine: 15 },
          ],
        },
      ],
    });
    const m = buildChangeModel(d);
    expect(m.unproven?.head).toBe(
      "3 of 7 changed executable lines not executed, 1 file without coverage data",
    );
    expect(m.unproven?.source).toBe(
      "shop.cover, lines from the diff shop.diff",
    );
    expect(m.unproven?.files).toEqual([
      {
        key: "u:0",
        file: "shop/ledger/ledger.go",
        pkg: "./shop/ledger",
        coverage: false,
        counts: "",
        ranges: [],
      },
      {
        key: "u:1",
        file: "shop/shop.go",
        pkg: "./shop",
        coverage: true,
        counts: "3 of 7",
        ranges: [
          { key: "u:1:0", text: "10-11" },
          { key: "u:1:1", text: "15" },
        ],
      },
    ]);
    const targets = targetsOf(d, m);
    expect(targets.get("u:1:0")).toEqual({
      module: "mod",
      head: { file: "shop/shop.go", line: 10, endLine: 11 },
    });
    expect(targets.get("u:0")?.head?.line).toBe(0);
    // Without a profile the audit looked for none.
    expect(buildChangeModel(doc()).unproven).toBeUndefined();
  });

  it("counts what the summary counts: evidence with streams, unproven lines with a profile", () => {
    const plain = buildChangeModel(
      doc({
        summary: { ...doc().summary, weakenings: 2, added: 3, incomplete: 1 },
      }),
    );
    const byLabel = (m: ReturnType<typeof buildChangeModel>) =>
      Object.fromEntries(m.counters.map((c) => [c.label, c.value]));
    expect(byLabel(plain)).toMatchObject({
      weakenings: 2,
      added: 3,
      "not compared": 1,
    });
    expect(byLabel(plain)).not.toHaveProperty("evidence");
    expect(byLabel(plain)).not.toHaveProperty("unproven");

    const proven = buildChangeModel(
      doc({
        streams: [current],
        coverage: { lines: "git diff", packages: [] },
        summary: {
          ...doc().summary,
          evidence: 11,
          failing: 2,
          notRun: 2,
          vacuous: 1,
          stale: 0,
          unproven: 3,
          changedLines: 7,
          noCoverage: 1,
        },
      }),
    );
    expect(byLabel(proven)).toMatchObject({
      evidence: 11,
      failing: 2,
      "not run": 2,
      vacuous: 1,
      stale: 0,
      unproven: "3 of 7 lines",
      "without coverage": 1,
    });
  });

  it("takes the head's branch from the document", () => {
    expect(buildChangeModel(doc()).headBranch).toBe("feat/x");
  });
});
