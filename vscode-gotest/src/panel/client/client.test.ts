import { describe, it, expect, vi } from "vitest";
import * as vm from "node:vm";

vi.mock("vscode", () => ({}));

import { panelApp } from "./app.js";
import { clientHelpers } from "./helpers.js";
import { renderChange } from "./renderChange.js";
import { renderNow } from "./renderNow.js";
import { renderRuns } from "./renderRuns.js";
import { fakeDom, FakeEl } from "./fakeDom.test-support.js";
import { buildShell, panelScript } from "../shell.js";
import type {
  ChangeTab,
  FolderRow,
  NowModel,
  RunCard,
  RunsTab,
  TreeNode,
} from "../protocol.js";

const H = clientHelpers();
const NOW = renderNow(H);
const CHANGE = renderChange(H);
const RUNS = renderRuns(H);

const pkgRow = (i: number) => ({
  pkg: `m/p${i}`,
  name: `p${i}`,
  counts: `${i} passed`,
  tone: i === 0 ? ("fail" as const) : ("pass" as const),
  time: "1s",
});

// Two folders: a holds the failed p0, b the rest.
function folders(map = (r: ReturnType<typeof pkgRow>) => r): FolderRow[] {
  return [
    {
      key: "a",
      name: "a",
      packages: 3,
      counts: "1 failed",
      tone: "fail",
      hot: true,
      rows: [0, 1, 2].map(pkgRow).map(map),
    },
    {
      key: "b",
      name: "b",
      packages: 11,
      counts: "60 passed",
      tone: "pass",
      hot: false,
      rows: Array.from({ length: 11 }, (_, i) => pkgRow(i + 3)).map(map),
    },
  ];
}

const OPEN = { open: {}, deep: false, deepSuites: [], depth: "packages" };

function nowModel(over: Partial<NowModel> = {}): NowModel {
  return {
    rev: 1,
    context: {
      branch: "main",
      commit: "abc",
      fresh: { text: "verdicts match this tree", tone: "pass" },
    },
    chips: [
      { status: "fail", label: "Failed", count: 1 },
      { status: "pass", label: "Passed", count: 12 },
    ],
    cards: [
      {
        key: "k1",
        kind: "failed",
        pkg: "m/a",
        where: "a › Cart",
        name: "adds <one>",
        message: [["Equal failed:"]],
        at: [["cart_test.go:3", "/src/a/cart_test.go", 3]],
        loc: { file: "/src/a/cart_test.go", line: 2 },
        id: "m/a/CartTestSuite/adds",
        pos: [0, 1],
      },
    ],
    total: "13 in 2 suites, 14 packages",
    groups: [],
    folders: folders(),
    filtering: false,
    failed: 1,
    ...over,
  };
}

const changeTab = (over: Partial<ChangeTab> = {}): ChangeTab => ({
  busy: false,
  canAudit: true,
  canChooseBase: true,
  head: "feat/x",
  base: "main",
  evidence: {
    text: "Nothing is proven yet: no kept run matches this tree.",
    picks: [],
    action: { act: "auditRun", label: "Run what changed" },
  },
  headline: "2 changes need your decision",
  sub: "1 more is routine.",
  decisions: [
    {
      key: "d1",
      kind: "Removed",
      tone: "fail",
      what: "Cart › <Remove>",
      fact: "gone",
      where: "shop/cart_test.go:32",
      open: "diff",
      accepted: false,
    },
    {
      key: "d2",
      kind: "Budget",
      tone: "warn",
      what: "Cart",
      fact: "10s → 1m",
      where: "shop/cart_test.go:9",
      accepted: false,
    },
  ],
  shown: 1,
  moreText: "1 more: 1 budget",
  accepted: [],
  routine: [
    {
      id: "asserts-by",
      count: 1,
      what: "tests now declare how they assert (AssertsBy)",
      keys: ["r1"],
      accepted: 0,
      rows: [
        {
          key: "r1",
          kind: "Asserts-by",
          tone: "warn",
          what: "Cart › Copies",
          fact: "golden",
          where: "",
          accepted: false,
        },
      ],
    },
  ],
  packages: [{ pkg: "./shop", name: "shop", delta: "+1 added" }],
  lists: [],
  openCount: 2,
  ...over,
});

const card = (over: Partial<RunCard> = {}): RunCard => ({
  key: "r1",
  time: "08:38",
  day: "today",
  title: "Everything",
  who: "You, from this editor",
  counts: "3 passed",
  verdict: "Passed",
  tone: "pass",
  fresh: "2 packages, current",
  freshTone: "pass",
  canOpen: true,
  evidence: "none",
  command: "gotest -json ./...",
  commit: "abc",
  ...over,
});

describe("the Now markup", () => {
  it("draws a card with its where, name, message, the line it names, and its actions, escaped", () => {
    const html = NOW.attention(nowModel(), {
      outputs: {},
      shown: {},
      noSuites: false,
    });
    expect(html).toContain("Needs attention");
    expect(html).toContain("adds &lt;one&gt;");
    expect(html).toContain(
      'data-act="run" data-id="m/a/CartTestSuite/adds" data-pos="0.1"',
    );
    expect(html).toContain('data-act="debug"');
    expect(html).toContain(
      'data-act="goTo" data-file="/src/a/cart_test.go" data-line="2">Open test',
    );
    expect(html).toContain('data-act="fullOutput"');
    expect(html).toContain(
      '<a class="src-link" href="#" data-act="goTo" data-file="/src/a/cart_test.go" data-line="3">cart_test.go:3</a>',
    );
    expect(html).toContain('<article class="card k-failed" data-pkg="m/a"');
  });

  it("shows a card's full output once fetched, and says it is loading before", () => {
    const st = { outputs: {}, shown: { k1: true }, noSuites: false };
    expect(NOW.attention(nowModel(), st)).toContain("Loading the output…");
    const got = { ...st, outputs: { k1: { lines: [["boom <x>"]], more: 3 } } };
    const html = NOW.attention(nowModel(), got);
    expect(html).toContain("boom &lt;x&gt;");
    expect(html).toContain("… 3 more lines");
    expect(html).toContain(">Hide output<");
  });

  it("presses the chips the user chose, and words status in text as well as color", () => {
    const html = NOW.chips(nowModel(), ["fail"]);
    expect(html).toContain(
      'data-status="fail" aria-pressed="true">Failed <strong>1</strong>',
    );
    expect(html).toContain('data-status="pass" aria-pressed="false"');
    expect(
      NOW.tree([{ d: "x", k: "block", s: "fail", pos: "0" }], undefined, OPEN),
    ).toContain('<span class="sw s-fail">FAIL</span>');
  });

  it("says the view is out of date when the last render failed, its reason's first line escaped, with a retry", () => {
    const html = NOW.context(
      nowModel({ error: "spec render timed out <x>\n<stack>" }),
    );
    expect(html).toContain("This view is out of date: the last render failed.");
    expect(html).toContain("spec render timed out &lt;x&gt;");
    expect(html).not.toContain("stack");
    expect(html).toContain('data-act="refresh"');
  });

  it("offers the imported streams beside the workspace's runs", () => {
    const html = NOW.context(
      nowModel({
        context: {
          sources: [
            { id: "", label: "Workspace runs", selected: false },
            { id: "file:/x", label: "Imported: ci.jsonl", selected: true },
          ],
        },
      }),
    );
    expect(html).toContain('<select data-change="source">');
    expect(html).toContain(
      '<option value="file:/x" selected>Imported: ci.jsonl</option>',
    );
  });
});

describe("the Change markup", () => {
  const ui = {
    allDecisions: false,
    showAccepted: false,
    review: {},
    open: {},
    rows: {},
  };

  it("opens on the sentence, shows the first decisions and counts the rest", () => {
    const html = CHANGE.body(changeTab(), ui);
    expect(html).toContain("<h1>2 changes need your decision</h1>");
    expect(html).toContain("Cart › &lt;Remove&gt;");
    expect(html).not.toContain("10s → 1m");
    expect(html).toContain("1 more: 1 budget");
    expect(CHANGE.body(changeTab(), { ...ui, allDecisions: true })).toContain(
      "10s → 1m",
    );
  });

  it("gives a decision its kind, Diff and Accept, keyed by its row", () => {
    const html = CHANGE.decision(changeTab().decisions[0]);
    expect(html).toContain('<span class="kind t-fail">Removed</span>');
    expect(html).toContain(
      'data-act="openDiff" data-key="d1" data-scope="change">Diff',
    );
    expect(html).toContain(
      'data-act="accept" data-key="d1" data-scope="change" aria-pressed="false">Accept',
    );
    expect(
      CHANGE.decision({ ...changeTab().decisions[0], accepted: true }),
    ).toContain(">Reopen<");
  });

  it("groups the routine rows with Review and Accept all", () => {
    const html = CHANGE.body(changeTab(), ui);
    expect(html).toContain("Routine, accept as a group");
    expect(html).toContain(
      'data-act="acceptAll" data-key="asserts-by" data-scope="change">Accept all',
    );
    expect(html).not.toContain("Cart › Copies");
    expect(
      CHANGE.body(changeTab(), { ...ui, review: { "asserts-by": true } }),
    ).toContain("Cart › Copies");
  });

  it("says which evidence is in use, or that nothing is proven yet with a run to prove it", () => {
    const ctx = CHANGE.context(changeTab());
    expect(ctx).toContain("Nothing is proven yet");
    expect(ctx).toContain(
      'data-act="auditRun" data-scope="change">Run what changed',
    );
    expect(ctx).toContain("compared with");
    const stopping = CHANGE.context(changeTab({ busy: "run" }));
    expect(stopping).toContain('data-act="cancel"');
  });

  it("shows a state without a document as its own page, with its action", () => {
    const html = CHANGE.body(
      changeTab({
        empty: {
          title: "Choose a base",
          text: "No remote.",
          action: { act: "chooseBase", label: "Choose a base…" },
        },
      }),
      ui,
    );
    expect(html).toContain("<h1>Choose a base</h1>");
    expect(html).toContain('data-act="chooseBase" data-scope="change"');
  });
});

describe("the Runs markup", () => {
  const ui = { expanded: {}, command: {}, earlier: false };

  it("draws a record's card: time, title, who, counts, verdict, freshness and actions", () => {
    const html = RUNS.card(card(), ui);
    for (const s of [
      "08:38",
      "Everything",
      "You, from this editor · 3 passed",
      "Passed",
      "2 packages, current",
      "Open results",
      "Use as evidence",
      ">Command<",
    ])
      expect(html).toContain(s);
    expect(RUNS.card(card({ evidence: "auto" }), ui)).toContain(
      "Evidence in use (newest current run)",
    );
    expect(RUNS.card(card({ canOpen: false }), ui)).not.toContain(
      "Open results",
    );
  });

  it("reveals the command line, and a folded card's runs, on request", () => {
    expect(RUNS.card(card(), { ...ui, command: { r1: true } })).toContain(
      '<code class="cmd">gotest -json ./...</code>',
    );
    const folded = card({
      runs: [card({ key: "a" }), card({ key: "b", verdict: "Failed" })],
    });
    expect(RUNS.card(folded, ui)).toContain("Show the 2 runs");
    expect(RUNS.card(folded, { ...ui, expanded: { r1: true } })).toContain(
      "Failed",
    );
  });

  it("puts each run in progress on top with its counts and a Stop", () => {
    const html = RUNS.running([
      { id: 7, label: "fuzz: m/a FuzzX", elapsed: "0:42", stopping: false },
    ]);
    expect(html).toContain("Running now");
    expect(html).toContain('data-act="stopSession" data-session="7"');
    expect(
      RUNS.running([{ id: 7, label: "x", elapsed: "0:01", stopping: true }]),
    ).toContain("Stopping…");
  });

  it("keeps the records of other commits folded", () => {
    const m: RunsTab = {
      busy: false,
      headline: "h",
      cards: [card()],
      earlier: [card({ key: "old", title: "Old run" })],
    };
    expect(RUNS.body(m, ui)).not.toContain("Old run");
    expect(RUNS.body(m, { ...ui, earlier: true })).toContain("Old run");
  });
});

// A stand-in observer: the test says which rows came into view.
class FakeIO {
  static last: FakeIO | undefined;
  seen = new Set<FakeEl>();
  constructor(readonly cb: (entries: unknown[]) => void) {
    FakeIO.last = this;
  }
  observe(el: FakeEl) {
    this.seen.add(el);
  }
  unobserve(el: FakeEl) {
    this.seen.delete(el);
  }
  show(els: FakeEl[]) {
    this.cb(els.map((target) => ({ target, isIntersecting: true })));
  }
}

function app(
  saved: unknown = null,
  initial: { tab?: "now" | "change" | "runs" } = {},
) {
  const dom = fakeDom();
  dom.setState(saved);
  (dom.window as { IntersectionObserver?: unknown }).IntersectionObserver =
    FakeIO;
  const a = panelApp({
    vscode: dom.vscode,
    document: dom.document,
    window: dom.window,
    initial: { ...initial, has: { change: true, runs: true } },
    NOW,
    CHANGE,
    RUNS,
  });
  return { dom, a };
}

const click = (a: ReturnType<typeof panelApp>, el: FakeEl) =>
  a.onClick({ target: el, preventDefault() {} });

const button = (data: Record<string, string>, parent?: FakeEl) => {
  const b = new FakeEl("button");
  Object.assign(b.dataset, data);
  if (parent) parent.appendChild(b);
  return b;
};

describe("the webview's app", () => {
  it("tells the host what it keeps once it starts: the tab, opened packages, search and chips", () => {
    const { dom } = app({
      tab: "runs",
      open: ["m/p1"],
      search: "cart",
      filters: ["fail"],
      changeOpen: ["./shop"],
    });
    expect(dom.posted).toEqual([
      {
        type: "ready",
        tab: "runs",
        changeOpen: ["./shop"],
        search: "cart",
        filters: ["fail"],
      },
      { type: "search", text: "cart" },
    ]);
    expect(dom.el("search").value).toBe("cart");
  });

  it("switches tabs at once, from what it kept, and tells the host after", () => {
    const { dom, a } = app();
    a.receive({ type: "change", model: changeTab() });
    dom.posted.length = 0;
    click(a, button({ act: "tab", tab: "change" }));
    expect(dom.el("tab-change").hidden).toBe(false);
    expect(dom.el("tab-now").hidden).toBe(true);
    expect(dom.el("change-body").innerHTML).toContain(
      "2 changes need your decision",
    );
    expect(dom.el("primary").textContent).toBe("Run what changed");
    expect(dom.posted).toEqual([{ type: "tab", tab: "change" }]);
    expect(dom.state()!.tab).toBe("change");
  });

  it("shows a small updating mark while the host fetches for the tab on show, never blanking it", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    a.receive({ type: "busy", tab: "now", busy: true });
    expect(dom.el("updating").hidden).toBe(false);
    expect(dom.el("now-ctx").innerHTML).toContain("verdicts match this tree");
    a.receive({ type: "busy", tab: "now", busy: false });
    expect(dom.el("updating").hidden).toBe(true);
  });

  it("puts the counts on the tabs as text with a label", () => {
    const { dom, a } = app();
    a.receive({ type: "badges", now: 3, change: 22 });
    expect(dom.el("badge-now").textContent).toBe("3");
    expect(dom.el("badge-now").getAttribute("aria-label")).toBe("3 failed");
    expect(dom.el("badge-change").textContent).toBe("22");
    a.receive({ type: "badges", now: 0, change: 0 });
    expect(dom.el("badge-now").textContent).toBe("");
  });

  const folderEl = (dom: ReturnType<typeof fakeDom>, i: number) =>
    dom.el("now-packages").children[i];
  const pkgEl = (dom: ReturnType<typeof fakeDom>, f: number, i: number) =>
    folderEl(dom, f).children[1].children[i];
  // A click on a package's chevron, as the button in its head.
  const toggle = (a: ReturnType<typeof panelApp>, row: FakeEl) =>
    click(a, button({ act: "togglePkg" }, row.children[0]));

  it("groups packages under folders, a failing folder open, the rest closed with their counts", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    expect(dom.el("now-packages").children).toHaveLength(2);
    expect(folderEl(dom, 0).children[0].innerHTML).toContain(
      'a <span class="dim">3 packages</span>',
    );
    expect(folderEl(dom, 0).children[1].children).toHaveLength(3);
    expect(folderEl(dom, 1).children[1].hidden).toBe(true);
    expect(folderEl(dom, 1).children[1].children).toHaveLength(0);
    click(a, button({ act: "toggleFolder", folder: "b" }));
    expect(folderEl(dom, 1).children[1].children).toHaveLength(11);
    expect(dom.state()!.folders).toEqual({ b: true });
    const filtered = nowModel({
      filtering: true,
      folders: folders((r) => ({ ...r, matches: Number(r.pkg.slice(3)) % 2 })),
    });
    a.receive({ type: "now", model: filtered });
    expect(folderEl(dom, 0).children[1].children).toHaveLength(1);
    expect(folderEl(dom, 0).children[0].innerHTML).toContain("1 of 3 packages");
  });

  it("opens a package when its rows come into view, and draws them when they arrive", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    dom.posted.length = 0;
    const row = pkgEl(dom, 0, 2);
    toggle(a, row);
    expect(dom.state()!.open).toEqual(["m/p2"]);
    expect(row.children[1].hidden).toBe(false);
    expect(row.children[1].innerHTML).toContain("Loading the rows…");
    expect(dom.posted).toEqual([]);
    FakeIO.last!.show([row]);
    expect(dom.posted).toEqual([{ type: "openPackage", pkg: "m/p2" }]);
    a.receive({
      type: "package",
      pkg: "m/p2",
      rev: 1,
      nodes: [{ d: "adds", k: "block", s: "pass", pos: "0" }],
    });
    expect(row.children[1].innerHTML).toContain("adds");
    toggle(a, row);
    expect(row.children[1].hidden).toBe(true);
  });

  it("opens every package at the Suites and Behaviors depths, asking only for the rows in view", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    dom.posted.length = 0;
    click(a, button({ act: "depth", depth: "behaviors" }));
    expect(dom.el("depth-behaviors").getAttribute("aria-pressed")).toBe("true");
    expect(dom.state()!.depth).toBe("behaviors");
    // Every folder opens, every package waits for its rows to come into view.
    expect(folderEl(dom, 1).children[1].children).toHaveLength(11);
    expect(FakeIO.last!.seen.size).toBe(14);
    expect(dom.posted).toEqual([]);
    FakeIO.last!.show([pkgEl(dom, 0, 0), pkgEl(dom, 0, 1)]);
    expect(dom.posted).toEqual([
      { type: "openPackage", pkg: "m/p0" },
      { type: "openPackage", pkg: "m/p1" },
    ]);
    const nodes: TreeNode[] = [
      {
        d: "Cart",
        k: "suite",
        s: "pass",
        pos: "0",
        n: "2 passed",
        nt: "pass",
        c: [
          {
            d: "M",
            k: "method",
            s: "pass",
            pos: "0.0",
            c: [{ d: "adds", k: "block", s: "pass", pos: "0.0.0" }],
          },
        ],
      },
    ];
    a.receive({ type: "package", pkg: "m/p0", rev: 1, nodes });
    const tree = pkgEl(dom, 0, 0).children[1].innerHTML;
    expect(tree).toMatch(/<details class="branch" data-pos="0" open>/);
    expect(tree).toMatch(/<details class="branch" data-pos="0.0" open>/);
    expect(tree).toContain('<span class="counts t-pass">2 passed</span>');
    expect(tree).toContain('<span class="sw s-pass">pass</span>');
    click(a, button({ act: "depth", depth: "suites" }));
    expect(pkgEl(dom, 0, 0).children[1].innerHTML).toMatch(
      /<details class="branch" data-pos="0">/,
    );
    // A new summary makes the rows on show out of date: asked again in view.
    dom.posted.length = 0;
    a.receive({ type: "now", model: { ...nowModel(), rev: 2 } });
    FakeIO.last!.show([pkgEl(dom, 0, 0)]);
    expect(dom.posted).toEqual([{ type: "openPackage", pkg: "m/p0" }]);
  });

  it("expands everything in a package, or a suite, with the button beside its name, and collapses it again", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    const nodes = [
      {
        d: "Cart",
        k: "suite",
        s: "pass",
        pos: "0",
        c: [
          {
            d: "M",
            k: "method",
            s: "pass",
            pos: "0.0",
            c: [{ d: "x", k: "block", s: "pass", pos: "0.0.0" }],
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
    const row = pkgEl(dom, 0, 1);
    expect(row.children[0].innerHTML).toContain(
      'aria-label="Expand everything in p1"',
    );
    click(a, button({ act: "deep", pkg: "m/p1" }));
    expect(dom.state()!.deep).toEqual(["m/p1"]);
    expect(row.children[0].innerHTML).toContain(
      'aria-pressed="true" aria-label="Collapse everything in p1"',
    );
    a.receive({ type: "package", pkg: "m/p1", rev: 1, nodes });
    expect(row.children[1].innerHTML.match(/ open>/g)).toHaveLength(3);
    click(a, button({ act: "deep", pkg: "m/p1" }));
    expect(row.children[1].innerHTML).not.toContain(" open>");
    // The package stays open at its suites; a suite's own button opens that
    // suite through.
    expect(row.children[1].hidden).toBe(false);
    const suiteBtn = button({ act: "deep", pos: "0" }, row.children[1]);
    click(a, suiteBtn);
    expect(dom.state()!.deepSuites).toEqual({ "m/p1": ["0"] });
    const html = row.children[1].innerHTML;
    expect(html).toMatch(/data-pos="0" open>/);
    expect(html).toMatch(/data-pos="0.0" open>/);
    expect(html).toMatch(/data-pos="1">/);
  });

  it("keeps a package's rows in place when a new summary comes, only its head changing", () => {
    const { dom, a } = app({ open: ["m/p0"] });
    a.receive({ type: "now", model: nowModel() });
    FakeIO.last!.show([pkgEl(dom, 0, 0)]);
    a.receive({
      type: "package",
      pkg: "m/p0",
      rev: 1,
      nodes: [{ d: "kept", k: "block", s: "pass", pos: "0" }],
    });
    const row = pkgEl(dom, 0, 0);
    a.receive({
      type: "now",
      model: nowModel({
        folders: folders((r) => ({ ...r, counts: "live " + r.counts })),
      }),
    });
    expect(pkgEl(dom, 0, 0)).toBe(row);
    expect(row.children[0].innerHTML).toContain("live 0 passed");
    expect(row.children[1].innerHTML).toContain("kept");
  });

  it("shows what a search finds, open in place under its folders, and the list as it was once cleared", () => {
    const { dom, a } = app({ depth: "suites" });
    a.receive({ type: "now", model: nowModel() });
    const box = dom.el("search");
    box.value = "adds";
    a.onInput({ target: box });
    dom.flushTimers();
    expect(dom.posted.at(-1)).toEqual({ type: "search", text: "adds" });
    expect(dom.el("now-packages").hidden).toBe(true);
    expect(dom.el("now-found").innerHTML).toContain("Searching…");
    a.receive({
      type: "found",
      query: "other",
      rev: 1,
      results: [],
      total: 0,
      shown: 0,
    });
    expect(dom.el("now-found").innerHTML).toContain("Searching…");
    a.receive({
      type: "found",
      query: "adds",
      rev: 1,
      results: [
        {
          pkg: "m/p4",
          nodes: [
            {
              d: "Cart",
              k: "suite",
              s: "pass",
              pos: "0",
              c: [{ d: "adds <it>", k: "block", s: "pass", pos: "0.0" }],
            },
          ],
        },
      ],
      total: 40,
      shown: 1,
    });
    const found = dom.el("now-found").innerHTML;
    expect(found).toContain("40 matches; the first 1 show.");
    expect(found).toContain('b <span class="dim">1 of 11 packages</span>');
    expect(found).toContain("adds &lt;it&gt;");
    expect(found).toMatch(/data-pos="0" open>/);
    expect(found).not.toContain("p5");
    box.value = "";
    a.onInput({ target: box });
    dom.flushTimers();
    expect(dom.el("now-packages").hidden).toBe(false);
    expect(dom.el("now-found").hidden).toBe(true);
    expect(dom.state()!.depth).toBe("suites");
  });

  it("asks a search again of a new summary and of other chips", () => {
    const { dom, a } = app({ search: "adds" });
    expect(dom.posted.at(-1)).toEqual({ type: "search", text: "adds" });
    a.receive({ type: "now", model: nowModel() });
    dom.posted.length = 0;
    a.receive({ type: "now", model: { ...nowModel(), rev: 2 } });
    click(a, button({ act: "chip", status: "fail" }));
    expect(dom.posted).toEqual([
      { type: "search", text: "adds" },
      { type: "filters", filters: ["fail"] },
      { type: "search", text: "adds" },
    ]);
  });

  it("offers Run all tests first in More actions, then Rerun failed, then the rest", () => {
    const { dom, a } = app();
    click(a, button({ act: "menu" }));
    const menu = dom.el("menu").innerHTML;
    const items = [...menu.matchAll(/data-item="([^"]+)">([^<]+)</g)].map(
      (m) => m[2],
    );
    expect(items).toEqual([
      "Run all tests",
      "Rerun failed",
      "Refresh",
      "Copy as Markdown",
      "Clear results",
    ]);
    expect(menu.indexOf('role="separator"')).toBeGreaterThan(
      menu.indexOf("Rerun failed"),
    );
    expect(menu.indexOf('role="separator"')).toBeLessThan(
      menu.indexOf("Refresh"),
    );
    dom.posted.length = 0;
    click(a, button({ act: "menuItem", item: "runTests" }));
    expect(dom.posted).toEqual([{ type: "runTests" }]);
  });

  it("sends Run and Debug with the row's item, package and place", () => {
    const { dom, a } = app();
    const tree = new FakeEl("div");
    tree.dataset.pkg = "m/a";
    const row = new FakeEl("div");
    Object.assign(row.dataset, { pos: "0.1", id: "m/a/S/x" });
    tree.appendChild(row);
    dom.posted.length = 0;
    click(a, button({ act: "run" }, row));
    click(a, button({ act: "debug" }, row));
    click(a, button({ act: "copyNode" }, row));
    expect(dom.posted).toEqual([
      { type: "run", pkg: "m/a", pos: "0.1", id: "m/a/S/x" },
      { type: "debug", pkg: "m/a", pos: "0.1", id: "m/a/S/x" },
      { type: "copyNode", pkg: "m/a", position: [0, 1] },
    ]);
  });

  it("asks for a card's output once, and narrows by a chip through the host", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    const art = new FakeEl("article");
    art.dataset.pkg = "m/a";
    dom.posted.length = 0;
    const b = button({ act: "fullOutput", key: "k1", pos: "0.1" }, art);
    click(a, b);
    a.receive({ type: "output", key: "k1", lines: [["boom"]] });
    expect(dom.el("now-attention").innerHTML).toContain("boom");
    click(a, b);
    click(a, b);
    expect(dom.posted).toEqual([
      { type: "output", key: "k1", pkg: "m/a", pos: "0.1" },
    ]);
    click(a, button({ act: "chip", status: "fail" }));
    expect(dom.posted.at(-1)).toEqual({ type: "filters", filters: ["fail"] });
    expect(dom.el("now-chips").innerHTML).toContain('aria-pressed="true"');
  });

  it("searches after the typing settles", () => {
    const { dom, a } = app();
    const box = dom.el("search");
    box.value = "cart";
    a.onInput({ target: box });
    expect(
      dom.posted.filter((m) => (m as { type: string }).type === "search"),
    ).toEqual([]);
    dom.flushTimers();
    expect(dom.posted.at(-1)).toEqual({ type: "search", text: "cart" });
    expect(dom.state()!.search).toBe("cart");
    box.value = "";
    a.onInput({ target: box });
    dom.flushTimers();
    expect(dom.posted.at(-1)).toEqual({ type: "search", text: "cart" });
  });

  it("routes a Change or Runs button by its scope, and a run's Stop by its session", () => {
    const { dom, a } = app();
    dom.posted.length = 0;
    click(a, button({ act: "accept", key: "d1", scope: "change" }));
    click(a, button({ act: "open", key: "r1", scope: "runs" }));
    click(a, button({ act: "stopSession", session: "7" }));
    expect(dom.posted).toEqual([
      { type: "change", act: "accept", key: "d1" },
      { type: "runs", act: "open", key: "r1" },
      { type: "stopSession", id: 7 },
    ]);
  });

  it("takes Change evidence to the Runs tab, and asks for a package's behaviors when one opens", () => {
    const { dom, a } = app(null, { tab: "change" });
    a.receive({ type: "change", model: changeTab() });
    dom.posted.length = 0;
    click(a, button({ act: "changePackage", pkg: "./shop" }));
    expect(dom.posted).toEqual([
      { type: "change", act: "changePackage", key: "./shop" },
    ]);
    a.receive({
      type: "changePackage",
      pkg: "./shop",
      rows: [
        {
          key: "b:0",
          suite: "Cart",
          sign: "+",
          change: "added",
          text: "copies",
          where: "c.go:1",
          canOpen: false,
          canRun: false,
        },
      ],
    });
    expect(dom.el("change-body").innerHTML).toContain("copies");
    click(a, button({ act: "evidence", scope: "change" }));
    expect(dom.el("tab-runs").hidden).toBe(false);
  });

  it("follows the host to a tab, and draws the Running block there", () => {
    const { dom, a } = app();
    a.receive({ type: "tab", tab: "runs" });
    a.receive({
      type: "running",
      items: [
        { id: 1, label: "run: 3 items", elapsed: "0:03", stopping: false },
      ],
    });
    expect(dom.el("tab-runs").hidden).toBe(false);
    expect(dom.el("runs-running").innerHTML).toContain("run: 3 items");
  });

  it("draws what it kept at once after a reload, before the host answers", () => {
    const first = app();
    first.a.receive({ type: "now", model: nowModel() });
    const kept = first.dom.state();
    const { dom } = app(kept);
    expect(dom.el("now-ctx").innerHTML).toContain("verdicts match this tree");
  });
});

describe("the panel's document", () => {
  it("embeds what the host gives it so no text can close the script", () => {
    const html = buildShell({
      nonce: "abc",
      cspSource: "vscode-resource:",
      has: { change: true, runs: false },
    });
    expect(html).toContain("script-src 'nonce-abc'");
    expect(html.match(/<\/script>/g)).toHaveLength(1);
    expect(
      panelScript({ tab: "now", has: { change: true, runs: true } }),
    ).not.toMatch(/<\/script/i);
  });

  it("runs on the webview's globals alone, drawing every tab from the host's messages", () => {
    const dom = fakeDom();
    const context = vm.createContext({
      acquireVsCodeApi: () => dom.vscode,
      document: dom.document,
      window: dom.window,
    });
    vm.runInContext(
      panelScript({ tab: "now", has: { change: true, runs: true } }),
      context,
    );
    expect(dom.posted[0]).toMatchObject({ type: "ready", tab: "now" });
    dom.send({ type: "now", model: nowModel() });
    dom.send({
      type: "package",
      pkg: "m/p0",
      rev: 1,
      nodes: [
        {
          d: "row <x>",
          k: "suite",
          s: "fail",
          pos: "0",
          c: [
            {
              d: "leaf",
              k: "block",
              s: "pass",
              pos: "0.0",
              tags: [{ text: "NOT RUN" }],
            },
          ],
        },
      ],
    });
    dom.send({ type: "change", model: changeTab() });
    dom.send({
      type: "runs",
      model: {
        busy: false,
        headline: "One run",
        cards: [card({ runs: [card()] })],
        earlier: [],
      },
    });
    dom.send({
      type: "running",
      items: [{ id: 1, label: "x", elapsed: "0:01", stopping: false }],
    });
    dom.send({ type: "tab", tab: "change" });
    dom.send({ type: "tab", tab: "runs" });
    expect(dom.el("now-ctx").innerHTML).toContain("verdicts match this tree");
    expect(dom.el("change-body").innerHTML).toContain(
      "2 changes need your decision",
    );
    expect(dom.el("runs-body").innerHTML).toContain("One run");
    expect(dom.el("runs-running").innerHTML).toContain("Running now");
  });
});
