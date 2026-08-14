import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  EventEmitter: class {},
  window: { showErrorMessage: vi.fn() },
}));

import { planBenchInvocations, benchTargetFromItemId } from "./benchRunner.js";

describe("planBenchInvocations", () => {
  it("keeps distinct method targets in selection order", () => {
    const plan = planBenchInvocations([
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkY" },
    ]);
    expect(plan).toHaveLength(2);
    expect(plan[0].methodName).toBe("BenchmarkX");
  });

  it("collapses duplicates", () => {
    const plan = planBenchInvocations([
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
    ]);
    expect(plan).toHaveLength(1);
  });

  it("lets a suite-level target subsume its method targets", () => {
    const plan = planBenchInvocations([
      { importPath: "a/b", suiteName: "S", methodName: "BenchmarkX" },
      { importPath: "a/b", suiteName: "S" },
      { importPath: "a/b", suiteName: "Other", methodName: "BenchmarkZ" },
    ]);
    expect(plan).toEqual([
      { importPath: "a/b", suiteName: "S" },
      { importPath: "a/b", suiteName: "Other", methodName: "BenchmarkZ" },
    ]);
  });
});

describe("benchTargetFromItemId", () => {
  it("splits import path, suite, and method from the right", () => {
    expect(
      benchTargetFromItemId(
        "github.com/x/y/pkg/CacheTestSuite/BenchmarkGetHit",
      ),
    ).toEqual({
      importPath: "github.com/x/y/pkg",
      suiteName: "CacheTestSuite",
      methodName: "BenchmarkGetHit",
    });
  });

  it("refuses ids whose leaf is not a benchmark method", () => {
    expect(
      benchTargetFromItemId("github.com/x/y/pkg/CacheTestSuite/TestGet"),
    ).toBeUndefined();
  });

  it("refuses ids that are too short to carry a method", () => {
    expect(benchTargetFromItemId("pkg/Suite")).toBeUndefined();
  });
});
