import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  TestMessage: class {},
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
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

import { executeBatch, runPiece } from "./batchRunner.js";
import { buildCliCommand } from "./cli.js";
import { setEventsDir } from "./eventsFiles.js";
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
