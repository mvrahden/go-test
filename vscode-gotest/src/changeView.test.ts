import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
  window: {},
  commands: {},
}));

import {
  buildChangeTab,
  changePackageRows,
  hasReason,
  ROUTINE,
  type ChangeView,
} from "./changeView.js";
import { buildChangeModel, type AuditDocument } from "./auditModel.js";
import { interpretAuditExit } from "./changeScope.js";

const removed = {
  id: "f-removed",
  class: "removed",
  weakens: true,
  package: "mod/shop",
  subject: "Cart / Remove",
  base: "test",
  head: "",
  location: { file: "shop/cart_test.go", line: 32, endLine: 34 },
};
const budget = {
  id: "f-budget",
  class: "budget",
  weakens: true,
  package: "mod/shop",
  subject: "Cart",
  base: "Timeout 10s",
  head: "Timeout 1m0s",
  location: { file: "shop/cart_test.go", line: 9, endLine: 9 },
};

function doc(over: Partial<AuditDocument> = {}): AuditDocument {
  return {
    version: "dev",
    schema: 1,
    base: { ref: "main", commit: "aaaa1111" },
    head: { commit: "bbbb2222", branch: "feat/x" },
    modules: ["mod"],
    moduleDirs: [{ path: "mod", dir: "." }],
    summary: {
      weakenings: 2,
      informational: 0,
      added: 1,
      removed: 1,
      reworded: 0,
      moved: 0,
      changed: 0,
      evidence: 0,
      unproven: 0,
      incomplete: 0,
    },
    findings: [removed, budget],
    behaviors: [
      {
        change: "added",
        kind: "it",
        package: "mod/shop",
        module: "mod",
        head: {
          path: ["Cart", "Copies", "the only copy"],
          file: "shop/cart_test.go",
          line: 49,
          endLine: 51,
        },
        behaviors: 1,
        evidence: "e-copy",
        order: 0,
      },
    ],
    streams: [],
    evidence: [],
    unproven: [],
    incomplete: [],
    notes: [],
    ...over,
  };
}

function docState(
  d: AuditDocument,
  exitCode: number | null = 0,
  accepted: (k: string) => boolean = () => false,
) {
  return {
    kind: "doc" as const,
    model: buildChangeModel(d, accepted),
    doc: d,
    exitCode,
  };
}

const finding = (
  id: string,
  cls: string,
  weakens: boolean,
  over: Partial<AuditDocument["findings"][number]> = {},
) => ({
  id,
  class: cls,
  weakens,
  subject: "shop/cart_test.go",
  base: "",
  head: "",
  location: { file: "shop/cart_test.go", line: 4, endLine: 4 },
  ...over,
});

function view(over: Partial<ChangeView> = {}): ChangeView {
  return {
    state: docState(doc()),
    busy: false,
    folders: [{ path: "/ws", label: "ws", selected: true }],
    baseLabel: "merge-base with origin/main (aaaa1111)",
    evidence: [],
    canRun: () => false,
    canOpen: () => false,
    ...over,
  };
}

describe("the Change tab", () => {
  it("opens on how many changes need a decision, and what else the audit found", () => {
    const t = buildChangeTab(view());
    expect(t.headline).toBe("2 changes need your decision");
    expect(t.sub).toBe("1 behaviors added, 1 removed, 0 changed.");
    expect(t.head).toBe("feat/x");
    expect(t.base).toBe("merge-base with origin/main (aaaa1111)");
    expect(t.openCount).toBe(2);
    const one = buildChangeTab(
      view({ state: docState(doc({ findings: [budget] })) }),
    );
    expect(one.headline).toBe("1 change needs your decision");
    expect(
      buildChangeTab(view({ state: docState(doc({ findings: [] })) })).headline,
    ).toBe("No change needs your decision");
  });

  it("gives each decision its kind, subject, fact and place, assertions and budgets before removals", () => {
    const t = buildChangeTab(view());
    expect(t.decisions).toEqual([
      {
        key: "f-budget",
        kind: "Budget",
        tone: "warn",
        what: "Cart",
        fact: "Timeout 10s → Timeout 1m0s",
        where: "shop/cart_test.go:9",
        open: undefined,
        mark: undefined,
        accepted: false,
      },
      {
        key: "f-removed",
        kind: "Removed",
        tone: "fail",
        what: "Cart › Remove",
        fact: "gone",
        where: "shop/cart_test.go:32",
        open: undefined,
        mark: undefined,
        accepted: false,
      },
    ]);
  });

  it("shows the first decisions and counts the rest by kind", () => {
    const many = Array.from({ length: 9 }, (_, i) => ({
      ...removed,
      id: `r${i}`,
    }));
    const t = buildChangeTab(
      view({ state: docState(doc({ findings: [budget, ...many] })) }),
    );
    expect(t.shown).toBe(6);
    expect(t.moreText).toBe("4 more: 4 removed");
  });

  it("offers Diff only where the editor can open the file, and Open for an evidence row", () => {
    expect(buildChangeTab(view()).decisions.map((d) => d.open)).toEqual([
      undefined,
      undefined,
    ]);
    const d = doc({
      streams: [{ file: "run.json", side: "head", run: null, current: true }],
      evidence: [
        {
          id: "e-total",
          package: "mod/shop",
          module: "mod",
          path: ["Cart", "Total"],
          location: { file: "shop/cart_test.go", line: 60, endLine: 62 },
          verdict: "fail",
          mark: "FAIL",
          stream: 0,
          current: true,
          needsDecision: true,
          why: ["changed"],
        },
      ],
    });
    const t = buildChangeTab(
      view({
        state: docState(d, 1),
        canOpen: (k) => k === "e-total" || k === budget.id,
      }),
    );
    expect(t.decisions.map((r) => [r.key, r.open])).toEqual([
      ["e-total", "source"],
      ["f-budget", "diff"],
      ["f-removed", undefined],
    ]);
    expect(t.exit).toBe("exit 1");
  });

  it("moves an accepted decision under the accepted, out of the count", () => {
    const t = buildChangeTab(
      view({ state: docState(doc(), 0, (k) => k === removed.id) }),
    );
    expect(t.openCount).toBe(1);
    expect(t.accepted.map((r) => [r.key, r.accepted])).toEqual([
      [removed.id, true],
    ]);
  });

  it("groups the routine findings by the table, informational ones included, and leaves the rest decisions", () => {
    const findings = [
      finding("a1", "asserts-by", true),
      finding("a2", "asserts-by", true),
      finding("s-reason", "suppression", true, {
        head: "//nolint:fail-guard // ring 0 only",
      }),
      finding("s-bare", "suppression", true, { head: "//nolint:fail-guard" }),
      finding("s-info", "suppression", false, {
        base: "//nolint:x",
        head: "//nolint:x // why",
      }),
      finding("i-other", "snapshot", false, { head: "abc" }),
      budget,
    ];
    const t = buildChangeTab(
      view({
        state: docState(doc({ findings })),
        accepted: (k) => k === "s-info",
      }),
    );
    expect(t.routine.map((g) => [g.id, g.count, g.keys, g.accepted])).toEqual([
      ["asserts-by", 2, ["a1", "a2"], 0],
      ["suppression-reason", 1, ["s-reason"], 0],
      ["suppression-info", 1, ["s-info"], 1],
    ]);
    expect(t.decisions.map((d) => d.key)).toEqual(["f-budget", "s-bare"]);
    expect(t.sub).toMatch(/^3 more are routine and grouped below\./);
    expect(t.lists).toEqual([
      { title: "Information", items: ["snapshot  shop/cart_test.go:4 abc"] },
    ]);
    expect(ROUTINE.map((r) => r.id)).toEqual([
      "asserts-by",
      "suppression-reason",
      "suppression-info",
    ]);
  });

  it("knows a nolint comment that says why", () => {
    expect(hasReason("//nolint:fail-guard // ring 0")).toBe(true);
    expect(hasReason("//nolint:fail-guard")).toBe(false);
    expect(hasReason("//nolint:fail-guard //")).toBe(false);
  });

  it("says which evidence is in use, the newest current run when none is chosen, or that nothing is proven", () => {
    expect(buildChangeTab(view()).evidence).toEqual({
      text: "Nothing is proven yet: no kept run matches this tree.",
      picks: [],
      action: { act: "auditRun", label: "Run what changed" },
    });
    const current = doc({
      streams: [{ file: "/e.jsonl", side: "head", run: null, current: true }],
    });
    const auto = buildChangeTab(
      view({
        state: docState(current),
        evidence: [
          {
            id: "r1",
            label: "the editor run of 08:38",
            missing: false,
            auto: true,
          },
        ],
      }),
    );
    expect(auto.evidence).toEqual({
      text: "Proven by the editor run of 08:38, which matches this tree.",
      picks: [],
      action: { act: "evidence", label: "Change evidence" },
    });
    const chosen = buildChangeTab(
      view({
        state: docState(
          doc({
            streams: [
              {
                file: "/old.jsonl",
                side: "head",
                run: null,
                current: false,
                stale: "another commit",
              },
            ],
          }),
        ),
        evidence: [
          {
            id: "r1",
            label: "the run of 09:10 by Claude Code",
            missing: false,
          },
          { id: "r2", label: "the terminal run of 08:00", missing: true },
        ],
      }),
    );
    expect(chosen.evidence!.text).toBe(
      "Proven by the run of 09:10 by Claude Code, partly from before the tree changed (/old.jsonl: another commit).",
    );
    expect(chosen.evidence!.picks).toEqual([
      { id: "r1", label: "the run of 09:10 by Claude Code", missing: false },
      { id: "r2", label: "the terminal run of 08:00", missing: true },
    ]);
    const ran = buildChangeTab(
      view({
        state: docState(
          doc({
            streams: [{ side: "head", ran: true, run: null, current: true }],
          }),
        ),
      }),
    );
    expect(ran.evidence!.text).toBe(
      "Proven by this audit's run, which matches this tree.",
    );
  });

  it("says how the judged behaviors fared", () => {
    const d = (over: object) =>
      doc({
        streams: [{ file: "e", side: "head", run: null, current: true }],
        summary: {
          ...doc().summary,
          evidence: 4,
          failing: 0,
          notRun: 0,
          ...over,
        },
      });
    expect(buildChangeTab(view({ state: docState(d({})) })).sub).toContain(
      "All 4 judged behaviors passed.",
    );
    expect(
      buildChangeTab(view({ state: docState(d({ failing: 1, notRun: 2 })) }))
        .sub,
    ).toContain("Of 4 judged behaviors, 1 failing, 2 not run.");
  });

  it("lists the changed behaviors per package, their rows on request, with Run where the editor can", () => {
    const t = buildChangeTab(view({ canRun: (k) => k === "b:0" }));
    expect(t.packages).toEqual([
      { pkg: "./shop", name: "shop", delta: "+1 added" },
    ]);
    expect(
      changePackageRows(view({ canRun: (k) => k === "b:0" }), "./shop"),
    ).toEqual([
      {
        key: "b:0",
        suite: "Cart",
        sign: "+",
        change: "added",
        text: "Copies / the only copy",
        where: "shop/cart_test.go:49",
        mark: undefined,
        canOpen: false,
        canRun: true,
      },
    ]);
    expect(changePackageRows(view(), "./gone")).toEqual([]);
  });

  it("names an exit that is not 0, a stopped run as stopped, and what was not compared", () => {
    const d = doc({ incomplete: ["./broken: does not compile"] });
    const t = buildChangeTab(view({ state: docState(d, 2) }));
    expect(t.exit).toBe("exit 2");
    expect(t.lists).toEqual([
      { title: "Not compared", items: ["./broken: does not compile"] },
    ]);
    const stopped = buildChangeTab(
      view({ state: { ...docState(d, 130), stopped: true } }),
    );
    expect(stopped.exit).toBe("the run was stopped");
  });

  it("lists the unproven lines per file, each range a place to open", () => {
    const d = doc({
      summary: {
        ...doc().summary,
        unproven: 9,
        changedLines: 12,
        noCoverage: 1,
      },
      coverage: { lines: "git diff", packages: ["mod/shop"] },
      unproven: [
        {
          module: "mod",
          file: "shop/ledger.go",
          package: "mod/shop",
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
          line: 1,
          coverage: true,
          changed: 12,
          unexecuted: 8,
          ranges: [{ line: 1, endLine: 2 }],
        },
      ],
    });
    const u = buildChangeTab(
      view({ state: docState(d), canOpen: (k) => k.startsWith("u:") }),
    ).unproven!;
    expect(u.head).toBe(
      "9 of 12 changed executable lines not executed, 1 file without coverage data",
    );
    expect(
      u.files.map((f) => [f.file, f.coverage, f.counts, f.canOpen, f.ranges]),
    ).toEqual([
      ["shop/ledger.go", false, "", true, []],
      ["shop/shop.go", true, "8 of 12", true, [{ key: "u:1:0", text: "1-2" }]],
    ]);
    expect(buildChangeTab(view()).unproven).toBeUndefined();
  });

  it("stands in for a document it lacks: no checkout, no base, an old CLI, a newer schema, an error", () => {
    expect(buildChangeTab(view({ state: { kind: "notGit" } }))).toMatchObject({
      empty: { title: "Not a git checkout" },
      canChooseBase: false,
      canAudit: false,
    });
    expect(
      buildChangeTab(view({ state: { kind: "noBase" } })).empty!.action,
    ).toEqual({ act: "chooseBase", label: "Choose a base…" });
    const old = buildChangeTab(
      view({
        state: {
          kind: "oldCli",
          version: "v1.41.0",
          floor: "v1.42.0",
          advice: "Run: go get -tool m@v1.42.0",
          canUpgrade: true,
        },
      }),
    );
    expect(old.empty!.title).toBe("The Change tab needs gotest v1.42.0");
    expect(old.empty!.text).toContain("This folder's CLI is v1.41.0.");
    expect(old.empty!.action).toEqual({ act: "upgrade", label: "Upgrade…" });
    expect(
      buildChangeTab(
        view({
          state: {
            kind: "oldCli",
            floor: "v1.42.0",
            advice: "x",
            canUpgrade: false,
          },
        }),
      ).empty!.action,
    ).toBeUndefined();
    expect(
      buildChangeTab(
        view({
          state: {
            kind: "newSchema",
            version: "v1.50.0",
            schema: 2,
            supported: 1,
          },
        }),
      ).empty!.text,
    ).toContain("schema 2; this extension reads schema 1");
    expect(
      buildChangeTab(
        view({ state: { kind: "error", message: "FAIL: <bad> ref" } }),
      ).error,
    ).toBe("FAIL: <bad> ref");
  });

  it("holds the audit while one runs, keeps a notice over the document, and offers the folders when there are several", () => {
    expect(buildChangeTab(view({ busy: "static" })).canAudit).toBe(false);
    expect(buildChangeTab(view()).canAudit).toBe(true);
    expect(
      buildChangeTab(view({ notice: "The audit was stopped." })).notice,
    ).toBe("The audit was stopped.");
    expect(buildChangeTab(view()).folders).toBeUndefined();
    const two = [
      { path: "/a", label: "a", selected: false },
      { path: "/b", label: "b", selected: true },
    ];
    expect(buildChangeTab(view({ folders: two })).folders).toEqual(two);
  });
});

describe("interpretAuditExit", () => {
  const out = JSON.stringify(doc());

  it("reads the document whatever the exit code", () => {
    for (const code of [0, 1, 2]) {
      const r = interpretAuditExit(code, out, "warning");
      expect(r.ok).toBe(true);
      if (r.ok) expect(r.exitCode).toBe(code);
    }
  });

  it("gives stderr, without go run's echo, when there is no document", () => {
    const r = interpretAuditExit(
      2,
      "",
      "FAIL: --base: no commit\nexit status 2\n",
    );
    expect(r).toEqual({ ok: false, message: "FAIL: --base: no commit" });
    expect(interpretAuditExit(2, "", "")).toEqual({
      ok: false,
      message: "gotest audit exited with code 2",
    });
  });
});
