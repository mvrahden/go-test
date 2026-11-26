// The webview's application: it keeps each tab's last model and draws it at
// once on a switch, patches the package rows in place, and asks the host
// for what it lacks. Embedded by source (see helpers.ts); the DOM it gets is
// the webview's, or a test's stand-in.
/* eslint-disable @typescript-eslint/no-explicit-any */
import type { ClientState, HostMessage, Tab } from "../protocol.js";
import type { ChangeRender, ChangeUi } from "./renderChange.js";
import type { CardState, NowView } from "./renderNow.js";
import type { RunsRender, RunsUi } from "./renderRuns.js";

export interface AppEnv {
  vscode: {
    postMessage(msg: unknown): void;
    getState(): unknown;
    setState(state: unknown): void;
  };
  document: any;
  window: any;
  initial: { tab?: Tab; has: { change: boolean; runs: boolean } };
  NOW: NowView;
  CHANGE: ChangeRender;
  RUNS: RunsRender;
}

export function panelApp(env: AppEnv) {
  const { vscode, NOW, CHANGE, RUNS } = env;
  const doc = env.document;
  const win = env.window;
  const tabs: Tab[] = ["now", "change", "runs"];
  const isTab = (v: unknown): v is Tab => tabs.indexOf(v as Tab) >= 0;
  const saved = (vscode.getState() || {}) as Partial<ClientState> & {
    models?: Record<string, any>;
    badges?: { now: number; change: number };
  };
  const has = env.initial.has;
  const shown = (t: Tab) => t === "now" || has[t];
  const first: Tab = env.initial.tab ?? (isTab(saved.tab) ? saved.tab : "now");

  const st = {
    tab: shown(first) ? first : ("now" as Tab),
    models: (saved.models || {}) as Record<Tab, any>,
    badges: saved.badges || { now: 0, change: 0 },
    open: Array.isArray(saved.open) ? saved.open.slice() : ([] as string[]),
    search: typeof saved.search === "string" ? saved.search : "",
    filters: Array.isArray(saved.filters)
      ? saved.filters.slice()
      : ([] as string[]),
    scroll: (saved.scroll || {}) as Record<string, number>,
    // Scroll positions still to apply once a tab's content is drawn.
    pendingScroll: { ...(saved.scroll || {}) } as Record<string, number>,
    trees: {} as Record<
      string,
      { rev: number; nodes: any[]; failure?: any[]; drawn?: boolean }
    >,
    treeOpen: {} as Record<string, Record<string, boolean>>,
    busy: {} as Record<string, boolean>,
    running: [] as any[],
    allPackages: false,
    menu: false,
    cards: { outputs: {}, shown: {}, noSuites: false } as CardState,
    change: {
      allDecisions: false,
      showAccepted: false,
      review: {},
      open: {} as Record<string, boolean>,
      rows: {},
    } as ChangeUi,
    runs: { expanded: {}, command: {}, earlier: false } as RunsUi,
  };
  for (const p of Array.isArray(saved.changeOpen) ? saved.changeOpen : [])
    st.change.open[p] = true;

  const $ = (id: string) => doc.getElementById(id);
  const post = (m: unknown) => vscode.postMessage(m);

  const save = () =>
    vscode.setState({
      tab: st.tab,
      open: st.open,
      changeOpen: Object.keys(st.change.open).filter((k) => st.change.open[k]),
      search: st.search,
      filters: st.filters,
      scroll: st.scroll,
      models: st.models,
      badges: st.badges,
    });

  // --- header ---

  const PRIMARY: Record<Tab, string> = {
    now: "Run changed",
    change: "Run what changed",
    runs: "Import a CI run",
  };

  const MENU: Record<Tab, [string, string][]> = {
    now: [
      ["rerunFailed", "Rerun failed"],
      ["refresh", "Refresh"],
      ["copySpec", "Copy as Markdown"],
      ["expandAll", "Expand all opened packages"],
      ["collapseAll", "Collapse all packages"],
      ["clearResults", "Clear results"],
    ],
    change: [
      ["changeRefresh", "Audit again from source"],
      ["chooseBase", "Compare with another base…"],
      ["cancel", "Stop the audit"],
    ],
    runs: [
      ["runsRefresh", "List the records again"],
      ["runChanged", "Run changed"],
    ],
  };

  const renderHeader = () => {
    for (const t of tabs) {
      const b = $(`tab-btn-${t}`);
      if (!b) continue;
      b.hidden = !shown(t);
      b.setAttribute("aria-selected", String(t === st.tab));
      b.setAttribute("tabindex", t === st.tab ? "0" : "-1");
      const sec = $(`tab-${t}`);
      if (sec) sec.hidden = t !== st.tab;
    }
    const nb = $("badge-now");
    const cb = $("badge-change");
    if (nb) {
      nb.textContent = st.badges.now > 0 ? String(st.badges.now) : "";
      nb.setAttribute(
        "aria-label",
        st.badges.now > 0 ? `${st.badges.now} failed` : "",
      );
    }
    if (cb) {
      cb.textContent = st.badges.change > 0 ? String(st.badges.change) : "";
      cb.setAttribute(
        "aria-label",
        st.badges.change > 0 ? `${st.badges.change} to decide` : "",
      );
    }
    const primary = $("primary");
    if (primary) {
      primary.textContent = PRIMARY[st.tab];
      primary.className = st.tab === "runs" ? "btn" : "primary";
      const cm = st.models.change;
      primary.disabled = st.tab === "change" && !!cm && !cm.canAudit;
    }
    const up = $("updating");
    if (up) up.hidden = !st.busy[st.tab];
    const menu = $("menu");
    if (menu) {
      menu.hidden = !st.menu;
      menu.innerHTML = MENU[st.tab]
        .filter(([id]) => id !== "cancel" || !!st.models.change?.busy)
        .map(
          ([id, label]) =>
            `<button type="button" role="menuitem" class="menu-item" data-act="menuItem" data-item="${id}">${label}</button>`,
        )
        .join("");
    }
    const more = $("more");
    if (more) more.setAttribute("aria-expanded", String(st.menu));
  };

  // --- Now ---

  const pkgEls = new Map<string, { row: any; head: any; tree: any }>();

  const elOf = (pkg: string) => {
    let e = pkgEls.get(pkg);
    if (!e) {
      const row = doc.createElement("div");
      row.className = "pkg";
      const head = doc.createElement("button");
      head.setAttribute("type", "button");
      head.className = "pkg-head";
      head.dataset.act = "togglePkg";
      head.dataset.pkg = pkg;
      const tree = doc.createElement("div");
      tree.className = "pkg-tree";
      tree.dataset.pkg = pkg;
      row.appendChild(head);
      row.appendChild(tree);
      e = { row, head, tree };
      pkgEls.set(pkg, e);
    }
    return e;
  };

  const drawTree = (pkg: string) => {
    const e = pkgEls.get(pkg);
    const t = st.trees[pkg];
    if (!e) return;
    const open = st.open.indexOf(pkg) >= 0;
    e.tree.hidden = !open;
    if (!open) return;
    if (!t) {
      if (!e.tree.innerHTML)
        e.tree.innerHTML = `<div class="dim pad">Loading the rows…</div>`;
      return;
    }
    if (t.drawn) return;
    e.tree.innerHTML = NOW.tree(t.nodes, t.failure, st.treeOpen[pkg] || {});
    t.drawn = true;
  };

  const MAX_ROWS = 10;

  const drawPackages = (m: any) => {
    const list = $("now-packages");
    if (!list) return;
    const rows = (m.packages as any[]).filter(
      (r) => !m.filtering || (r.matches ?? 1) > 0,
    );
    const cut = !m.filtering && !st.allPackages && rows.length > MAX_ROWS + 2;
    const visible = cut ? rows.slice(0, MAX_ROWS) : rows;
    const keep = new Set<string>();
    for (const r of visible) {
      keep.add(r.pkg);
      const e = elOf(r.pkg);
      const open = st.open.indexOf(r.pkg) >= 0;
      e.head.innerHTML = NOW.pkgHead(r, open);
      e.head.setAttribute("aria-expanded", String(open));
      if (r.title) e.head.setAttribute("title", r.title);
      list.appendChild(e.row);
      drawTree(r.pkg);
    }
    for (const [pkg, e] of Array.from(pkgEls)) {
      if (keep.has(pkg)) continue;
      e.row.remove();
      pkgEls.delete(pkg);
      if (st.trees[pkg]) st.trees[pkg].drawn = false;
    }
    const more = $("now-more");
    if (more) {
      const hidden = rows.slice(MAX_ROWS);
      more.innerHTML = cut
        ? NOW.more(
            hidden.length,
            hidden.every((r) => r.tone === "pass"),
          )
        : "";
    }
    if (m.filtering && rows.length === 0)
      list.setAttribute("data-empty", "No behavior matches.");
    else list.removeAttribute("data-empty");
  };

  const drawNow = () => {
    const m = st.models.now;
    const empty = $("now-empty");
    const parts = ["now-ctx", "now-chips", "now-attention", "now-all"];
    if (!m || m.empty) {
      for (const p of parts) if ($(p)) $(p).hidden = true;
      // Nothing kept: the rows of what was cleared go too.
      if (m?.empty) {
        for (const e of pkgEls.values()) e.row.remove();
        pkgEls.clear();
        st.trees = {};
        $("now-attention").innerHTML = "";
      }
      if (empty) {
        empty.hidden = false;
        empty.innerHTML = m?.empty
          ? `<div class="empty"><h1>${NOW_ESC(m.empty.title)}</h1><p>${NOW_ESC(m.empty.text)}</p><button type="button" class="primary" data-act="runTests">Run all tests</button></div>`
          : `<div class="dim pad" role="status">Loading…</div>`;
      }
      return;
    }
    if (empty) {
      empty.hidden = true;
      empty.innerHTML = "";
    }
    for (const p of parts) if ($(p)) $(p).hidden = false;
    $("now-ctx").innerHTML = NOW.context(m);
    $("now-chips").innerHTML = NOW.chips(m, st.filters);
    $("now-chips").hidden = m.chips.length === 0;
    $("now-attention").innerHTML = NOW.attention(m, st.cards);
    $("now-total").textContent = m.total;
    $("now-groups").textContent = (m.groups || []).join(" · ");
    drawPackages(m);
    applyScroll("now");
  };

  const NOW_ESC = (s: string) =>
    String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");

  // --- Change and Runs ---

  const drawChange = () => {
    const m = st.models.change;
    const ctx = $("change-ctx");
    const body = $("change-body");
    if (!ctx || !body) return;
    if (!m) {
      ctx.innerHTML = "";
      body.innerHTML = `<div class="dim pad" role="status">Loading…</div>`;
      return;
    }
    ctx.innerHTML = m.empty && !m.folders ? "" : CHANGE.context(m);
    body.innerHTML = CHANGE.body(m, st.change);
    applyScroll("change");
  };

  const drawRunning = () => {
    const el = $("runs-running");
    if (el) el.innerHTML = RUNS.running(st.running);
  };

  const drawRuns = () => {
    const m = st.models.runs;
    const ctx = $("runs-ctx");
    const body = $("runs-body");
    drawRunning();
    if (!body) return;
    if (!m) {
      body.innerHTML = `<div class="dim pad" role="status">Loading…</div>`;
      return;
    }
    if (ctx) ctx.innerHTML = RUNS.folders(m);
    body.innerHTML = RUNS.body(m, st.runs);
    applyScroll("runs");
  };

  const draw: Record<Tab, () => void> = {
    now: drawNow,
    change: drawChange,
    runs: drawRuns,
  };

  const applyScroll = (t: Tab) => {
    const y = st.pendingScroll[t];
    if (y === undefined || t !== st.tab) return;
    const sec = $(`tab-${t}`);
    if (sec) sec.scrollTop = y;
    delete st.pendingScroll[t];
  };

  // setTab shows a tab's last content at once; the host is told after.
  const setTab = (t: Tab, tell = true) => {
    if (!isTab(t) || !shown(t)) return;
    const sec = $(`tab-${st.tab}`);
    if (sec) st.scroll[st.tab] = sec.scrollTop;
    st.tab = t;
    st.menu = false;
    if (st.scroll[t] !== undefined) st.pendingScroll[t] = st.scroll[t];
    renderHeader();
    draw[t]();
    save();
    if (tell) post({ type: "tab", tab: t });
  };

  // --- what the host sends ---

  const receive = (msg: HostMessage) => {
    if (!msg || typeof msg !== "object") return;
    switch (msg.type) {
      case "tab":
        setTab(msg.tab, false);
        return;
      case "busy":
        st.busy[msg.tab] = msg.busy;
        renderHeader();
        return;
      case "badges":
        st.badges = { now: msg.now, change: msg.change };
        renderHeader();
        save();
        return;
      case "now": {
        const old = st.models.now;
        if (old && old.rev !== msg.model.rev) st.cards.outputs = {};
        st.models.now = msg.model;
        save();
        if (st.tab === "now") drawNow();
        return;
      }
      case "package":
        st.trees[msg.pkg] = {
          rev: msg.rev,
          nodes: msg.nodes,
          failure: msg.failure,
        };
        drawTree(msg.pkg);
        return;
      case "output":
        st.cards.outputs[msg.key] = { lines: msg.lines, more: msg.more };
        if (st.tab === "now" && st.models.now)
          $("now-attention").innerHTML = NOW.attention(st.models.now, st.cards);
        return;
      case "change":
        st.models.change = msg.model;
        st.change.rows = {};
        for (const p of Object.keys(st.change.open))
          if (st.change.open[p])
            post({ type: "change", act: "changePackage", key: p });
        save();
        renderHeader();
        if (st.tab === "change") drawChange();
        return;
      case "changePackage":
        (st.change.rows as any)[msg.pkg] = msg.rows;
        if (st.tab === "change") drawChange();
        return;
      case "runs":
        st.models.runs = msg.model;
        save();
        if (st.tab === "runs") drawRuns();
        return;
      case "running":
        st.running = msg.items;
        if (st.tab === "runs") drawRunning();
        return;
    }
  };

  // --- what the user does ---

  const posOf = (el: any): { pkg?: string; pos?: string; id?: string } => {
    const row = el.closest("[data-pos]");
    const owner = el.closest("[data-pkg]");
    return {
      pkg: owner ? owner.dataset.pkg : undefined,
      pos: el.dataset.pos ?? (row ? row.dataset.pos : undefined),
      id: el.dataset.id ?? (row ? row.dataset.id : undefined),
    };
  };

  const menuItem = (item: string) => {
    st.menu = false;
    renderHeader();
    switch (item) {
      case "expandAll":
        for (const p of st.open) {
          const e = pkgEls.get(p);
          if (!e) continue;
          const all = e.tree.querySelectorAll("details");
          for (let i = 0; i < all.length; i++) all[i].open = true;
        }
        return;
      case "collapseAll":
        for (const p of st.open.slice()) post({ type: "closePackage", pkg: p });
        st.open = [];
        save();
        if (st.models.now) drawPackages(st.models.now);
        return;
      case "changeRefresh":
        post({ type: "change", act: "refresh" });
        return;
      case "chooseBase":
      case "cancel":
        post({ type: "change", act: item });
        return;
      case "runsRefresh":
        post({ type: "runs", act: "refresh" });
        return;
      case "runChanged":
        post({ type: "runChanged" });
        return;
      case "clearResults":
        st.models.now = undefined;
        st.trees = {};
        save();
        post({ type: "clearResults" });
        return;
      default:
        post({ type: item });
    }
  };

  const act = (el: any) => {
    const a = el.dataset.act as string;
    const d = el.dataset;
    switch (a) {
      case "tab":
        setTab(d.tab);
        return;
      case "menu":
        st.menu = !st.menu;
        renderHeader();
        if (st.menu) {
          const firstItem = $("menu")?.querySelector?.("button");
          firstItem?.focus?.();
        }
        return;
      case "menuItem":
        menuItem(d.item);
        return;
      case "primary":
        if (st.tab === "now") post({ type: "runChanged" });
        else if (st.tab === "change") post({ type: "change", act: "auditRun" });
        else post({ type: "runs", act: "import" });
        return;
      case "togglePkg": {
        const pkg = d.pkg as string;
        const i = st.open.indexOf(pkg);
        if (i >= 0) {
          st.open.splice(i, 1);
          post({ type: "closePackage", pkg });
        } else {
          st.open.push(pkg);
          if (st.trees[pkg]) st.trees[pkg].drawn = false;
          post({ type: "openPackage", pkg });
        }
        save();
        if (st.models.now) drawPackages(st.models.now);
        return;
      }
      case "allPackages":
        st.allPackages = true;
        if (st.models.now) drawPackages(st.models.now);
        return;
      case "chip": {
        const s = d.status as string;
        const i = st.filters.indexOf(s);
        if (i >= 0) st.filters.splice(i, 1);
        else st.filters.push(s);
        save();
        if (st.models.now)
          $("now-chips").innerHTML = NOW.chips(st.models.now, st.filters);
        post({ type: "filters", filters: st.filters });
        return;
      }
      case "noSuites":
        st.cards.noSuites = !st.cards.noSuites;
        if (st.models.now)
          $("now-attention").innerHTML = NOW.attention(st.models.now, st.cards);
        return;
      case "fullOutput": {
        const key = d.key as string;
        st.cards.shown[key] = !st.cards.shown[key];
        const owner = el.closest("[data-pkg]");
        if (st.cards.shown[key] && !st.cards.outputs[key])
          post({ type: "output", key, pkg: owner?.dataset.pkg, pos: d.pos });
        if (st.models.now)
          $("now-attention").innerHTML = NOW.attention(st.models.now, st.cards);
        return;
      }
      case "run":
      case "debug":
        if (d.scope === "change") {
          post({ type: "change", act: a, key: d.key });
          return;
        }
        post({ type: a, ...posOf(el) });
        return;
      case "copyNode": {
        const p = posOf(el);
        post({
          type: "copyNode",
          pkg: p.pkg,
          position: p.pos ? p.pos.split(".").map(Number) : [],
        });
        return;
      }
      case "goTo":
        post({ type: "goToLocation", file: d.file, line: Number(d.line) });
        return;
      case "refresh":
      case "runTests":
        post({ type: a });
        return;
      case "changePackage": {
        const pkg = d.pkg as string;
        st.change.open[pkg] = !st.change.open[pkg];
        if (st.change.open[pkg] && !(st.change.rows as any)[pkg])
          post({ type: "change", act: "changePackage", key: pkg });
        save();
        drawChange();
        return;
      }
      case "review":
        st.change.review[d.group] = !st.change.review[d.group];
        drawChange();
        return;
      case "allDecisions":
        st.change.allDecisions = true;
        drawChange();
        return;
      case "showAccepted":
        st.change.showAccepted = !st.change.showAccepted;
        drawChange();
        return;
      case "expandRuns":
        st.runs.expanded[d.key] = !st.runs.expanded[d.key];
        drawRuns();
        return;
      case "command":
        st.runs.command[d.key] = !st.runs.command[d.key];
        drawRuns();
        return;
      case "earlier":
        st.runs.earlier = !st.runs.earlier;
        drawRuns();
        return;
      case "stopSession":
        post({ type: "stopSession", id: Number(d.session) });
        return;
      case "evidence":
        if (d.scope === "change") {
          setTab("runs");
          return;
        }
        post({ type: "runs", act: a, key: d.key });
        return;
      default:
        if (d.scope === "change" || d.scope === "runs")
          post({ type: d.scope, act: a, key: d.key });
    }
  };

  const onClick = (e: any) => {
    const t = e.target;
    const el = t && t.closest ? t.closest("[data-act]") : null;
    if (!el) {
      if (st.menu && !(t && t.closest && t.closest("#menu"))) {
        st.menu = false;
        renderHeader();
      }
      return;
    }
    if (el.disabled) return;
    e.preventDefault();
    e.stopPropagation?.();
    act(el);
  };

  let searchTimer: any;
  const onInput = (e: any) => {
    const t = e.target;
    if (!t || t.id !== "search") return;
    win.clearTimeout(searchTimer);
    searchTimer = win.setTimeout(() => {
      st.search = t.value;
      save();
      post({ type: "search", text: st.search });
    }, 200);
  };

  const onChange = (e: any) => {
    const t = e.target;
    if (!t || !t.dataset) return;
    if (t.dataset.change === "source")
      post({ type: "selectSource", id: t.value });
    else if (t.dataset.change === "folder")
      post({ type: t.dataset.scope, act: "folder", folder: t.value });
  };

  const onToggle = (e: any) => {
    const t = e.target;
    if (!t || !t.matches || !t.matches("details.branch")) return;
    const owner = t.closest("[data-pkg]");
    if (!owner) return;
    const pkg = owner.dataset.pkg;
    (st.treeOpen[pkg] ||= {})[t.dataset.pos] = !!t.open;
  };

  let scrollTimer: any;
  const onScroll = (e: any) => {
    const t = e.target;
    if (!t || t.id !== `tab-${st.tab}`) return;
    win.clearTimeout(scrollTimer);
    scrollTimer = win.setTimeout(() => {
      st.scroll[st.tab] = t.scrollTop;
      save();
    }, 150);
  };

  const onKey = (e: any) => {
    if (e.key === "Escape" && st.menu) {
      st.menu = false;
      renderHeader();
      $("more")?.focus?.();
      return;
    }
    const t = e.target;
    if (
      t &&
      t.getAttribute &&
      t.getAttribute("role") === "tab" &&
      (e.key === "ArrowRight" || e.key === "ArrowLeft")
    ) {
      const list = tabs.filter(shown);
      const i = list.indexOf(st.tab);
      const next =
        list[
          (i + (e.key === "ArrowRight" ? 1 : list.length - 1)) % list.length
        ];
      setTab(next);
      $(`tab-btn-${next}`)?.focus?.();
      e.preventDefault();
    }
  };

  doc.addEventListener("click", onClick);
  doc.addEventListener("input", onInput);
  doc.addEventListener("change", onChange);
  doc.addEventListener("toggle", onToggle, true);
  doc.addEventListener("scroll", onScroll, true);
  doc.addEventListener("keydown", onKey);
  win.addEventListener("message", (e: any) => receive(e.data));

  const box = $("search");
  if (box) box.value = st.search;
  renderHeader();
  for (const t of tabs) draw[t]();
  post({
    type: "ready",
    tab: st.tab,
    open: st.open,
    changeOpen: Object.keys(st.change.open).filter((k) => st.change.open[k]),
    search: st.search,
    filters: st.filters,
  });

  return { st, receive, onClick, onInput };
}
