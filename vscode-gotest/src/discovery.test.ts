import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SpawnScript } from "./scriptedSpawn.test-support.js";

const {
  script,
  mockKill,
  mockClearBinaryCache,
  mockAccess,
  mockReadFile,
  mockShowWarningMessage,
} = vi.hoisted(() => ({
  script: { once: [], always: undefined } as SpawnScript,
  mockKill: vi.fn(),
  mockClearBinaryCache: vi.fn(),
  mockAccess: vi.fn(async () => {}),
  mockReadFile: vi.fn(async (): Promise<string> => {
    throw new Error("ENOENT");
  }),
  mockShowWarningMessage: vi.fn(async () => undefined),
}));

vi.mock("vscode", () => ({
  workspace: {
    workspaceFolders: [],
    getConfiguration: () => ({ get: () => undefined }),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  window: { showWarningMessage: mockShowWarningMessage },
  EventEmitter: class {
    private listeners: Array<() => void> = [];
    event = (listener: () => void) => {
      this.listeners.push(listener);
      return { dispose: () => {} };
    };
    fire = () => {
      for (const l of this.listeners) l();
    };
    dispose = () => {};
  },
}));

vi.mock("node:fs/promises", () => ({
  access: mockAccess,
  readFile: mockReadFile,
}));

vi.mock("node:child_process", async () => {
  const { createScriptedSpawn } =
    await import("./scriptedSpawn.test-support.js");
  return { spawn: createScriptedSpawn(script, mockKill) };
});

vi.mock("./cli.js", () => ({
  buildCliCommand: async () => ({ bin: "go", args: ["run", "discover"] }),
  formatCliCommand: () => "go run discover",
  clearBinaryCache: mockClearBinaryCache,
  scopedConfig: () => ({ get: () => undefined }),
  // Used by capture.js on a failed exit. A stub, not a copy: what it filters
  // out is pinned in specView.test.ts, where the real one interprets an exit.
  stripGoRunExitEcho: (stderr: string) => stderr.trim(),
}));

import { DiscoveryCache, DiscoveryService } from "./discovery.js";

function makeOutputChannel() {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
    show: vi.fn(),
  } as unknown as import("vscode").LogOutputChannel;
}

type Pkg = { importPath: string; dir: string };

function discoverJson(pkgs: Pkg[]): string {
  return JSON.stringify({
    packages: pkgs.map((p) => ({
      importPath: p.importPath,
      dir: p.dir,
      suites: [],
    })),
  });
}

function succeedsOnce(pkgs: Pkg[]) {
  script.once.push({ stdout: [discoverJson(pkgs)] });
}

function succeedsAlways(pkgs: Pkg[]) {
  script.always = { stdout: [discoverJson(pkgs)] };
}

function failsOnce(message: string) {
  script.once.push({ code: 2, stderr: message });
}

function failsAlways(message: string) {
  script.always = { code: 2, stderr: message };
}

describe("DiscoveryService", () => {
  let cache: DiscoveryCache;
  let outputChannel: ReturnType<typeof makeOutputChannel>;
  let service: DiscoveryService;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    script.once = [];
    script.always = undefined;
    mockAccess.mockResolvedValue(undefined);
    mockReadFile.mockRejectedValue(new Error("ENOENT"));
    cache = new DiscoveryCache();
    outputChannel = makeOutputChannel();
    service = new DiscoveryService(
      cache,
      outputChannel as unknown as import("vscode").LogOutputChannel,
    );
  });

  describe("when discovery succeeds immediately", () => {
    it("updates the cache with discovered packages", async () => {
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      await service.discover("/ws", ["./..."]);

      expect(cache.packages).toHaveLength(1);
      expect(cache.getPackage("example.com/pkg")).toBeDefined();
    });

    it("does not show a warning toast", async () => {
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      await service.discover("/ws", ["./..."]);

      expect(mockShowWarningMessage).not.toHaveBeenCalled();
    });

    it("does not log at debug level", async () => {
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      await service.discover("/ws", ["./..."]);

      expect(outputChannel.debug).not.toHaveBeenCalled();
    });
  });

  describe("when discovery fails transiently then recovers", () => {
    it("retries after 2s and updates cache on success", async () => {
      failsOnce("cannot find package");
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await p;

      expect(cache.packages).toHaveLength(1);
    });

    it("logs the transient failure at debug level", async () => {
      failsOnce("cannot find package");
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await p;

      expect(outputChannel.debug).toHaveBeenCalledWith(
        expect.stringContaining("attempt 1/3 failed, retrying"),
      );
    });

    it("does not show a warning toast", async () => {
      failsOnce("cannot find package");
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await p;

      expect(mockShowWarningMessage).not.toHaveBeenCalled();
    });

    it("recovers on third attempt after two failures", async () => {
      failsOnce("fail 1");
      failsOnce("fail 2");
      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p;

      expect(cache.packages).toHaveLength(1);
      expect(outputChannel.debug).toHaveBeenCalledTimes(2);
      expect(mockShowWarningMessage).not.toHaveBeenCalled();
    });
  });

  describe("when all retry attempts fail", () => {
    beforeEach(async () => {
      failsAlways("persistent failure");
      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p;
    });

    it("does not update the cache", () => {
      expect(cache.packages).toHaveLength(0);
    });

    it("logs the final failure at error level", () => {
      expect(outputChannel.error).toHaveBeenCalledWith(
        expect.stringContaining("failed after 3 attempts"),
      );
      expect(outputChannel.error).toHaveBeenCalledTimes(1);
    });

    it("shows a warning toast naming what went wrong", () => {
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
      expect(mockShowWarningMessage).toHaveBeenCalledWith(
        expect.stringContaining(
          "discovery failed: exited with code 2: persistent failure",
        ),
        "Open Output",
      );
      // The toolchain advice belongs to a missing binary, not to this.
      expect(mockShowWarningMessage).not.toHaveBeenCalledWith(
        expect.stringContaining("Ensure 'go' is installed"),
        "Open Output",
      );
    });

    it("does not show duplicate toasts on subsequent failures", async () => {
      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p;

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
    });
  });

  describe("when discovery recovers after a previous total failure", () => {
    it("re-enables the warning toast for future failures", async () => {
      failsAlways("fail");

      const p1 = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p1;
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);

      succeedsOnce([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);
      await service.discover("/ws", ["./..."]);

      failsAlways("fail again");
      const p3 = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p3;

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(2);
    });
  });

  // The payload has no bound: it carries every behavior every suite declares.
  // Reading it through a fixed buffer is what made a large repo undiscoverable,
  // and the size below is past the 1 MiB such a read used to allow.
  describe("when the CLI writes more than a buffered read would hold", () => {
    it("keeps every package in a payload larger than 1 MiB", async () => {
      const pkgs = Array.from({ length: 9_000 }, (_, i) => ({
        importPath: `example.com/org/repo/internal/service/component${i}`,
        dir: `/ws/internal/service/component${i}`,
      }));
      const json = discoverJson(pkgs);
      expect(json.length).toBeGreaterThan(1024 * 1024);

      const chunks: string[] = [];
      for (let i = 0; i < json.length; i += 64 * 1024) {
        chunks.push(json.slice(i, i + 64 * 1024));
      }
      script.once.push({ stdout: chunks });

      await service.discover("/ws", ["./..."]);

      expect(cache.packages).toHaveLength(9_000);
      expect(outputChannel.error).not.toHaveBeenCalled();
    });

    // A read ends wherever the pipe filled, which can be mid-character. Decoding
    // each chunk on its own would replace the halves with U+FFFD, and the id it
    // corrupts is the one a run has to match to land on the same tree node.
    it("decodes a character split across two reads", async () => {
      const importPath = "example.com/café_日本語_🧪";
      const json = discoverJson([{ importPath, dir: "/ws/unicode" }]);
      const bytes = Buffer.from(json, "utf-8");
      const split = Buffer.byteLength(json.slice(0, json.indexOf("日"))) + 1;
      script.once.push({
        stdout: [bytes.subarray(0, split), bytes.subarray(split)],
      });

      await service.discover("/ws", ["./..."]);

      expect(cache.getPackage(importPath)).toBeDefined();
    });
  });

  describe("when the CLI never finishes", () => {
    it("kills it and reports the timeout", async () => {
      script.always = { neverExits: true };

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(120_000);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(120_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await vi.advanceTimersByTimeAsync(120_000);
      await p;

      expect(mockKill).toHaveBeenCalledTimes(3);
      expect(outputChannel.error).toHaveBeenCalledWith(
        expect.stringContaining("timed out after 120s"),
      );
    });
  });

  // A missing binary arrives as an error event, not an exit code. The cached
  // path has to be dropped either way, or every later run repeats the mistake.
  describe("when the go binary has moved", () => {
    it("clears the cached binary path", async () => {
      const enoent: NodeJS.ErrnoException = new Error("spawn go ENOENT");
      enoent.code = "ENOENT";
      script.always = { error: enoent };

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p;

      expect(mockClearBinaryCache).toHaveBeenCalledTimes(3);
      expect(outputChannel.error).toHaveBeenCalledWith(
        expect.stringContaining("ENOENT"),
      );
      expect(mockShowWarningMessage).toHaveBeenCalledWith(
        expect.stringContaining("Ensure 'go' is installed"),
        "Open Output",
      );
    });
  });

  describe("when a newer discovery request is queued during retry", () => {
    it("aborts the retry loop", async () => {
      failsOnce("transient");
      succeedsAlways([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]);

      const p1 = service.discover("/ws", ["./..."]);
      // While retrying, queue a second request for same workspace
      const p2 = service.discover("/ws", ["./..."]);

      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p1;
      await p2;

      expect(outputChannel.debug).toHaveBeenCalledWith(
        expect.stringContaining("superseded by queued request"),
      );
    });
  });
});

describe("DiscoveryCache broken packages", () => {
  function suiteEntry(name: string) {
    return {
      name,
      parallel: false,
      focused: false,
      excluded: false,
      guarded: false,
      file: `${name.toLowerCase()}_test.go`,
      line: 1,
      col: 1,
      lifecycle: [],
      fixtures: [],
      methods: [],
    };
  }

  it("keeps the last known suites when a package turns broken", () => {
    const cache = new DiscoveryCache();
    cache.update(
      [
        {
          importPath: "example.com/pkg",
          dir: "/ws/pkg",
          suites: [suiteEntry("MySuite")],
        },
      ],
      true,
      "/ws",
    );
    cache.update(
      [{ importPath: "example.com/pkg", dir: "", broken: true, suites: [] }],
      false,
      "/ws",
    );

    const pkg = cache.getPackage("example.com/pkg");
    expect(pkg?.broken).toBe(true);
    expect(pkg?.suites.map((s) => s.name)).toEqual(["MySuite"]);
    // The empty incoming dir must not clobber the known location.
    expect(pkg?.dir).toBe("/ws/pkg");
    expect(cache.resolveFileToPackage("/ws/pkg/mysuite_test.go")).toBe(
      "example.com/pkg",
    );
  });

  it("drops the broken flag when the package builds again", () => {
    const cache = new DiscoveryCache();
    cache.update(
      [
        {
          importPath: "example.com/pkg",
          dir: "/ws/pkg",
          broken: true,
          suites: [],
        },
      ],
      true,
      "/ws",
    );
    cache.update(
      [
        {
          importPath: "example.com/pkg",
          dir: "/ws/pkg",
          suites: [suiteEntry("MySuite")],
        },
      ],
      false,
      "/ws",
    );

    const pkg = cache.getPackage("example.com/pkg");
    expect(pkg?.broken).toBeUndefined();
    expect(pkg?.suites.map((s) => s.name)).toEqual(["MySuite"]);
  });

  it("stores a never-seen broken package as-is without indexing an empty dir", () => {
    const cache = new DiscoveryCache();
    cache.update(
      [{ importPath: "example.com/gone", dir: "", broken: true, suites: [] }],
      true,
      "/ws",
    );

    expect(cache.getPackage("example.com/gone")?.broken).toBe(true);
    expect(cache.resolveFileToPackage("")).toBeUndefined();
  });

  it("returns per-package warnings for the badge", () => {
    const cache = new DiscoveryCache();
    cache.update(
      [
        {
          importPath: "example.com/pkg",
          dir: "/ws/pkg",
          broken: true,
          suites: [],
        },
      ],
      true,
      "/ws",
      [
        {
          importPath: "example.com/pkg",
          message: "svc.go:4:17: cannot use 42",
        },
        { importPath: "example.com/other", message: "unrelated" },
      ],
    );

    expect(cache.getWarnings("example.com/pkg").map((w) => w.message)).toEqual([
      "svc.go:4:17: cannot use 42",
    ]);
  });
});
