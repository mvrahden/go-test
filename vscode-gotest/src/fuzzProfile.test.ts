import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class TestTag {
    constructor(public readonly id: string) {}
  }
  class TestMessage {
    constructor(public readonly message: string) {}
  }
  return { TestTag, TestMessage, window: {}, workspace: {} };
});

import {
  collectFuzzItems,
  fuzzTargetFromItemId,
  planFuzzSessions,
  parseCrasherLine,
  fuzzVerdicts,
  sessionEnd,
} from "./fuzzProfile.js";
import { markFailedRun } from "./runnerUtils.js";

interface FakeItem {
  id: string;
  tags: { id: string }[];
  children: { forEach: (fn: (c: FakeItem) => void) => void; size: number };
}
function item(
  id: string,
  tags: string[] = [],
  kids: FakeItem[] = [],
): FakeItem {
  return {
    id,
    tags: tags.map((t) => ({ id: t })),
    children: { forEach: (fn) => kids.forEach(fn), size: kids.length },
  };
}
const tree = () => {
  const trim = item("example.com/p/S/FuzzTrim", ["fuzz"]);
  const summary = item("example.com/p/S/FuzzSummary", ["fuzz"]);
  const test = item("example.com/p/S/TestX");
  const suite = item("example.com/p/S", [], [test, trim, summary]);
  const other = item("example.com/q/T/FuzzQ", ["fuzz"]);
  const suiteQ = item("example.com/q/T", [], [other]);
  const pkgP = item("example.com/p", ["package"], [suite]);
  const pkgQ = item("example.com/q", ["package"], [suiteQ]);
  return {
    trim,
    summary,
    test,
    suite,
    other,
    suiteQ,
    pkgP,
    pkgQ,
    roots: [pkgP, pkgQ],
  };
};
const roots = (items: FakeItem[]) => ({
  forEach: (fn: (c: FakeItem) => void) => items.forEach(fn),
  size: items.length,
});

describe("collectFuzzItems", () => {
  it("takes every fuzz item in the tree when the request names nothing", () => {
    const t = tree();
    const got = collectFuzzItems(undefined, roots(t.roots) as never, undefined);
    expect(got.map((i) => i.id)).toEqual([t.trim.id, t.summary.id, t.other.id]);
  });
  it("descends from a selected suite to its fuzz items only", () => {
    const t = tree();
    const got = collectFuzzItems(
      [t.suite] as never,
      roots(t.roots) as never,
      undefined,
    );
    expect(got.map((i) => i.id)).toEqual([t.trim.id, t.summary.id]);
  });
  it("keeps a selected fuzz item and honours the exclusion list", () => {
    const t = tree();
    const got = collectFuzzItems(
      [t.pkgP] as never,
      roots(t.roots) as never,
      [t.summary] as never,
    );
    expect(got.map((i) => i.id)).toEqual([t.trim.id]);
  });
});

describe("fuzzTargetFromItemId", () => {
  it("splits the package, suite and method from the right, so slashes in the import path survive", () => {
    expect(
      fuzzTargetFromItemId("github.com/org/repo/pkg/sub/MyTestSuite/FuzzParse"),
    ).toEqual({
      importPath: "github.com/org/repo/pkg/sub",
      suiteName: "MyTestSuite",
      methodName: "FuzzParse",
      wrapper: "FuzzMyTestSuite_FuzzParse",
    });
  });
  it("names an external twin's target by the wrapper go test runs", () => {
    expect(
      fuzzTargetFromItemId("example.com/p/TwinsTestSuite~external/FuzzParse"),
    ).toEqual({
      importPath: "example.com/p",
      suiteName: "TwinsTestSuite",
      methodName: "FuzzParse",
      wrapper: "FuzzTwinsTestSuite_FuzzParse",
    });
  });
  it("rejects an id whose last segment is not a fuzz method", () => {
    expect(fuzzTargetFromItemId("example.com/p/S/TestX")).toBeUndefined();
  });
});

describe("planFuzzSessions", () => {
  const targetsOf = (importPath: string) =>
    importPath === "example.com/p"
      ? ["FuzzS_FuzzTrim", "FuzzS_FuzzSummary"]
      : ["FuzzT_FuzzQ"];
  it("runs one session per package when every target of the package is selected", () => {
    const sessions = planFuzzSessions(
      [
        "example.com/p/S/FuzzTrim",
        "example.com/p/S/FuzzSummary",
        "example.com/q/T/FuzzQ",
      ],
      targetsOf,
    );
    expect(sessions).toEqual([
      {
        importPath: "example.com/p",
        target: undefined,
        wrappers: ["FuzzS_FuzzTrim", "FuzzS_FuzzSummary"],
      },
      {
        importPath: "example.com/q",
        target: undefined,
        wrappers: ["FuzzT_FuzzQ"],
      },
    ]);
  });
  it("narrows to one session per target when only some of a package's targets are selected", () => {
    const sessions = planFuzzSessions(
      ["example.com/p/S/FuzzSummary"],
      targetsOf,
    );
    expect(sessions).toEqual([
      {
        importPath: "example.com/p",
        target: "FuzzS_FuzzSummary",
        wrappers: ["FuzzS_FuzzSummary"],
      },
    ]);
  });
});

describe("parseCrasherLine", () => {
  it("names the wrapper and the file", () => {
    expect(
      parseCrasherLine(
        "[FuzzS_FuzzTrim] new crasher: /w/testdata/fuzz/FuzzS_FuzzTrim/ab12",
      ),
    ).toEqual({
      wrapper: "FuzzS_FuzzTrim",
      path: "/w/testdata/fuzz/FuzzS_FuzzTrim/ab12",
    });
    expect(
      parseCrasherLine("[FuzzS_FuzzTrim] fuzz: elapsed: 3s"),
    ).toBeUndefined();
  });
});

describe("fuzzVerdicts", () => {
  const wrappers = ["FuzzS_FuzzTrim", "FuzzS_FuzzSummary"];
  it("passes every target of a clean session, carrying the session line", () => {
    const v = fuzzVerdicts(wrappers, {
      exitCode: 0,
      cancelled: false,
      crashers: new Map(),
      failed: new Set(),
      sessionLine:
        "fuzzed 2 targets in 1m0s: 9 execs, 0 new interesting inputs, no crashers",
    });
    expect(v.get("FuzzS_FuzzTrim")).toEqual({
      status: "passed",
      message:
        "fuzzed 2 targets in 1m0s: 9 execs, 0 new interesting inputs, no crashers",
    });
  });
  it("fails the target that found a crasher and names the file, leaving its sibling passing", () => {
    const v = fuzzVerdicts(wrappers, {
      exitCode: 1,
      cancelled: false,
      crashers: new Map([
        ["FuzzS_FuzzTrim", ["/w/testdata/fuzz/FuzzS_FuzzTrim/ab12"]],
      ]),
      failed: new Set(["FuzzS_FuzzTrim"]),
      sessionLine:
        "fuzzed 2 targets in 3.2s: 9 execs, 0 new interesting inputs, 1 new crasher (FuzzS_FuzzTrim)",
    });
    expect(v.get("FuzzS_FuzzTrim")?.status).toBe("failed");
    expect(v.get("FuzzS_FuzzTrim")?.message).toContain(
      "/w/testdata/fuzz/FuzzS_FuzzTrim/ab12",
    );
    expect(v.get("FuzzS_FuzzSummary")?.status).toBe("passed");
  });
  it("fails a target that failed without a new file, since a seed or corpus entry reproduces it", () => {
    const v = fuzzVerdicts(wrappers, {
      exitCode: 1,
      cancelled: false,
      crashers: new Map(),
      failed: new Set(["FuzzS_FuzzSummary"]),
      sessionLine: "",
    });
    expect(v.get("FuzzS_FuzzSummary")?.status).toBe("failed");
    expect(v.get("FuzzS_FuzzSummary")?.message).toContain("seed");
  });
  it("marks a cancelled session skipped and leaves a session that could not run to the failed-run rule", () => {
    const c = fuzzVerdicts(wrappers, {
      exitCode: 0,
      cancelled: true,
      crashers: new Map(),
      failed: new Set(),
      sessionLine: "",
    });
    expect(c.get("FuzzS_FuzzTrim")?.status).toBe("skipped");
    const outcome = {
      exitCode: 2,
      cancelled: false,
      crashers: new Map(),
      failed: new Set<string>(),
      sessionLine: "",
      stderr: "FAIL: boom",
    };
    const run = { errored: vi.fn() };
    expect(
      markFailedRun(
        run as never,
        [{ id: "example.com/p/S/FuzzTrim" }] as never,
        sessionEnd(outcome),
      ),
    ).toBe(true);
    expect(run.errored.mock.calls[0][1].message).toBe(
      "FAIL: boom\n\nexit code 2",
    );
  });
});

describe("sessionEnd", () => {
  const base = {
    cancelled: false,
    crashers: new Map<string, string[]>(),
    failed: new Set<string>(),
    sessionLine: "",
  };
  it.each([1, 2, 3, 130, 137])(
    "reads exit %i without a finding as a failed run",
    (exitCode) => {
      const end = sessionEnd({
        ...base,
        exitCode,
        stderr: "go: module lookup failed\nexit status 1\n",
      });
      expect(end).toEqual({
        exitCode,
        cancelled: false,
        failed: false,
        reason: "go: module lookup failed",
      });
      const run = { errored: vi.fn() };
      const item = { id: "example.com/p/S/FuzzTrim" };
      expect(markFailedRun(run as never, [item] as never, end)).toBe(true);
      expect(run.errored.mock.calls[0][1].message).toBe(
        `go: module lookup failed\n\nexit code ${exitCode}`,
      );
    },
  );
  it("reads a crasher as the session's failing verdict", () => {
    const end = sessionEnd({
      ...base,
      exitCode: 1,
      crashers: new Map([["FuzzS_FuzzTrim", ["/w/ab12"]]]),
    });
    expect(end.failed).toBe(true);
    expect(markFailedRun({ errored: vi.fn() } as never, [] as never, end)).toBe(
      false,
    );
  });
  it("reads a cancelled session as cancelled", () => {
    expect(
      sessionEnd({ ...base, exitCode: 130, cancelled: true }).cancelled,
    ).toBe(true);
  });
});
