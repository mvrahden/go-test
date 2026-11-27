import { describe, it, expect } from "vitest";
import {
  LiveRuns,
  RunProgress,
  countEvent,
  elapsedText,
  tallyText,
  type Tally,
} from "./liveProgress.js";
import type { TestEvent } from "./outputParser.js";

const ev = (e: Partial<TestEvent>) =>
  ({ Time: "", Package: "m/a", ...e }) as TestEvent;

describe("countEvent", () => {
  it("counts each test's run, pass and fail event, subtests too", () => {
    const t: Tally = { started: 0, passed: 0, failed: 0 };
    for (const e of [
      ev({ Action: "run", Test: "TestS" }),
      ev({ Action: "run", Test: "TestS/TestA" }),
      ev({ Action: "pass", Test: "TestS/TestA" }),
      ev({ Action: "run", Test: "TestS/TestB" }),
      ev({ Action: "output", Test: "TestS/TestB", Output: "x\n" }),
      ev({ Action: "fail", Test: "TestS/TestB" }),
      ev({ Action: "skip", Test: "TestS/TestC" }),
      ev({ Action: "fail", Test: "TestS" }),
    ])
      countEvent(t, e);
    expect(t).toEqual({ started: 3, passed: 1, failed: 2 });
  });

  it("counts no package verdict and no run-level event", () => {
    const t: Tally = { started: 0, passed: 0, failed: 0 };
    countEvent(t, ev({ Action: "start" }));
    countEvent(t, ev({ Action: "fail" }));
    countEvent(
      t,
      ev({ Action: "fail", Package: "gotest run", RunLevel: true }),
    );
    countEvent(
      t,
      ev({
        Action: "run",
        Test: "TestS",
        Package: "gotest run",
        RunLevel: true,
      }),
    );
    expect(t).toEqual({ started: 0, passed: 0, failed: 0 });
  });
});

describe("how progress reads", () => {
  it("words a tally and an elapsed time", () => {
    expect(tallyText({ started: 12, passed: 10, failed: 1 })).toBe(
      "12 started · 10 passed · 1 failed",
    );
    expect(elapsedText(0)).toBe("0:00");
    expect(elapsedText(42_900)).toBe("0:42");
    expect(elapsedText(61_000)).toBe("1:01");
    expect(elapsedText(3_725_000)).toBe("1:02:05");
  });
});

describe("RunProgress", () => {
  it("holds the finished invocations' streams and the running one's so far", () => {
    const run = new RunProgress("run: x", () => {}, 0);
    expect(run.pieces()).toEqual([]);
    run.finished({ stream: "a\n", folder: "/w1" });
    let sofar = "b";
    run.streaming(() => ({ stream: sofar, folder: "/w2" }));
    sofar = "b\nc\n";
    expect(run.pieces().map((p) => p.stream)).toEqual(["a\n", "b\nc\n"]);
    expect(run.folders()).toEqual(["/w1", "/w2"]);
    run.finished({ stream: "b\nc\nd\n", folder: "/w2" });
    expect(run.pieces().map((p) => p.stream)).toEqual(["a\n", "b\nc\nd\n"]);
  });
});

describe("LiveRuns", () => {
  it("lists a run until it ends, telling each start, event and end", () => {
    const live = new LiveRuns();
    const told: string[] = [];
    live.onDidChange((r) => told.push(`${r.label} ${r.active}`));
    const run = live.start("run: x", 5);
    expect(live.active()).toEqual([run]);
    expect(run.startedAt).toBe(5);
    run.note(ev({ Action: "run", Test: "TestS" }));
    run.end();
    run.end();
    run.note(ev({ Action: "run", Test: "TestS/TestA" }));
    expect(live.active()).toEqual([]);
    expect(run.tally.started).toBe(1);
    expect(told).toEqual(["run: x true", "run: x true", "run: x false"]);
  });
});
