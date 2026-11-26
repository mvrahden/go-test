// The panel's document: styles, empty regions and the webview's script. It
// is assigned once per panel; everything after travels as messages.
import { panelApp } from "./client/app.js";
import { clientHelpers } from "./client/helpers.js";
import { renderChange } from "./client/renderChange.js";
import { renderNow } from "./client/renderNow.js";
import { renderRuns } from "./client/renderRuns.js";
import type { Tab } from "./protocol.js";
import { encodeStateForScript } from "./specData.js";

export interface ShellOptions {
  nonce: string;
  cspSource: string;
  tab?: Tab;
  has: { change: boolean; runs: boolean };
}

// panelScript is the webview's script: the client factories by source, and
// the call that starts them.
export function panelScript(initial: {
  tab?: Tab;
  has: { change: boolean; runs: boolean };
}): string {
  return `"use strict";
const INITIAL = ${encodeStateForScript(initial)};
const H = (${clientHelpers.toString()})();
const NOW = (${renderNow.toString()})(H);
const CHANGE = (${renderChange.toString()})(H);
const RUNS = (${renderRuns.toString()})(H);
(${panelApp.toString()})({ vscode: acquireVsCodeApi(), document, window, initial: INITIAL, NOW, CHANGE, RUNS });
`;
}

const DOTS = `<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><circle cx="3" cy="8" r="1.4"></circle><circle cx="8" cy="8" r="1.4"></circle><circle cx="13" cy="8" r="1.4"></circle></svg>`;

export function buildShell(o: ShellOptions): string {
  const tab = (id: Tab, label: string, badge: string) =>
    `<button type="button" role="tab" class="tab-btn" id="tab-btn-${id}" data-act="tab" data-tab="${id}" aria-controls="tab-${id}"${id !== "now" && !o.has[id as "change" | "runs"] ? " hidden" : ""}>${label} ${badge}</button>`;
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${o.cspSource}; style-src 'nonce-${o.nonce}'; script-src 'nonce-${o.nonce}';">
<style nonce="${o.nonce}">
${CSS}
</style>
</head>
<body>
<header class="topbar">
  <div class="tabs" role="tablist" aria-label="Panel">
    ${tab("now", "Now", `<span class="badge-n t-fail" id="badge-now"></span>`)}
    ${tab("change", "Change", `<span class="badge-n t-warn" id="badge-change"></span>`)}
    ${tab("runs", "Runs", "")}
  </div>
  <span class="grow"></span>
  <span class="actions">
    <span id="updating" class="updating" role="status" hidden><span class="dot" aria-hidden="true"></span>Updating…</span>
    <button type="button" id="primary" class="primary" data-act="primary">Run changed</button>
    <span class="menu-wrap">
      <button type="button" id="more" class="icon-btn" data-act="menu" aria-label="More actions" aria-haspopup="menu" aria-expanded="false">${DOTS}</button>
      <div id="menu" class="menu" role="menu" aria-label="More actions" hidden></div>
    </span>
  </span>
</header>
<main>
  <section id="tab-now" class="tab" role="tabpanel" aria-labelledby="tab-btn-now">
    <div id="now-empty"></div>
    <div id="now-ctx" class="ctx"></div>
    <div id="now-chips" class="chips" role="group" aria-label="Show only"></div>
    <div id="now-attention" class="block"></div>
    <div id="now-all" class="block">
      <div class="all-head">
        <h2 class="eyebrow">All behaviors</h2><span id="now-total" class="dim"></span>
      </div>
      <div class="all-tools">
        <div class="depth" role="group" aria-label="Depth">
          <button type="button" id="depth-packages" data-act="depth" data-depth="packages" aria-pressed="true">Packages</button><button type="button" id="depth-suites" data-act="depth" data-depth="suites" aria-pressed="false">Suites</button><button type="button" id="depth-behaviors" data-act="depth" data-depth="behaviors" aria-pressed="false">Behaviors</button>
        </div>
        <span class="grow"></span>
        <label for="search" class="sr">Search behaviors</label><input id="search" type="search" placeholder="Search opens what it finds" autocomplete="off">
      </div>
      <div id="now-groups" class="dim"></div>
      <div id="now-packages" class="list"></div>
      <div id="now-found" class="list" hidden></div>
    </div>
  </section>
  <section id="tab-change" class="tab" role="tabpanel" aria-labelledby="tab-btn-change" hidden>
    <div id="change-ctx" class="ctx"></div>
    <div id="change-body" class="block"></div>
  </section>
  <section id="tab-runs" class="tab" role="tabpanel" aria-labelledby="tab-btn-runs" hidden>
    <div id="runs-running"></div>
    <div id="runs-ctx"></div>
    <div id="runs-body" class="block"></div>
  </section>
</main>
<script nonce="${o.nonce}">
${panelScript({ tab: o.tab, has: o.has })}
</script>
</body>
</html>`;
}

export function getNonce(): string {
  const chars =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  let result = "";
  for (let i = 0; i < 32; i++)
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  return result;
}

export const CSS = `
:root { color-scheme: light dark; }
* { box-sizing: border-box; }
body {
  margin: 0; height: 100vh; display: flex; flex-direction: column;
  background: var(--vscode-editor-background);
  color: var(--vscode-foreground);
  font-family: var(--vscode-font-family);
  font-size: var(--vscode-font-size, 13px);
  line-height: 1.45;
}
button { font: inherit; color: inherit; cursor: pointer; }
button:disabled { opacity: 0.5; cursor: default; }
:focus-visible { outline: 1px solid var(--vscode-focusBorder); outline-offset: 1px; }
.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.grow { flex-grow: 1; }
.mono, code { font-family: var(--vscode-editor-font-family); font-size: 0.95em; }
.strong { color: var(--vscode-editor-foreground); font-weight: 600; }
.dim { color: var(--vscode-descriptionForeground); }
.t-fail { color: var(--vscode-testing-iconFailed, var(--vscode-errorForeground)); }
.t-warn { color: var(--vscode-editorWarning-foreground, var(--vscode-testing-iconQueued)); }
.t-pass { color: var(--vscode-testing-iconPassed); }
.t-info { color: var(--vscode-textLink-foreground); }
.t-none { color: var(--vscode-descriptionForeground); }
.pad { padding: 8px 14px; }

/* header */
.topbar { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 6px; padding: 0 16px; min-height: 44px; border-bottom: 1px solid var(--vscode-panel-border); flex-shrink: 0; }
.tabs { display: flex; align-self: stretch; }
.actions { display: flex; align-items: center; gap: 6px; margin-left: auto; padding: 4px 0; }
.tab-btn { background: none; border: 0; border-bottom: 2px solid transparent; padding: 0 12px; min-height: 42px; color: var(--vscode-descriptionForeground); }
.tab-btn:hover { color: var(--vscode-foreground); }
.tab-btn[aria-selected="true"] { color: var(--vscode-foreground); font-weight: 600; border-bottom-color: var(--vscode-focusBorder); }
.tab-btn[hidden] { display: none; }
.badge-n { font-weight: 600; }
.badge-n:empty { display: none; }
.primary { height: 28px; padding: 0 12px; border: 0; border-radius: 4px; background: var(--vscode-button-background); color: var(--vscode-button-foreground); font-weight: 600; white-space: nowrap; }
.primary:hover { background: var(--vscode-button-hoverBackground); }
.btn { height: 28px; padding: 0 12px; border-radius: 4px; border: 1px solid var(--vscode-button-border, var(--vscode-panel-border)); background: var(--vscode-button-secondaryBackground); color: var(--vscode-button-secondaryForeground); white-space: nowrap; }
.btn:hover { background: var(--vscode-button-secondaryHoverBackground); }
.btn.small { height: 24px; padding: 0 10px; }
.btn.danger { color: var(--vscode-testing-iconFailed, var(--vscode-errorForeground)); border-color: currentColor; background: none; }
.icon-btn { height: 28px; width: 32px; display: inline-flex; align-items: center; justify-content: center; background: none; border: 1px solid var(--vscode-panel-border); border-radius: 4px; }
.link, .link-text { background: none; border: 0; padding: 0; color: var(--vscode-textLink-foreground); }
.link:hover, .link-text:hover { color: var(--vscode-textLink-activeForeground); text-decoration: underline; }
.link.wrap { text-align: left; overflow-wrap: anywhere; }
.menu-wrap { position: relative; }
.menu { position: absolute; right: 0; top: 32px; z-index: 10; min-width: 220px; padding: 4px; background: var(--vscode-menu-background, var(--vscode-editorWidget-background)); color: var(--vscode-menu-foreground, var(--vscode-foreground)); border: 1px solid var(--vscode-menu-border, var(--vscode-panel-border)); border-radius: 4px; box-shadow: 0 2px 8px var(--vscode-widget-shadow); }
.menu-item { display: block; width: 100%; text-align: left; background: none; border: 0; padding: 5px 10px; border-radius: 3px; }
.menu-item:hover, .menu-item:focus { background: var(--vscode-menu-selectionBackground, var(--vscode-list-hoverBackground)); color: var(--vscode-menu-selectionForeground, inherit); }
.updating { display: inline-flex; align-items: center; gap: 6px; color: var(--vscode-descriptionForeground); font-size: 0.9em; }
.updating[hidden] { display: none; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: var(--vscode-progressBar-background); animation: pulse 1.2s ease-in-out infinite; flex-shrink: 0; }
@keyframes pulse { 50% { opacity: 0.3; } }
@media (prefers-reduced-motion: reduce) { .dot, .bar-run { animation: none; } }

/* tabs */
main { flex: 1; min-height: 0; display: flex; }
.tab { flex: 1; overflow-y: auto; overflow-x: hidden; min-width: 0; }
.tab[hidden] { display: none; }
[hidden] { display: none !important; }
.ctx { display: flex; flex-direction: column; gap: 4px; padding: 14px 16px; border-bottom: 1px solid var(--vscode-panel-border); }
.ctx:empty { display: none; }
.ctx-top { display: flex; gap: 4px 10px; align-items: baseline; flex-wrap: wrap; }
#runs-ctx .ctx-top { padding: 10px 16px 0; }
.ctx-top > * { overflow-wrap: anywhere; }
.field select { background: var(--vscode-dropdown-background); color: var(--vscode-dropdown-foreground); border: 1px solid var(--vscode-dropdown-border, var(--vscode-panel-border)); font: inherit; max-width: 100%; }
.live-line { display: flex; align-items: center; gap: 8px; }
.notice { padding: 8px 12px; border: 1px solid var(--vscode-panel-border); border-radius: 4px; }
.notice.warn { border-color: var(--vscode-inputValidation-warningBorder, var(--vscode-editorWarning-foreground)); background: var(--vscode-inputValidation-warningBackground, transparent); }
.chips { display: flex; gap: 8px; padding: 12px 16px 4px; flex-wrap: wrap; }
.chips:empty { display: none; }
.chip { padding: 4px 10px; border-radius: 14px; border: 1px solid var(--vscode-panel-border); background: none; color: var(--vscode-foreground); }
.chip.on { border-color: var(--vscode-focusBorder); background: var(--vscode-list-activeSelectionBackground, var(--vscode-list-hoverBackground)); color: var(--vscode-list-activeSelectionForeground, inherit); }
.chip.c-fail.on { border-color: var(--vscode-testing-iconFailed); }
.block { display: flex; flex-direction: column; gap: 8px; padding: 12px 16px 16px; }
.block:empty { display: none; }
.eyebrow { margin: 4px 0 0; font-size: 11px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--vscode-descriptionForeground); font-weight: 600; }
.empty { display: flex; flex-direction: column; align-items: center; text-align: center; padding: 48px 24px; gap: 8px; }
.empty h1 { font-size: 1.3em; margin: 0; font-weight: 600; }
.empty p { margin: 0 0 8px; max-width: 420px; color: var(--vscode-descriptionForeground); }
.headline h1 { font-size: 18px; margin: 4px 0 2px; font-weight: 600; color: var(--vscode-editor-foreground); }
.headline p { margin: 0; }
.footnote { margin: 4px 0 0; }
ul.plain { margin: 0; padding-left: 20px; }

/* cards */
.card { border: 1px solid var(--vscode-panel-border); border-radius: 6px; padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; min-width: 0; }
.card.k-failed { border-color: color-mix(in srgb, var(--vscode-testing-iconFailed) 45%, transparent); background: color-mix(in srgb, var(--vscode-testing-iconFailed) 5%, transparent); }
.card-top { display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; }
.card-top .where { overflow-wrap: anywhere; min-width: 0; }
.badge { font-size: 11px; font-weight: 600; border: 1px solid currentColor; border-radius: 3px; padding: 0 6px; white-space: nowrap; }
.badge.k-failed { color: var(--vscode-testing-iconFailed); }
.badge.k-stale, .badge.k-vacuous { color: var(--vscode-editorWarning-foreground); }
.card-name { font-size: 14px; color: var(--vscode-editor-foreground); overflow-wrap: anywhere; }
pre { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-family: var(--vscode-editor-font-family); font-size: 12px; }
.card-msg, .full-out { background: var(--vscode-textCodeBlock-background); border-radius: 4px; padding: 8px 10px; }
.full-out { max-height: 420px; overflow: auto; }
.acts { display: flex; gap: 8px 14px; flex-wrap: wrap; align-items: center; }
.quiet-row { display: flex; justify-content: space-between; gap: 12px; border: 1px solid var(--vscode-panel-border); border-radius: 6px; padding: 10px 14px; color: var(--vscode-descriptionForeground); }
.src-link { color: var(--vscode-textLink-foreground); }

/* package rows */
.all-head { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; }
.all-tools { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; }
.depth { display: inline-flex; border: 1px solid var(--vscode-panel-border); border-radius: 4px; overflow: hidden; }
.depth button { height: 28px; padding: 0 12px; background: none; border: 0; }
.depth button + button { border-left: 1px solid var(--vscode-panel-border); }
.depth button[aria-pressed="true"] { background: var(--vscode-list-activeSelectionBackground, var(--vscode-button-background)); color: var(--vscode-list-activeSelectionForeground, var(--vscode-button-foreground)); font-weight: 600; }
.all-tools input { width: 260px; max-width: 100%; height: 28px; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border, var(--vscode-panel-border)); border-radius: 4px; padding: 0 8px; font: inherit; }
.list { display: flex; flex-direction: column; border: 1px solid var(--vscode-panel-border); border-radius: 6px; overflow: hidden; }
.list:empty { display: none; }
.list[data-empty]::after { content: attr(data-empty); padding: 9px 14px; color: var(--vscode-descriptionForeground); }
.folder + .folder, .pkg, .list > .pkg-head + .pkg-head, .list > .pkg-tree + .pkg-head, .group-row + .group-row, .group-row + .decisions + .group-row { border-top: 1px solid var(--vscode-panel-border); }
.folder-head { display: grid; grid-template-columns: 16px minmax(0, 1fr) auto; gap: 10px; align-items: center; width: 100%; padding: 8px 14px; background: var(--vscode-sideBar-background, var(--vscode-editorWidget-background)); border: 0; text-align: left; }
.folder-head.static { display: flex; gap: 10px; }
.folder-head.static .chev { display: none; }
.folder-name { font-weight: 600; color: var(--vscode-editor-foreground); }
.folder-name .dim { font-weight: normal; }
.folder-body > .pkg { border-top: 1px solid var(--vscode-panel-border); }
#tab-now .pkg-head { display: flex; align-items: center; gap: 8px; padding: 6px 14px 6px 30px; min-height: 36px; }
#tab-now .pkg-head.static { padding-left: 30px; }
.pkg-toggle { display: inline-flex; align-items: center; gap: 10px; background: none; border: 0; padding: 2px 0; text-align: left; min-width: 0; }
.pkg-toggle .name { overflow-wrap: anywhere; }
#tab-now .pkg-head .time { width: 52px; text-align: right; flex-shrink: 0; }
#tab-now .pkg-head .counts { text-align: right; }
.deep { width: 24px; height: 24px; flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; padding: 0; background: none; border: 1px solid var(--vscode-panel-border); border-radius: 4px; vertical-align: middle; margin-left: 6px; }
.deep.on { background: var(--vscode-list-activeSelectionBackground, var(--vscode-button-background)); color: var(--vscode-list-activeSelectionForeground, var(--vscode-button-foreground)); border-color: var(--vscode-focusBorder); }
summary.row { display: flex; align-items: baseline; gap: 4px; }
summary.row .lbl { min-width: 0; }
#now-found .deep { display: none; }
.folder-head.static .counts { margin-left: auto; }
summary.row .counts { font-size: 0.95em; }
.sw { display: inline-block; min-width: 4.2em; font-size: 0.85em; font-weight: 600; }
.sw.s-pass { color: var(--vscode-testing-iconPassed); font-weight: normal; }
.sw.s-fail { color: var(--vscode-testing-iconFailed); }
.sw.s-skip { color: var(--vscode-testing-iconSkipped); }
.sw.s-none { color: var(--vscode-descriptionForeground); font-weight: normal; }
.row.leaf.s-fail { background: color-mix(in srgb, var(--vscode-testing-iconFailed) 10%, transparent); border-radius: 3px; }
.row.k-block:not(.leaf) .lbl, .row.k-method .lbl { color: var(--vscode-descriptionForeground); }
.found-note { padding: 8px 14px; }
.menu-sep { height: 1px; margin: 4px 0; background: var(--vscode-menu-separatorBackground, var(--vscode-panel-border)); }
#tab-change .pkg-head { display: grid; grid-template-columns: 16px minmax(0, 1fr) auto; gap: 10px; align-items: center; width: 100%; padding: 9px 14px; background: none; border: 0; text-align: left; }
.pkg-head:hover { background: var(--vscode-list-hoverBackground); }
.pkg-head .name { overflow-wrap: anywhere; }
.pkg-head .time { text-align: right; }
.chev { fill: none; stroke: var(--vscode-descriptionForeground); stroke-width: 1.6; transition: transform 0.1s; }
.chev.open { transform: rotate(90deg); }
.pkg-tree { padding: 4px 14px 10px 56px; border-top: 1px solid var(--vscode-panel-border); overflow-x: auto; }
.row { padding: 1px 0; display: block; }
summary.row { cursor: pointer; list-style: none; }
summary.row::-webkit-details-marker { display: none; }
details.branch > summary::before { content: "▸"; display: inline-block; width: 1em; color: var(--vscode-descriptionForeground); }
details.branch[open] > summary::before { content: "▾"; }
details.branch > :not(summary) { margin-left: 18px; }
.leaf { margin-left: 1em; }
.ic { display: inline-block; width: 1em; text-align: center; }
.ic.s-pass { color: var(--vscode-testing-iconPassed); }
.ic.s-fail { color: var(--vscode-testing-iconFailed); }
.ic.s-skip { color: var(--vscode-testing-iconSkipped); }
.ic.s-none { color: var(--vscode-descriptionForeground); }
.row.s-none .lbl { color: var(--vscode-descriptionForeground); }
.lbl { overflow-wrap: anywhere; }
.tag { font-size: 0.8em; margin-left: 4px; }
.dur { font-size: 0.85em; }
.row-acts { margin-left: 8px; white-space: nowrap; }
/* Out of the flow until wanted, still reachable by keyboard. */
.row:not(:hover):not(:focus-within) > .row-acts { position: absolute; opacity: 0; pointer-events: none; }
.act { background: none; border: 0; padding: 0 4px; color: var(--vscode-textLink-foreground); font-size: 0.85em; }
.act:hover { text-decoration: underline; }
.row-out { color: var(--vscode-testing-iconFailed); margin: 2px 0 6px 2em; }

/* Change */
.base-btn { background: none; border: 1px solid var(--vscode-panel-border); border-radius: 4px; padding: 1px 8px; }
.pick { border: 1px solid var(--vscode-focusBorder); border-radius: 3px; padding: 0 2px 0 6px; margin-left: 4px; }
.decisions { display: flex; flex-direction: column; gap: 8px; }
.decisions.inset { padding: 8px 14px; }
.decision { border: 1px solid var(--vscode-panel-border); border-radius: 6px; padding: 10px 14px; display: grid; grid-template-columns: 92px minmax(0, 1fr) auto; gap: 4px 12px; align-items: baseline; }
.decision.accepted .what { text-decoration: line-through; color: var(--vscode-descriptionForeground); }
.decision .kind { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; white-space: nowrap; }
.decision .what { color: var(--vscode-editor-foreground); overflow-wrap: break-word; min-width: 0; }
.decision .fact { text-align: right; overflow-wrap: anywhere; }
.decision .where { grid-column: 2; overflow-wrap: break-word; font-size: 12px; min-width: 0; }
.decision .d-acts { display: flex; gap: 12px; justify-content: flex-end; align-items: center; }
.mark { font-size: 0.85em; font-family: var(--vscode-editor-font-family); }
.more-row { display: flex; justify-content: space-between; gap: 12px; width: 100%; border: 1px dashed var(--vscode-panel-border); border-radius: 6px; padding: 10px 14px; background: none; text-align: left; }
.group-row { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; gap: 12px; align-items: center; padding: 9px 14px; }
.behavior { padding: 2px 0; overflow-wrap: anywhere; }
.section-head { display: flex; gap: 12px; align-items: baseline; flex-wrap: wrap; }
.section-head .eyebrow { margin: 0; }
.line-row { display: grid; grid-template-columns: minmax(12ch, 1fr) minmax(0, max-content) auto; gap: 12px; align-items: center; padding: 9px 14px; }
.line-row + .line-row { border-top: 1px solid var(--vscode-panel-border); }
.line-row .mono { overflow-wrap: anywhere; }
.line-row.note { grid-template-columns: minmax(0, 1fr) auto; }
.ranges { text-align: right; overflow-wrap: anywhere; }
.warn-link { color: var(--vscode-editorWarning-foreground); }
.sign { display: inline-block; width: 1.2em; font-weight: 600; }
.sign.c-added { color: var(--vscode-testing-iconPassed); }
.sign.c-removed { color: var(--vscode-testing-iconFailed); }
.sign.c-reworded, .sign.c-moved, .sign.c-changed { color: var(--vscode-editorWarning-foreground); }
details.extra > summary { cursor: pointer; color: var(--vscode-descriptionForeground); }

/* Runs */
.running { display: flex; flex-direction: column; gap: 8px; padding: 14px 16px; border-bottom: 1px solid var(--vscode-panel-border); background: var(--vscode-sideBar-background, transparent); }
.running-top { display: flex; align-items: center; gap: 6px 10px; flex-wrap: wrap; }
.running .bar { height: 4px; border-radius: 2px; background: var(--vscode-panel-border); overflow: hidden; }
.bar-run { width: 30%; height: 4px; background: var(--vscode-progressBar-background); animation: slide 1.6s ease-in-out infinite; }
@keyframes slide { from { transform: translateX(-100%); } to { transform: translateX(340%); } }
.run-card { border: 1px solid var(--vscode-panel-border); border-radius: 6px; padding: 12px 14px; display: grid; grid-template-columns: 64px minmax(0, 1fr) auto; gap: 4px 12px; align-items: baseline; }
.run-card.chosen { border-left: 3px solid var(--vscode-focusBorder); }
.run-card.inner { border-style: dashed; }
.run-card .title { overflow-wrap: anywhere; min-width: 0; }
.run-card .verdict { font-weight: 600; text-align: right; }
.run-card .right { text-align: right; }
.run-card .acts { padding-top: 4px; }
.run-card .cmd { grid-column: 2 / 4; overflow-wrap: anywhere; background: var(--vscode-textCodeBlock-background); padding: 6px 8px; border-radius: 4px; }
.inset { display: flex; flex-direction: column; gap: 6px; padding-left: 16px; }

@media (max-width: 520px) {
  .topbar { padding: 0 8px; gap: 0 4px; }
  .tab-btn { padding: 0 6px; }
  .primary { padding: 0 8px; }
  .ctx, .block, .chips { padding-left: 12px; padding-right: 12px; }
  #tab-now .pkg-head { flex-wrap: wrap; padding-left: 20px; }
  #tab-now .pkg-head .counts { text-align: left; }
  .pkg-tree { padding-left: 24px; }
  .decision { grid-template-columns: minmax(0, 1fr); }
  .decision .kind, .decision .what, .decision .fact, .decision .where, .decision .d-acts { grid-column: 1; }
  .decision .fact { text-align: left; }
  .decision .d-acts { justify-content: flex-start; }
  .run-card { grid-template-columns: 48px minmax(0, 1fr); }
  .run-card .verdict, .run-card .right { text-align: left; grid-column: 2; }
  .run-card .cmd { grid-column: 1 / 3; }
  .pkg-tree { padding-left: 14px; }
  .all-tools input { width: 100%; }
}
`;
