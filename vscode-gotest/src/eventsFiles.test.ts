import { describe, it, expect, afterEach } from "vitest";
import {
  mkdtempSync,
  readdirSync,
  rmSync,
  utimesSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import {
  KEEP_FILES,
  KEEP_MS,
  nextEventsFile,
  pruneEventsFiles,
  setEventsDir,
} from "./eventsFiles.js";

let dir = "";
afterEach(() => {
  setEventsDir(undefined);
  if (dir) rmSync(dir, { recursive: true, force: true });
});

describe("events files", () => {
  it("names a fresh file per run, sorted by when it was made", () => {
    dir = mkdtempSync(path.join(tmpdir(), "gotest-ev-"));
    setEventsDir(path.join(dir, "events"));
    const a = nextEventsFile(new Date("2026-01-01T00:00:00Z"))!;
    const b = nextEventsFile(new Date("2026-01-02T00:00:00Z"))!;
    expect(path.dirname(a)).toBe(path.join(dir, "events"));
    expect(a < b).toBe(true);
    expect(nextEventsFile()).not.toBe(nextEventsFile());
  });

  it("names none without a directory", () => {
    expect(nextEventsFile()).toBeUndefined();
  });

  it("keeps the newest files and none past the age the CLI keeps records", () => {
    dir = mkdtempSync(path.join(tmpdir(), "gotest-ev-"));
    setEventsDir(dir);
    const now = Date.now();
    const names: string[] = [];
    for (let i = 0; i < KEEP_FILES + 3; i++) {
      const name = `2026-${String(i).padStart(4, "0")}.jsonl`;
      writeFileSync(path.join(dir, name), "");
      names.push(name);
    }
    // One of the kept is too old.
    const old = path.join(dir, names[KEEP_FILES + 2]);
    const then = (now - KEEP_MS - 1000) / 1000;
    utimesSync(old, then, then);
    writeFileSync(path.join(dir, "other.txt"), "");
    pruneEventsFiles(now);
    const left = readdirSync(dir).sort();
    expect(left).toEqual(
      [...names.slice(3, KEEP_FILES + 2), "other.txt"].sort(),
    );
  });
});
