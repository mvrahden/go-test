// The webview's string helpers. Every factory under client/ is embedded in
// the panel's document by its source, so it may use its parameters and the
// webview's globals only: no imports but types.
import type { OutputLine } from "../protocol.js";

export type Helpers = ReturnType<typeof clientHelpers>;

export function clientHelpers() {
  // esc makes any text safe as HTML content or a quoted attribute.
  const esc = (s: unknown): string =>
    String(s ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");

  const data = (d: Record<string, string | number | undefined>): string =>
    Object.keys(d)
      .filter((k) => d[k] !== undefined)
      .map((k) => ` data-${k}="${esc(d[k])}"`)
      .join("");

  // line is an output line, its file:line tokens as links to the source.
  const line = (segs: OutputLine): string =>
    segs
      .map((s) =>
        typeof s === "string"
          ? esc(s)
          : `<a class="src-link" href="#" data-act="goTo"${data({ file: s[1], line: s[2] })}>${esc(s[0])}</a>`,
      )
      .join("");

  const lines = (ls: OutputLine[]): string => ls.map(line).join("\n");

  // button is a real button: an act and what it acts on, or a label alone.
  const button = (
    label: string,
    act: string,
    d: Record<string, string | number | undefined> = {},
    cls = "link",
    extra = "",
  ): string =>
    `<button type="button" class="${cls}" data-act="${esc(act)}"${data(d)}${extra}>${esc(label)}</button>`;

  const STATUS: Record<string, [string, string]> = {
    pass: ["✓", "passed"],
    fail: ["✗", "failed"],
    skip: ["~", "skipped"],
    none: ["?", "not run"],
  };

  // icon is a status as a glyph and, for a screen reader, a word.
  const icon = (status: string): string => {
    const [glyph, word] = STATUS[status] ?? STATUS.none;
    return `<span class="ic s-${esc(status)}" aria-hidden="true">${glyph}</span><span class="sr">${word}</span>`;
  };

  const section = (title: string, extra = ""): string =>
    `<h2 class="eyebrow">${esc(title)}</h2>${extra}`;

  const empty = (
    e: { title: string; text: string; action?: { act: string; label: string } },
    scope = "",
  ): string =>
    `<div class="empty"><h1>${esc(e.title)}</h1><p>${esc(e.text)}</p>${
      e.action
        ? button(
            e.action.label,
            e.action.act,
            { scope: scope || undefined },
            "primary",
          )
        : ""
    }</div>`;

  return { esc, data, line, lines, button, icon, section, empty };
}
