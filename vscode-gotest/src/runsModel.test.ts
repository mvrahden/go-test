import { describe, it, expect } from "vitest";
import {
  buildRunRows,
  evidenceLabel,
  judgesText,
  narrowedText,
  parseRunsDocument,
  whenText,
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

const opts = (over = {}) => ({
  now: new Date("2026-11-25T18:00:00"),
  exists: () => true,
  evidence: () => false,
  ...over,
});

describe("parseRunsDocument", () => {
  it("reads the CLI's list and drops what is not a record", () => {
    const doc = parseRunsDocument(
      JSON.stringify({ version: "v1", dir: "/w", runs: [entry(), { x: 1 }] }),
    );
    expect(doc?.dir).toBe("/w");
    expect(doc?.runs.map((r) => r.id)).toEqual([entry().id]);
  });

  it("is undefined for what is not the document", () => {
    expect(parseRunsDocument("FAIL: runs takes no packages")).toBeUndefined();
    expect(parseRunsDocument('{"version":"v1"}')).toBeUndefined();
  });
});

describe("buildRunRows", () => {
  it("words a run: verdict, who, commit, the command and what it judges now", () => {
    const [row] = buildRunRows(
      {
        version: "v1",
        dir: "/w",
        runs: [
          entry({
            eventsFile: "/s/a.jsonl",
            manifest: {
              args: ["-json", "./..."],
              commit: "abc1234",
              dirty: true,
              agent: "claude-code",
            },
            result: {
              exitCode: 1,
              packages: { passed: 0, failed: 1, skipped: 0 },
              tests: { passed: 3, failed: 2, skipped: 1 },
              notRun: 4,
            },
          }),
        ],
      },
      opts(),
    );
    expect(row).toMatchObject({
      pass: false,
      verdict: "FAIL exit 1",
      who: "agent claude-code",
      commit: "abc1234 dirty",
      args: "gotest -json ./...",
      counts: "3 passed, 2 failed, 1 skipped, 4 not run",
      judges: "judges this tree: 1 current / 1 stale / 0 unknown",
      judgesTone: "warn",
      canOpen: true,
      evidence: false,
    });
    expect(row.narrowed).toBeUndefined();
    expect(row.imported).toBeUndefined();
  });

  it("marks a narrowed run, an imported one, and one whose file is gone", () => {
    const rows = buildRunRows(
      {
        version: "v1",
        dir: "/w",
        runs: [
          entry({
            manifest: { args: [], changed: { ran: 1, unchanged: 3 } },
            imported: true,
            eventsFile: "/dl/ci-run.json",
          }),
          entry({ id: "b" }),
        ],
      },
      opts({ exists: () => false, evidence: (id: string) => id === "b" }),
    );
    expect(rows[0].narrowed).toBe(
      "--changed: ran 1 package, left out 3 unchanged",
    );
    expect(rows[0].imported).toBe("ci-run.json");
    expect(rows[0].canOpen).toBe(false);
    expect(rows[1].canOpen).toBe(false);
    expect(rows[1].evidence).toBe(true);
    expect(rows[1].who).toBe("no agent");
  });
});

describe("the run's words", () => {
  it("says how a --changed run narrowed itself", () => {
    expect(narrowedText({ args: [] })).toBeUndefined();
    expect(
      narrowedText({
        args: [],
        changed: { ran: 5, unchanged: 0, noRecord: true },
      }),
    ).toBe("--changed found no record and ran all 5");
  });

  it("counts the packages as the CLI judged them", () => {
    expect(judgesText({ total: 3, current: 2, stale: 0, unknown: 1 })).toBe(
      "judges this tree: 2 current / 0 stale / 1 unknown",
    );
  });

  it("names a time today by the clock, another day with its date", () => {
    const now = new Date(2026, 10, 25, 18, 0);
    expect(whenText(new Date(2026, 10, 25, 9, 5), now)).toBe("09:05");
    expect(whenText(new Date(2026, 10, 24, 9, 5), now)).toBe(
      "2026-11-24 09:05",
    );
    expect(evidenceLabel({ when: "09:05", commit: "abc" })).toBe("09:05 abc");
    expect(evidenceLabel({ when: "09:05", commit: "" })).toBe("09:05");
  });
});
