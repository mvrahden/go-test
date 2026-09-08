import * as path from "node:path";
import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockAccess, mockExecFileAsync } = vi.hoisted(() => ({
  mockAccess: vi.fn(async (_p: unknown, _mode?: unknown): Promise<void> => {
    throw new Error("ENOENT");
  }),
  mockExecFileAsync: vi.fn(async (): Promise<{ stdout: string }> => {
    throw new Error("not found");
  }),
}));

vi.mock("vscode", () => ({}));

vi.mock("node:fs/promises", () => ({
  access: mockAccess,
  readFile: vi.fn(async () => "module github.com/myapp\n\ngo 1.24.0\n"),
  readdir: vi.fn(async () => []),
  constants: { F_OK: 0, X_OK: 1 },
}));

vi.mock("node:child_process", async () => {
  const util = await import("node:util");
  const execFileFn: Record<symbol, unknown> = vi.fn() as never;
  execFileFn[util.promisify.custom] = mockExecFileAsync;
  return { execFile: execFileFn };
});

import { resolveGoBinary, clearGoBinaryCache } from "./goBinary.js";

describe("resolveGoBinary", () => {
  const goExe = process.platform === "win32" ? "go.exe" : "go";
  const gorootGo = path.join("/goroot", "bin", goExe);

  beforeEach(() => {
    clearGoBinaryCache();
    vi.stubEnv("HOME", "/home/dev");
    vi.stubEnv("USERPROFILE", "/home/dev");
    vi.stubEnv("GOROOT", "/goroot");
  });

  it("does not derive a toolchain from the go directive", async () => {
    // A go directive is a minimum, not a choice: GOTOOLCHAIN satisfies it
    // from the default go. ~/sdk/go1.24.0 exists here and must be ignored.
    const sdkGo = path.join("/home/dev", "sdk", "go1.24.0", "bin", goExe);
    mockAccess.mockImplementation(async (p: unknown) => {
      if (p === sdkGo || p === gorootGo) return;
      throw new Error("ENOENT");
    });

    const go = await resolveGoBinary(undefined, "/workspace");

    expect(go).toBe(gorootGo);
  });
});
