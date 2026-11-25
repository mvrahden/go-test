// The Change scope's HTML: the audit document's view model, or what stands
// in for it while there is none. Pure strings; the panel wires the clicks.
import type {
  BehaviorRow,
  ChangeModel,
  DecisionGroup,
  DecisionRow,
} from "./auditModel.js";

// What the Change scope shows for its folder.
export type ChangeState =
  | { kind: "idle" }
  | { kind: "notGit" }
  | { kind: "noBase" }
  | {
      kind: "oldCli";
      version?: string;
      floor: string;
      advice: string;
      canUpgrade: boolean;
    }
  | { kind: "error"; message: string }
  | { kind: "doc"; model: ChangeModel; exitCode: number | null };

export interface ChangeView {
  state: ChangeState;
  busy: boolean;
  folders: { path: string; label: string; selected: boolean }[];
  // The folder's base as the user chose it, and the branch HEAD is on.
  baseLabel?: string;
  headBranch?: string;
  // Which rows' buttons do something here.
  canRun: (key: string) => boolean;
  canOpen: (key: string) => boolean;
}

const ACCEPT_NOTE =
  "Accepting is a note for yourself in this editor: it is kept in this workspace, never sent anywhere, and forgotten when the head commit changes.";

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function attr(s: string): string {
  return esc(s).replace(/"/g, "&quot;");
}

export function buildChangeBody(view: ChangeView): string {
  return `<div class="change-container">
  ${changeToolbar(view)}
  <div class="change-body" id="change-body">${changeContent(view)}</div>
</div>`;
}

function changeToolbar(view: ChangeView): string {
  const picker =
    view.folders.length > 1
      ? `<div class="toolbar-group"><select class="source-select" id="change-folder" title="Which folder to audit">${view.folders
          .map(
            (f) =>
              `<option value="${attr(f.path)}"${f.selected ? " selected" : ""}>${esc(f.label)}</option>`,
          )
          .join("")}</select></div>`
      : "";
  const canAudit = !["notGit", "idle"].includes(view.state.kind);
  const busy = view.busy
    ? `<span class="busy" role="status">Auditing…</span>`
    : "";
  return `<div class="toolbar">
  ${picker}
  <div class="toolbar-group">
    <button class="tool-btn" data-change="chooseBase" title="Compare with another ref"${view.state.kind === "notGit" ? " disabled" : ""}>Base: ${esc(view.baseLabel ?? "choose…")}</button>
  </div>
  <div class="toolbar-group search-group">${busy}</div>
  <div class="toolbar-group">
    <button class="tool-btn" data-change="refresh" title="Audit the working tree against the base again"${canAudit && !view.busy ? "" : " disabled"}>Refresh</button>
  </div>
</div>`;
}

function changeContent(view: ChangeView): string {
  const s = view.state;
  switch (s.kind) {
    case "idle":
      return emptyHtml(
        "Change",
        view.busy
          ? "Auditing the working tree against its base…"
          : "No gotest folder to audit.",
      );
    case "notGit":
      return emptyHtml(
        "Not a git checkout",
        "The Change scope compares the working tree with a commit, so it needs the folder to be in a git repository.",
      );
    case "noBase":
      return emptyHtml(
        "Choose a base",
        "No remote default branch was found to compare with.",
        `<button class="empty-button" data-change="chooseBase">Choose a base…</button>`,
      );
    case "oldCli": {
      const which = s.version ? `This folder's CLI is ${esc(s.version)}.` : "";
      const button = s.canUpgrade
        ? `<button class="empty-button" data-change="upgrade">Upgrade…</button>`
        : "";
      return emptyHtml(
        `The Change scope needs gotest ${esc(s.floor)}`,
        `${which} It reads <code>gotest audit</code>, which ${esc(s.floor)} brings; the rest of the extension works as before.<br/>${esc(s.advice)}`,
        button,
      );
    }
    case "error":
      return `<div class="render-error" role="alert">The audit failed.<pre class="error-output">${esc(s.message)}</pre></div>`;
    case "doc":
      return docHtml(view, s.model, s.exitCode);
  }
}

function emptyHtml(title: string, text: string, extra = ""): string {
  return `<div class="empty-state">
  <h1 class="empty-title">${esc(title)}</h1>
  <p class="empty-text">${text}</p>
  ${extra}
</div>`;
}

function docHtml(
  view: ChangeView,
  m: ChangeModel,
  exitCode: number | null,
): string {
  const branch = view.headBranch ? `<b>${esc(view.headBranch)}</b> ` : "";
  const exit =
    exitCode !== 0 && exitCode !== null
      ? ` · <span class="fail">exit ${exitCode}</span>`
      : "";
  const counters = m.counters
    .map(
      (c) =>
        `<span class="counter${c.value > 0 && c.tone ? " " + c.tone : ""}">${esc(c.label)} <b>${c.value}</b></span>`,
    )
    .join("");
  const decisions = decisionsHtml(view, m);
  const parts = [
    `<div class="ctx">
  <div class="ctx-line">head ${branch}${esc(m.head)} ← base ${esc(m.base)}${exit}</div>
  <div class="counters">${counters}</div>
</div>`,
    decisions,
    behaviorsHtml(view, m),
    listHtml("Information", m.information),
    evidenceHtml(m),
    unprovenHtml(m),
    listHtml("Not compared", m.incomplete),
    listHtml("Notes", m.notes),
  ];
  return parts.filter(Boolean).join("\n");
}

function decisionsHtml(view: ChangeView, m: ChangeModel): string {
  const head = `<div class="band-head">Needs a decision <span class="band-count">${m.openCount} open · ${m.acceptedCount} accepted</span></div>`;
  if (m.openCount === 0 && m.acceptedCount === 0) {
    return `<section class="band" id="decisions">${head}<div class="band-empty">Nothing got weaker.</div></section>`;
  }
  const groups = m.open.map((g) => groupHtml(view, g)).join("");
  const accepted =
    m.accepted.length > 0
      ? `<details class="accepted-group"><summary class="group-head">accepted · ${m.accepted.length}</summary>${m.accepted.map((r) => decisionHtml(view, r)).join("")}</details>`
      : "";
  return `<section class="band" id="decisions">${head}<div class="caption" title="${attr(ACCEPT_NOTE)}">Accept is local: kept in this workspace, never sent anywhere.</div>${groups}${accepted}</section>`;
}

function groupHtml(view: ChangeView, g: DecisionGroup): string {
  const rows = g.rows.map((r) => decisionHtml(view, r)).join("");
  if (!g.folded) return rows;
  return `<details class="decision-group" data-tag="${attr(g.tag)}"><summary class="group-head"><span class="tag-chip ${g.rows[0].tone}">${esc(g.tag)}</span> ${g.rows.length} findings</summary>${rows}</details>`;
}

function decisionHtml(view: ChangeView, r: DecisionRow): string {
  const last = r.labels[r.labels.length - 1] ?? "";
  const above = r.labels.slice(0, -1).join(" › ");
  const pkg = r.pkg ? `<span class="dim">${esc(r.pkg)} </span>` : "";
  const path = `${pkg}${above ? `<span class="dim">${esc(above)} › </span>` : ""}${esc(last)}`;
  const k = attr(r.key);
  const acts = [
    view.canOpen(r.key)
      ? `<button class="act" data-change="openDiff" data-key="${k}" title="Open the diff of this file against the base">Open diff</button>`
      : "",
    view.canRun(r.key)
      ? `<button class="act" data-change="run" data-key="${k}" title="Run this test">Run</button>`
      : "",
    `<button class="act" data-change="accept" data-key="${k}" title="${attr(ACCEPT_NOTE)}">${r.accepted ? "Reopen" : "Accept"}</button>`,
  ].join("");
  return `<div class="decision ${r.tone}${r.accepted ? " accepted" : ""}" data-key="${k}">
  <span class="tag-chip ${r.tone}">${esc(r.tag)}</span>
  <span class="path">${path}</span>
  <span class="fact">${esc(r.fact)}</span>
  ${r.where ? `<span class="where">${esc(r.where)}</span>` : ""}
  <span class="row-acts">${acts}</span>
</div>`;
}

function behaviorsHtml(view: ChangeView, m: ChangeModel): string {
  if (m.behaviors.length === 0) return "";
  const pkgs = m.behaviors
    .map((p) => {
      const suites = p.suites
        .map((s) => {
          const rows = s.rows.map((r) => behaviorHtml(view, r)).join("");
          return `<div class="suite-head">${esc(s.suite)}</div>${rows}`;
        })
        .join("");
      return `<details class="pkg-section" open><summary class="pkg-header">${esc(p.pkg)}</summary>${suites}</details>`;
    })
    .join("");
  return `<section class="band" id="behaviors"><div class="band-head">Changed behaviors <span class="band-count">${m.behaviorCount}</span></div>${pkgs}</section>`;
}

function behaviorHtml(view: ChangeView, r: BehaviorRow): string {
  const text = view.canOpen(r.key)
    ? `<a class="src-link" data-change="openSource" data-key="${attr(r.key)}">${esc(r.text)}</a>`
    : esc(r.text);
  return `<div class="behavior ${r.change}"><span class="sign ${r.change}">${esc(r.sign)}</span> <span class="change-word">${esc(r.change)}</span> ${text} <span class="where">${esc(r.where)}</span></div>`;
}

function listHtml(title: string, items: string[]): string {
  if (items.length === 0) return "";
  return `<section class="band"><div class="band-head">${esc(title)}</div><ul class="plain-list">${items.map((i) => `<li>${esc(i)}</li>`).join("")}</ul></section>`;
}

function evidenceHtml(m: ChangeModel): string {
  if (m.evidence.length === 0) return "";
  const rows = m.evidence
    .map(
      (e) =>
        `<div class="behavior"><span class="tag-chip ${e.verdict === "pass" ? "pass" : "warn"}">${esc(e.verdict)}</span> <span class="dim">${esc(e.pkg)}</span> ${esc(e.text)} <span class="where">${esc(e.facts)}</span></div>`,
    )
    .join("");
  return `<section class="band"><div class="band-head">Evidence</div>${rows}</section>`;
}

function unprovenHtml(m: ChangeModel): string {
  if (m.unproven.length === 0) return "";
  const rows = m.unproven
    .map((u) => {
      const n = u.lines.length;
      return `<div class="behavior"><span class="mono">${esc(u.file)}</span> <span class="fact">${n} changed line${n === 1 ? "" : "s"} no test executed: ${esc(u.lines.join(", "))}</span></div>`;
    })
    .join("");
  return `<section class="band"><div class="band-head">Unproven</div>${rows}</section>`;
}

export const CHANGE_CSS = `
.scopes { display: flex; align-items: stretch; border-bottom: 1px solid var(--vscode-panel-border); flex-shrink: 0; padding: 0 8px; }
.scope-tab { background: none; border: 0; border-bottom: 2px solid transparent; color: var(--vscode-descriptionForeground); padding: 6px 14px; cursor: pointer; font-size: 0.9em; }
.scope-tab:hover { color: var(--vscode-editor-foreground); }
.scope-tab[aria-selected="true"] { color: var(--vscode-editor-foreground); border-bottom-color: var(--vscode-focusBorder); }
.change-container { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.change-body { flex: 1; overflow-y: auto; padding: 0 0 12px 0; line-height: 1.6; }
.busy { color: var(--vscode-descriptionForeground); font-size: 0.85em; align-self: center; }
.ctx { padding: 8px 12px; border-bottom: 1px solid var(--vscode-panel-border); }
.ctx-line { color: var(--vscode-descriptionForeground); font-size: 0.9em; overflow-wrap: anywhere; }
.ctx-line b { color: var(--vscode-editor-foreground); font-weight: 600; }
.counters { display: flex; flex-wrap: wrap; gap: 4px 16px; font-size: 0.85em; color: var(--vscode-descriptionForeground); margin-top: 4px; }
.counter b { color: var(--vscode-editor-foreground); }
.counter.warn b { color: var(--vscode-testing-iconSkipped); }
.counter.fail b, .counter.del b { color: var(--vscode-testing-iconFailed); }
.counter.add b { color: var(--vscode-testing-iconPassed); }
.band { border-bottom: 1px solid var(--vscode-panel-border); padding: 4px 0 8px 0; }
.band-head { padding: 6px 12px 2px; font-size: 0.8em; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--vscode-descriptionForeground); }
.band-count { font-weight: normal; letter-spacing: 0; text-transform: none; margin-left: 6px; }
.band-empty, .caption { padding: 2px 12px; color: var(--vscode-descriptionForeground); font-size: 0.85em; }
.decision { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 10px; padding: 3px 12px; }
.decision:hover { background: var(--vscode-list-hoverBackground); }
.decision .path { flex: 1 1 240px; min-width: 0; overflow-wrap: anywhere; }
.decision.accepted .path { text-decoration: line-through; color: var(--vscode-descriptionForeground); }
.dim { color: var(--vscode-descriptionForeground); }
.fact, .where { font-size: 0.85em; color: var(--vscode-descriptionForeground); }
.where { font-family: var(--vscode-editor-font-family); }
.row-acts { display: inline-flex; gap: 2px; }
.tag-chip { font-size: 0.75em; font-weight: 600; padding: 0 6px; border-radius: 3px; min-width: 9ch; text-align: center; border: 1px solid currentColor; }
.tag-chip.fail { color: var(--vscode-testing-iconFailed); }
.tag-chip.warn { color: var(--vscode-testing-iconSkipped); }
.tag-chip.info, .tag-chip.none { color: var(--vscode-descriptionForeground); }
.tag-chip.pass { color: var(--vscode-testing-iconPassed); }
details.decision-group > summary, details.accepted-group > summary { cursor: pointer; padding: 3px 12px; list-style: none; }
details.decision-group > summary::-webkit-details-marker, details.accepted-group > summary::-webkit-details-marker { display: none; }
details.decision-group > summary::before, details.accepted-group > summary::before { content: "▸ "; color: var(--vscode-descriptionForeground); }
details.decision-group[open] > summary::before, details.accepted-group[open] > summary::before { content: "▾ "; }
.group-head { color: var(--vscode-descriptionForeground); font-size: 0.9em; }
.change-body details.pkg-section { margin: 4px 12px 0; }
.suite-head { padding: 2px 0 0 12px; font-weight: 600; font-size: 0.9em; }
.behavior { padding: 1px 12px 1px 24px; overflow-wrap: anywhere; }
.sign { font-weight: 600; display: inline-block; width: 1.2em; }
.sign.added { color: var(--vscode-testing-iconPassed); }
.sign.removed { color: var(--vscode-testing-iconFailed); }
.sign.reworded, .sign.moved, .sign.changed { color: var(--vscode-testing-iconSkipped); }
.change-word { font-size: 0.8em; color: var(--vscode-descriptionForeground); display: inline-block; min-width: 8ch; }
.plain-list { margin: 0; padding: 0 12px 0 32px; font-size: 0.9em; }
.mono { font-family: var(--vscode-editor-font-family); }
.fail { color: var(--vscode-testing-iconFailed); }
`;
