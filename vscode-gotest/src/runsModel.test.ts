import { describe, it, expect } from "vitest";
import {
  agentName,
  commandParts,
  countsText,
  dayText,
  evidenceLabel,
  flakyHint,
  freshText,
  judgesText,
  narrowedText,
  newestCurrent,
  parseRunsDocument,
  runTitle,
  verdictText,
  whenText,
  whoText,
  type RunsDocument,
  type RunsEntry,
} from "./runsModel.js";

function entry(over: Partial<RunsEntry> = {}): RunsEntry {
  return {
    id: "20261125T150742.574336546Z-1-aa",
    recorded: "2026-11-25T15:07:42.574Z",
    manifest: { args: ["-json", "./..."] },
    result: {
      exitCode: 0,
      packages: { passed: 1, failed: 0, skipped: 0 },
      tests: { passed: 19, failed: 0, skipped: 0 },
    },
    failed: [],
    packages: { total: 2, current: 1, stale: 1, unknown: 0 },
    ...over,
  };
}

const docOf = (...runs: RunsEntry[]): RunsDocument => ({
  version: "v1",
  dir: "/w",
  runs,
});

describe("parseRunsDocument", () => {
  it("reads the CLI's list and drops what is not a record", () => {
    const doc = parseRunsDocument(
      JSON.stringify({ version: "v1", dir: "/w", runs: [entry(), { x: 1 }] }),
    );
    expect(doc?.dir).toBe("/w");
    expect(doc?.runs.map((r) => r.id)).toEqual([entry().id]);
  });

  it("reads the packages a run read and the tests it failed, absent from an older CLI", () => {
    const raw = {
      version: "v1",
      dir: "/w",
      runs: [
        {
          ...entry({ id: "new" }),
          ran: [
            { package: "m/a", status: "fail", inputHash: "h" },
            { bogus: 1 },
          ],
          failed: [{ package: "m/a", test: "TestATestSuite/TestX" }, 3],
        },
        {
          ...entry({ id: "old" }),
          failed: undefined,
          result: {
            exitCode: 0,
            packages: { passed: 1, failed: 0, skipped: 0 },
          },
        },
      ],
    };
    const doc = parseRunsDocument(JSON.stringify(raw))!;
    expect(doc.runs[0].ran).toEqual([
      { package: "m/a", status: "fail", inputHash: "h" },
    ]);
    expect(doc.runs[0].failed).toEqual([
      { package: "m/a", test: "TestATestSuite/TestX" },
    ]);
    expect(doc.runs[1].ran).toBeUndefined();
    expect(doc.runs[1].failed).toEqual([]);
    expect(doc.runs[1].result.tests).toBeUndefined();
  });

  it("is undefined for what is not the document", () => {
    expect(parseRunsDocument("FAIL: runs takes no packages")).toBeUndefined();
    expect(parseRunsDocument('{"version":"v1"}')).toBeUndefined();
  });
});

describe("how a record reads", () => {
  it("names what it ran: everything, packages, a -run filter, Run changed", () => {
    expect(runTitle({ args: ["-json", "./..."] })).toBe("Everything");
    expect(
      runTitle({
        args: [
          "--agent=false",
          "./pkg/gotestruntime/",
          "-run",
          "TestControlLinkTestSuite",
          "-count=1",
        ],
      }),
    ).toBe("pkg/gotestruntime, only ControlLink");
    expect(
      runTitle({
        args: [
          "-run=^TestCartTestSuite$/^TestAdd$",
          "./shop",
          "./order",
          "./bill/...",
        ],
      }),
    ).toBe("shop, order and 1 more, only Cart › Add");
    expect(
      runTitle({
        args: ["--changed", "--", "-json", "./..."],
        changed: { ran: 95, unchanged: 0, noRecord: true },
      }),
    ).toBe("Everything: Run changed found no record, ran all 95 packages");
    expect(
      runTitle({
        args: ["--changed", "./..."],
        changed: { ran: 3, unchanged: 92 },
      }),
    ).toBe("Run changed: ran 3 packages, left out 92 unchanged");
    expect(runTitle({ args: ["--changed", "./..."] })).toBe("Run changed");
    expect(
      commandParts(["-count", "2", "-timeout=3m", "./a"]).patterns,
    ).toEqual(["./a"]);
  });

  it("names who ran it: this editor, an agent, a terminal, an import", () => {
    expect(whoText(entry({ manifest: { args: ["--control=stdin"] } }))).toBe(
      "You, from this editor",
    );
    expect(
      whoText(
        entry({ manifest: { args: [], agent: "claude-code_2-1-331_agent" } }),
      ),
    ).toBe("Agent (Claude Code)");
    expect(whoText(entry())).toBe("From a terminal");
    expect(whoText(entry({ imported: true, eventsFile: "/dl/ci.json" }))).toBe(
      "Imported from ci.json",
    );
    expect(
      whoText(
        entry({
          imported: true,
          eventsFile: "/dl/ci.json",
          manifest: { args: [], agent: "codex" },
        }),
      ),
    ).toBe("Imported from ci.json, agent (codex)");
    expect(agentName("claude-code_2-1-331_agent")).toBe("Claude Code");
  });

  it("counts its tests, and says so when it kept none rather than zero passed", () => {
    expect(countsText(entry().result)).toBe("19 passed");
    expect(
      countsText({
        exitCode: 1,
        packages: { passed: 0, failed: 1, skipped: 0 },
        tests: { passed: 11672, failed: 4, skipped: 18 },
        notRun: 2,
      }),
    ).toBe("11,672 passed · 4 failed · 18 skipped · 2 not run");
    expect(
      countsText({
        exitCode: 0,
        packages: { passed: 1, failed: 0, skipped: 0 },
      }),
    ).toBe("no test counts kept");
    expect(
      countsText({
        exitCode: 0,
        packages: { passed: 1, failed: 0, skipped: 0 },
        tests: { passed: 0, failed: 0, skipped: 0 },
      }),
    ).toBe("no test counts kept");
    expect(
      countsText({
        exitCode: 0,
        packages: { passed: 0, failed: 0, skipped: 1 },
        tests: { passed: 0, failed: 0, skipped: 0 },
      }),
    ).toBe("0 passed");
  });

  it("words its verdict and what it says of the tree now", () => {
    expect([0, 1, 2].map(verdictText)).toEqual([
      "Passed",
      "Failed",
      "Failed, exit 2",
    ]);
    expect(freshText({ total: 95, current: 95, stale: 0, unknown: 0 })).toEqual(
      { text: "95 packages, current", tone: "pass" },
    );
    expect(freshText({ total: 2, current: 1, stale: 1, unknown: 0 })).toEqual({
      text: "1 of 2 packages stale",
      tone: "warn",
    });
    expect(freshText({ total: 1, current: 0, stale: 0, unknown: 1 })).toEqual({
      text: "1 package, freshness unknown",
      tone: "none",
    });
    expect(freshText({ total: 0, current: 0, stale: 0, unknown: 0 }).text).toBe(
      "no packages",
    );
    expect(judgesText({ total: 3, current: 2, stale: 0, unknown: 1 })).toBe(
      "judges this tree: 2 current / 0 stale / 1 unknown",
    );
  });

  it("names a time today by the clock, another day with its date, and the day", () => {
    const now = new Date(2026, 10, 25, 18, 0);
    expect(whenText(new Date(2026, 10, 25, 9, 5), now)).toBe("09:05");
    expect(whenText(new Date(2026, 10, 24, 9, 5), now)).toBe("yesterday 09:05");
    expect(whenText(new Date(2026, 10, 20, 9, 5), now)).toBe(
      "2026-11-20 09:05",
    );
    expect(dayText(new Date(2026, 10, 25, 1), now)).toBe("today");
  });

  it("says how a --changed run narrowed itself", () => {
    expect(narrowedText({ args: [] })).toBeUndefined();
    expect(
      narrowedText({
        args: [],
        changed: { ran: 5, unchanged: 0, noRecord: true },
      }),
    ).toBe("Run changed found no record, ran all 5");
  });
});

describe("the evidence a record gives", () => {
  const now = new Date("2026-11-25T18:00:00");
  it("is the newest record whose every package is current and whose events file is there", () => {
    const doc = docOf(
      entry({ id: "stale", eventsFile: "/e/1" }),
      entry({
        id: "gone",
        eventsFile: "/e/gone",
        packages: { total: 1, current: 1, stale: 0, unknown: 0 },
      }),
      entry({
        id: "ok",
        eventsFile: "/e/2",
        manifest: { args: ["--control=stdin"] },
        packages: { total: 2, current: 2, stale: 0, unknown: 0 },
      }),
    );
    expect(newestCurrent(doc, (f) => f !== "/e/gone", now)).toEqual({
      id: "ok",
      file: "/e/2",
      label: `the editor run of ${whenText(new Date(entry().recorded), now)}`,
    });
    expect(newestCurrent(docOf(entry()), () => true, now)).toBeUndefined();
    expect(evidenceLabel(entry({ imported: true }), now)).toMatch(
      /^the imported run of /,
    );
  });
});

describe("the flaky hint", () => {
  const rec = (id: string, hash: string | undefined, failed: string[] = []) =>
    entry({
      id,
      ran:
        hash === undefined
          ? undefined
          : [
              {
                package: "m/a",
                status: failed.length ? "fail" : "pass",
                inputHash: hash,
              },
            ],
      failed: failed.map((test) => ({ package: "m/a", test })),
    });
  const test = "TestLinkTestSuite/TestReader/a_suite_process/reads_once";

  it("counts the other runs of the same inputs that did not fail the behavior's method", () => {
    const doc = docOf(
      rec("this", "h", ["TestLinkTestSuite/TestReader"]),
      rec("p1", "h"),
      rec("f1", "h", ["TestLinkTestSuite/TestReader"]),
      rec("p2", "h", ["TestLinkTestSuite/TestOther"]),
      rec("other-tree", "h2"),
      rec("old-cli", undefined),
    );
    expect(flakyHint(doc, "m/a", test, "h", (r) => r.id === "this")).toBe(
      "Passed in 2 of 3 other runs on this same tree. Likely flaky.",
    );
  });

  it("says the behavior failed every time, and nothing without the data", () => {
    const doc = docOf(
      rec("f1", "h", ["TestLinkTestSuite/TestReader"]),
      rec("f2", "h", ["TestLinkTestSuite/TestReader"]),
    );
    expect(flakyHint(doc, "m/a", test, "h")).toBe(
      "Failed in all 2 other runs on this same tree too.",
    );
    expect(
      flakyHint(
        docOf(rec("f1", "h", ["TestLinkTestSuite/TestReader"])),
        "m/a",
        test,
        "h",
      ),
    ).toBe("Failed in the 1 other run on this same tree too.");
    expect(
      flakyHint(docOf(rec("old", undefined)), "m/a", test, "h"),
    ).toBeUndefined();
    expect(flakyHint(doc, "m/a", test, undefined)).toBeUndefined();
    expect(flakyHint(undefined, "m/a", test, "h")).toBeUndefined();
  });
});
