import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  workspace: {},
  window: {},
  env: {},
}));

import { buildTestResultsTable } from "./reporting.js";
import type { TestResultStore, TestResult } from "./testResultStore.js";

interface FakeItem {
  id: string;
  label: string;
  tags: { id: string }[];
  kids: FakeItem[];
}

function node(
  id: string,
  label: string,
  kids: FakeItem[] = [],
  tags: string[] = [],
): FakeItem {
  return { id, label, kids, tags: tags.map((t) => ({ id: t })) };
}

// asItem shapes a FakeItem like the slice of vscode.TestItem the walk touches:
// an id, a label, tags, and a children collection with size and forEach.
function asItem(n: FakeItem): never {
  return {
    id: n.id,
    label: n.label,
    tags: n.tags,
    children: {
      size: n.kids.length,
      forEach: (fn: (child: unknown) => void) =>
        n.kids.forEach((k) => fn(asItem(k))),
    },
  } as never;
}

function store(results: Record<string, TestResult>): TestResultStore {
  return { get: (id: string) => results[id] } as never;
}

function rootsOf(items: FakeItem[]): never {
  return {
    forEach: (fn: (i: unknown) => void) => items.forEach((i) => fn(asItem(i))),
  } as never;
}

const BASE = Date.parse("2026-08-16T10:00:00.000Z");

/** A pass bracketed from +offset to +offset+span, measuring `measured`. */
function at(offset: number, span: number, measured = span): TestResult {
  return {
    status: "pass",
    duration: measured,
    startedAt: BASE + offset,
    endedAt: BASE + offset + span,
  };
}

/** Marks a result as one that called t.Parallel, so it was parked before it ran. */
function parked(r: TestResult): TestResult {
  return { ...r, paused: true };
}

/** The cell in the Time column of the row whose label contains `label`. */
function timeOf(table: string, label: string): string {
  const line = table.split("\n").find((l) => l.includes(label));
  return line ? (line.trim().split(/\s{2,}/)[1] ?? "") : "";
}

describe("buildTestResultsTable", () => {
  it("reports what a container held when its children report nothing", () => {
    // The shape of every When that drives a subprocess: the work happens in the
    // container body and the leaves only assert on what it produced.
    const tree = node("suite:Hangs", "Hangs", [
      node("when:setup", "the setup never returns", [
        node("it:budget", "names the budget"),
        node("it:fails", "fails the run"),
      ]),
    ]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({
        "suite:Hangs": at(0, 60290),
        "when:setup": at(0, 60290),
        "it:budget": at(60290, 0),
        "it:fails": at(60290, 0),
      }),
      () => undefined,
    );

    expect(table).toBeDefined();
    expect(timeOf(table!, "the setup never returns")).toBe("60.290s");
    expect(timeOf(table!, "Hangs")).toBe("60.290s");
    expect(table).toContain("Total: 2 passed (60.290s)");
  });

  it("reports the interval a parallel suite occupied, not the sum", () => {
    // go test stops a parent's clock when it returns, so the suite measures
    // nothing while its methods together claim more than the clock passed.
    const tree = node("suite:Parallel", "Parallel", [
      node("m:first", "first"),
      node("m:second", "second"),
      node("m:third", "third"),
    ]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({
        "suite:Parallel": at(0, 58, 0),
        "m:first": parked(at(0, 58)),
        "m:second": parked(at(0, 50)),
        "m:third": parked(at(0, 52)),
      }),
      () => undefined,
    );

    expect(timeOf(table!, "Parallel")).toBe("0.058s");
    expect(table).toContain("Total: 3 passed (0.058s)");
  });

  it("charges a leaf only for what it executed, not for queueing", () => {
    // Registered at once, resumed 50ms later, then done in a millisecond. The
    // wait belongs to whatever set the concurrency, not to the test.
    const tree = node("suite:S", "S", [node("m:regexp", "matches correctly")]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({ "suite:S": at(0, 51), "m:regexp": parked(at(0, 51, 1)) }),
      () => undefined,
    );

    expect(timeOf(table!, "matches correctly")).toBe("0.001s");
    expect(timeOf(table!, "S")).toBe("0.051s");
  });

  it("ignores the bracket of a method whose report was flushed late", () => {
    // Measured against a real stream: the method resumed at 5.1s, ran 300ms,
    // and had its terminal event held behind a 60s sibling. Its children keep
    // clean timestamps; only the parked node's own end is delayed.
    const tree = node("suite:Panic", "Panic", [
      node("m:each", "PanicInEachAfterRecordedFailure", [
        node("w:each", "an Each entry records a failure and then panics"),
      ]),
      node("m:hang", "SetupThatNeverReturns"),
    ]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({
        "suite:Panic": at(0, 60784),
        "m:each": parked(at(0, 60783, 300)),
        "w:each": at(5107, 300),
        "m:hang": parked(at(0, 60784, 60400)),
      }),
      () => undefined,
    );

    expect(timeOf(table!, "PanicInEachAfterRecordedFailure")).toBe("0.300s");
    expect(timeOf(table!, "an Each entry")).toBe("0.300s");
    expect(timeOf(table!, "SetupThatNeverReturns")).toBe("60.400s");
    // The minute is not lost: it stays on the suite, which really did span it.
    expect(timeOf(table!, "Panic ")).toBe("60.784s");
  });

  it("never lets a row read shorter than what it contains", () => {
    // go test reports a parked node's measure to 10ms while a child's bracket
    // is exact, so a 15ms method can read 10ms beside its own 15ms child — and
    // a parked child can round above the exact bracket of the suite holding it.
    const tree = node("suite:Spec", "Spec", [
      node("m:grace", "GraceKill", [
        node("w:grace", "using GraceKill strategy"),
      ]),
    ]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({
        "suite:Spec": at(0, 229),
        "m:grace": parked(at(0, 9000, 10)),
        "w:grace": at(0, 15),
      }),
      () => undefined,
    );

    expect(timeOf(table!, "GraceKill")).toBe("0.015s");
    expect(timeOf(table!, "Spec")).toBe("0.229s");
  });

  it("counts overlap once when a grouping row assembles several suites", () => {
    // A directory is not something that executes, so it is worth the clock
    // during which something under it was running — no more.
    const tree = node("dir:internal", "internal", [
      node("suite:Alpha", "Alpha", [node("m:one", "one")], []),
      node("suite:Beta", "Beta", [node("m:two", "two")], []),
    ]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({
        "suite:Alpha": at(0, 100),
        "m:one": at(0, 100),
        "suite:Beta": at(50, 100),
        "m:two": at(50, 100),
      }),
      () => undefined,
    );

    // Two 100ms suites overlapping by 50ms occupied 150ms, not 200ms.
    expect(timeOf(table!, "internal")).toBe("0.150s");
  });

  it("skips the gap between results recorded hours apart", () => {
    // The store keeps results for a week, so a tree can hold one package from
    // this morning beside another from just now. The interval between two runs
    // is nobody's cost.
    const morning = node("dir:a", "a", [
      node("suite:Alpha", "Alpha", [node("m:one", "one")]),
    ]);
    const afternoon = node("dir:b", "b", [
      node("suite:Beta", "Beta", [node("m:two", "two")]),
    ]);
    const fourHours = 4 * 60 * 60 * 1000;

    const table = buildTestResultsTable(
      rootsOf([morning, afternoon]),
      store({
        "suite:Alpha": at(0, 1000),
        "m:one": at(0, 1000),
        "suite:Beta": at(fourHours, 2000),
        "m:two": at(fourHours, 2000),
      }),
      () => undefined,
    );

    expect(table).toContain("Total: 2 passed (3.000s)");
  });

  it("bounds a subtree by both measures when no timestamps were recorded", () => {
    const tree = node("suite:Held", "Held", [node("m:one", "one")]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({
        "suite:Held": { status: "pass", duration: 5000 },
        "m:one": { status: "pass", duration: 0 },
      }),
      () => undefined,
    );

    expect(timeOf(table!, "Held")).toBe("5.000s");
  });

  it("reports no results rather than a zero total when nothing ran", () => {
    const tree = node("suite:A", "A", [node("m:one", "one")]);

    const table = buildTestResultsTable(
      rootsOf([tree]),
      store({}),
      () => undefined,
    );

    expect(table).toContain("Total: no results");
  });

  it("returns undefined when there is nothing to render", () => {
    expect(
      buildTestResultsTable(rootsOf([]), store({}), () => undefined),
    ).toBeUndefined();
  });
});
