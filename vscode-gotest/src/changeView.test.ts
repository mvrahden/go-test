import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
  window: {},
  commands: {},
}));

import { buildChangeBody, type ChangeView } from "./changeView.js";
import {
  buildChangeModel,
  findingKey,
  type AuditDocument,
} from "./auditModel.js";
import { interpretAuditExit } from "./changeScope.js";

const removed = {
  class: "removed",
  weakens: true,
  package: "mod/shop",
  subject: "Cart / Remove",
  base: "test",
  head: "",
  location: { file: "shop/cart_test.go", line: 32, endLine: 34 },
};
const budget = {
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
    head: { commit: "bbbb2222" },
    modules: ["mod"],
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
        head: {
          path: ["Cart", "Copies", "the only copy"],
          file: "shop/cart_test.go",
          line: 49,
          endLine: 51,
        },
        behaviors: 1,
      },
    ],
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
    headBranch: "feat/x",
    canRun: () => false,
    canOpen: () => false,
    ...over,
  };
}

describe("buildChangeBody", () => {
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
    const key = findingKey(budget);
    const some = buildChangeBody(
      view({ canOpen: (k) => k === key, canRun: (k) => k === key }),
    );
    expect(some.match(/data-change="openDiff"/g)).toHaveLength(1);
    expect(some.match(/data-change="run"/g)).toHaveLength(1);
  });

  it("strikes an accepted row through and moves it under the accepted", () => {
    const model = buildChangeModel(doc(), (k) => k === findingKey(removed));
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
    const html = buildChangeBody(view({ busy: true }));
    expect(html).toContain("Auditing…");
    expect(html).toMatch(/data-change="refresh"[^>]*disabled/);
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

  it("renders evidence and unproven lines when the document has them", () => {
    const html = buildChangeBody(
      view({
        state: {
          kind: "doc",
          model: buildChangeModel(
            doc({
              evidence: [
                {
                  package: "mod/shop",
                  path: ["Cart", "Add"],
                  verdict: "fail",
                  commit: "cccc3333",
                },
              ],
              unproven: [{ file: "shop/shop.go", line: 4 }],
            }),
          ),
          exitCode: 0,
        },
      }),
    );
    expect(html).toContain("Evidence");
    expect(html).toContain("@cccc3333");
    expect(html).toContain("1 changed line no test executed: 4");
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
