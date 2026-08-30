import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockReadFile, mockWriteFile, mockMkdir, mockRename, files } =
  vi.hoisted(() => {
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
    };
  });

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
  writeFile: mockWriteFile,
  mkdir: mockMkdir,
  rename: mockRename,
}));

import * as path from "node:path";
import { JsonStore } from "./jsonStore.js";

const DIR = path.join(path.sep, "storage");
const FILE = path.join(DIR, "thing.json");

describe("JsonStore", () => {
  beforeEach(() => {
    files.clear();
    vi.clearAllMocks();
  });

  it("round-trips through a versioned envelope", async () => {
    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    store.save(() => ({ a: 1 }));
    await store.flush();

    const reopened = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    expect(await reopened.read()).toEqual({ a: 1 });
  });

  // The rule the three ad-hoc guards were each reinventing.
  it("refuses to read once memory is ahead of disk", async () => {
    const seed = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    seed.save(() => ({ a: 1 }));
    await seed.flush();

    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    store.markMutated();
    expect(await store.read()).toBeUndefined();
  });

  it("drops a file written by another version", async () => {
    const seed = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    seed.save(() => ({ a: 1 }));
    await seed.flush();

    const newer = new JsonStore<{ a: number }>(DIR, "thing.json", 2);
    expect(await newer.read()).toBeUndefined();
  });

  it("returns undefined rather than throwing on corrupt data", async () => {
    files.set(FILE, "{ not json");
    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    expect(await store.read()).toBeUndefined();
  });

  // A crash between the write and the rename leaves the previous file intact,
  // rather than a truncated one.
  it("writes to a temp file and renames it into place", async () => {
    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    store.save(() => ({ a: 1 }));
    await store.flush();

    const [tmpPath] = mockWriteFile.mock.calls[0];
    // Built from FILE, not a literal: the separator is the platform's.
    expect(tmpPath).toMatch(
      new RegExp(
        `^${FILE.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\.\\d+\\.\\d+\\.tmp$`,
      ),
    );
    expect(mockRename).toHaveBeenCalledWith(tmpPath, FILE);
    expect(files.has(tmpPath)).toBe(false);
  });

  // Two VS Code windows on one folder share workspace storage. A fixed temp
  // name lets them interleave writes and rename a mixture into place.
  it("gives each writer its own temp file", async () => {
    const a = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    const b = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    a.save(() => ({ a: 1 }));
    b.save(() => ({ a: 2 }));
    await Promise.all([a.flush(), b.flush()]);

    const temps = mockWriteFile.mock.calls.map((c) => c[0]);
    expect(new Set(temps).size).toBe(temps.length);
  });

  it("coalesces a burst into one write", async () => {
    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    for (let i = 0; i < 5; i++) store.save(() => ({ a: i }));
    await store.flush();
    expect(mockRename).toHaveBeenCalledTimes(1);
  });

  it("serialises the state at write time, not at save time", async () => {
    let value = 1;
    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    store.save(() => ({ a: value }));
    value = 2;
    await store.flush();

    const reopened = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    expect(await reopened.read()).toEqual({ a: 2 });
  });

  it("never touches disk without a storage location", async () => {
    const store = new JsonStore<{ a: number }>(undefined, "thing.json", 1);
    store.save(() => ({ a: 1 }));
    await expect(store.flush()).resolves.toBeUndefined();
    expect(await store.read()).toBeUndefined();
    expect(mockWriteFile).not.toHaveBeenCalled();
  });

  it("keeps working after a failed write", async () => {
    const store = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    mockRename.mockRejectedValueOnce(new Error("disk full"));
    store.save(() => ({ a: 1 }));
    await expect(store.flush()).resolves.toBeUndefined();

    store.save(() => ({ a: 2 }));
    await store.flush();
    const reopened = new JsonStore<{ a: number }>(DIR, "thing.json", 1);
    expect(await reopened.read()).toEqual({ a: 2 });
  });
});
