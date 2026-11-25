import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
  window: {},
  commands: {},
}));

import { buildChangeBody, type ChangeView } from "./changeView.js";
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

function view(over: Partial<ChangeView> = {}): ChangeView {
  return {
    state: { kind: "doc", model: buildChangeModel(doc()), exitCode: 0 },
    busy: false,
    folders: [{ path: "/ws", label: "ws", selected: true }],
    baseLabel: "merge-base with origin/main (aaaa1111)",
    evidence: [],
    canRun: () => false,
    canOpen: () => false,
    ...over,
  };
}

describe("buildChangeBody", () => {
  it("shows the runs taken as evidence as chips, each with a remove", () => {
    const html = buildChangeBody(
      view({
        evidence: [
          { id: "r1", label: "14:02 abc1234", missing: false },
          { id: "r2", label: "09:10", missing: true },
        ],
      }),
    );
    expect(html).toContain("Evidence from runs:");
    expect(html).toMatch(
      /<span class="evidence-chip" [^>]*>14:02 abc1234<button class="act" data-change="dropEvidence" data-key="r1"/,
    );
    expect(html).toMatch(
      /<span class="evidence-chip missing" title="Its events file is gone/,
    );
    expect(buildChangeBody(view())).not.toContain("Evidence from runs:");
  });

  it("opens with the head, the base and the summary's counts", () => {
    const html = buildChangeBody(view());
    expect(html).toContain("head <b>feat/x</b> bbbb2222 ← base main aaaa1111");
    expect(html).toContain("weakenings <b>2</b>");
    expect(html).toContain("Base: merge-base with origin/main (aaaa1111)");
  });

  it("puts the decisions before the behaviors, in the document's order", () => {
    const html = buildChangeBody(view());
    const at = (s: string) => html.indexOf(s);
    expect(at("Needs a decision")).toBeGreaterThan(-1);
    expect(at("Needs a decision")).toBeLessThan(at("Changed behaviors"));
    expect(at(">removed<")).toBeLessThan(at(">budget<"));
    expect(html).toContain("2 open · 0 accepted");
    expect(html).toContain("the only copy");
  });

  it("says that accepting is local", () => {
    const html = buildChangeBody(view());
    expect(html).toMatch(/class="caption"[^>]*>[^<]*never sent anywhere/);
  });

  it("offers Open diff and Run only where the editor can do them", () => {
    const none = buildChangeBody(view());
    expect(none).not.toContain('data-change="openDiff"');
    expect(none).not.toContain('data-change="run"');
    expect(none).toContain('data-change="accept"');
    const key = budget.id;
    const some = buildChangeBody(
      view({ canOpen: (k) => k === key, canRun: (k) => k === key }),
    );
    expect(some.match(/data-change="openDiff"/g)).toHaveLength(1);
    expect(some.match(/data-change="run"/g)).toHaveLength(1);
  });

  it("opens an evidence row that needs a decision at its source, a finding at its diff", () => {
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
    const html = buildChangeBody(
      view({
        state: { kind: "doc", model: buildChangeModel(d), exitCode: 1 },
        canOpen: (k) => k === "e-total" || k === budget.id,
      }),
    );
    expect(html).toMatch(
      /data-key="e-total"[\s\S]*?<button class="act" data-change="openSource" data-key="e-total"[^>]*>Open</,
    );
    expect(html.match(/data-change="openDiff"/g)).toHaveLength(1);
    expect(html).toContain('data-change="openDiff" data-key="f-budget"');
  });

  it("strikes an accepted row through and moves it under the accepted", () => {
    const model = buildChangeModel(doc(), (k) => k === removed.id);
    const html = buildChangeBody(
      view({ state: { kind: "doc", model, exitCode: 0 } }),
    );
    expect(html).toContain("1 open · 1 accepted");
    expect(html.indexOf("accepted · 1")).toBeLessThan(
      html.indexOf('class="decision fail accepted"'),
    );
    expect(html).toContain(">Reopen<");
  });

  it("names an exit that is not 0, and still shows the document", () => {
    const html = buildChangeBody(
      view({
        state: {
          kind: "doc",
          model: buildChangeModel(
            doc({ incomplete: ["./broken: does not compile"] }),
          ),
          exitCode: 2,
        },
      }),
    );
    expect(html).toContain("exit 2");
    expect(html).toContain("Not compared");
    expect(html).toContain("./broken: does not compile");
  });

  it("explains a CLI without audit and offers the upgrade when go.mod pins it", () => {
    const html = buildChangeBody(
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
    expect(html).toContain("needs gotest v1.42.0");
    expect(html).toContain("This folder's CLI is v1.41.0.");
    expect(html).toContain('data-change="upgrade"');
    const unpinned = buildChangeBody(
      view({
        state: {
          kind: "oldCli",
          floor: "v1.42.0",
          advice: "Run: x",
          canUpgrade: false,
        },
      }),
    );
    expect(unpinned).not.toContain('data-change="upgrade"');
  });

  it("says a folder outside git cannot be audited, with no base to choose", () => {
    const html = buildChangeBody(view({ state: { kind: "notGit" } }));
    expect(html).toContain("Not a git checkout");
    expect(html).toMatch(/data-change="chooseBase"[^>]*disabled/);
  });

  it("shows the CLI's complaint when it wrote no document", () => {
    const html = buildChangeBody(
      view({ state: { kind: "error", message: "FAIL: <bad> ref" } }),
    );
    expect(html).toContain("FAIL: &lt;bad&gt; ref");
  });

  it("marks an audit in progress and holds Refresh until it ends", () => {
    const html = buildChangeBody(view({ busy: "static" }));
    expect(html).toContain("Auditing…");
    expect(html).toMatch(/data-change="refresh"[^>]*disabled/);
    expect(html).toMatch(/data-change="auditRun"[^>]*disabled/);
    expect(html).not.toContain('data-change="cancel"');
    expect(buildChangeBody(view())).not.toMatch(
      /data-change="auditRun"[^>]*disabled/,
    );
  });

  it("offers to stop an audit with a run while it runs", () => {
    const html = buildChangeBody(view({ busy: "run" }));
    expect(html).toContain("Auditing with a run…");
    expect(html).toContain('data-change="cancel"');
    expect(html).toMatch(/data-change="auditRun"[^>]*disabled/);
  });

  it("says when the last audit was stopped, over the document it keeps", () => {
    const html = buildChangeBody(
      view({
        notice: "The audit was stopped; this is the last one that finished.",
      }),
    );
    expect(html.indexOf("The audit was stopped")).toBeLessThan(
      html.indexOf("Needs a decision"),
    );
  });

  it("shows a stopped run's document as stopped, not as a failed exit", () => {
    const d = doc({
      incomplete: [
        "--run: the run was stopped before its end; what it did not reach reads as not run",
      ],
    });
    const state = { kind: "doc" as const, model: buildChangeModel(d) };
    const stopped = buildChangeBody(
      view({ state: { ...state, exitCode: 130, stopped: true } }),
    );
    expect(stopped).toContain('<span class="warn">run stopped</span>');
    expect(stopped).not.toContain("exit 130");
    expect(stopped).toContain("--run: the run was stopped before its end");
    const failed = buildChangeBody(
      view({ state: { ...state, exitCode: 130 } }),
    );
    expect(failed).toContain('<span class="fail">exit 130</span>');
  });

  it("asks for a newer extension for an audit schema it does not read", () => {
    const html = buildChangeBody(
      view({
        state: {
          kind: "newSchema",
          version: "v1.50.0",
          schema: 2,
          supported: 1,
        },
      }),
    );
    expect(html).toContain("schema 2; this extension reads schema 1");
    expect(html).toMatch(/data-change="auditRun"[^>]*disabled/);
  });

  it("offers the folders to choose from when there are several", () => {
    const one = buildChangeBody(view());
    expect(one).not.toContain('id="change-folder"');
    const two = buildChangeBody(
      view({
        folders: [
          { path: "/a", label: "a", selected: false },
          { path: "/b", label: "b", selected: true },
        ],
      }),
    );
    expect(two).toContain('<option value="/b" selected>b</option>');
  });

  it("names the runs that judged the change, current or stale and why, before the decisions", () => {
    const html = buildChangeBody(
      view({
        state: {
          kind: "doc",
          model: buildChangeModel(
            doc({
              streams: [
                {
                  file: "run.json",
                  side: "head",
                  run: {
                    commit: "bbbb2222",
                    agent: "claude-code",
                    exitCode: 1,
                  },
                  current: true,
                },
                {
                  file: "old.json",
                  side: "head",
                  run: { commit: "0a1b2c3", exitCode: 0 },
                  current: false,
                  stale: "another commit: the stream ran 0a1b2c3",
                },
              ],
            }),
          ),
          exitCode: 0,
        },
      }),
    );
    expect(html).toContain(
      "evidence: 2 streams, 1 current; * marks a verdict of a stale one",
    );
    expect(html).toContain(
      '>current</span> <span class="mono">run.json of bbbb2222, exit 1</span> <span class="dim">· agent claude-code</span>',
    );
    expect(html).toContain(
      '>stale</span> <span class="mono">old.json of 0a1b2c3, exit 0: another commit: the stream ran 0a1b2c3</span>',
    );
    expect(html.indexOf('id="streams"')).toBeLessThan(
      html.indexOf('id="decisions"'),
    );
    expect(buildChangeBody(view())).not.toContain('id="streams"');
  });

  it("marks each row with its evidence, and runs a behavior where it can", () => {
    const d = doc({
      streams: [{ file: "run.json", side: "head", run: null, current: true }],
      evidence: [
        {
          id: "e-copy",
          package: "mod/shop",
          path: ["Cart", "Copies", "the only copy"],
          verdict: "pass",
          mark: "vacuous",
          assertions: 0,
          vacuous: true,
          stream: 0,
          current: true,
          needsDecision: true,
          why: ["added"],
          run: "^TestCartTestSuite$/^TestCopies$/^the_only_copy$",
        },
      ],
    });
    const html = buildChangeBody(
      view({
        state: { kind: "doc", model: buildChangeModel(d), exitCode: 0 },
        canRun: (k) => k === "b:0",
      }),
    );
    // The decision is tagged by its mark; the behavior carries it beside.
    expect(html).toMatch(/class="tag-chip warn"[^>]*>vacuous</);
    expect(html).toMatch(
      /<span class="mark warn"[^>]*>vacuous<\/span> Copies \/ the only copy/,
    );
    expect(html).toMatch(
      /class="behavior added"[\s\S]*data-change="run" data-key="b:0"/,
    );
  });

  it("lists the unproven lines per file, each range opening the file at it", () => {
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
          ranges: [1, 3, 5, 7, 9, 11, 13, 15].map((l) => ({
            line: l,
            endLine: l === 1 ? 2 : l,
          })),
        },
      ],
    });
    const html = buildChangeBody(
      view({
        state: { kind: "doc", model: buildChangeModel(d), exitCode: 0 },
        canOpen: (k) => k.startsWith("u:"),
      }),
    );
    expect(html).toContain(
      "9 of 12 changed executable lines not executed, 1 file without coverage data",
    );
    expect(html).toMatch(
      /shop\/ledger\.go<\/a> <span class="dim">\.\/shop<\/span> <span class="fact">no coverage data/,
    );
    expect(html).toContain("8 of 12:");
    expect(html).toContain('data-change="openSource" data-key="u:1:0">1-2</a>');
    // Past six ranges the rest fold, as the CLI's text does without -v.
    expect(html).toMatch(/… 2 more<\/summary>[^]*data-key="u:1:7">15</);
    expect(html).toContain("unproven <b>9 of 12 lines</b>");
    expect(buildChangeBody(view())).not.toContain('id="unproven"');
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
