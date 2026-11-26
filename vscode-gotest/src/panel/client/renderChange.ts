// The Change tab's markup, from the host's model: pure strings, embedded in
// the webview by source (see helpers.ts).
import type {
  ChangeBehaviorView,
  ChangeTab,
  DecisionView,
  Mark,
  Picker,
} from "../protocol.js";
import type { Helpers } from "./helpers.js";

export type ChangeRender = ReturnType<typeof renderChange>;

export interface ChangeUi {
  allDecisions: boolean;
  showAccepted: boolean;
  review: Record<string, boolean>;
  open: Record<string, boolean>;
  rows: Record<string, ChangeBehaviorView[]>;
}

export function renderChange(h: Helpers) {
  const { esc, button } = h;

  const picker = (folders: Picker[] | undefined, scope: string): string =>
    folders
      ? `<label class="field"><span class="dim">Folder</span> <select data-change="folder" data-scope="${scope}">${folders
          .map(
            (f) =>
              `<option value="${esc(f.path)}"${f.selected ? " selected" : ""}>${esc(f.label)}</option>`,
          )
          .join("")}</select></label>`
      : "";

  const mark = (m: Mark | undefined): string =>
    m
      ? ` <span class="mark t-${m.tone}" title="${esc(m.detail)}">${esc(m.text)}</span>`
      : "";

  const decision = (d: DecisionView): string => {
    const k = { key: d.key, scope: "change" };
    const open =
      d.open === "diff"
        ? button("Diff", "openDiff", k)
        : d.open === "source"
          ? button("Open", "openSource", k)
          : "";
    return `<div class="decision${d.accepted ? " accepted" : ""}">
<span class="kind t-${d.tone}">${esc(d.kind)}</span>
<span class="what">${esc(d.what)}${mark(d.mark)}</span>
<span class="fact mono t-${d.tone}">${esc(d.fact)}</span>
<span class="where mono dim">${esc(d.where).replace(/\//g, "/<wbr>")}</span>
<span class="d-acts">${open}${button(d.accepted ? "Reopen" : "Accept", "accept", k, "btn small", ` aria-pressed="${d.accepted}"`)}</span>
</div>`;
  };

  const behavior = (r: ChangeBehaviorView): string => {
    const k = { key: r.key, scope: "change" };
    const text = r.canOpen
      ? button(r.text, "openSource", k, "link wrap")
      : esc(r.text);
    return `<div class="behavior"><span class="sign c-${esc(r.change)}" aria-hidden="true">${esc(r.sign)}</span><span class="sr">${esc(r.change)}</span> <span class="dim">${esc(r.suite)} ›</span> ${text}${mark(r.mark)} <span class="mono dim">${esc(r.where)}</span>${r.canRun ? " " + button("Run", "run", k, "act") : ""}</div>`;
  };

  const context = (m: ChangeTab): string => {
    const head = m.head
      ? `<span class="mono strong">${esc(m.head)}</span>`
      : "";
    const base = `<span class="dim">compared with</span> ${button(m.base ?? "choose a base…", "chooseBase", { scope: "change" }, "base-btn mono", m.canChooseBase ? ` aria-label="Base: ${esc(m.base ?? "none")}. Choose another"` : " disabled")}`;
    const ev = m.evidence;
    const evidence = ev
      ? `<div class="dim">${esc(ev.text)}${ev.action ? " " + button(ev.action.label, ev.action.act, { scope: "change" }) : ""}${ev.picks
          .map(
            (p) =>
              ` <span class="pick${p.missing ? " t-warn" : ""}"${p.missing ? ` title="Its events file is gone; the audit does not get it"` : ""}>${esc(p.label)}${button("×", "dropEvidence", { key: p.id, scope: "change" }, "act", ` aria-label="Stop using ${esc(p.label)} as evidence"`)}</span>`,
          )
          .join("")}</div>`
      : "";
    const busy =
      m.busy === "run"
        ? `<div class="live-line" role="status"><span class="dot" aria-hidden="true"></span>Auditing with a run… ${button("Stop", "cancel", { scope: "change" }, "btn small danger")}</div>`
        : m.busy
          ? `<div class="dim" role="status">Auditing…</div>`
          : "";
    return `<div class="ctx-top">${head}${base}${picker(m.folders, "change")}${m.exit ? `<span class="t-warn">${esc(m.exit)}</span>` : ""}</div>${evidence}${busy}`;
  };

  const body = (m: ChangeTab, ui: ChangeUi): string => {
    if (m.empty) return h.empty(m.empty, "change");
    const parts: string[] = [];
    if (m.notice)
      parts.push(`<div class="notice" role="status">${esc(m.notice)}</div>`);
    if (m.error) {
      parts.push(
        `<div class="notice warn" role="alert">The audit failed.<pre class="row-out">${esc(m.error)}</pre></div>`,
      );
      return parts.join("");
    }
    parts.push(
      `<div class="headline"><h1>${esc(m.headline ?? "")}</h1><p class="dim">${esc(m.sub ?? "")}</p></div>`,
    );
    const shown = ui.allDecisions ? m.decisions : m.decisions.slice(0, m.shown);
    if (shown.length > 0)
      parts.push(
        `<div class="decisions">${shown.map(decision).join("")}</div>`,
      );
    if (m.moreText && !ui.allDecisions)
      parts.push(
        `<button type="button" class="more-row" data-act="allDecisions"><span>${esc(m.moreText)}</span><span class="link-text">Show all ${m.decisions.length}</span></button>`,
      );
    if (m.accepted.length > 0)
      parts.push(
        `<div class="quiet-row"><span>${m.accepted.length} accepted</span>${button(ui.showAccepted ? "Hide" : "Show", "showAccepted", {}, "link", ` aria-expanded="${ui.showAccepted}"`)}</div>${
          ui.showAccepted
            ? `<div class="decisions">${m.accepted.map(decision).join("")}</div>`
            : ""
        }`,
      );
    if (m.routine.length > 0)
      parts.push(
        `${h.section("Routine, accept as a group")}<div class="list">${m.routine
          .map((g) => {
            const all = g.accepted === g.count;
            const review = !!ui.review[g.id];
            return `<div class="group-row"><span><strong>${g.count.toLocaleString("en-US")}</strong> ${esc(g.what)}${g.accepted > 0 && !all ? ` <span class="dim">· ${g.accepted} accepted</span>` : ""}${all ? ` <span class="t-pass">· accepted</span>` : ""}</span>${button(review ? "Hide" : "Review", "review", { group: g.id }, "link", ` aria-expanded="${review}"`)}${button(all ? "Reopen all" : "Accept all", "acceptAll", { key: g.id, scope: "change" }, "btn small")}</div>${
              review
                ? `<div class="decisions inset">${g.rows.map(decision).join("")}</div>`
                : ""
            }`;
          })
          .join("")}</div>`,
      );
    if (m.lines) {
      const l = m.lines;
      const open = (
        key: string,
        text: string,
        ok: boolean,
        cls = "link mono",
      ) =>
        ok
          ? button(text, "openSource", { key, scope: "change" }, cls)
          : `<span class="mono">${esc(text)}</span>`;
      const rows = l.files.map(
        (f) =>
          `<div class="line-row"><span class="mono">${esc(f.file)}</span><span class="ranges">${f.ranges
            .slice(0, 6)
            .map((r) => open(r.key, r.text, f.canOpen, "link mono warn-link"))
            .join(
              ", ",
            )}${f.ranges.length > 6 ? ` <span class="dim">+${f.ranges.length - 6} more</span>` : ""}</span>${f.canOpen ? button("Open", "openSource", { key: f.key, scope: "change" }) : ""}</div>`,
      );
      if (l.noCoverage)
        rows.push(
          `<div class="line-row note"><span class="dim">${esc(l.noCoverage)}</span>${button("Run what changed", "auditRun", { scope: "change" })}</div>`,
        );
      if (l.note)
        rows.push(
          `<div class="line-row note"><span class="dim">${esc(l.note)}</span>${l.head ? "" : button("Run what changed", "auditRun", { scope: "change" })}</div>`,
        );
      parts.push(
        `<div class="section-head">${h.section("Changed lines no test ran")}${l.head ? `<span class="t-warn">${esc(l.head)}</span>` : ""}</div><div class="list">${rows.join("")}</div><p class="dim footnote">Per-file percentages and line gutters stay in VS Code's Test Coverage view.</p>`,
      );
    }
    if (m.packages.length > 0)
      parts.push(
        `${h.section("Behaviors by package")}<div class="list">${m.packages
          .map((p) => {
            const open = !!ui.open[p.pkg];
            const rows = ui.rows[p.pkg];
            return `<button type="button" class="pkg-head" data-act="changePackage" data-pkg="${esc(p.pkg)}" aria-expanded="${open}"><svg class="chev${open ? " open" : ""}" aria-hidden="true" width="12" height="12" viewBox="0 0 12 12"><path d="M4 2l4 4-4 4"/></svg><span class="mono name">${esc(p.name)}</span><span class="dim">${esc(p.delta)}</span></button>${
              open
                ? `<div class="pkg-tree">${rows ? rows.map(behavior).join("") : `<div class="dim pad">Loading…</div>`}</div>`
                : ""
            }`;
          })
          .join("")}</div>`,
      );
    for (const l of m.lists)
      parts.push(
        `<details class="extra"><summary>${esc(l.title)} <span class="dim">${l.items.length}</span></summary><ul class="plain">${l.items
          .map((i) => `<li>${esc(i)}</li>`)
          .join("")}</ul></details>`,
      );
    parts.push(
      `<p class="dim footnote">Accepting is a note in this workspace only. It resets when the head commit changes.</p>`,
    );
    return parts.join("");
  };

  return { context, body, decision, behavior };
}
