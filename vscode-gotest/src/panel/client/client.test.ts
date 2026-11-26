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
import type { ChangeTab, NowModel, RunCard, RunsTab } from "../protocol.js";

const H = clientHelpers();
const NOW = renderNow(H);
const CHANGE = renderChange(H);
const RUNS = renderRuns(H);

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
    packages: Array.from({ length: 14 }, (_, i) => ({
      pkg: `m/p${i}`,
      name: `p${i}`,
      counts: `${i} passed`,
      tone: i === 0 ? ("fail" as const) : ("pass" as const),
      time: "1s",
    })),
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
      NOW.tree([{ d: "x", k: "block", s: "fail", pos: "0" }], undefined, {}),
    ).toContain('<span class="sr">failed</span>');
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

function app(
  saved: unknown = null,
  initial: { tab?: "now" | "change" | "runs" } = {},
) {
  const dom = fakeDom();
  dom.setState(saved);
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
        open: ["m/p1"],
        changeOpen: ["./shop"],
        search: "cart",
        filters: ["fail"],
      },
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

  it("shows ten package rows and the rest behind one button; a filter shows all that match", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    expect(dom.el("now-packages").children).toHaveLength(10);
    expect(dom.el("now-more").innerHTML).toContain(
      "4 more packages, all passed",
    );
    click(a, button({ act: "allPackages" }));
    expect(dom.el("now-packages").children).toHaveLength(14);
    const filtered = nowModel({
      filtering: true,
      packages: nowModel().packages.map((p, i) => ({ ...p, matches: i % 2 })),
    });
    a.receive({ type: "now", model: filtered });
    expect(dom.el("now-packages").children).toHaveLength(7);
  });

  it("opens a package by asking the host for its rows, and draws them when they come", () => {
    const { dom, a } = app();
    a.receive({ type: "now", model: nowModel() });
    dom.posted.length = 0;
    const head = dom.el("now-packages").children[2].children[0];
    click(a, head);
    expect(dom.posted).toEqual([{ type: "openPackage", pkg: "m/p2" }]);
    expect(dom.state()!.open).toEqual(["m/p2"]);
    const tree = dom.el("now-packages").children[2].children[1];
    expect(tree.hidden).toBe(false);
    expect(tree.innerHTML).toContain("Loading the rows…");
    a.receive({
      type: "package",
      pkg: "m/p2",
      rev: 1,
      nodes: [{ d: "adds", k: "block", s: "pass", pos: "0" }],
    });
    expect(tree.innerHTML).toContain("adds");
    click(a, head);
    expect(dom.posted.at(-1)).toEqual({ type: "closePackage", pkg: "m/p2" });
    expect(tree.hidden).toBe(true);
  });

  it("keeps an opened package's rows in place when a new summary comes, only its head changing", () => {
    const { dom, a } = app({ open: ["m/p0"] });
    a.receive({ type: "now", model: nowModel() });
    a.receive({
      type: "package",
      pkg: "m/p0",
      rev: 1,
      nodes: [{ d: "kept", k: "block", s: "pass", pos: "0" }],
    });
    const row = dom.el("now-packages").children[0];
    a.receive({
      type: "now",
      model: nowModel({
        packages: nowModel().packages.map((p) => ({
          ...p,
          counts: "live " + p.counts,
        })),
      }),
    });
    expect(dom.el("now-packages").children[0]).toBe(row);
    expect(row.children[0].innerHTML).toContain("live 0 passed");
    expect(row.children[1].innerHTML).toContain("kept");
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
