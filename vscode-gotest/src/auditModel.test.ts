import { describe, it, expect } from "vitest";
import {
  buildChangeModel,
  evidenceKey,
  findingKey,
  FOLD_ABOVE,
  parseAuditDocument,
  shortPackage,
  sideText,
  targetsOf,
  type AuditDocument,
  type AuditFinding,
} from "./auditModel.js";

function finding(over: Partial<AuditFinding> = {}): AuditFinding {
  return {
    class: "removed",
    weakens: true,
    package: "mod/shop",
    subject: "Cart / Remove",
    base: "test",
    head: "",
    location: { file: "shop/cart_test.go", line: 32, endLine: 34 },
    ...over,
  };
}

function doc(over: Partial<AuditDocument> = {}): AuditDocument {
  return {
    version: "v1.42.0",
    schema: 1,
    base: { ref: "main", commit: "aaaa1111" },
    head: { commit: "bbbb2222", dirty: true },
    modules: ["mod"],
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
    expect(d?.unproven).toEqual([]);
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
          }),
          finding({ class: "changed", weakens: false, subject: "Cart / Add" }),
        ],
      }),
    );
    expect(m.open.map((g) => g.tag)).toEqual(["removed", "focus"]);
    expect(m.openCount).toBe(2);
    const focus = m.open[1].rows[0];
    expect(focus.fact).toBe("TestTotal → F_TestTotal");
    expect(focus.pkg).toBe("./shop");
    expect(focus.labels).toEqual(["Cart", "Total"]);
    expect(focus.where).toBe("shop/cart_test.go:32");
  });

  it("names what a removed behavior was and opens it on the base's side", () => {
    const m = buildChangeModel(doc({ findings: [finding()] }));
    const row = m.open[0].rows[0];
    expect(row.fact).toBe("test");
    expect(row.tone).toBe("fail");
    expect(row.target).toMatchObject({ side: "base", module: "mod", line: 32 });
  });

  it("puts evidence a run did not prove after the findings", () => {
    const m = buildChangeModel(
      doc({
        findings: [finding()],
        evidence: [
          { package: "mod/shop", path: ["Cart", "Add"], verdict: "pass" },
          { package: "mod/shop", path: ["Cart", "Rounds"], verdict: "vacuous" },
        ],
      }),
    );
    expect(m.open.map((g) => g.tag)).toEqual(["removed", "vacuous"]);
    expect(m.evidence).toHaveLength(2);
  });

  it("moves accepted rows to the bottom and counts both", () => {
    const keep = finding({ subject: "Cart / Keep" });
    const m = buildChangeModel(
      doc({ findings: [finding(), keep] }),
      (k) => k === findingKey(keep),
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
    const m = buildChangeModel(
      doc({
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
      }),
    );
    expect(
      m.behaviors.map((p) => [p.pkg, p.suites.map((s) => s.suite)]),
    ).toEqual([
      ["./shop", ["Cart", "Stock"]],
      ["./other", ["Stock"]],
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
    // Each row opens its place; a removed one the base's.
    const targets = targetsOf(m);
    expect(targets.get("b:0")).toMatchObject({ side: "base", line: 50 });
    expect(targets.get("b:1")).toMatchObject({ side: "head", line: 30 });
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

  it("groups the unproven lines by file", () => {
    const m = buildChangeModel(
      doc({
        unproven: [
          { file: "shop/shop.go", line: 3 },
          { file: "shop/shop.go", line: 9 },
          { file: "shop/other.go", line: 1 },
        ],
      }),
    );
    expect(m.unproven).toEqual([
      { file: "shop/shop.go", lines: [3, 9], module: undefined },
      { file: "shop/other.go", lines: [1], module: undefined },
    ]);
  });

  it("counts what the summary counts, and the not compared when there are any", () => {
    const m = buildChangeModel(
      doc({
        summary: { ...doc().summary, weakenings: 2, added: 3, incomplete: 1 },
      }),
    );
    const byLabel = Object.fromEntries(
      m.counters.map((c) => [c.label, c.value]),
    );
    expect(byLabel).toMatchObject({
      weakenings: 2,
      added: 3,
      "not compared": 1,
    });
    expect(byLabel).not.toHaveProperty("unproven");
  });
});

describe("findingKey", () => {
  it("tells findings apart by class, subject and values, not by line", () => {
    const a = finding();
    expect(findingKey(a)).toBe(
      findingKey({ ...a, location: { file: "x", line: 1, endLine: 1 } }),
    );
    expect(findingKey(a)).not.toBe(findingKey({ ...a, head: "X_TestRemove" }));
    expect(findingKey(a)).not.toBe(findingKey({ ...a, class: "excluded" }));
    expect(
      evidenceKey({ package: "p", path: ["a", "b"], verdict: "fail" }),
    ).not.toBe(evidenceKey({ package: "p", path: ["a/b"], verdict: "fail" }));
  });
});
