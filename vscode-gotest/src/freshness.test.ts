import { describe, it, expect } from "vitest";
import {
  CHECK_FRESH_ARGS,
  parseFreshness,
  staleHeadline,
  type Freshness,
} from "./freshness.js";

describe("parseFreshness", () => {
  it("reads each package's freshness from the CLI's document", () => {
    const doc = JSON.stringify({
      packages: [
        { path: "m/a", freshness: "stale", inputHash: "1" },
        { path: "m/b", freshness: "current", inputHash: "2" },
        { path: "m/c", freshness: "unknown" },
        { path: "m/d" },
      ],
      stats: { stale: 1 },
    });
    expect(Object.fromEntries(parseFreshness(doc)!)).toEqual({
      "m/a": "stale",
      "m/b": "current",
      "m/c": "unknown",
    });
  });

  it("is undefined for what is not the document", () => {
    expect(parseFreshness("FAIL: --check-fresh needs --input")).toBeUndefined();
    expect(parseFreshness("{}")).toBeUndefined();
  });
});

describe("staleHeadline", () => {
  const f = (m: Record<string, Freshness>) => (p: string) => m[p];

  it("counts the stale packages among those judged by a hash", () => {
    expect(
      staleHeadline(
        ["a", "b", "c", "d"],
        f({ a: "stale", b: "current", c: "unknown" }),
      ),
    ).toBe("1 of 2 packages stale: their inputs changed since the run");
  });

  it("says nothing when no package is stale", () => {
    expect(
      staleHeadline(["a", "b"], f({ a: "current", b: "unknown" })),
    ).toBeUndefined();
  });
});

describe("CHECK_FRESH_ARGS", () => {
  it("asks the CLI to judge a piped stream and render no rows", () => {
    expect(CHECK_FRESH_ARGS).toContain("--check-fresh");
    expect(CHECK_FRESH_ARGS).toContain("--input=-");
    expect(CHECK_FRESH_ARGS).toContain("--render-only");
  });
});
