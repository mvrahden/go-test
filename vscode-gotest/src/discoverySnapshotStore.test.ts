import { describe, it, expect, vi, beforeEach } from "vitest";

const {
  mockReadFile,
  mockWriteFile,
  mockMkdir,
  mockReaddir,
  mockStat,
  mockUnlink,
  files,
  mtimes,
} = vi.hoisted(() => {
  const files = new Map<string, string>();
  const mtimes = new Map<string, number>();
  return {
    files,
    mtimes,
    mockReadFile: vi.fn(async (p: string) => {
      const content = files.get(p);
      if (content === undefined) throw new Error("ENOENT");
      return content;
    }),
    mockWriteFile: vi.fn(async (p: string, content: string) => {
      files.set(p, content);
      if (!mtimes.has(p)) mtimes.set(p, Date.now());
    }),
    mockMkdir: vi.fn(async () => undefined),
    mockReaddir: vi.fn(async () =>
      [...files.keys()].map((p) => p.split("/").pop()!),
    ),
    mockStat: vi.fn(async (p: string) => ({
      mtimeMs: mtimes.get(p) ?? Date.now(),
    })),
    mockUnlink: vi.fn(async (p: string) => {
      files.delete(p);
      mtimes.delete(p);
    }),
  };
});

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
  writeFile: mockWriteFile,
  mkdir: mockMkdir,
  readdir: mockReaddir,
  stat: mockStat,
  unlink: mockUnlink,
}));

import { DiscoverySnapshotStore } from "./discoverySnapshotStore.js";
import type { DiscoverPackage, DiscoverWarning } from "./types.js";

const storage = { fsPath: "/storage" } as import("vscode").Uri;

function pkg(importPath: string): DiscoverPackage {
  return {
    importPath,
    dir: `/ws/${importPath}`,
    modulePath: "example.com/m",
    suites: [],
  } as DiscoverPackage;
}

describe("DiscoverySnapshotStore", () => {
  beforeEach(() => {
    files.clear();
    mtimes.clear();
    vi.clearAllMocks();
  });

  it("round-trips packages and warnings for a workspace", async () => {
    const warnings: DiscoverWarning[] = [
      { importPath: "example.com/m/a", message: "boom" },
    ];
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws", [pkg("example.com/m/a")], warnings);
    store.save();
    await store.flush();

    const reloaded = new DiscoverySnapshotStore(storage);
    await reloaded.load(["/ws"]);
    const snap = reloaded.get("/ws");
    expect(snap?.packages).toEqual([pkg("example.com/m/a")]);
    expect(snap?.warnings).toEqual(warnings);
  });

  // The reason for one file per workspace: updating one tree used to rewrite
  // every tree the extension had ever seen.
  it("writes only the workspace that changed", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws/a", [pkg("example.com/a")], []);
    store.update("/ws/b", [pkg("example.com/b")], []);
    store.save();
    await store.flush();
    expect(files.size).toBe(2);

    mockWriteFile.mockClear();
    store.update("/ws/a", [pkg("example.com/a2")], []);
    store.save();
    await store.flush();

    expect(mockWriteFile).toHaveBeenCalledTimes(1);
  });

  it("keeps workspaces apart, so one corrupt file costs one tree", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws/a", [pkg("example.com/a")], []);
    store.update("/ws/b", [pkg("example.com/b")], []);
    store.save();
    await store.flush();

    const [first] = [...files.keys()];
    files.set(first, "{ not json");

    const reloaded = new DiscoverySnapshotStore(storage);
    await reloaded.load(["/ws/a", "/ws/b"]);
    expect(reloaded.size).toBe(1);
  });

  it("coalesces a burst of saves into one write per workspace", async () => {
    const store = new DiscoverySnapshotStore(storage);
    for (let i = 0; i < 5; i++) {
      store.update("/ws", [pkg(`example.com/m/p${i}`)], []);
      store.save();
    }
    await store.flush();
    expect(mockWriteFile).toHaveBeenCalledTimes(1);
  });

  // activate() registers the file watchers before initializeAsync awaits the
  // load, so a save in that window produces a discovery whose result must not
  // be thrown away by the load that arrives afterwards.
  it("does not let a late load discard what discovery already found", async () => {
    const seed = new DiscoverySnapshotStore(storage);
    seed.update("/ws", [pkg("example.com/m/stale")], []);
    seed.save();
    await seed.flush();

    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws", [pkg("example.com/m/fresh")], []);
    await store.load(["/ws"]);

    expect(store.get("/ws")?.packages).toEqual([pkg("example.com/m/fresh")]);
  });

  it("drops snapshots for workspaces nobody has opened in a month", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws/old", [pkg("example.com/old")], []);
    store.save();
    await store.flush();

    const [stale] = [...files.keys()];
    mtimes.set(stale, Date.now() - 40 * 24 * 60 * 60 * 1000);

    const reopened = new DiscoverySnapshotStore(storage);
    await reopened.load(["/ws/current"]);

    expect(files.has(stale)).toBe(false);
  });

  it("never touches disk without a storage location", async () => {
    const store = new DiscoverySnapshotStore(undefined);
    store.update("/ws", [pkg("example.com/m/a")], []);
    store.save();
    await expect(store.flush()).resolves.toBeUndefined();
    await expect(store.load(["/ws"])).resolves.toBeUndefined();
    expect(mockWriteFile).not.toHaveBeenCalled();
    expect(mockReadFile).not.toHaveBeenCalled();
  });

  it("writes compactly", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws", [pkg("example.com/m/a")], []);
    store.save();
    await store.flush();
    // A large project's snapshot is hundreds of kilobytes; indentation is pure
    // cost on a file only this extension reads.
    expect([...files.values()][0]).not.toContain("\n");
  });

  it("ignores a snapshot written by an older layout", async () => {
    files.set("/storage/discovery.json", JSON.stringify({ version: 1 }));
    const store = new DiscoverySnapshotStore(storage);
    await store.load(["/ws"]);
    expect(store.size).toBe(0);
  });
});
