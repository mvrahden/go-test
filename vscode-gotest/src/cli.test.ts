import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [],
    getConfiguration: () => ({
      get: () => undefined,
    }),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  window: { showWarningMessage: vi.fn(), showErrorMessage: vi.fn() },
}));

import { compareVersions, escapeRegExp, formatCliCommand } from "./cli.js";

describe("compareVersions", () => {
  it("returns 0 for equal versions", () => {
    expect(compareVersions("v1.2.3", "v1.2.3")).toBe(0);
  });

  it("returns positive when first is greater (major)", () => {
    expect(compareVersions("v2.0.0", "v1.9.9")).toBeGreaterThan(0);
  });

  it("returns negative when first is less (minor)", () => {
    expect(compareVersions("v1.2.0", "v1.3.0")).toBeLessThan(0);
  });

  it("returns positive when first has higher patch", () => {
    expect(compareVersions("v1.0.2", "v1.0.1")).toBeGreaterThan(0);
  });

  it("handles missing patch component", () => {
    expect(compareVersions("v1.0", "v1.0.0")).toBe(0);
    expect(compareVersions("v1.0", "v1.0.1")).toBeLessThan(0);
  });

  it("strips v prefix correctly", () => {
    expect(compareVersions("1.2.3", "v1.2.3")).toBe(0);
  });
});

describe("escapeRegExp", () => {
  it("escapes special regex characters", () => {
    expect(escapeRegExp("a.b*c")).toBe("a\\.b\\*c");
  });

  it("escapes brackets and parens", () => {
    expect(escapeRegExp("foo[0](bar)")).toBe("foo\\[0\\]\\(bar\\)");
  });

  it("escapes module paths", () => {
    expect(escapeRegExp("github.com/foo/bar")).toBe("github\\.com/foo/bar");
  });
});

describe("formatCliCommand", () => {
  it("joins bin and args", () => {
    expect(formatCliCommand({ bin: "gotest", args: ["run", "./..."] })).toBe(
      "gotest run ./...",
    );
  });

  it("handles empty args", () => {
    expect(formatCliCommand({ bin: "gotest", args: [] })).toBe("gotest ");
  });
});
