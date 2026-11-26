// The Runs tab's markup, from the host's model: pure strings, embedded in the
// webview by source (see helpers.ts).
import type { RunCard, RunningItem, RunsTab } from "../protocol.js";
import type { Helpers } from "./helpers.js";

export type RunsRender = ReturnType<typeof renderRuns>;

export interface RunsUi {
  expanded: Record<string, boolean>;
  command: Record<string, boolean>;
  earlier: boolean;
}

export function renderRuns(h: Helpers) {
  const { esc, button } = h;

  const running = (items: RunningItem[]): string =>
    items
      .map(
        (
          r,
        ) => `<div class="running" role="group" aria-label="Running: ${esc(r.label)}">
<div class="running-top"><span class="dot" aria-hidden="true"></span><strong>Running now</strong><span class="dim">${esc(r.label)}, started ${esc(r.elapsed)} ago</span><span class="grow"></span>${
          r.stopping
            ? `<span class="dim" role="status">Stopping…</span>`
            : button(
                "Stop",
                "stopSession",
                { session: r.id },
                "btn small danger",
                ` aria-label="Stop ${esc(r.label)}"`,
              )
        }</div>
<div class="bar" aria-hidden="true"><div class="bar-run"></div></div>${r.counts ? `<div class="dim">${esc(r.counts)}</div>` : ""}</div>`,
      )
      .join("");

  const evidenceButton = (c: RunCard): string => {
    if (!c.canOpen) return "";
    const label =
      c.evidence === "chosen"
        ? "Evidence in use"
        : c.evidence === "auto"
          ? "Evidence in use (newest current run)"
          : "Use as evidence";
    return button(
      label,
      "evidence",
      { key: c.key, scope: "runs" },
      "link",
      ` aria-pressed="${c.evidence === "chosen"}"`,
    );
  };

  // An earlier commit's card names its commit beside the day.
  const card = (
    c: RunCard,
    ui: RunsUi,
    inner = false,
    other = false,
  ): string => {
    const cmd = !!ui.command[c.key];
    const open = !!ui.expanded[c.key];
    const acts = [
      c.canOpen
        ? button("Open results", "open", { key: c.key, scope: "runs" })
        : "",
      evidenceButton(c),
      button(
        cmd ? "Hide command" : "Command",
        "command",
        { key: c.key },
        "link",
        ` aria-expanded="${cmd}"`,
      ),
      c.runs
        ? button(
            open ? "Hide the runs" : `Show the ${c.runs.length} runs`,
            "expandRuns",
            { key: c.key },
            "link",
            ` aria-expanded="${open}"`,
          )
        : "",
    ].join("");
    return `<article class="run-card${inner ? " inner" : ""}${c.evidence !== "none" ? " chosen" : ""}" aria-label="${esc(c.time)} ${esc(c.title)}">
<span class="mono strong">${esc(c.time)}</span><span class="strong title">${esc(c.title)}</span><span class="verdict t-${c.tone}">${esc(c.verdict)}</span>
<span class="dim">${esc(c.day)}${other && c.commit ? ` <span class="mono">${esc(c.commit)}</span>` : ""}</span><span class="dim">${esc(c.who)} · ${esc(c.counts)}</span><span class="t-${c.freshTone} right">${esc(c.fresh)}</span>
<span></span><span class="acts">${acts}</span><span></span>${
      cmd
        ? `<span></span><code class="cmd">${esc(c.command)}</code><span></span>`
        : ""
    }</article>${open && c.runs ? `<div class="inset">${c.runs.map((r) => card(r, ui, true)).join("")}</div>` : ""}`;
  };

  const body = (m: RunsTab, ui: RunsUi): string => {
    if (m.empty) return h.empty(m.empty, "runs");
    const parts: string[] = [];
    if (m.notice)
      parts.push(`<div class="notice" role="status">${esc(m.notice)}</div>`);
    if (m.error) {
      parts.push(
        `<div class="notice warn" role="alert">The run records could not be read.<pre class="row-out">${esc(m.error)}</pre></div>`,
      );
      return parts.join("");
    }
    if (m.headline)
      parts.push(
        `<div class="headline"><h1>${esc(m.headline)}</h1>${m.sub ? `<p class="dim">${esc(m.sub)}</p>` : ""}</div>`,
      );
    parts.push(m.cards.map((c) => card(c, ui)).join(""));
    if (m.earlier.length > 0)
      parts.push(
        `${h.section("Earlier commits")}<button type="button" class="more-row" data-act="earlier" aria-expanded="${ui.earlier}"><span>${m.earlier.length} ${m.earlier.length === 1 ? "record" : "records"} of other commits</span><span class="link-text">${ui.earlier ? "Hide" : "Show"}</span></button>${
          ui.earlier
            ? m.earlier.map((c) => card(c, ui, false, true)).join("")
            : ""
        }`,
      );
    if (m.footer) parts.push(`<p class="dim footnote">${esc(m.footer)}</p>`);
    return parts.join("");
  };

  const folders = (m: RunsTab): string =>
    m.folders
      ? `<div class="ctx-top"><label class="field"><span class="dim">Folder</span> <select data-change="folder" data-scope="runs">${m.folders
          .map(
            (f) =>
              `<option value="${esc(f.path)}"${f.selected ? " selected" : ""}>${esc(f.label)}</option>`,
          )
          .join("")}</select></label></div>`
      : "";

  return { running, card, body, folders };
}
