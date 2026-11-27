// The behavior tree's markup: one line per row, its rows in a group below
// it, one arrow per row and the row's actions as icon buttons at its right.
// Pure strings, embedded in the webview by source (see helpers.ts).
import type { OutputLine } from "../protocol.js";
import type { Helpers } from "./helpers.js";
import type { TreeLogic, VRow } from "./tree.js";

export type TreeView = ReturnType<typeof renderTree>;

export function renderTree(h: Helpers) {
  const { esc, lines, icon } = h;

  const svg = (body: string) =>
    `<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3">${body}</svg>`;

  // Shaped like VS Code's expand-all, collapse-all, copy, run, debug and
  // go-to-file codicons.
  const ICON: Record<string, string> = {
    openBelow: svg(
      `<rect x="2.5" y="2.5" width="8" height="8"/><path d="M5 13.5h8.5V5"/><path d="M6.5 4.5v4M4.5 6.5h4"/>`,
    ),
    closeBelow: svg(
      `<rect x="2.5" y="2.5" width="8" height="8"/><path d="M5 13.5h8.5V5"/><path d="M4.5 6.5h4"/>`,
    ),
    copy: svg(
      `<rect x="5.5" y="5.5" width="8" height="8"/><path d="M10.5 5V2.5h-8v8H5"/>`,
    ),
    run: svg(`<path d="M4.5 3l8 5-8 5z"/>`),
    debug: svg(`<path d="M4.5 4l6 4-6 4z"/><circle cx="12" cy="12" r="2"/>`),
    source: svg(`<path d="M3.5 2.5h6l3 3v8h-9z"/><path d="M9.5 2.5v3h3"/>`),
  };

  // The words an action has, for its label and its menu item.
  const WORDS: Record<string, (name: string) => string> = {
    openBelow: (n) => `Open everything below ${n}`,
    closeBelow: (n) => `Close everything below ${n}`,
    copy: (n) => `Copy ${n} as Markdown`,
    run: (n) => `Run ${n}`,
    debug: (n) => `Debug ${n}`,
    source: (n) => `Open the source of ${n}`,
  };

  const MENU: Record<string, string> = {
    openBelow: "Open everything below",
    closeBelow: "Close everything below",
    copy: "Copy as Markdown",
    run: "Run",
    debug: "Debug",
    source: "Open source",
  };

  const WORD: Record<string, string> = {
    pass: "pass",
    fail: "FAIL",
    skip: "skip",
    none: "not run",
  };

  const CHEV = (open: boolean) =>
    `<svg class="chev${open ? " open" : ""}" aria-hidden="true" width="12" height="12" viewBox="0 0 12 12"><path d="M4 2l4 4-4 4"/></svg>`;

  const tags = (r: VRow) =>
    (r.node?.tags ?? [])
      .map(
        (t) => ` <span class="tag t-${t.tone ?? "none"}">${esc(t.text)}</span>`,
      )
      .join("");

  const buttons = (r: VRow, T: TreeLogic) =>
    T.actions(r)
      .map(
        (a) =>
          `<button type="button" class="ibtn" data-act="treeAct" data-op="${a}" tabindex="-1" aria-label="${esc(WORDS[a](r.label))}" title="${esc(WORDS[a](r.label))}">${ICON[a]}</button>`,
      )
      .join("");

  const line = (r: VRow, T: TreeLogic, focus: string | undefined): string => {
    const n = r.node;
    const e = r.entry;
    let head = "";
    let counts = "";
    let time = "";
    switch (r.kind) {
      case "dir":
        if (e?.kind === "dir") {
          head = `<span class="lbl dir-name">${esc(r.label)} <span class="dim">${e.packages} ${e.packages === 1 ? "package" : "packages"}</span></span>`;
          counts = `<span class="counts t-${e.tone}">${esc(e.counts)}</span>`;
        }
        break;
      case "pkg":
        if (e?.kind === "pkg") {
          const c =
            e.matches !== undefined
              ? `${e.matches.toLocaleString("en-US")} ${e.matches === 1 ? "match" : "matches"} · ${e.counts}`
              : e.counts;
          head = `<span class="lbl mono">${esc(r.label)}</span>`;
          counts = `<span class="counts t-${e.tone}">${esc(c)}${e.note ? ` <span class="dim">· ${esc(e.note)}</span>` : ""}</span>`;
          time = `<span class="dim time">${esc(e.time)}</span>`;
        }
        break;
      case "leaf":
        head = `<span class="sw s-${esc(n!.s)}">${WORD[n!.s] ?? esc(n!.s)}</span><span class="lbl">${esc(r.label)}</span>${tags(r)}${n!.t ? ` <span class="dim dur">${esc(n!.t)}</span>` : ""}`;
        break;
      case "when":
        head = `<span class="lbl when">${esc(r.label)}</span>${tags(r)}`;
        break;
      case "fail":
        head = `${icon("fail")} <strong>${esc(r.label)}</strong>`;
        break;
      default:
        head = `${icon(n!.s)} <span class="lbl${r.kind === "suite" ? " suite" : ""}">${esc(r.label)}</span>${tags(r)}${n!.t ? ` <span class="dim dur">${esc(n!.t)}</span>` : ""}`;
        if (n!.n)
          counts = `<span class="counts t-${n!.nt ?? "none"}">${esc(n!.n)}</span>`;
    }
    const chev = `<span class="chev-slot">${r.expandable ? CHEV(r.open) : ""}</span>`;
    return `<div class="tr${r.key === focus ? " focused" : ""}">${chev}${head}<span class="grow"></span><span class="ractions">${buttons(r, T)}</span>${counts}${time}</div>`;
  };

  const row = (
    r: VRow,
    T: TreeLogic,
    focus: string | undefined,
    first: string,
  ): string => {
    if (r.kind === "loading")
      return `<div class="ti-note lv-${Math.min(r.level, 12)} dim" role="none">${esc(r.label)}</div>`;
    const attrs = [
      `class="ti k-${r.kind} lv-${Math.min(r.level, 12)}${r.node ? ` s-${esc(r.node.s)}` : ""}"`,
      `role="treeitem"`,
      `aria-level="${r.level}"`,
      r.expandable ? `aria-expanded="${r.open}"` : "",
      `aria-label="${esc(r.label)}"`,
      `tabindex="${r.key === (focus ?? first) ? 0 : -1}"`,
      `data-key="${esc(r.key)}"`,
      r.pkg ? `data-pkg="${esc(r.pkg)}"` : "",
      r.pos !== undefined ? `data-pos="${esc(r.pos)}"` : "",
      r.node?.id ? `data-id="${esc(r.node.id)}"` : "",
      r.node?.loc
        ? `data-file="${esc(r.node.loc[0])}" data-line="${r.node.loc[1]}"`
        : "",
      r.kind === "pkg" && r.open && r.children[0]?.kind === "loading"
        ? `data-lazy="${esc(r.pkg!)}"`
        : "",
    ]
      .filter(Boolean)
      .join(" ");
    const out = r.node?.out
      ? `<pre class="row-out">${lines(r.node.out)}</pre>`
      : r.lines
        ? `<pre class="row-out">${lines(r.lines as OutputLine[])}</pre>`
        : "";
    const kids =
      r.children.length > 0
        ? `<div role="group">${r.children.map((c) => row(c, T, focus, first)).join("")}</div>`
        : "";
    return `<div ${attrs}>${line(r, T, focus)}${out}${kids}</div>`;
  };

  // tree is the whole list; the focused row, else the first, is its one
  // tab stop.
  const tree = (
    rows: VRow[],
    T: TreeLogic,
    focus: string | undefined,
  ): string => {
    if (rows.length === 0) return "";
    const present = focus !== undefined && !!T.find(rows, focus);
    return rows
      .map((r) => row(r, T, present ? focus : undefined, rows[0].key))
      .join("");
  };

  // menu is a row's actions as menu items, Copy with its shortcut.
  const menu = (r: VRow, T: TreeLogic, mac: boolean): string =>
    T.menuItems(r)
      .map((a) =>
        a === "-"
          ? `<div class="menu-sep" role="separator"></div>`
          : `<button type="button" role="menuitem" class="menu-item" data-act="ctxItem" data-op="${a}" data-key="${esc(r.key)}"><span>${MENU[a]}</span>${a === "copy" ? `<span class="dim kbd">${mac ? "⌘C" : "Ctrl+C"}</span>` : ""}</button>`,
      )
      .join("");

  return { tree, menu, WORDS };
}
