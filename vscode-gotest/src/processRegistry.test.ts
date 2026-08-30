import { describe, it, expect, vi, beforeEach } from "vitest";
import * as nodePath from "node:path";

const {
  mockReadFile,
  mockWriteFile,
  mockMkdir,
  mockRename,
  files,
  mockIdentify,
  mockToken,
  mockReaddir,
  mockUnlink,
} = vi.hoisted(() => {
  const files = new Map<string, string>();
  return {
    files,
    mockReadFile: vi.fn(async (p: string) => {
      const content = files.get(p);
      if (content === undefined) throw new Error("ENOENT");
      return content;
    }),
    mockWriteFile: vi.fn(async (p: string, content: string) => {
      files.set(p, content);
    }),
    mockMkdir: vi.fn(async () => undefined),
    mockRename: vi.fn(async (from: string, to: string) => {
      const content = files.get(from);
      if (content === undefined) throw new Error("ENOENT");
      files.set(to, content);
      files.delete(from);
    }),
    mockIdentify: vi.fn((_pid: number, _token: string) => "same-process"),
    mockToken: vi.fn((_pid: number) => "tok" as string | undefined),
    mockReaddir: vi.fn(async () =>
      // readdir yields basenames, and only path.basename knows which separator
      // this platform used to build the key.
      [...files.keys()].map((p) => nodePath.basename(p)),
    ),
    mockUnlink: vi.fn(async (p: string) => {
      if (!files.delete(p)) throw new Error("ENOENT");
    }),
  };
});

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
  writeFile: mockWriteFile,
  mkdir: mockMkdir,
  rename: mockRename,
  readdir: mockReaddir,
  unlink: mockUnlink,
}));

vi.mock("./processIdentity.js", () => ({
  identify: mockIdentify,
  readProcessStartToken: mockToken,
}));

import * as path from "node:path";
import { ProcessRegistry } from "./processRegistry.js";

const DIR = path.join(path.sep, "storage");
const GRACE = 360_000;

// Each session owns one file; tests address them by the session id they chose.
function fileFor(session: string): string {
  return path.join(DIR, `child-processes-${session}.json`);
}

function storedIn(session: string): { pid: number; kind: string }[] {
  const raw = files.get(fileFor(session));
  return raw ? JSON.parse(raw).processes : [];
}

function newRegistry(session: string, hostPid = 500) {
  return new ProcessRegistry(DIR, { sessionId: session, hostPid });
}

describe("ProcessRegistry", () => {
  beforeEach(() => {
    files.clear();
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockIdentify.mockReturnValue("same-process");
    mockToken.mockReturnValue("tok");
  });

  it("persists a spawn immediately, so a crash cannot lose it", async () => {
    const registry = newRegistry("me");
    registry.add(4242, "watch");
    await registry.flush();

    expect(JSON.parse(files.get(fileFor("me"))!).version).toBe(2);
    expect(storedIn("me")[0]).toMatchObject({ pid: 4242, kind: "watch" });
  });

  it("forgets a child that exited normally", async () => {
    const registry = newRegistry("me");
    const key = registry.add(4242, "watch");
    registry.remove(key);
    await registry.flush();
    expect(storedIn("me")).toHaveLength(0);
  });

  // A process nothing can identify later can never be reaped, so recording it
  // is pure overhead. Off Linux the table is never written at all.
  it("records nothing when the platform cannot identify a process", async () => {
    mockToken.mockReturnValue(undefined);
    const registry = newRegistry("me");
    expect(registry.add(4242, "watch")).toBeUndefined();
    await registry.flush();
    expect(files.size).toBe(0);
  });

  describe("reaping what a dead session left", () => {
    // A previous session: its host pid is 900, and it is gone.
    async function seedDeadSession(pid: number, session = "old") {
      const previous = new ProcessRegistry(DIR, {
        sessionId: session,
        hostPid: 900,
      });
      previous.add(pid, "watch");
      await previous.flush();
    }

    // identify() answers for both hosts and children; route by pid.
    function aliveExcept(deadPids: number[]) {
      mockIdentify.mockImplementation((pid: number) =>
        deadPids.includes(pid) ? "gone-or-different" : "same-process",
      );
    }

    it("terminates an orphan's process group", async () => {
      await seedDeadSession(4242);
      aliveExcept([900]); // the old host is gone; its child is not

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      const { signalled } = registry.reapOrphans({ kill, graceMs: GRACE });

      expect(signalled).toHaveLength(1);
      // Negative pid: the group, not just the `go run` that leads it.
      expect(kill).toHaveBeenCalledWith(-4242, "SIGTERM");
    });

    // THE regression this class was rewritten for. Activation registers file
    // watchers and commands before it gets here, so a discovery can already be
    // running by the time the reaper looks — and it is not an orphan.
    it("never signals a process THIS session started", async () => {
      await seedDeadSession(2222);
      aliveExcept([900]);

      const registry = newRegistry("me");
      // A watcher fires during activation, before load() is reached.
      registry.add(1111, "read");
      await registry.flush();
      await registry.load();

      const kill = vi.fn();
      registry.reapOrphans({ kill, graceMs: GRACE });

      const signalled = kill.mock.calls.map((c) => c[0]);
      expect(signalled).toContain(-2222);
      expect(signalled).not.toContain(-1111);
    });

    // The same window used to destroy the table before it was ever read: one
    // shared file meant a spawn during activation overwrote the inherited
    // records. A session only ever writes its own file now.
    it("does not let an early spawn erase the inherited table", async () => {
      await seedDeadSession(2222);
      aliveExcept([900]);

      const registry = newRegistry("me");
      registry.add(1111, "read");
      await registry.flush();
      await registry.load();

      expect(registry.inheritedSize).toBe(1);
      expect(storedIn("old").map((r) => r.pid)).toEqual([2222]);
      expect(storedIn("me").map((r) => r.pid)).toEqual([1111]);
    });

    // Two windows on one folder share workspace storage. Neither may touch the
    // other's children, and a shared table could not tell them apart.
    it("leaves a live sibling window's processes strictly alone", async () => {
      // A dead session is seeded alongside the live one on purpose. Asserting
      // only that the sibling was spared cannot tell "correctly skipped" from
      // "read nothing at all" — a Windows run passed this test while load()
      // was absorbing no files whatsoever. The dead session is the control.
      await seedDeadSession(2222);

      const sibling = new ProcessRegistry(DIR, {
        sessionId: "other",
        hostPid: 700,
      });
      sibling.add(3333, "test");
      await sibling.flush();

      aliveExcept([900]); // the old host is gone; the sibling's host is not

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      const { signalled } = registry.reapOrphans({ kill, graceMs: GRACE });

      // The dead session's child is reaped...
      expect(signalled.map((r) => r.pid)).toEqual([2222]);
      expect(kill).toHaveBeenCalledWith(-2222, "SIGTERM");
      // ...and the live sibling's is not, nor is its table touched.
      expect(kill).not.toHaveBeenCalledWith(-3333, "SIGTERM");
      expect(storedIn("other").map((r) => r.pid)).toEqual([3333]);
    });

    it("escalates to SIGKILL when an orphan ignores SIGTERM", async () => {
      await seedDeadSession(4242);
      aliveExcept([900]);

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      registry.reapOrphans({ kill, graceMs: GRACE });
      expect(kill).toHaveBeenCalledWith(-4242, "SIGTERM");

      await vi.advanceTimersByTimeAsync(GRACE);
      expect(kill).toHaveBeenCalledWith(-4242, "SIGKILL");
    });

    it("does not escalate against an orphan that died on SIGTERM", async () => {
      await seedDeadSession(4242);
      aliveExcept([900]);

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      registry.reapOrphans({ kill, graceMs: GRACE });

      aliveExcept([900, 4242]); // it obeyed
      await vi.advanceTimersByTimeAsync(GRACE);

      expect(kill).not.toHaveBeenCalledWith(-4242, "SIGKILL");
    });

    // One signal is a request, not a guarantee. A survivor stays on the books.
    it("keeps a record for a process that outlived SIGKILL", async () => {
      await seedDeadSession(4242);
      aliveExcept([900]);

      const registry = newRegistry("me");
      await registry.load();
      registry.reapOrphans({ kill: vi.fn(), graceMs: GRACE });
      await vi.advanceTimersByTimeAsync(GRACE);
      await registry.flush();

      expect(storedIn("old").map((r) => r.pid)).toEqual([4242]);
    });

    it("removes a dead session's file once nothing in it survives", async () => {
      await seedDeadSession(4242);
      aliveExcept([900, 4242]); // host and child both gone

      const registry = newRegistry("me");
      await registry.load();
      registry.reapOrphans({ kill: vi.fn(), graceMs: GRACE });
      await registry.flush();

      expect(files.has(fileFor("old"))).toBe(false);
    });

    it("drops a record whose pid is now a different process", async () => {
      await seedDeadSession(4242);
      aliveExcept([900, 4242]);

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      const { signalled, vanished } = registry.reapOrphans({
        kill,
        graceMs: GRACE,
      });

      expect(kill).not.toHaveBeenCalled();
      expect(signalled).toHaveLength(0);
      expect(vanished).toHaveLength(1);
    });

    it("never signals when the platform cannot identify the process", async () => {
      await seedDeadSession(4242);
      mockIdentify.mockImplementation((pid: number) =>
        pid === 900 ? "gone-or-different" : "unknown",
      );

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      registry.reapOrphans({ kill, graceMs: GRACE });
      expect(kill).not.toHaveBeenCalled();
    });

    it("ignores a session file from a future version", async () => {
      files.set(
        fileFor("old"),
        JSON.stringify({ version: 3, hostPid: 900, processes: [{ pid: 1 }] }),
      );
      const registry = newRegistry("me");
      await registry.load();
      expect(registry.inheritedSize).toBe(0);
    });

    it("stops the escalation when the extension goes away", async () => {
      await seedDeadSession(4242);
      aliveExcept([900]);

      const registry = newRegistry("me");
      await registry.load();
      const kill = vi.fn();
      registry.reapOrphans({ kill, graceMs: GRACE });
      registry.dispose();

      await vi.advanceTimersByTimeAsync(GRACE * 2);
      expect(kill).not.toHaveBeenCalledWith(-4242, "SIGKILL");
    });
  });
});
