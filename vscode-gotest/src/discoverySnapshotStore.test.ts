import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockReadFile, mockWriteFile, mockMkdir, files } = vi.hoisted(() => {
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
  };
});

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
  writeFile: mockWriteFile,
  mkdir: mockMkdir,
}));

import { DiscoverySnapshotStore } from "./discoverySnapshotStore.js";
import type { DiscoverPackage, DiscoverWarning } from "./types.js";

const storage = { fsPath: "/storage" } as import("vscode").Uri;
const SNAPSHOT = "/storage/discovery.json";

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
    await reloaded.load();
    const snap = reloaded.get("/ws");
    expect(snap?.packages).toEqual([pkg("example.com/m/a")]);
    expect(snap?.warnings).toEqual(warnings);
  });

  it("keeps workspaces apart", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws1", [pkg("example.com/m/a")], []);
    store.update("/ws2", [pkg("example.com/m/b")], []);
    store.save();
    await store.flush();

    const reloaded = new DiscoverySnapshotStore(storage);
    await reloaded.load();
    expect(reloaded.get("/ws1")?.packages).toEqual([pkg("example.com/m/a")]);
    expect(reloaded.get("/ws2")?.packages).toEqual([pkg("example.com/m/b")]);
    expect(reloaded.get("/ws3")).toBeUndefined();
  });

  it("replaces a workspace's snapshot rather than merging it", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws", [pkg("example.com/m/a"), pkg("example.com/m/b")], []);
    store.update("/ws", [pkg("example.com/m/a")], []);
    store.save();
    await store.flush();

    const reloaded = new DiscoverySnapshotStore(storage);
    await reloaded.load();
    // A package deleted from the workspace must not survive in the snapshot,
    // or the tree would show it again on every activation.
    expect(reloaded.get("/ws")?.packages).toEqual([pkg("example.com/m/a")]);
  });

  it("ignores a snapshot written by a different schema version", async () => {
    files.set(SNAPSHOT, JSON.stringify({ version: 99, workspaces: {} }));
    const store = new DiscoverySnapshotStore(storage);
    await store.load();
    expect(store.get("/ws")).toBeUndefined();
  });

  it("starts fresh on a corrupt snapshot instead of throwing", async () => {
    files.set(SNAPSHOT, "{not json");
    const store = new DiscoverySnapshotStore(storage);
    await expect(store.load()).resolves.toBeUndefined();
    expect(store.get("/ws")).toBeUndefined();
  });

  it("starts fresh when nothing has been stored yet", async () => {
    const store = new DiscoverySnapshotStore(storage);
    await store.load();
    expect(store.get("/ws")).toBeUndefined();
  });

  it("never touches disk without a storage location", async () => {
    const store = new DiscoverySnapshotStore(undefined);
    store.update("/ws", [pkg("example.com/m/a")], []);
    store.save();
    await expect(store.flush()).resolves.toBeUndefined();
    await expect(store.load()).resolves.toBeUndefined();
    expect(mockWriteFile).not.toHaveBeenCalled();
    expect(mockReadFile).not.toHaveBeenCalled();
  });

  it("writes compactly", async () => {
    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws", [pkg("example.com/m/a")], []);
    store.save();
    await store.flush();
    // A large project's snapshot is megabytes; indentation is pure cost on a
    // file written after every discovery.
    expect(files.get(SNAPSHOT)).not.toContain("\n");
  });
});

describe("DiscoverySnapshotStore write behaviour", () => {
  beforeEach(() => {
    files.clear();
    vi.clearAllMocks();
  });

  // Every save of a _test.go file rediscovers its package, and each one used to
  // rewrite the whole workspace tree synchronously.
  it("coalesces a burst of saves into one write", async () => {
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
    files.set(
      SNAPSHOT,
      JSON.stringify({
        version: 1,
        workspaces: {
          "/ws": { packages: [pkg("example.com/m/stale")], warnings: [] },
        },
      }),
    );

    const store = new DiscoverySnapshotStore(storage);
    store.update("/ws", [pkg("example.com/m/fresh")], []);
    await store.load();

    expect(store.get("/ws")?.packages).toEqual([pkg("example.com/m/fresh")]);
  });
});
