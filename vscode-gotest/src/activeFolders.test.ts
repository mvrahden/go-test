import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

const { execFileMock } = vi.hoisted(() => ({ execFileMock: vi.fn() }));

vi.mock("vscode", () => ({
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
}));

vi.mock("node:child_process", async (importOriginal) => ({
  ...(await importOriginal<typeof import("node:child_process")>()),
  execFile: execFileMock,
}));

import { ActiveFolders, moduleFileChanged } from "./activeFolders.js";

const PLAIN = "module example.com/app\n\ngo 1.26.0\n";
const REQUIRING =
  "module example.com/app\n\ngo 1.26.0\n\nrequire github.com/mvrahden/go-test v1.38.0\n";

describe("ActiveFolders", () => {
  let dir = "";

  beforeEach(() => {
    dir = mkdtempSync(path.join(os.tmpdir(), "gotest-active-"));
    execFileMock.mockClear();
  });
  afterEach(() => {
    if (dir) rmSync(dir, { recursive: true, force: true });
  });

  it("follows a folder's go.mod as it starts and stops requiring gotest", async () => {
    const active = new ActiveFolders();
    writeFileSync(path.join(dir, "go.mod"), PLAIN);
    expect(await active.refresh(dir)).toBe("unchanged");
    expect(active.has(dir)).toBe(false);

    writeFileSync(path.join(dir, "go.mod"), REQUIRING);
    expect(await active.refresh(dir)).toBe("activated");
    expect(active.has(dir)).toBe(true);
    expect(active.size).toBe(1);
    expect(await active.refresh(dir)).toBe("unchanged");

    writeFileSync(path.join(dir, "go.mod"), PLAIN);
    expect(await active.refresh(dir)).toBe("deactivated");
    expect(active.has(dir)).toBe(false);
    expect(active.size).toBe(0);
  });

  it("reads a go.work's use modules instead of the folder's go.mod", async () => {
    const active = new ActiveFolders();
    writeFileSync(path.join(dir, "go.mod"), PLAIN);
    for (const sub of ["a", "b"]) mkdirSync(path.join(dir, sub));
    writeFileSync(path.join(dir, "a", "go.mod"), PLAIN);
    writeFileSync(path.join(dir, "b", "go.mod"), REQUIRING);
    writeFileSync(
      path.join(dir, "go.work"),
      "go 1.26.0\n\nuse (\n\t./a\n\t./b\n)\n",
    );
    expect(await active.refresh(dir)).toBe("activated");
  });

  it("does not count a go.mod its go.work leaves out", async () => {
    const active = new ActiveFolders();
    writeFileSync(path.join(dir, "go.mod"), REQUIRING);
    mkdirSync(path.join(dir, "a"));
    writeFileSync(path.join(dir, "a", "go.mod"), PLAIN);
    writeFileSync(path.join(dir, "go.work"), "go 1.26.0\n\nuse ./a\n");
    expect(await active.refresh(dir)).toBe("unchanged");
    expect(active.has(dir)).toBe(false);
  });

  it("only decides: refreshing writes no git setting", async () => {
    const active = new ActiveFolders();
    writeFileSync(path.join(dir, "go.mod"), REQUIRING);
    mkdirSync(path.join(dir, ".git"));
    await active.refresh(dir);
    expect(execFileMock).not.toHaveBeenCalled();
    expect(existsSync(path.join(dir, ".git", "info", "exclude"))).toBe(false);
  });

  it("activates a folder whose go.mod starts to require gotest, then discovers it", async () => {
    const calls: string[] = [];
    const deps = {
      active: new ActiveFolders(),
      discover: async (f: string) =>
        void calls.push(`discover ${path.basename(f)}`),
      resetCli: () => void calls.push("reset"),
      setActiveContext: (on: boolean) => void calls.push(`context ${on}`),
      writeGitSettings: (f: string) =>
        void calls.push(`git ${path.basename(f)}`),
    };
    const name = path.basename(dir);
    writeFileSync(path.join(dir, "go.mod"), PLAIN);
    await moduleFileChanged(dir, deps);
    expect(calls).toEqual(["reset", "context false", `discover ${name}`]);

    calls.length = 0;
    writeFileSync(path.join(dir, "go.mod"), REQUIRING);
    await moduleFileChanged(dir, deps);
    expect(calls).toEqual([
      "reset",
      `git ${name}`,
      "context true",
      `discover ${name}`,
    ]);

    calls.length = 0;
    await moduleFileChanged(dir, deps);
    expect(calls).toEqual(["reset", "context true", `discover ${name}`]);
  });
});
