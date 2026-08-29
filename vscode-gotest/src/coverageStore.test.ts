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
      // The shared store writes to a sibling temp file and renames it into place.
      mockRename: vi.fn(async (from: string, to: string) => {
        const content = files.get(from);
        if (content !== undefined) {
          files.set(to, content);
          files.delete(from);
        }
      }),
    };
  });

vi.mock("node:fs/promises", () => ({
  readFile: mockReadFile,
  writeFile: mockWriteFile,
  mkdir: mockMkdir,
  rename: mockRename,
}));

vi.mock("vscode", () => ({
  Uri: { file: (p: string) => ({ fsPath: p }) },
  Position: class {},
  Range: class {},
  StatementCoverage: class {},
  DeclarationCoverage: class {},
  FileCoverage: class {},
}));

import { CoverageStore } from "./coverageStore.js";

const storage = { fsPath: "/storage" } as import("vscode").Uri;

describe("CoverageStore load ordering", () => {
  beforeEach(() => {
    files.clear();
    vi.clearAllMocks();
  });

  it("round-trips a stored profile", async () => {
    const store = new CoverageStore(storage);
    store.update("example.com/m/a", "mode: set\n");
    store.save();
    await store.flush();

    const reopened = new CoverageStore(storage);
    await reopened.load();
    expect(reopened.size).toBe(1);
  });

  // The file watchers are registered synchronously in activate(), while
  // initializeAsync awaits load(). A .go file touched in that window
  // invalidated a package in memory, and the load that followed read the
  // pre-invalidation state back — resurrecting coverage for a package whose
  // source had already changed.
  it("does not let a late load undo an invalidation", async () => {
    const seed = new CoverageStore(storage);
    seed.update("example.com/m/a", "mode: set\n");
    seed.save();
    await seed.flush();

    const store = new CoverageStore(storage);
    expect(store.invalidate("example.com/m/a")).toBe(false);
    store.update("example.com/m/a", "mode: set\n");
    expect(store.invalidate("example.com/m/a")).toBe(true);

    await store.load();

    expect(store.size).toBe(0);
  });

  it("still loads when nothing has been recorded yet", async () => {
    const seed = new CoverageStore(storage);
    seed.update("example.com/m/a", "mode: set\n");
    seed.save();
    await seed.flush();

    const store = new CoverageStore(storage);
    await store.load();
    expect(store.size).toBe(1);
  });
});
