import { describe, it, expect, vi } from "vitest";

const settings = vi.hoisted(() => ({
  values: {} as Record<string, unknown>,
  scopes: [] as unknown[],
}));

vi.mock("vscode", () => ({
  TestMessage: class {},
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: {
    getConfiguration: (_section: string, scope?: unknown) => {
      settings.scopes.push(scope);
      return { get: (key: string) => settings.values[key] };
    },
  },
  window: {},
}));

vi.mock("./cli.js", async (orig) => ({
  ...(await orig<typeof import("./cli.js")>()),
  buildCliCommand: vi.fn(async (args: string[]) => ({ bin: "gotest", args })),
}));
vi.mock("./runnerUtils.js", async (orig) => ({
  ...(await orig<typeof import("./runnerUtils.js")>()),
  spawnTestProcess: vi.fn(async () => ({
    stdout: "",
    stderr: "",
    exitCode: 0,
  })),
}));

import {
  changedOf,
  executeBatch,
  narrowedLine,
  runPiece,
} from "./batchRunner.js";
import { spawnTestProcess } from "./runnerUtils.js";
import { buildCliCommand } from "./cli.js";
import { setEventsDir } from "./eventsFiles.js";
import { RunProgress } from "./liveProgress.js";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";

interface Item {
  id: string;
  label: string;
  parent?: Item;
  tags: { id: string }[];
}

function item(id: string, label: string, parent?: Item): Item {
  return { id, label, parent, tags: [] };
}

const pkg: Item = {
  ...item("example.com/cart", "cart"),
  tags: [{ id: "package" }],
};
const suite = item("example.com/cart/CartTestSuite", "CartTestSuite", pkg);
const add = item("example.com/cart/CartTestSuite/TestAdd", "TestAdd", suite);

describe("runPiece", () => {
  it("says a filtered run covered exactly the test paths it asked for", () => {
    const piece = runPiece("stream", {
      pkgInfos: [
        { importPath: pkg.id, items: [add] as never, dir: "/ws/cart" },
      ],
      filter: "^TestCartTestSuite$/^TestAdd$",
      workspaceDir: "/ws",
    });

    expect(piece.stream).toBe("stream");
    expect(piece.folder).toBe("/ws");
    const covers = piece.partial?.get(pkg.id);
    expect(covers, "a one-method rerun is partial").toBeDefined();
    expect(covers!("TestCartTestSuite/TestAdd")).toBe(true);
    expect(covers!("TestCartTestSuite/TestAdd/when_empty")).toBe(true);
    expect(covers!("TestCartTestSuite/TestTotal")).toBe(false);
    expect(covers!("TestCartTestSuite")).toBe(false);
  });

  it("says an unfiltered run replaced its packages whole", () => {
    const piece = runPiece("stream", {
      pkgInfos: [
        { importPath: pkg.id, items: [pkg] as never, dir: "/ws/cart" },
      ],
      filter: undefined,
      workspaceDir: "/ws",
    });
    expect(piece.partial).toBeUndefined();
  });

  it("says when the run finished", () => {
    const config = {
      pkgInfos: [],
      filter: undefined,
      workspaceDir: "/ws",
    };
    expect(runPiece("stream", config, 1234).at).toBe(1234);
  });
});

describe("executeBatch's events file", () => {
  const config = () =>
    ({
      pkgInfos: [{ importPath: "example.com/cart", items: [], dir: "/w/cart" }],
      filter: "^TestX$",
      workspaceDir: "/w",
      testFlags: [],
      run: { appendOutput: () => {} },
      token: { isCancellationRequested: false },
      controller: {},
      outputChannel: { info: () => {}, warn: () => {}, error: () => {} },
      label: "runner",
    }) as never;

  it("has the CLI save the run's stream under the events directory", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-events-"));
    setEventsDir(dir);
    try {
      await executeBatch(config());
      const args = vi.mocked(buildCliCommand).mock.calls.at(-1)![0];
      const flag = args.find((a) => a.startsWith("--events-file="));
      expect(flag).toBeDefined();
      expect(path.dirname(flag!.slice("--events-file=".length))).toBe(dir);
      // A gotest flag: before the go test flags.
      expect(args.indexOf(flag!)).toBeLessThan(args.indexOf("--"));
    } finally {
      setEventsDir(undefined);
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("asks for none without a directory", async () => {
    await executeBatch(config());
    const args = vi.mocked(buildCliCommand).mock.calls.at(-1)![0];
    expect(args.some((a) => a.startsWith("--events-file="))).toBe(false);
  });
});

const manifest = (changed: object) =>
  JSON.stringify({
    Action: "start",
    Package: "gotest run",
    RunLevel: true,
    Manifest: { Version: "v1.42.0", Args: [], Changed: changed },
  });

describe("a --changed batch", () => {
  it("reads how the run narrowed itself from its manifest", () => {
    expect(changedOf(manifest({ Ran: 1, Unchanged: 3 }) + "\n")).toEqual({
      ran: 1,
      unchanged: 3,
      noRecord: false,
    });
    expect(changedOf('{"Action":"start","Package":"p"}\n')).toBeUndefined();
    expect(narrowedLine({ ran: 1, unchanged: 3, noRecord: false })).toBe(
      "[gotest] narrowed by --changed: ran 1 package, left out 3 packages whose inputs did not change; their verdicts are the last record's, not this run's.",
    );
  });

  it("asks for --changed, says the run was narrowed, and leaves the packages it left out alone", async () => {
    const out: string[] = [];
    const errored: string[] = [];
    vi.mocked(spawnTestProcess).mockResolvedValueOnce({
      stdout: manifest({ Ran: 0, Unchanged: 1 }) + "\n",
      stderr: "",
      exitCode: 1,
    } as never);
    const item = {
      id: "example.com/cart",
      tags: [{ id: "package" }],
      children: { forEach: () => {} },
    };
    await executeBatch({
      pkgInfos: [
        { importPath: "example.com/cart", items: [item], dir: "/w/cart" },
      ],
      filter: undefined,
      workspaceDir: "/w",
      testFlags: [],
      run: {
        appendOutput: (s: string) => out.push(s),
        errored: (i: { id: string }) => errored.push(i.id),
        skipped: () => {},
      },
      token: { isCancellationRequested: false },
      controller: { recordResult: () => {}, getResult: () => undefined },
      outputChannel: { info: () => {}, warn: () => {}, error: () => {} },
      label: "runner",
      changed: true,
    } as never);
    const args = vi.mocked(buildCliCommand).mock.calls.at(-1)![0];
    expect(args.slice(0, args.indexOf("--"))).toContain("--changed");
    expect(out.join("")).toContain("narrowed by --changed: ran 0 packages");
    expect(errored).toEqual([]);
  });
});

describe("a batch's live stream", () => {
  const config = (extra: object = {}) =>
    ({
      pkgInfos: [{ importPath: "example.com/cart", items: [], dir: "/w/cart" }],
      filter: "^TestX$",
      workspaceDir: "/w",
      testFlags: [],
      run: {
        appendOutput: () => {},
        started: () => {},
        passed: () => {},
        failed: () => {},
      },
      token: { isCancellationRequested: false },
      controller: {
        findItem: () => undefined,
        recordResult: () => {},
        noteStart: () => {},
      },
      outputChannel: { info: () => {}, warn: () => {}, error: () => {} },
      label: "runner",
      ...extra,
    }) as never;
  const envOfLastRun = () =>
    vi.mocked(spawnTestProcess).mock.calls.at(-1)![6] as
      Record<string, string> | undefined;

  it("has the CLI write each suite's events as they happen", async () => {
    settings.values = {};
    await executeBatch(config({ env: { GOTEST_UPDATE_SNAPSHOTS: "1" } }));
    expect(envOfLastRun()).toEqual({
      GOTEST_UPDATE_SNAPSHOTS: "1",
      GOTEST_LIVE: "1",
    });
  });

  it("leaves the variable out where the folder turned live results off", async () => {
    settings.values = { liveResults: false };
    settings.scopes = [];
    try {
      await executeBatch(config());
      expect(envOfLastRun()).toBeUndefined();
      expect(settings.scopes).toContainEqual({ fsPath: "/w" });
    } finally {
      settings.values = {};
    }
  });

  it("counts each event of its packages as it applies, and keeps the stream", async () => {
    const lines = [
      '{"Action":"run","Package":"example.com/cart","Test":"TestX"}',
      '{"Action":"run","Package":"example.com/cart","Test":"TestX/TestA"}',
      '{"Action":"pass","Package":"example.com/cart","Test":"TestX/TestA"}',
      '{"Action":"run","Package":"example.com/other","Test":"TestY"}',
    ];
    let midRun: string | undefined;
    vi.mocked(spawnTestProcess).mockImplementationOnce(
      async (...args: unknown[]) => {
        const onLine = args[7] as (l: string) => void;
        for (const l of lines) onLine(l);
        midRun = progress.pieces().at(-1)?.stream;
        return { stdout: lines.join("\n") + "\n", stderr: "", exitCode: 0 };
      },
    );
    const progress = new RunProgress("run: cart", () => {});
    await executeBatch(config({ progress }));
    expect(progress.tally).toEqual({ started: 2, passed: 1, failed: 0 });
    expect(midRun).toBe(lines.join("\n") + "\n");
    expect(progress.pieces()).toHaveLength(1);
    expect(progress.pieces()[0].folder).toBe("/w");
  });
});

describe("a --changed batch's running state", () => {
  interface Node {
    id: string;
    label: string;
    parent?: Node;
    tags: { id: string }[];
    kids: Map<string, Node>;
    children: {
      forEach: (fn: (n: Node) => void) => void;
      get: (id: string) => Node | undefined;
    };
  }
  function node(id: string, parent?: Node, tags: string[] = []): Node {
    const kids = new Map<string, Node>();
    const n: Node = {
      id,
      label: id.split("/").pop()!,
      parent,
      tags: tags.map((t) => ({ id: t })),
      kids,
      children: {
        forEach: (fn) => kids.forEach(fn),
        get: (k) => kids.get(k),
      },
    };
    parent?.kids.set(id, n);
    return n;
  }
  const root = node("dir:pkgs");
  const cart = node("example.com/cart", root, ["package"]);
  const cartSuite = node("example.com/cart/CartSuite", cart);
  const cartAdd = node("example.com/cart/CartSuite/TestAdd", cartSuite);
  const shop = node("example.com/shop", root, ["package"]);
  const shopSuite = node("example.com/shop/ShopSuite", shop);
  const all = [root, cart, cartSuite, cartAdd, shop, shopSuite];

  function harness() {
    const calls: string[] = [];
    const cleared: string[] = [];
    const say = (what: string) => (i: Node) => calls.push(`${what} ${i.id}`);
    const run = {
      appendOutput: () => {},
      started: say("started"),
      enqueued: say("enqueued"),
      passed: say("passed"),
      failed: say("failed"),
      skipped: say("skipped"),
      errored: say("errored"),
    };
    const results = new Map<string, string>();
    const controller = {
      findItem: (id: string) => all.find((n) => n.id === id),
      clearResults: (i: Node) => cleared.push(i.id),
      recordResult: (id: string, status: string) => results.set(id, status),
      getResult: (id: string) =>
        results.has(id) ? { status: results.get(id) } : undefined,
      noteStart: () => {},
      notePaused: () => {},
      createDynamicSubtest: () => undefined,
    };
    const config = {
      pkgInfos: [
        { importPath: cart.id, items: [cart], dir: "/w/cart" },
        { importPath: shop.id, items: [shop], dir: "/w/shop" },
      ],
      filter: undefined,
      workspaceDir: "/w",
      testFlags: [],
      run,
      token: { isCancellationRequested: false },
      controller,
      outputChannel: { info: () => {}, warn: () => {}, error: () => {} },
      label: "runner",
      changed: true,
    } as never;
    return { calls, cleared, config };
  }

  // Feeds the lines one by one, recording what the run showed after each.
  function stream(lines: string[], calls: string[]) {
    const after: string[][] = [];
    vi.mocked(spawnTestProcess).mockImplementationOnce(
      async (...args: unknown[]) => {
        const onLine = args[7] as (l: string) => void;
        for (const l of lines) {
          onLine(l);
          after.push([...calls]);
        }
        return { stdout: lines.join("\n") + "\n", stderr: "", exitCode: 0 };
      },
    );
    return after;
  }

  const cartEvents = [
    '{"Action":"start","Package":"example.com/cart"}',
    '{"Action":"run","Package":"example.com/cart","Test":"TestCartSuite","Gotest":{"Subtest":"TestCartSuite"}}',
    '{"Action":"pass","Package":"example.com/cart","Test":"TestCartSuite"}',
    '{"Action":"pass","Package":"example.com/cart"}',
  ];
  const touches = (calls: string[], n: Node) =>
    calls.filter((c) => c.endsWith(" " + n.id) || c.includes(" " + n.id + "/"));

  it("marks the packages the manifest lists as running before their events, and leaves the rest alone", async () => {
    const { calls, cleared, config } = harness();
    const after = stream(
      [manifest({ Ran: 1, Unchanged: 1, Packages: [cart.id] }), ...cartEvents],
      calls,
    );
    await executeBatch(config);

    expect(after[0]).toEqual([
      `started ${cart.id}`,
      `enqueued ${cartSuite.id}`,
      `enqueued ${cartAdd.id}`,
      `started ${root.id}`,
    ]);
    expect(cleared).toEqual([cart.id]);
    expect(touches(calls, shop)).toEqual([]);
    // The method no event resolved ends skipped, as in a full run.
    expect(calls).toContain(`skipped ${cartAdd.id}`);
  });

  it("errors a package it said it would run that never reported, not one it left out", async () => {
    const { calls, config } = harness();
    vi.mocked(spawnTestProcess).mockImplementationOnce(
      async (...args: unknown[]) => {
        const line = manifest({ Ran: 1, Unchanged: 1, Packages: [cart.id] });
        (args[7] as (l: string) => void)(line);
        return { stdout: line + "\n", stderr: "build broke", exitCode: 2 };
      },
    );
    await executeBatch(config);
    expect(calls).toContain(`errored ${cart.id}`);
    expect(touches(calls, shop)).toEqual([]);
  });

  it("marks a package at its own start event when the manifest lists none", async () => {
    const { calls, cleared, config } = harness();
    const after = stream(
      [manifest({ Ran: 1, Unchanged: 1 }), ...cartEvents],
      calls,
    );
    await executeBatch(config);

    expect(after[0]).toEqual([]);
    expect(after[1]).toEqual([
      `started ${cart.id}`,
      `enqueued ${cartSuite.id}`,
      `enqueued ${cartAdd.id}`,
      `started ${root.id}`,
    ]);
    expect(cleared).toEqual([cart.id]);
    expect(touches(calls, shop)).toEqual([]);
  });
});
