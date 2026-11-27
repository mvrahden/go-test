// The Now tab's markup, from the host's model: pure strings, embedded in the
// webview by source (see helpers.ts).
import type {
  Card,
  NowModel,
  OutputLine,
  PackageRow,
  TreeNode,
} from "../protocol.js";
import type { Helpers } from "./helpers.js";

export type NowView = ReturnType<typeof renderNow>;

// How a tree opens: rows toggled by hand, everything under a package or a
// suite, else what the depth opens; a failed branch opens by itself.
export interface TreeOpen {
  open: Record<string, boolean>;
  deep: boolean;
  depth: string;
}

export interface CardState {
  // Full output fetched per card key, and which cards show it.
  outputs: Record<string, { lines: OutputLine[]; more?: number }>;
  shown: Record<string, boolean>;
  noSuites: boolean;
}

export function renderNow(h: Helpers) {
  const { esc, button, icon, line, lines } = h;

  const context = (m: NowModel): string => {
    const c = m.context;
    const top = [
      c.branch ? `<span class="mono strong">${esc(c.branch)}</span>` : "",
      c.commit ? `<span class="mono dim">${esc(c.commit)}</span>` : "",
      c.fresh
        ? `<span class="t-${c.fresh.tone}">${esc(c.fresh.text)}</span>`
        : "",
    ].filter(Boolean);
    const sources = c.sources
      ? `<label class="field"><span class="dim">Showing</span> <select data-change="source">${c.sources
          .map(
            (o) =>
              `<option value="${esc(o.id)}"${o.selected ? " selected" : ""}>${esc(o.label)}</option>`,
          )
          .join("")}</select></label>`
      : "";
    const live = (c.live ?? [])
      .map(
        (l) =>
          `<div class="live-line" role="status"><span class="dot" aria-hidden="true"></span>${esc(l)}</div>`,
      )
      .join("");
    const error = m.error
      ? `<div class="notice warn" role="alert">This view is out of date: the last render failed. <span class="dim">${esc(m.error.split("\n")[0])}</span> ${button("Retry", "refresh")}</div>`
      : "";
    return `${error}${top.length > 0 || sources ? `<div class="ctx-top">${top.join("")}${sources}</div>` : ""}${
      c.lastRun
        ? `<div class="dim"${c.runTitle ? ` title="${esc(c.runTitle)}"` : ""}>${esc(c.lastRun)}</div>`
        : ""
    }${live}`;
  };

  const chips = (m: NowModel, pressed: string[]): string =>
    m.chips
      .map((c) => {
        const on = pressed.includes(c.status);
        return `<button type="button" class="chip c-${c.status}${on ? " on" : ""}" data-act="chip" data-status="${c.status}" aria-pressed="${on}">${esc(c.label)} <strong>${c.count.toLocaleString("en-US")}</strong></button>`;
      })
      .join("");

  const KIND: Record<Card["kind"], string> = {
    failed: "FAILED",
    stale: "STALE",
    vacuous: "VACUOUS",
  };

  const card = (c: Card, st: CardState): string => {
    const out = st.outputs[c.key];
    const shown = !!st.shown[c.key];
    const msg =
      c.message && c.message.length > 0
        ? `<pre class="card-msg">${lines(c.message)}${c.at ? `\n<span class="dim">${line(c.at)}</span>` : ""}</pre>`
        : "";
    const d = {
      id: c.id,
      pos: c.pos ? c.pos.join(".") : undefined,
      key: c.key,
    };
    const acts = [
      c.id ? button("Rerun", "run", d, "btn") : "",
      c.id && c.kind !== "stale" ? button("Debug", "debug", d, "btn") : "",
      c.loc
        ? button("Open test", "goTo", { file: c.loc.file, line: c.loc.line })
        : "",
      c.pos && !c.live
        ? button("Reveal in list", "reveal", {
            pkg: c.pkg,
            pos: c.pos.join("."),
          })
        : "",
      c.pos && c.kind === "failed"
        ? button(
            shown ? "Hide output" : "Full output",
            "fullOutput",
            d,
            "link",
            ` aria-expanded="${shown}"`,
          )
        : "",
    ].join("");
    const full =
      shown && out
        ? `<pre class="full-out" tabindex="0">${lines(out.lines)}${out.more ? `\n<span class="dim">… ${out.more} more lines</span>` : ""}</pre>`
        : shown
          ? `<div class="dim">Loading the output…</div>`
          : "";
    return `<article class="card k-${c.kind}" data-pkg="${esc(c.pkg)}" aria-label="${esc(KIND[c.kind])}: ${esc(c.name)}">
<div class="card-top"><span class="badge k-${c.kind}">${KIND[c.kind]}</span><span class="dim where">${esc(c.where)}</span>${c.live ? `<span class="dim">· from the run in progress</span>` : ""}</div>
<div class="card-name">${esc(c.name)}</div>${msg}${c.flaky ? `<div class="t-warn">${esc(c.flaky)}</div>` : ""}
<div class="acts">${acts}</div>${full}</article>`;
  };

  const attention = (m: NowModel, st: CardState): string => {
    const parts = m.cards.map((c) => card(c, st));
    if (m.moreCards) parts.push(`<div class="dim">${esc(m.moreCards)}</div>`);
    if (m.noSuites)
      parts.push(
        `<div class="quiet-row"><span>${esc(m.noSuites.text)}</span>${button(st.noSuites ? "Hide" : "Show", "noSuites", {}, "link", ` aria-expanded="${st.noSuites}"`)}</div>${
          st.noSuites
            ? `<ul class="plain">${m.noSuites.names.map((n) => `<li class="mono">${esc(n)}</li>`).join("")}</ul>`
            : ""
        }`,
      );
    if (parts.length === 0) return "";
    const nothing =
      m.cards.length === 0
        ? `<div class="dim">Nothing failed, nothing is stale or vacuous.</div>`
        : "";
    return `${h.section("Needs attention")}${nothing}${parts.join("")}`;
  };

  // pkgHead is the inside of a package row's button.
  const CHEV = (open: boolean) =>
    `<svg class="chev${open ? " open" : ""}" aria-hidden="true" width="12" height="12" viewBox="0 0 12 12"><path d="M4 2l4 4-4 4"/></svg>`;

  // folderHead is the inside of a folder's button.
  const folderHead = (
    f: { name: string; packages: number; counts: string; tone: string },
    open: boolean,
    shown?: number,
  ): string =>
    `${CHEV(open)}<span class="folder-name">${esc(f.name)} <span class="dim">${
      shown === undefined
        ? `${f.packages} ${f.packages === 1 ? "package" : "packages"}`
        : `${shown} of ${f.packages} ${f.packages === 1 ? "package" : "packages"}`
    }</span></span><span class="counts t-${f.tone}">${esc(f.counts)}</span>`;

  const tagHtml = (n: TreeNode): string =>
    (n.tags ?? [])
      .map(
        (t) => ` <span class="tag t-${t.tone ?? "none"}">${esc(t.text)}</span>`,
      )
      .join("");

  const acts = (n: TreeNode, branch: boolean): string =>
    `<span class="row-acts">${n.id ? button("Run", "run", {}, "act") + button("Debug", "debug", {}, "act") : ""}${
      n.loc
        ? button("Open", "goTo", { file: n.loc[0], line: n.loc[1] }, "act")
        : ""
    }${branch ? button("Copy", "copyNode", {}, "act") : ""}</span>`;

  const BOLD = new Set(["suite", "fixture", "test", "fuzz"]);

  const WORD: Record<string, string> = {
    pass: "pass",
    fail: "FAIL",
    skip: "skip",
    none: "not run",
  };

  const node = (n: TreeNode, o: TreeOpen, deepAbove: boolean): string => {
    const label = BOLD.has(n.k) ? `<strong>${esc(n.d)}</strong>` : esc(n.d);
    const time = n.t ? ` <span class="dim dur">${esc(n.t)}</span>` : "";
    const out = n.out ? `<pre class="row-out">${lines(n.out)}</pre>` : "";
    const ids = `${n.id ? ` data-id="${esc(n.id)}"` : ""} data-pos="${esc(n.pos)}"`;
    if (!n.c)
      return `<div class="row leaf k-${esc(n.k)} s-${esc(n.s)}"${ids}><span class="sw s-${esc(n.s)}">${WORD[n.s] ?? esc(n.s)}</span> <span class="lbl">${label}</span>${tagHtml(n)}${time}${acts(n, false)}</div>${out}`;
    const deep = deepAbove;
    const isOpen =
      o.open[n.pos] ??
      (deep ||
        (o.depth === "behaviors"
          ? true
          : o.depth === "suites"
            ? false
            : n.s === "fail"));
    const counts = n.n
      ? `<span class="grow"></span><span class="counts t-${n.nt ?? "none"}">${esc(n.n)}</span>`
      : "";
    return `<details class="branch"${ids}${isOpen ? " open" : ""}><summary class="row k-${esc(n.k)} s-${esc(n.s)}">${icon(n.s)} <span class="lbl">${label}</span>${tagHtml(n)}${time}${acts(n, true)}${counts}</summary>${out}${n.c
      .map((c) => node(c, o, deep))
      .join("")}</details>`;
  };

  const tree = (
    nodes: TreeNode[],
    failure: OutputLine[] | undefined,
    o: TreeOpen,
  ): string => {
    const fail = failure
      ? `<div class="row s-fail">${icon("fail")} <strong>The package failed</strong></div><pre class="row-out">${lines(failure)}</pre>`
      : "";
    if (!fail && nodes.length === 0)
      return `<div class="dim pad">No rows to show.</div>`;
    return fail + nodes.map((n) => node(n, o, o.deep)).join("");
  };

  // foundNote says how many rows a search found and how many it shows.
  const foundNote = (total: number, shown: number): string =>
    total === 0
      ? `<div class="dim pad">No behavior matches.</div>`
      : `<div class="dim found-note">${total.toLocaleString("en-US")} ${total === 1 ? "match" : "matches"}${
          shown < total
            ? `; the first ${shown.toLocaleString("en-US")} show. Search for more of a name to see the rest.`
            : ""
        }</div>`;

  return {
    context,
    chips,
    attention,
    card,
    folderHead,
    tree,
    foundNote,
  };
}
