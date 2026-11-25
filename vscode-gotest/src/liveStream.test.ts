// A live run writes each suite's events as they happen, so two suites of a
// package, and two packages, interleave; a burst run writes each suite whole.
// The fixtures hold the same events in both orders. What the Test Explorer,
// the Spec store and the imported-stream path make of them must not depend on
// which order arrived.

import { describe, it, expect, vi } from "vitest";
import { readFileSync } from "node:fs";
import * as path from "node:path";

vi.mock("vscode", () => {
  class Position {
    constructor(
      public readonly line: number,
      public readonly character: number,
    ) {}
  }
  class Location {
    constructor(
      public readonly uri: { fsPath: string },
      public readonly range: Position,
    ) {}
  }
  class TestMessage {
    location?: Location;
    expectedOutput?: string;
    actualOutput?: string;
    constructor(public readonly message: string) {}
  }
  return {
    Position,
    Location,
    TestMessage,
    TestRunRequest: class {},
    Uri: { file: (p: string) => ({ fsPath: p }) },
    workspace: { getConfiguration: () => ({ get: () => undefined }) },
    window: {},
  };
});

const lines = vi.hoisted(() => ({ stream: "" }));

vi.mock("./cli.js", async (orig) => ({
  ...(await orig<typeof import("./cli.js")>()),
  buildCliCommand: vi.fn(async (args: string[]) => ({ bin: "gotest", args })),
}));
vi.mock("./runnerUtils.js", async (orig) => ({
  ...(await orig<typeof import("./runnerUtils.js")>()),
  spawnTestProcess: vi.fn(
    async (
      _bin: string,
      _args: string[],
      _cwd: string,
      _token: unknown,
      _channel: unknown,
      _label: string,
      _env: unknown,
      onStdoutLine?: (line: string) => void,
    ) => {
      for (const line of lines.stream.split("\n")) {
        if (line.trim()) onStdoutLine?.(line.trim());
      }
      return { stdout: lines.stream, stderr: "", exitCode: 1 };
    },
  ),
}));

import { executeBatch } from "./batchRunner.js";
import { StreamApplier } from "./runnerUtils.js";
import { parseTestEvents } from "./outputParser.js";
import { SpecStore } from "./specStore.js";

const fixture = (name: string) =>
  readFileSync(
    path.join(__dirname, "..", "testdata", "live", `${name}.jsonl`),
    "utf-8",
  );

interface Item {
  id: string;
  label: string;
  parent?: Item;
  children: Map<string, Item>;
  tags: { id: string }[];
}

// The packages the fixtures run, as discovery would hand them over.
const TREE: Record<string, Record<string, string[]>> = {
  "example.com/shop": {
    CartTestSuite: ["TestAdd", "TestRemove"],
    OrderTestSuite: ["TestPlace", "TestCancel"],
    FirstTestSuite: ["TestAPasses", "TestBHolds"],
    SecondTestSuite: ["TestAPasses", "TestBHolds"],
    CrashTestSuite: ["TestPanics"],
    OtherTestSuite: ["TestA", "TestB"],
  },
  "example.com/pay": { PayTestSuite: ["TestCharge", "TestRefund"] },
};

// world is a Test Explorer and a run that keep each item's last state.
function world() {
  const items = new Map<string, Item>();
  const add = (id: string, label: string, parent?: Item, tags = []) => {
    const it: Item = { id, label, parent, children: new Map(), tags };
    items.set(id, it);
    parent?.children.set(id, it);
    return it;
  };
  for (const [pkg, suites] of Object.entries(TREE)) {
    const p = add(pkg, pkg, undefined, [{ id: "package" }] as never);
    for (const [suite, methods] of Object.entries(suites)) {
      const s = add(`${pkg}/${suite}`, suite, p);
      for (const m of methods) add(`${pkg}/${suite}/${m}`, m, s);
    }
  }
  const recorded = new Map<string, string>();
  const controller = {
    findItem: (id: string) => items.get(id),
    recordResult: (id: string, status: string, duration?: number) =>
      recorded.set(id, `${status} ${duration ?? ""}`),
    getResult: (id: string) => recorded.get(id),
    noteStart: () => {},
    notePaused: () => {},
    createDynamicSubtest: (parent: Item, level: string, label: string) =>
      add(`${parent.id}/${level}`, label, parent),
  };
  const state = new Map<string, string>();
  const output = new Map<string, string>();
  const said =
    (kind: string) => (item: Item, messages?: unknown, duration?: unknown) => {
      const list = Array.isArray(messages)
        ? messages
        : messages
          ? [messages]
          : [];
      const text = list
        .map((m: { message: string; location?: { range: { line: number } } }) =>
          [m.message, m.location?.range.line].join("@"),
        )
        .join("|");
      state.set(
        item.id,
        [kind, text, typeof duration === "number" ? duration : ""].join(" "),
      );
    };
  const run = {
    started: (item: Item) => {
      if (!state.has(item.id)) state.set(item.id, "started");
    },
    passed: (item: Item, duration?: number) =>
      state.set(item.id, `passed ${duration ?? ""}`),
    failed: said("failed"),
    errored: said("errored"),
    skipped: (item: Item) => state.set(item.id, "skipped"),
    enqueued: () => {},
    appendOutput: (text: string, _loc?: unknown, item?: Item) => {
      const key = item?.id ?? "";
      output.set(key, (output.get(key) ?? "") + text);
    },
  };
  // What the run said per test; the run's own lines interleave by nature.
  const perTest = () =>
    new Map(Array.from(output).filter(([key]) => key !== ""));
  return { controller, run, items, recorded, state, perTest };
}

async function viaBatch(stream: string) {
  const w = world();
  lines.stream = stream;
  await executeBatch({
    // The packages the stream runs: one it leaves out is errored with the
    // run's whole output, which is no test's.
    pkgInfos: Object.keys(TREE)
      .filter((p) => stream.includes(`"Package":"${p}"`))
      .map((importPath) => ({
        importPath,
        items: [w.items.get(importPath)!] as never,
        dir: `/ws/${importPath.split("/")[1]}`,
      })),
    filter: "^Test",
    workspaceDir: "/ws",
    testFlags: [],
    run: w.run as never,
    token: { isCancellationRequested: false } as never,
    controller: w.controller as never,
    outputChannel: { info: () => {}, warn: () => {}, error: () => {} } as never,
    label: "runner",
  });
  return w;
}

function viaApplier(stream: string) {
  const w = world();
  const applier = new StreamApplier(
    w.controller as never,
    w.run as never,
    (p) => (p in TREE ? `/ws/${p.split("/")[1]}` : undefined),
    "/ws",
  );
  applier.apply(parseTestEvents(stream));
  return w;
}

// kept is a store's records without the merge sequence, which follows the
// order lines arrived in.
function kept(stream: string) {
  const store = new SpecStore();
  store.merge({ stream, folder: "/ws", at: 1 });
  return store
    .serialize()
    .map((r) => ({
      ...r,
      tests: r.tests
        .map(([key, ls]) => [key, ls.map((k) => k.line)] as const)
        .sort(([a], [b]) => a.localeCompare(b)),
    }))
    .sort((a, b) => a.path.localeCompare(b.path));
}

// Each test's lines in the order the store hands them to the CLI.
function composedByTest(stream: string) {
  const store = new SpecStore();
  store.merge({ stream, folder: "/ws", at: 1 });
  const byTest = new Map<string, string[]>();
  for (const line of store.compose("/ws").split("\n")) {
    if (!line) continue;
    const ev = JSON.parse(line) as { Package: string; Test?: string };
    const key = `${ev.Package} ${ev.Test ?? ""}`;
    byTest.set(key, [...(byTest.get(key) ?? []), line]);
  }
  return byTest;
}

const CASES = [
  ["two suites of one package", "suites"],
  ["two packages", "packages"],
  ["a --timeout that writes a verdict after the package's", "timeout"],
  ["a suite process that died under a method", "crash"],
] as const;

describe.each(CASES)("an interleaved stream of %s", (_, name) => {
  const burst = fixture(`${name}-burst`);
  const live = fixture(`${name}-live`);

  it("holds the same events as the burst, in another order", () => {
    expect(live).not.toBe(burst);
    expect(live.split("\n").sort()).toEqual(burst.split("\n").sort());
  });

  it("leaves the Test Explorer as the burst does", async () => {
    const a = await viaBatch(burst);
    const b = await viaBatch(live);
    expect(b.state.size).toBeGreaterThan(0);
    expect(Object.fromEntries(b.state)).toEqual(Object.fromEntries(a.state));
    expect(Object.fromEntries(b.recorded)).toEqual(
      Object.fromEntries(a.recorded),
    );
    expect(Object.fromEntries(b.perTest())).toEqual(
      Object.fromEntries(a.perTest()),
    );
  });

  it("leaves an imported or watched stream's results as the burst does", () => {
    const a = viaApplier(burst);
    const b = viaApplier(live);
    expect(Object.fromEntries(b.state)).toEqual(Object.fromEntries(a.state));
    expect(Object.fromEntries(b.recorded)).toEqual(
      Object.fromEntries(a.recorded),
    );
  });

  it("keeps the Spec store's records as the burst does", () => {
    expect(kept(live)).toEqual(kept(burst));
  });

  it("hands the CLI each test's lines in the order its suite wrote them", () => {
    expect(composedByTest(live)).toEqual(composedByTest(burst));
  });
});

describe("a --timeout's verdicts after the package's", () => {
  const live = fixture("timeout-live");
  const hung = "example.com/shop/SecondTestSuite/TestBHolds";

  it("fails the hung method with everything it printed, not only the deadline line", async () => {
    const w = await viaBatch(live);
    const state = w.state.get(hung)!;
    expect(state).toMatch(/^failed /);
    expect(state).toContain("did not return within 5s");
    expect(state).toContain("--timeout expired while this test was running");
  });

  it("fails the package with what it printed before either verdict", async () => {
    const w = await viaBatch(live);
    expect(w.state.get("example.com/shop")).toContain(
      "gotest: suite binary terminated by signal",
    );
  });
});

describe("a suite process that died under a method, its sibling suite interleaved", () => {
  it("errors the method the package failed under, with what it printed", async () => {
    for (const name of ["crash-burst", "crash-live"]) {
      const w = await viaBatch(fixture(name));
      const state = w.state.get("example.com/shop/CrashTestSuite/TestPanics");
      expect(state, name).toMatch(/^errored No verdict/);
      expect(state, name).toContain("panic: boom");
      expect(w.state.get("example.com/shop/OtherTestSuite/TestB"), name).toBe(
        "passed 0",
      );
    }
  });
});
