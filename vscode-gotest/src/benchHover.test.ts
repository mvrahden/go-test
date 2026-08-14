import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  Hover: class {
    constructor(public contents: unknown) {}
  },
  MarkdownString: class {
    constructor(public value: string) {}
  },
}));

import { sparkline, buildBenchHoverMarkdown } from "./benchHover.js";
import type { BenchEntry } from "./benchResultStore.js";

function entry(overrides: Partial<BenchEntry> = {}): BenchEntry {
  return {
    nsPerOp: 100,
    bytesPerOp: 480,
    allocsPerOp: 3,
    iterations: 10,
    sampleCount: 1,
    minNsPerOp: 100,
    maxNsPerOp: 100,
    recordedAt: Date.parse("2026-08-14T11:58:00Z"),
    goos: "linux",
    goarch: "amd64",
    ...overrides,
  };
}

describe("sparkline", () => {
  it("scales values into the block range, oldest first", () => {
    const line = sparkline([0, 50, 100]);
    expect(line).toHaveLength(3);
    expect(line[0]).toBe("▁");
    expect(line[2]).toBe("█");
  });

  it("renders a flat line for identical values", () => {
    expect(sparkline([5, 5, 5])).toBe("▁▁▁");
  });

  it("renders nothing for no values", () => {
    expect(sparkline([])).toBe("");
  });
});

describe("buildBenchHoverMarkdown", () => {
  const now = Date.parse("2026-08-14T12:00:00Z");

  it("returns nothing when the method has never been measured here", () => {
    expect(buildBenchHoverMarkdown("BenchmarkX", undefined, [], now)).toBe(
      undefined,
    );
  });

  it("shows the latest numbers, platform, and age", () => {
    const md = buildBenchHoverMarkdown("BenchmarkGetHit", entry(), [], now)!;
    expect(md).toContain("**BenchmarkGetHit** — 100 ns/op · 2m ago");
    expect(md).toContain("480 B/op · 3 allocs/op · linux/amd64");
    expect(md).not.toContain("Trend");
  });

  it("shows mean ± spread for a stable multi-count run", () => {
    const md = buildBenchHoverMarkdown(
      "BenchmarkGetHit",
      entry({ sampleCount: 5, minNsPerOp: 95, maxNsPerOp: 105 }),
      [],
      now,
    )!;
    expect(md).toContain("100 ns/op ±5.0% (mean of 5×)");
  });

  it("draws the trend for repeated runs, endpoints spelled out", () => {
    const history = [
      { nsPerOp: 100, recordedAt: 1, sampleCount: 1 },
      { nsPerOp: 150, recordedAt: 2, sampleCount: 1 },
      { nsPerOp: 80, recordedAt: 3, sampleCount: 1 },
    ];
    const md = buildBenchHoverMarkdown(
      "BenchmarkGetHit",
      entry(),
      history,
      now,
    )!;
    expect(md).toContain("Trend (last 3 runs)");
    expect(md).toContain("100 ns/op → 80 ns/op");
  });
});
