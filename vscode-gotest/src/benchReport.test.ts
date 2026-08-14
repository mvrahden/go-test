import { describe, it, expect } from "vitest";
import {
  parseBenchReport,
  formatNsPerOp,
  formatBenchAnnotation,
  formatAge,
} from "./benchReport.js";

const validReport = JSON.stringify({
  schemaVersion: 1,
  baseline: {
    schemaVersion: 1,
    createdAt: "2026-08-14T10:00:00Z",
    goVersion: "go1.26.5",
    goos: "linux",
    goarch: "amd64",
    results: [
      {
        package: "example.com/pkg",
        suite: "CacheTestSuite",
        name: "BenchmarkGetHit",
        samples: [
          { iterations: 100, nsPerOp: 56.1, bytesPerOp: 0, allocsPerOp: 0 },
        ],
      },
    ],
  },
});

describe("parseBenchReport", () => {
  it("parses a valid schema-1 report", () => {
    const report = parseBenchReport(validReport);
    expect(report.baseline.goos).toBe("linux");
    expect(report.baseline.results).toHaveLength(1);
    expect(report.baseline.results[0].name).toBe("BenchmarkGetHit");
    expect(report.deltas).toBeUndefined();
    expect(report.gate).toBeUndefined();
  });

  it("rejects an unknown schema version instead of guessing", () => {
    const doc = JSON.stringify({ schemaVersion: 2, baseline: {} });
    expect(() => parseBenchReport(doc)).toThrow(/schema version 2/);
  });

  it("rejects non-JSON output with the head of the text for context", () => {
    expect(() => parseBenchReport("FAIL: something broke")).toThrow(
      /not a bench report/,
    );
  });

  it("carries deltas and gate through untouched", () => {
    const doc = JSON.stringify({
      schemaVersion: 1,
      baseline: {
        schemaVersion: 1,
        goos: "linux",
        goarch: "amd64",
        results: [],
      },
      deltas: [
        {
          key: "example.com/pkg CacheTestSuite/BenchmarkGetHit",
          oldNs: 100,
          newNs: 112.3,
          percentChange: 12.3,
          significant: true,
          insufficientSample: false,
        },
      ],
      gate: { thresholdPct: 5, worstPct: 12.3, worstKey: "k", breached: true },
    });
    const report = parseBenchReport(doc);
    expect(report.deltas).toHaveLength(1);
    expect(report.deltas?.[0].significant).toBe(true);
    expect(report.gate?.breached).toBe(true);
  });
});

describe("formatNsPerOp", () => {
  it("scales through ns, µs, ms and s", () => {
    expect(formatNsPerOp(56.1)).toBe("56 ns/op");
    expect(formatNsPerOp(999)).toBe("999 ns/op");
    expect(formatNsPerOp(1240)).toBe("1.24µs/op");
    expect(formatNsPerOp(359245)).toBe("359.25µs/op");
    expect(formatNsPerOp(2_500_000)).toBe("2.50ms/op");
    expect(formatNsPerOp(1_200_000_000)).toBe("1.20s/op");
  });
});

describe("formatAge", () => {
  it("buckets into now/seconds/minutes/hours/days", () => {
    const now = Date.parse("2026-08-14T12:00:00Z");
    expect(formatAge(now - 2_000, now)).toBe("just now");
    expect(formatAge(now - 42_000, now)).toBe("42s ago");
    expect(formatAge(now - 2 * 60_000, now)).toBe("2m ago");
    expect(formatAge(now - 3 * 3_600_000, now)).toBe("3h ago");
    expect(formatAge(now - 2 * 86_400_000, now)).toBe("2d ago");
  });
});

describe("formatBenchAnnotation", () => {
  it("renders the ns/op · B/op · allocs/op — age line", () => {
    const now = Date.parse("2026-08-14T12:00:00Z");
    const line = formatBenchAnnotation(
      { nsPerOp: 1240, bytesPerOp: 480, allocsPerOp: 3, iterations: 100 },
      now - 2 * 60_000,
      now,
    );
    expect(line).toBe("1.24µs/op · 480 B/op · 3 allocs/op — 2m ago");
  });

  it("omits the alloc fields for allocation-free benchmarks", () => {
    const now = Date.parse("2026-08-14T12:00:00Z");
    const line = formatBenchAnnotation(
      { nsPerOp: 56.1, bytesPerOp: 0, allocsPerOp: 0, iterations: 100 },
      now - 42_000,
      now,
    );
    expect(line).toBe("56 ns/op · 0 allocs/op — 42s ago");
  });
});
