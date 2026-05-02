import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({}));

import { ansiToHtml } from "./specView.js";

describe("ansiToHtml", () => {
  it("returns plain text unchanged", () => {
    expect(ansiToHtml("hello world")).toBe("hello world");
  });

  it("escapes HTML entities", () => {
    expect(ansiToHtml("<div>&test</div>")).toBe("&lt;div&gt;&amp;test&lt;/div&gt;");
  });

  it("converts bold escape to span", () => {
    expect(ansiToHtml("\x1b[1mbold\x1b[0m")).toBe(
      '<span class="ansi-bold">bold</span>',
    );
  });

  it("converts dim escape to span", () => {
    expect(ansiToHtml("\x1b[2mdim\x1b[0m")).toBe(
      '<span class="ansi-dim">dim</span>',
    );
  });

  it("converts color codes", () => {
    expect(ansiToHtml("\x1b[31mred\x1b[0m")).toBe('<span class="ansi-red">red</span>');
    expect(ansiToHtml("\x1b[32mgreen\x1b[0m")).toBe('<span class="ansi-green">green</span>');
    expect(ansiToHtml("\x1b[33myellow\x1b[0m")).toBe('<span class="ansi-yellow">yellow</span>');
  });

  it("handles nested styles", () => {
    const result = ansiToHtml("\x1b[1m\x1b[31mbold red\x1b[0m");
    expect(result).toBe('<span class="ansi-bold"><span class="ansi-red">bold red</span></span>');
  });

  it("closes unclosed spans at end", () => {
    const result = ansiToHtml("\x1b[32mno reset");
    expect(result).toBe('<span class="ansi-green">no reset</span>');
  });

  it("ignores unknown escape codes", () => {
    expect(ansiToHtml("\x1b[99munknown\x1b[0m")).toBe("unknown");
  });

  it("handles empty string", () => {
    expect(ansiToHtml("")).toBe("");
  });

  it("handles text with mixed ANSI and HTML chars", () => {
    const result = ansiToHtml("\x1b[31m<error>\x1b[0m");
    expect(result).toBe('<span class="ansi-red">&lt;error&gt;</span>');
  });
});
