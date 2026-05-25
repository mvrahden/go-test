import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockExecFileAsync, mockAccess, mockReadFile, mockShowWarningMessage } =
  vi.hoisted(() => ({
    mockExecFileAsync: vi.fn(
      async (): Promise<{ stdout: string; stderr: string }> => ({
        stdout: "{}",
        stderr: "",
      }),
    ),
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
  const util = await import("node:util");
  const execFileFn: Record<symbol, unknown> = vi.fn() as never;
  execFileFn[util.promisify.custom] = mockExecFileAsync;
  return { execFile: execFileFn };
});

vi.mock("./cli.js", () => ({
  buildCliCommand: async () => ({ bin: "go", args: ["run", "discover"] }),
  formatCliCommand: () => "go run discover",
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

function successResponse(pkgs: Array<{ importPath: string; dir: string }>) {
  return {
    stdout: JSON.stringify({
      packages: pkgs.map((p) => ({
        importPath: p.importPath,
        dir: p.dir,
        suites: [],
      })),
    }),
    stderr: "",
  };
}

describe("DiscoveryService", () => {
  let cache: DiscoveryCache;
  let outputChannel: ReturnType<typeof makeOutputChannel>;
  let service: DiscoveryService;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
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
      mockExecFileAsync.mockResolvedValueOnce(
        successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
      );

      await service.discover("/ws", ["./..."]);

      expect(cache.packages).toHaveLength(1);
      expect(cache.getPackage("example.com/pkg")).toBeDefined();
    });

    it("does not show a warning toast", async () => {
      mockExecFileAsync.mockResolvedValueOnce(
        successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
      );

      await service.discover("/ws", ["./..."]);

      expect(mockShowWarningMessage).not.toHaveBeenCalled();
    });

    it("does not log at debug level", async () => {
      mockExecFileAsync.mockResolvedValueOnce(
        successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
      );

      await service.discover("/ws", ["./..."]);

      expect(outputChannel.debug).not.toHaveBeenCalled();
    });
  });

  describe("when discovery fails transiently then recovers", () => {
    it("retries after 2s and updates cache on success", async () => {
      mockExecFileAsync
        .mockRejectedValueOnce(new Error("cannot find package"))
        .mockResolvedValueOnce(
          successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
        );

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await p;

      expect(cache.packages).toHaveLength(1);
    });

    it("logs the transient failure at debug level", async () => {
      mockExecFileAsync
        .mockRejectedValueOnce(new Error("cannot find package"))
        .mockResolvedValueOnce(
          successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
        );

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await p;

      expect(outputChannel.debug).toHaveBeenCalledWith(
        expect.stringContaining("attempt 1/3 failed, retrying"),
      );
    });

    it("does not show a warning toast", async () => {
      mockExecFileAsync
        .mockRejectedValueOnce(new Error("cannot find package"))
        .mockResolvedValueOnce(
          successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
        );

      const p = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await p;

      expect(mockShowWarningMessage).not.toHaveBeenCalled();
    });

    it("recovers on third attempt after two failures", async () => {
      mockExecFileAsync
        .mockRejectedValueOnce(new Error("fail 1"))
        .mockRejectedValueOnce(new Error("fail 2"))
        .mockResolvedValueOnce(
          successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
        );

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
      mockExecFileAsync.mockRejectedValue(new Error("persistent failure"));
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

    it("shows a warning toast to the user", () => {
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);
      expect(mockShowWarningMessage).toHaveBeenCalledWith(
        expect.stringContaining("discovery failed"),
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
      mockExecFileAsync.mockRejectedValue(new Error("fail"));

      const p1 = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p1;
      expect(mockShowWarningMessage).toHaveBeenCalledTimes(1);

      mockExecFileAsync.mockResolvedValueOnce(
        successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
      );
      await service.discover("/ws", ["./..."]);

      mockExecFileAsync.mockRejectedValue(new Error("fail again"));
      const p3 = service.discover("/ws", ["./..."]);
      await vi.advanceTimersByTimeAsync(2_000);
      await vi.advanceTimersByTimeAsync(4_000);
      await p3;

      expect(mockShowWarningMessage).toHaveBeenCalledTimes(2);
    });
  });

  describe("when a newer discovery request is queued during retry", () => {
    it("aborts the retry loop", async () => {
      mockExecFileAsync
        .mockRejectedValueOnce(new Error("transient"))
        .mockResolvedValue(
          successResponse([{ importPath: "example.com/pkg", dir: "/ws/pkg" }]),
        );

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
