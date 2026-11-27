// The webview's application: it keeps each tab's last model and draws it at
// once on a switch, patches the package rows in place, and asks the host
// for what it lacks. Embedded by source (see helpers.ts); the DOM it gets is
// the webview's, or a test's stand-in.
/* eslint-disable @typescript-eslint/no-explicit-any */
import type { ClientState, HostMessage, Tab } from "../protocol.js";
import type { ChangeRender, ChangeUi } from "./renderChange.js";
import type { CardState, NowView } from "./renderNow.js";
import type { RunsRender, RunsUi } from "./renderRuns.js";
import type { TreeView } from "./renderTree.js";
import type { Level, Slice, TreeLogic, TreeState, VRow } from "./tree.js";

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
  TREE: TreeLogic;
  TREEVIEW: TreeView;
}

export function panelApp(env: AppEnv) {
  const { vscode, NOW, CHANGE, RUNS, TREE, TREEVIEW } = env;
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
    tree: {
      open: saved.tree?.open ?? {},
      deep: saved.tree?.deep ?? {},
      level: (saved.tree?.level === "suites" ||
      saved.tree?.level === "behaviors"
        ? saved.tree.level
        : "packages") as Level,
      focus: saved.tree?.focus,
    } as TreeState,
    // The tree holds the focus: a redraw gives it back to the same row.
    treeFocused: false,
    // A row to bring into view and focus once it shows.
    revealing: undefined as string | undefined,
    copiedAt: 0,
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
      changeOpen: Object.keys(st.change.open).filter((k) => st.change.open[k]),
      search: st.search,
      filters: st.filters,
      scroll: st.scroll,
      models: st.models,
      badges: st.badges,
      tree: st.tree,
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

  const searching = () => st.search.trim() !== "";
  // The key a package's rows are asked under: a new summary or other chips
  // make the rows on show out of date.
  const treeKey = () => `${st.models.now?.rev ?? 0}|${st.filters.join(",")}`;

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
            const pkg = e.target.dataset.lazy || e.target.dataset.pkg;
            if (pkg) request(pkg);
          }
        },
        { root: $("tab-now"), rootMargin: "400px" },
      )
    : undefined;

  // Slices on show, out of date ones too: they show until the new arrives.
  const slices = (): Record<string, Slice | undefined> => {
    const out: Record<string, Slice | undefined> = {};
    for (const k of Object.keys(st.trees)) out[k] = st.trees[k];
    return out;
  };

  let rows: VRow[] = [];
  const visible = () => {
    rows = TREE.rows(st.models.now?.tree ?? [], slices(), st.tree);
    return rows;
  };

  const rowEl = (key: string) => {
    const list = $("now-packages");
    if (!list || !list.querySelector) return null;
    const css =
      win.CSS && win.CSS.escape
        ? win.CSS.escape(key)
        : key.replace(/"/g, '\\"');
    return list.querySelector(`[data-key="${css}"]`);
  };

  // drawList draws the tree, asks for the slices its open packages lack,
  // and gives the focus back to its row.
  const drawList = (m: any) => {
    const list = $("now-packages");
    if (!list) return;
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
    visible();
    // A row not shown (its package still loading, or closed above it) keeps
    // the focus to come back to; its nearest shown parent is the tab stop.
    let shownFocus = st.tree.focus;
    while (shownFocus && !TREE.find(rows, shownFocus))
      shownFocus = parentOf(shownFocus);
    list.innerHTML = TREEVIEW.tree(rows, TREE, shownFocus || undefined);
    if (m.filtering && rows.length === 0)
      list.setAttribute("data-empty", "No behavior matches.");
    else list.removeAttribute("data-empty");
    // Open packages without their rows, or with rows of an older summary,
    // ask for them as they come into view.
    for (const r of TREE.flat(rows)) {
      if (r.kind !== "pkg" || !r.open || !needTree(r.pkg!)) continue;
      const el = io ? rowEl(r.key) : null;
      if (el) io.observe(el);
      else request(r.pkg!);
    }
    const want =
      st.revealing && TREE.find(rows, st.revealing) ? st.revealing : undefined;
    if (want || st.treeFocused) {
      const el = shownFocus ? rowEl(shownFocus) : null;
      el?.focus?.({ preventScroll: !want });
      if (want) {
        el?.scrollIntoView?.({ block: "center" });
        st.revealing = undefined;
      }
    }
  };

  // parentOf is the key a row's parent has, from the key alone.
  const parentOf = (key: string): string => {
    const r = TREE.find(rows, key);
    if (r?.parent) return r.parent;
    const m = /^n:(.*):([\d.]+)$/.exec(key);
    if (!m) return "";
    const cut = m[2].lastIndexOf(".");
    return cut < 0 ? `p:${m[1]}` : `n:${m[1]}:${m[2].slice(0, cut)}`;
  };

  // A burst of slices draws once.
  let drawSoon = false;
  const redraw = () => {
    if (drawSoon || !st.models.now) return;
    const raf = win.requestAnimationFrame;
    if (!raf) {
      drawList(st.models.now);
      return;
    }
    drawSoon = true;
    raf(() => {
      drawSoon = false;
      if (st.models.now && st.tab === "now") drawList(st.models.now);
    });
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
    for (const e of m.tree as any[]) {
      const pkgs = TREE.pkgsBelow(e).filter((p: string) => by.has(p));
      if (pkgs.length === 0) continue;
      const rowsOf = (list: any[]): any[] =>
        list.flatMap((x) => (x.kind === "pkg" ? [x] : rowsOf(x.children)));
      const found = rowsOf([e]).filter((r) => by.has(r.pkg));
      const head =
        e.kind === "dir"
          ? `<div class="folder-head static">${NOW.folderHead(e, true, found.length)}</div>`
          : "";
      parts.push(
        `<div class="folder">${head}<div class="folder-body">${found
          .map(
            (r: any) =>
              `<div class="pkg"><div class="pkg-head static" data-pkg="${H_ESC(r.pkg)}"><span class="mono name">${H_ESC(r.name)}</span><span class="grow"></span><span class="counts t-${r.tone}">${H_ESC(r.counts)}</span></div><div class="pkg-tree" data-pkg="${H_ESC(r.pkg)}">${NOW.tree(by.get(r.pkg), undefined, { open: {}, deep: true, depth: "behaviors" })}</div></div>`,
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

  // --- what a row does ---

  const label = (r: VRow) => (r.kind === "dir" ? `${r.label}/` : r.label);

  const doOp = (op: string, r: VRow | undefined) => {
    if (!r) return;
    st.tree.focus = r.key;
    switch (op) {
      case "toggle":
        TREE.toggle(st.tree, r);
        if (r.kind === "pkg" && !r.open) request(r.pkg!);
        break;
      case "openBelow":
        TREE.openBelow(st.tree, r);
        break;
      case "closeBelow":
        TREE.closeBelow(st.tree, r);
        break;
      case "copy":
        post(
          r.kind === "dir" || r.kind === "pkg"
            ? {
                type: "copy",
                label: label(r),
                pkgs: r.kind === "pkg" ? [r.pkg] : TREE.pkgsBelow(r.entry!),
              }
            : { type: "copy", label: r.label, pkg: r.pkg, pos: r.pos },
        );
        save();
        return;
      case "run":
        post(
          r.kind === "dir"
            ? { type: "run", ids: TREE.pkgsBelow(r.entry!) }
            : r.kind === "pkg"
              ? { type: "run", id: r.pkg }
              : { type: "run", id: r.node?.id, pkg: r.pkg, pos: r.pos },
        );
        return;
      case "debug":
        post({ type: "debug", id: r.node?.id, pkg: r.pkg, pos: r.pos });
        return;
      case "source":
        if (r.node?.loc)
          post({
            type: "goToLocation",
            file: r.node.loc[0],
            line: r.node.loc[1],
          });
        return;
      case "focus":
        break;
    }
    save();
    drawList(st.models.now);
  };

  const rowOf = (el: any): VRow | undefined => {
    const ti = el && el.closest ? el.closest("[data-key]") : null;
    return ti ? TREE.find(rows, ti.dataset.key) : undefined;
  };

  // The row menu: the header menu's look, at the row.
  const MAC = /Mac|iPhone|iPad/.test(String(win.navigator?.platform ?? ""));
  const showRowMenu = (r: VRow, x: number, y: number) => {
    const menu = $("ctxmenu");
    if (!menu) return;
    st.tree.focus = r.key;
    menu.innerHTML = TREEVIEW.menu(r, TREE, MAC);
    menu.setAttribute("aria-label", `Actions for ${r.label}`);
    menu.hidden = false;
    if (menu.style) {
      // Inside the panel, whichever corner the row is near.
      const w = menu.offsetWidth || 240;
      const hgt = menu.offsetHeight || 200;
      const W = win.innerWidth || x + w + 8;
      const Hh = win.innerHeight || y + hgt + 8;
      menu.style.left = `${Math.max(4, Math.min(x, W - w - 8))}px`;
      menu.style.top = `${Math.max(4, y + hgt + 8 > Hh ? y - hgt : y)}px`;
    }
    menu.querySelector?.("button")?.focus?.();
  };

  const hideRowMenu = (refocus: boolean) => {
    const menu = $("ctxmenu");
    if (!menu || menu.hidden) return;
    menu.hidden = true;
    if (refocus && st.tree.focus) rowEl(st.tree.focus)?.focus?.();
  };

  // reveal opens the way to a row, then brings it into view and focuses it;
  // a package whose rows are not here yet is asked for them first.
  const reveal = (pkg: string, pos: string) => {
    if (searching()) {
      st.search = "";
      const box = $("search");
      if (box) box.value = "";
    }
    if (st.tab !== "now") setTab("now");
    TREE.reveal(st.tree, st.models.now?.tree ?? [], pkg, pos);
    st.revealing = st.tree.focus;
    if (pos) request(pkg);
    save();
    if (st.models.now) drawList(st.models.now);
  };

  const showToast = (text: string) => {
    const t = $("toast");
    if (!t) return;
    t.textContent = text;
    t.hidden = false;
    win.clearTimeout(toastTimer);
    toastTimer = win.setTimeout(() => {
      t.hidden = true;
    }, 4000);
  };
  let toastTimer: any;

  const setLevel = (level: Level) => {
    TREE.setLevel(st.tree, level);
    save();
    if (st.models.now) drawList(st.models.now);
  };

  const drawNow = () => {
    const m = st.models.now;
    const empty = $("now-empty");
    const parts = ["now-ctx", "now-chips", "now-attention", "now-all"];
    if (!m || m.empty) {
      for (const p of parts) if ($(p)) $(p).hidden = true;
      // Nothing kept: the rows of what was cleared go too.
      if (m?.empty) {
        $("now-packages").innerHTML = "";
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
    drawList(m);
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
        if (st.tab === "now") redraw();
        return;
      case "toast":
        showToast(msg.text);
        return;
      case "reveal":
        reveal(msg.pkg, msg.pos);
        return;
      case "found":
        if (msg.query !== st.search.trim()) return;
        st.found = msg;
        if (st.tab === "now" && st.models.now) drawList(st.models.now);
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
      case "level":
        setLevel(d.level as Level);
        return;
      case "treeAct":
        doOp(d.op, rowOf(el));
        return;
      case "ctxItem":
        hideRowMenu(false);
        doOp(d.op, TREE.find(rows, d.key));
        return;
      case "reveal":
        reveal(d.pkg, d.pos ?? "");
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
    if (!(t && t.closest && t.closest("#ctxmenu"))) hideRowMenu(false);
    if (!el && t && t.closest && t.closest(".tr")) {
      const r = rowOf(t);
      if (r) {
        e.preventDefault();
        if (
          e.altKey &&
          (r.kind === "dir" ||
            r.kind === "pkg" ||
            r.kind === "suite" ||
            r.kind === "method")
        )
          doOp(TREE.allOpen(r) ? "closeBelow" : "openBelow", r);
        else if (r.expandable) doOp("toggle", r);
        else doOp("focus", r);
        st.treeFocused = true;
        rowEl(r.key)?.focus?.({ preventScroll: true });
      }
      return;
    }
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
      if (st.models.now) drawList(st.models.now);
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

  // A key in a menu moves over its items; Escape leaves it.
  const menuKey = (e: any, menu: any): boolean => {
    if (!menu || menu.hidden || !menu.contains?.(e.target)) return false;
    const items = Array.from(
      menu.querySelectorAll?.("[role=menuitem]") ?? [],
    ) as any[];
    const i = items.indexOf(e.target);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      const n = items.length;
      items[(i + (e.key === "ArrowDown" ? 1 : n - 1) + n) % n]?.focus?.();
      e.preventDefault();
      return true;
    }
    return false;
  };

  const treeKeyDown = (e: any): boolean => {
    const t = e.target;
    const ti = t && t.closest ? t.closest("[role=treeitem]") : null;
    if (!ti || t !== ti) return false;
    const op = TREE.keyOp(rows, ti.dataset.key, e);
    if (!op) return false;
    if (op.op === "copy") {
      const sel = win.getSelection?.();
      if (sel && String(sel) !== "") return false;
      st.copiedAt = Date.now();
    }
    e.preventDefault();
    const r = TREE.find(rows, op.key);
    if (op.op === "menu") {
      const box = ti.getBoundingClientRect?.() ?? { left: 0, bottom: 0 };
      if (r) showRowMenu(r, box.left + 24, box.bottom);
      return true;
    }
    if (op.op === "focus") {
      st.tree.focus = op.key;
      save();
      drawList(st.models.now);
      rowEl(op.key)?.scrollIntoView?.({ block: "nearest" });
      return true;
    }
    doOp(op.op, r);
    return true;
  };

  const onKey = (e: any) => {
    if (e.key === "Escape" && !$("ctxmenu")?.hidden) {
      hideRowMenu(true);
      return;
    }
    if (menuKey(e, $("ctxmenu")) || menuKey(e, $("menu"))) return;
    if (treeKeyDown(e)) return;
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
  // The row menu replaces the webview's own on tree rows only.
  doc.addEventListener("contextmenu", (e: any) => {
    const t = e.target;
    if (!(t && t.closest && t.closest("#now-packages .tr"))) return;
    const r = rowOf(t);
    if (!r) return;
    e.preventDefault();
    showRowMenu(r, e.clientX, e.clientY);
  });
  // A copy the editor starts on a focused branch, as Ctrl/Cmd+C does when
  // its key is not seen here.
  doc.addEventListener("copy", (e: any) => {
    if (Date.now() - st.copiedAt < 500) return;
    const t = doc.activeElement;
    const ti =
      t && t.closest ? t.closest("#now-packages [role=treeitem]") : null;
    const sel = win.getSelection?.();
    if (!ti || ti !== t || (sel && String(sel) !== "")) return;
    const r = TREE.find(rows, ti.dataset.key);
    if (
      !r ||
      TREE.keyOp(rows, r.key, { key: "c", ctrlKey: true })?.op !== "copy"
    )
      return;
    e.preventDefault?.();
    doOp("copy", r);
  });
  doc.addEventListener("focusin", (e: any) => {
    const t = e.target;
    st.treeFocused = !!(t && t.closest && t.closest("#now-packages"));
    const ti =
      t && t.closest ? t.closest("#now-packages [role=treeitem]") : null;
    if (ti && ti === t) st.tree.focus = ti.dataset.key;
  });
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

  return { st, receive, onClick, onInput, onKey };
}
