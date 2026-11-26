// The webview's application: it keeps each tab's last model and draws it at
// once on a switch, patches the package rows in place, and asks the host
// for what it lacks. Embedded by source (see helpers.ts); the DOM it gets is
// the webview's, or a test's stand-in.
/* eslint-disable @typescript-eslint/no-explicit-any */
import type { ClientState, Depth, HostMessage, Tab } from "../protocol.js";
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
    // Each package's rows, with the summary and chips they were asked under.
    trees: {} as Record<
      string,
      {
        key: string;
        nodes: any[];
        failure?: any[];
        drawn?: boolean;
      }
    >,
    pending: {} as Record<string, string>,
    treeOpen: {} as Record<string, Record<string, boolean>>,
    depth: (saved.depth === "suites" || saved.depth === "behaviors"
      ? saved.depth
      : "packages") as Depth,
    shut: Array.isArray(saved.shut) ? saved.shut.slice() : ([] as string[]),
    folders: (saved.folders || {}) as Record<string, boolean>,
    deep: Array.isArray(saved.deep) ? saved.deep.slice() : ([] as string[]),
    deepSuites: (saved.deepSuites || {}) as Record<string, string[]>,
    // What the host found for the search on show.
    found: undefined as
      | {
          query: string;
          rev: number;
          results: any[];
          total: number;
          shown: number;
        }
      | undefined,
    busy: {} as Record<string, boolean>,
    running: [] as any[],
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
      depth: st.depth,
      shut: st.shut,
      folders: st.folders,
      deep: st.deep,
      deepSuites: st.deepSuites,
    });

  // --- header ---

  const PRIMARY: Record<Tab, string> = {
    now: "Run changed",
    change: "Run what changed",
    runs: "Import a CI run",
  };

  const MENU: Record<Tab, [string, string][]> = {
    now: [
      ["runTests", "Run all tests"],
      ["rerunFailed", "Rerun failed"],
      ["-", ""],
      ["refresh", "Refresh"],
      ["copySpec", "Copy as Markdown"],
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
        .map(([id, label]) =>
          id === "-"
            ? `<div class="menu-sep" role="separator"></div>`
            : `<button type="button" role="menuitem" class="menu-item" data-act="menuItem" data-item="${id}">${label}</button>`,
        )
        .join("");
    }
    const more = $("more");
    if (more) more.setAttribute("aria-expanded", String(st.menu));
  };

  // --- Now ---

  const pkgEls = new Map<string, { row: any; head: any; tree: any }>();
  const folderEls = new Map<string, { wrap: any; head: any; body: any }>();

  const searching = () => st.search.trim() !== "";
  // The key a package's rows are asked under: a new summary or other chips
  // make the rows on show out of date.
  const treeKey = () => `${st.models.now?.rev ?? 0}|${st.filters.join(",")}`;

  const isOpen = (pkg: string) =>
    st.deep.indexOf(pkg) >= 0 ||
    (st.depth === "packages"
      ? st.open.indexOf(pkg) >= 0
      : st.shut.indexOf(pkg) < 0);

  const folderOpen = (f: any) =>
    st.folders[f.key] ?? (st.depth !== "packages" || f.hot);

  const needTree = (pkg: string) => {
    const t = st.trees[pkg];
    const key = treeKey();
    return (!t || t.key !== key) && st.pending[pkg] !== key;
  };

  const request = (pkg: string) => {
    if (!needTree(pkg)) return;
    st.pending[pkg] = treeKey();
    post({ type: "openPackage", pkg });
  };

  // Rows load as their package scrolls into view; without an observer, at once.
  const IO = win.IntersectionObserver;
  const io = IO
    ? new IO(
        (entries: any[]) => {
          for (const e of entries) {
            if (!e.isIntersecting) continue;
            io.unobserve(e.target);
            const pkg = e.target.dataset.pkg;
            if (pkg && isOpen(pkg)) request(pkg);
          }
        },
        { root: $("tab-now"), rootMargin: "400px" },
      )
    : undefined;

  const elOf = (pkg: string) => {
    let e = pkgEls.get(pkg);
    if (!e) {
      const row = doc.createElement("div");
      row.className = "pkg";
      row.dataset.pkg = pkg;
      const head = doc.createElement("div");
      head.className = "pkg-head";
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

  const folderEl = (key: string) => {
    let e = folderEls.get(key);
    if (!e) {
      const wrap = doc.createElement("div");
      wrap.className = "folder";
      const head = doc.createElement("button");
      head.setAttribute("type", "button");
      head.className = "folder-head";
      head.dataset.act = "toggleFolder";
      head.dataset.folder = key;
      const body = doc.createElement("div");
      body.className = "folder-body";
      wrap.appendChild(head);
      wrap.appendChild(body);
      e = { wrap, head, body };
      folderEls.set(key, e);
    }
    return e;
  };

  const treeOpenOf = (pkg: string) => ({
    open: st.treeOpen[pkg] || {},
    deep: st.deep.indexOf(pkg) >= 0,
    deepSuites: st.deepSuites[pkg] || [],
    depth: st.depth,
  });

  const drawTree = (pkg: string) => {
    const e = pkgEls.get(pkg);
    if (!e) return;
    const open = isOpen(pkg);
    e.tree.hidden = !open;
    if (!open) return;
    const t = st.trees[pkg];
    if (needTree(pkg)) {
      if (io) io.observe(e.row);
      else request(pkg);
    }
    if (!t) {
      if (!e.tree.innerHTML)
        e.tree.innerHTML = `<div class="dim pad">Loading the rows…</div>`;
      return;
    }
    if (t.drawn) return;
    e.tree.innerHTML = NOW.tree(t.nodes, t.failure, treeOpenOf(pkg));
    t.drawn = true;
  };

  const redrawTree = (pkg: string) => {
    if (st.trees[pkg]) st.trees[pkg].drawn = false;
    drawTree(pkg);
  };

  const drawDepth = () => {
    for (const d of ["packages", "suites", "behaviors"]) {
      const b = $(`depth-${d}`);
      if (b) b.setAttribute("aria-pressed", String(st.depth === d));
    }
  };

  const drawFound = (m: any) => {
    const out = $("now-found");
    if (!out) return;
    const f = st.found;
    if (!f || f.query !== st.search.trim()) {
      out.innerHTML = `<div class="dim pad" role="status">Searching…</div>`;
      return;
    }
    const by = new Map(f.results.map((r: any) => [r.pkg, r.nodes]));
    const parts = [NOW.foundNote(f.total, f.shown)];
    for (const folder of m.folders as any[]) {
      const rows = folder.rows.filter((r: any) => by.has(r.pkg));
      if (rows.length === 0) continue;
      parts.push(
        `<div class="folder"><div class="folder-head static">${NOW.folderHead(folder, true, rows.length)}</div><div class="folder-body">${rows
          .map(
            (r: any) =>
              `<div class="pkg"><div class="pkg-head static" data-pkg="${H_ESC(r.pkg)}"><span class="mono name">${H_ESC(r.name)}</span><span class="grow"></span><span class="counts t-${r.tone}">${H_ESC(r.counts)}</span></div><div class="pkg-tree" data-pkg="${H_ESC(r.pkg)}">${NOW.tree(by.get(r.pkg), undefined, { open: {}, deep: true, deepSuites: [], depth: "behaviors" })}</div></div>`,
          )
          .join("")}</div></div>`,
      );
    }
    out.innerHTML = parts.join("");
  };

  const H_ESC = (s: string) =>
    String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");

  const drawPackages = (m: any) => {
    const list = $("now-packages");
    if (!list) return;
    drawDepth();
    const found = $("now-found");
    if (searching()) {
      list.hidden = true;
      if (found) found.hidden = false;
      drawFound(m);
      return;
    }
    list.hidden = false;
    if (found) {
      found.hidden = true;
      found.innerHTML = "";
    }
    const keepF = new Set<string>();
    const keepP = new Set<string>();
    for (const f of m.folders as any[]) {
      const rows = f.rows.filter(
        (r: any) => !m.filtering || (r.matches ?? 1) > 0,
      );
      if (m.filtering && rows.length === 0) continue;
      const fe = folderEl(f.key);
      const open = folderOpen(f);
      fe.head.innerHTML = NOW.folderHead(
        f,
        open,
        m.filtering ? rows.length : undefined,
      );
      fe.head.setAttribute("aria-expanded", String(open));
      fe.body.hidden = !open;
      list.appendChild(fe.wrap);
      keepF.add(f.key);
      if (!open) continue;
      for (const r of rows) {
        keepP.add(r.pkg);
        const e = elOf(r.pkg);
        e.head.innerHTML = NOW.pkgHead(
          r,
          isOpen(r.pkg),
          st.deep.indexOf(r.pkg) >= 0,
        );
        if (r.title) e.head.setAttribute("title", r.title);
        fe.body.appendChild(e.row);
        drawTree(r.pkg);
      }
    }
    for (const [key, e] of Array.from(folderEls)) {
      if (keepF.has(key)) continue;
      e.wrap.remove();
      folderEls.delete(key);
    }
    for (const [pkg, e] of Array.from(pkgEls)) {
      if (keepP.has(pkg)) continue;
      io?.unobserve(e.row);
      e.row.remove();
      pkgEls.delete(pkg);
      if (st.trees[pkg]) st.trees[pkg].drawn = false;
    }
    if (m.filtering && keepP.size === 0)
      list.setAttribute("data-empty", "No behavior matches.");
    else list.removeAttribute("data-empty");
  };

  // branchPositions lists the places of a package's branches at or under a
  // place ("" for all).
  const branchPositions = (nodes: any[], under: string): string[] => {
    const out: string[] = [];
    const walk = (list: any[]) => {
      for (const n of list) {
        if (!n.c) continue;
        if (!under || n.pos === under || n.pos.startsWith(under + "."))
          out.push(n.pos);
        walk(n.c);
      }
    };
    walk(nodes);
    return out;
  };

  const setDepth = (d: Depth) => {
    st.depth = d;
    st.open = [];
    st.shut = [];
    st.treeOpen = {};
    st.deep = [];
    st.deepSuites = {};
    if (d !== "packages") st.folders = {};
    for (const t of Object.values(st.trees)) t.drawn = false;
    save();
    if (st.models.now) drawPackages(st.models.now);
  };

  const drawNow = () => {
    const m = st.models.now;
    const empty = $("now-empty");
    const parts = ["now-ctx", "now-chips", "now-attention", "now-all"];
    if (!m || m.empty) {
      for (const p of parts) if ($(p)) $(p).hidden = true;
      // Nothing kept: the rows of what was cleared go too.
      if (m?.empty) {
        for (const e of folderEls.values()) e.wrap.remove();
        folderEls.clear();
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
        if (old && old.rev !== msg.model.rev) {
          st.cards.outputs = {};
          // A search answers for one summary: ask again of the new one.
          if (searching()) post({ type: "search", text: st.search });
        }
        st.models.now = msg.model;
        save();
        if (st.tab === "now") drawNow();
        return;
      }
      case "package":
        st.trees[msg.pkg] = {
          key: st.pending[msg.pkg] ?? treeKey(),
          nodes: msg.nodes,
          failure: msg.failure,
        };
        delete st.pending[msg.pkg];
        drawTree(msg.pkg);
        return;
      case "found":
        if (msg.query !== st.search.trim()) return;
        st.found = msg;
        if (st.tab === "now" && st.models.now) drawPackages(st.models.now);
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
        const pkg = el.closest("[data-pkg]")?.dataset.pkg as string;
        if (!pkg) return;
        const open = isOpen(pkg);
        const list = st.depth === "packages" ? st.open : st.shut;
        const i = list.indexOf(pkg);
        if (st.depth === "packages" ? open : !open) {
          if (i >= 0) list.splice(i, 1);
        } else if (i < 0) list.push(pkg);
        if (open) st.deep = st.deep.filter((p) => p !== pkg);
        save();
        if (st.models.now) drawPackages(st.models.now);
        return;
      }
      case "toggleFolder": {
        const f = st.models.now?.folders.find((x: any) => x.key === d.folder);
        if (!f) return;
        st.folders[f.key] = !folderOpen(f);
        save();
        drawPackages(st.models.now);
        return;
      }
      case "depth":
        setDepth(d.depth as Depth);
        return;
      case "deep": {
        const pkg = (d.pkg ?? el.closest("[data-pkg]")?.dataset.pkg) as string;
        if (!pkg) return;
        const nodes = st.trees[pkg]?.nodes ?? [];
        const closed = (under: string) => {
          const map = (st.treeOpen[pkg] ||= {});
          for (const pos of branchPositions(nodes, under)) map[pos] = false;
        };
        if (d.pos === undefined) {
          if (st.deep.indexOf(pkg) >= 0) {
            st.deep = st.deep.filter((p) => p !== pkg);
            st.deepSuites[pkg] = [];
            closed("");
          } else {
            // Open in its own right, so a collapse leaves its suites showing.
            if (!isOpen(pkg)) {
              if (st.depth === "packages") st.open.push(pkg);
              else st.shut = st.shut.filter((p) => p !== pkg);
            }
            st.deep.push(pkg);
            st.treeOpen[pkg] = {};
          }
        } else {
          const list = (st.deepSuites[pkg] ||= []);
          const i = list.indexOf(d.pos);
          if (i >= 0) {
            list.splice(i, 1);
            closed(d.pos);
          } else {
            list.push(d.pos);
            const map = st.treeOpen[pkg] || {};
            for (const k of Object.keys(map))
              if (k === d.pos || k.startsWith(d.pos + ".")) delete map[k];
          }
        }
        save();
        if (st.models.now) drawPackages(st.models.now);
        redrawTree(pkg);
        return;
      }
      case "chip": {
        const s = d.status as string;
        const i = st.filters.indexOf(s);
        if (i >= 0) st.filters.splice(i, 1);
        else st.filters.push(s);
        save();
        if (st.models.now)
          $("now-chips").innerHTML = NOW.chips(st.models.now, st.filters);
        post({ type: "filters", filters: st.filters });
        if (searching()) post({ type: "search", text: st.search });
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
    if (!el) noteBranch(t);
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
      st.found = undefined;
      save();
      if (searching()) post({ type: "search", text: st.search });
      if (st.models.now) drawPackages(st.models.now);
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

  const noteBranch = (t: any) => {
    const sum = t && t.closest ? t.closest("summary") : null;
    const det = sum ? sum.closest("details.branch") : null;
    const owner = det ? det.closest("[data-pkg]") : null;
    if (!det || !owner) return;
    (st.treeOpen[owner.dataset.pkg] ||= {})[det.dataset.pos] = !det.open;
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
    changeOpen: Object.keys(st.change.open).filter((k) => st.change.open[k]),
    search: st.search,
    filters: st.filters,
  });
  // A search kept across a reload asks again.
  if (searching()) post({ type: "search", text: st.search });

  return { st, receive, onClick, onInput };
}
