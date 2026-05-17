import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Uri: {
    joinPath: (...args: string[]) => ({ toString: () => args.join("/") }),
  },
  workspace: {
    getConfiguration: () => ({ get: () => true }),
  },
  window: {},
  ViewColumn: { Beside: 2 },
  commands: {},
}));

import { specDataToReport } from "./specView.js";

function leaf(
  name: string,
  status: string,
  duration = 0,
): {
  name: string;
  display: string;
  kind: string;
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  external: boolean;
  output: string[];
  children: never[];
} {
  return {
    name,
    display: name,
    kind: "behavior",
    status,
    duration,
    focused: false,
    excluded: false,
    external: false,
    output: [],
    children: [],
  };
}

function suite(
  name: string,
  children: ReturnType<typeof leaf>[],
): {
  name: string;
  display: string;
  kind: string;
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  external: boolean;
  output: string[];
  children: ReturnType<typeof leaf>[];
} {
  return {
    name,
    display: name,
    kind: "suite",
    status: "pass",
    duration: 0,
    focused: false,
    excluded: false,
    external: false,
    output: [],
    children,
  };
}

describe("specDataToReport", () => {
  const data = {
    packages: [
      {
        path: "example.com/pkg",
        status: "fail",
        duration: 1.5,
        nodes: [
          suite("MySuite", [
            leaf("passes", "pass", 0.5),
            leaf("fails", "fail", 0.8),
            leaf("skipped", "skip", 0),
          ]),
        ],
      },
    ],
    stats: {
      suites: 1,
      behaviors: 3,
      tests: 0,
      passed: 1,
      failed: 1,
      skipped: 1,
    },
  };

  it("includes all statuses when no filter is set", () => {
    const report = specDataToReport(data, []);
    expect(report).toContain("passes");
    expect(report).toContain("fails");
    expect(report).toContain("skipped");
    expect(report).toContain("1 passed");
    expect(report).toContain("1 failed");
    expect(report).toContain("1 skipped");
  });

  it("excludes passed leaves when pass is hidden", () => {
    const report = specDataToReport(data, [], new Set(["pass"]));
    expect(report).not.toContain("passes");
    expect(report).toContain("fails");
    expect(report).toContain("skipped");
    expect(report).not.toContain("1 passed");
    expect(report).toContain("1 failed");
    expect(report).toContain("1 skipped");
  });

  it("excludes failed leaves when fail is hidden", () => {
    const report = specDataToReport(data, [], new Set(["fail"]));
    expect(report).toContain("passes");
    expect(report).not.toContain("  fails");
    expect(report).toContain("skipped");
    expect(report).toContain("1 passed");
    expect(report).not.toContain("1 failed");
    expect(report).toContain("1 skipped");
  });

  it("excludes skipped leaves when skip is hidden", () => {
    const report = specDataToReport(data, [], new Set(["skip"]));
    expect(report).toContain("passes");
    expect(report).toContain("fails");
    expect(report).not.toMatch(/\bskipped\b/);
    expect(report).toContain("1 passed");
    expect(report).toContain("1 failed");
  });

  it("hides multiple statuses at once", () => {
    const report = specDataToReport(data, [], new Set(["pass", "skip"]));
    expect(report).not.toContain("passes");
    expect(report).toContain("fails");
    expect(report).not.toMatch(/\bskipped\b/);
    expect(report).toContain("1 failed");
  });

  it("omits branches with no visible leaves", () => {
    const report = specDataToReport(
      data,
      [],
      new Set(["pass", "fail", "skip"]),
    );
    expect(report).not.toContain("MySuite");
    expect(report).not.toContain("pkg");
  });

  it("preserves package duration when unfiltered", () => {
    const report = specDataToReport(data, []);
    expect(report).toContain("1.500s");
  });

  it("uses leaf-aggregated duration when filtered", () => {
    const report = specDataToReport(data, [], new Set(["pass", "skip"]));
    expect(report).not.toContain("1.500s");
    expect(report).toContain("0.800s");
  });

  it("preserves structural counts in summary", () => {
    const report = specDataToReport(data, [], new Set(["pass"]));
    expect(report).toContain("1 suites");
    expect(report).toContain("3 behaviors");
  });
});
