// The Runs scope's HTML: the folder's run records as the CLI lists them,
// or what stands in for them. Pure strings; the panel wires the clicks.
import type { RunRow } from "./runsModel.js";

export type RunsState =
  | { kind: "idle" }
  | {
      kind: "oldCli";
      version?: string;
      floor: string;
      advice: string;
      canUpgrade: boolean;
    }
  | { kind: "error"; message: string }
  | { kind: "doc"; rows: RunRow[] };

export interface RunsView {
  state: RunsState;
  busy: boolean;
  folders: { path: string; label: string; selected: boolean }[];
  notice?: string;
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function attr(s: string): string {
  return esc(s).replace(/"/g, "&quot;");
}

export const RUNS_EMPTY_TEXT =
  "Every gotest run of this folder leaves a record, whether it was started from a terminal, by an agent or from this editor; none is kept yet. Run the tests, or import the events file of a run made elsewhere, such as a CI run you downloaded.";

export function buildRunsBody(view: RunsView): string {
  return `<div class="change-container">
  ${toolbar(view)}
  <div class="change-body" id="runs-body">${content(view)}</div>
</div>`;
}

function toolbar(view: RunsView): string {
  const picker =
    view.folders.length > 1
      ? `<div class="toolbar-group"><select class="source-select" id="runs-folder" title="Whose runs to list">${view.folders
          .map(
            (f) =>
              `<option value="${attr(f.path)}"${f.selected ? " selected" : ""}>${esc(f.label)}</option>`,
          )
          .join("")}</select></div>`
      : "";
  const ready = view.state.kind === "doc";
  const off = ready && !view.busy ? "" : " disabled";
  const busy = view.busy
    ? `<span class="busy" role="status">Reading the run records…</span>`
    : "";
  return `<div class="toolbar">
  ${picker}
  <div class="toolbar-group search-group">${busy}</div>
  <div class="toolbar-group">
    <button class="tool-btn" data-runs="runChanged" title="Run only the packages whose inputs changed since the last record, and those that failed or never ran"${off}>Run changed</button>
    <button class="tool-btn" data-runs="import" title="Keep the run of a saved events file among this folder's records"${off}>Import run…</button>
    <button class="tool-btn" data-runs="refresh" title="List the records again"${view.busy ? " disabled" : ""}>Refresh</button>
  </div>
</div>`;
}

function emptyHtml(title: string, text: string, extra = ""): string {
  return `<div class="empty-state">
  <h1 class="empty-title">${esc(title)}</h1>
  <p class="empty-text">${text}</p>
  ${extra}
</div>`;
}

function content(view: RunsView): string {
  const notice = view.notice
    ? `<div class="notice" role="status">${esc(view.notice)}</div>`
    : "";
  const s = view.state;
  switch (s.kind) {
    case "idle":
      return emptyHtml(
        "Runs",
        view.busy ? "Reading the run records…" : "No gotest folder.",
      );
    case "oldCli": {
      const which = s.version ? `This folder's CLI is ${esc(s.version)}.` : "";
      const button = s.canUpgrade
        ? `<button class="empty-button" data-runs="upgrade">Upgrade…</button>`
        : "";
      return emptyHtml(
        `The Runs scope needs gotest ${esc(s.floor)}`,
        `${which} It reads <code>gotest runs</code>, which ${esc(s.floor)} brings, as do --changed and the stale marks of the Spec scope; the rest of the extension works as before.<br/>${esc(s.advice)}`,
        button,
      );
    }
    case "error":
      return `${notice}<div class="render-error" role="alert">The run records could not be read.<pre class="error-output">${esc(s.message)}</pre></div>`;
    case "doc":
      if (s.rows.length === 0)
        return (
          notice +
          emptyHtml(
            "No run recorded yet",
            esc(RUNS_EMPTY_TEXT),
            `<button class="empty-button" data-runs="import">Import run…</button>`,
          )
        );
      return `${notice}<section class="band" id="runs"><div class="band-head">Runs <span class="band-count">${s.rows.length}, newest first</span></div>${s.rows.map(rowHtml).join("")}</section>`;
  }
}

function rowHtml(r: RunRow): string {
  const k = attr(r.id);
  const facts = [
    `<span class="dim">${esc(r.who)}</span>`,
    r.commit ? `<span class="mono">${esc(r.commit)}</span>` : "",
    `<span class="fact">${esc(r.counts)}</span>`,
  ]
    .filter(Boolean)
    .join(" · ");
  const marks = [
    r.narrowed
      ? `<span class="tag-chip warn" title="${attr(r.narrowed)}">narrowed</span>`
      : "",
    r.imported
      ? `<span class="tag-chip info" title="Imported from ${attr(r.imported)}">imported</span>`
      : "",
  ].join("");
  const noFile = r.eventsFile
    ? "The events file is gone"
    : "The run wrote no events file (--events-file)";
  const acts = r.canOpen
    ? `<button class="act" data-runs="open" data-key="${k}" title="Render this run's events file in the Spec scope">Open in Spec</button><button class="act" data-runs="evidence" data-key="${k}" aria-pressed="${r.evidence}" title="Pass this run's events file to the Change scope's audits as --evidence">${r.evidence ? "Evidence ✓" : "Use as evidence"}</button>`
    : `<span class="dim" title="${attr(noFile)}">no events file</span>`;
  return `<div class="run-row${r.evidence ? " chosen" : ""}" data-key="${k}">
  <div class="run-line"><span class="tag-chip ${r.pass ? "pass" : "fail"}">${esc(r.verdict)}</span> <span class="run-when" title="${attr(r.whenTitle)}">${esc(r.when)}</span> ${marks} <span class="mono run-args" title="${attr(r.args)}">${esc(r.args)}</span> <span class="row-acts">${acts}</span></div>
  <div class="run-facts">${facts}${r.narrowed ? ` · <span class="fact">${esc(r.narrowed)}</span>` : ""}</div>
  <div class="run-judges ${r.judgesTone}">${esc(r.judges)}</div>
</div>`;
}

export const RUNS_CSS = `
.run-row { padding: 4px 12px; border-bottom: 1px solid var(--vscode-panel-border); }
.run-row:hover { background: var(--vscode-list-hoverBackground); }
.run-row.chosen { border-left: 2px solid var(--vscode-focusBorder); padding-left: 10px; }
.run-line { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 8px; }
.run-args { flex: 1 1 200px; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--vscode-descriptionForeground); font-size: 0.85em; }
.run-facts, .run-judges { font-size: 0.85em; padding-left: 2px; color: var(--vscode-descriptionForeground); overflow-wrap: anywhere; }
.run-judges.warn { color: var(--vscode-testing-iconSkipped); }
.evidence-chips { display: flex; flex-wrap: wrap; gap: 4px; padding: 4px 12px; border-bottom: 1px solid var(--vscode-panel-border); font-size: 0.85em; align-items: center; }
.evidence-chip { border: 1px solid var(--vscode-focusBorder); border-radius: 3px; padding: 0 2px 0 6px; }
.evidence-chip.missing { border-color: var(--vscode-testing-iconSkipped); color: var(--vscode-testing-iconSkipped); }
`;
