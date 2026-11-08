import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class Position {
    constructor(
      public readonly line: number,
      public readonly character: number,
    ) {}
  }
  class Location {
    constructor(
      public readonly uri: unknown,
      public readonly range: unknown,
    ) {}
  }
  class TestMessage {
    location?: unknown;
    expectedOutput?: string;
    actualOutput?: string;
    constructor(public readonly message: string) {}
  }
  return {
    Position,
    Location,
    TestMessage,
    Uri: { file: (p: string) => ({ fsPath: p }) },
    window: {},
    workspace: { getConfiguration: () => ({ get: () => undefined }) },
  };
});

import { WatchCycle } from "./watch.js";

interface Item {
  id: string;
  label: string;
  parent?: Item;
  children: Map<string, Item>;
  tags: { id: string }[];
}

function item(id: string, label: string, parent?: Item): Item {
  const it: Item = { id, label, parent, children: new Map(), tags: [] };
  parent?.children.set(id, it);
  return it;
}

function fixture() {
  const pkg = item("example.com/pkg", "example.com/pkg");
  pkg.tags = [{ id: "package" }];
  const suite = item("example.com/pkg/MySuite", "MySuite", pkg);
  const method = item("example.com/pkg/MySuite/TestFail", "TestFail", suite);
  const items = new Map([pkg, suite, method].map((i) => [i.id, i]));
  const results = new Map<string, { status: string }>();
  const controller = {
    findItem: (id: string) => items.get(id),
    recordResult: (id: string, status: string) => results.set(id, { status }),
    getResult: (id: string) => results.get(id),
    noteStart: () => {},
    notePaused: () => {},
    createDynamicSubtest: (parent: Item) => parent,
    testController: {
      items: { forEach: (cb: (i: Item) => void) => cb(pkg) },
    },
  };
  const failures: string[] = [];
  const run = {
    started: vi.fn(),
    passed: vi.fn(),
    skipped: vi.fn(),
    appendOutput: vi.fn(),
    end: vi.fn(),
    failed: vi.fn((_item: unknown, messages: { message: string }[]) => {
      failures.push(...messages.map((m) => m.message));
    }),
  };
  const cache = { resolveImportPath: () => "/ws/pkg" };
  return { controller, run, cache, failures };
}

const ev = (e: Record<string, unknown>) =>
  JSON.stringify({ Package: "example.com/pkg", ...e }) + "\n";

describe("WatchCycle", () => {
  it("keeps a test's output across the pieces the pipe split the cycle into", () => {
    const { controller, run, cache, failures } = fixture();
    const cycle = new WatchCycle(
      run as never,
      controller as never,
      cache as never,
      "/ws",
    );

    cycle.apply(
      ev({ Action: "run", Test: "TestMySuite/TestFail" }) +
        ev({
          Action: "output",
          Test: "TestMySuite/TestFail",
          Output: "    suite_test.go:12: expected 1, got 2\n",
        }),
    );
    cycle.apply(
      ev({ Action: "fail", Test: "TestMySuite/TestFail", Elapsed: 0.01 }),
    );

    expect(failures.join("\n")).toContain("expected 1, got 2");
  });

  it("ends its run and returns everything the cycle streamed", () => {
    const { controller, run, cache } = fixture();
    const cycle = new WatchCycle(
      run as never,
      controller as never,
      cache as never,
      "/ws",
    );
    const first = ev({ Action: "run", Test: "TestMySuite/TestFail" });
    const second = ev({
      Action: "pass",
      Test: "TestMySuite/TestFail",
      Elapsed: 0.01,
    });
    cycle.apply(first);
    cycle.apply(second);

    expect(cycle.end(0)).toBe(first + second);
    expect(run.end).toHaveBeenCalledTimes(1);
  });

  it("says in the run when the cycle ended with a failing exit code", () => {
    const { controller, run, cache } = fixture();
    const cycle = new WatchCycle(
      run as never,
      controller as never,
      cache as never,
      "/ws",
    );
    cycle.end(2);

    expect(run.appendOutput).toHaveBeenCalledWith(
      expect.stringContaining("exit code 2"),
    );
  });
});
