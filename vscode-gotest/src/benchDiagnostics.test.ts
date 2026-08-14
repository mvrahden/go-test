import { describe, it, expect, vi } from "vitest";

const collections: MockCollection[] = [];

class MockCollection {
  entries = new Map<string, unknown[]>();
  cleared = 0;
  set(uri: { fsPath: string }, diagnostics: unknown[]) {
    this.entries.set(uri.fsPath, diagnostics);
  }
  clear() {
    this.cleared++;
    this.entries.clear();
  }
  dispose() {}
}

vi.mock("vscode", () => ({
  languages: {
    createDiagnosticCollection: () => {
      const c = new MockCollection();
      collections.push(c);
      return c;
    },
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  Range: class {
    constructor(
      public a: number,
      public b: number,
      public c: number,
      public d: number,
    ) {}
  },
  Diagnostic: class {
    source?: string;
    constructor(
      public range: unknown,
      public message: string,
      public severity: number,
    ) {}
  },
  DiagnosticSeverity: { Warning: 1 },
}));

import { BenchGateDiagnostics, parseDeltaKey } from "./benchDiagnostics.js";
import type { BenchReport } from "./benchReport.js";
import type { DiscoveryCache } from "./discovery.js";

describe("parseDeltaKey", () => {
  it("splits the CLI's 'pkg Suite/Name' shape", () => {
    expect(
      parseDeltaKey("example.com/pkg CacheTestSuite/BenchmarkGetHit"),
    ).toEqual({
      importPath: "example.com/pkg",
      suiteName: "CacheTestSuite",
      methodName: "BenchmarkGetHit",
    });
  });

  it("refuses malformed keys", () => {
    expect(parseDeltaKey("no-space")).toBeUndefined();
    expect(parseDeltaKey("pkg noslash")).toBeUndefined();
  });
});

function mockCache(): DiscoveryCache {
  return {
    getPackage: (ip: string) =>
      ip === "example.com/pkg"
        ? {
            importPath: ip,
            dir: "/ws/pkg",
            suites: [
              {
                name: "CacheTestSuite",
                benchmarks: [
                  {
                    name: "BenchmarkGetHit",
                    file: "cache_test.go",
                    line: 20,
                    col: 9,
                  },
                ],
              },
            ],
          }
        : undefined,
  } as unknown as DiscoveryCache;
}

function gateReport(breachedKeys: string[]): BenchReport {
  return {
    schemaVersion: 1,
    baseline: { schemaVersion: 1, goos: "linux", goarch: "amd64", results: [] },
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
    gate: {
      thresholdPct: 5,
      worstPct: 12.3,
      worstKey: "example.com/pkg CacheTestSuite/BenchmarkGetHit",
      breached: breachedKeys.length > 0,
      breachedKeys,
    },
  };
}

describe("BenchGateDiagnostics", () => {
  it("places a warning on each breached benchmark method", () => {
    const diags = new BenchGateDiagnostics(mockCache());
    const collection = collections[collections.length - 1];

    diags.apply(gateReport(["example.com/pkg CacheTestSuite/BenchmarkGetHit"]));

    const fileDiags = collection.entries.get("/ws/pkg/cache_test.go") as Array<{
      message: string;
    }>;
    expect(fileDiags).toHaveLength(1);
    expect(fileDiags[0].message).toContain("BenchmarkGetHit regressed +12.3%");
    expect(fileDiags[0].message).toContain("5% bench gate");
  });

  it("clears previous verdicts when a report carries no breaches", () => {
    const diags = new BenchGateDiagnostics(mockCache());
    const collection = collections[collections.length - 1];

    diags.apply(gateReport(["example.com/pkg CacheTestSuite/BenchmarkGetHit"]));
    diags.apply(gateReport([]));

    expect(collection.entries.size).toBe(0);
  });
});
