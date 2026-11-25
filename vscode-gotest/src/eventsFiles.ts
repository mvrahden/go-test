// Where the extension's own runs save their -json stream (--events-file),
// so the CLI's run record names a file the Runs scope can open and the
// Change scope can take as evidence. The files live under the extension's
// storage and are pruned to the bounds the CLI keeps its records in.
import * as path from "node:path";
import { mkdirSync, readdirSync, rmSync, statSync } from "node:fs";
import { randomBytes } from "node:crypto";

// The CLI's history bounds: the newest 50 records, none older than 30 days.
export const KEEP_FILES = 50;
export const KEEP_MS = 30 * 24 * 60 * 60 * 1000;

let eventsDir: string | undefined;

// setEventsDir names the directory runs write their streams into; undefined
// writes none.
export function setEventsDir(dir: string | undefined): void {
  eventsDir = dir;
}

// nextEventsFile is a fresh file for one run's stream, or undefined when
// there is no directory to put it in.
export function nextEventsFile(now = new Date()): string | undefined {
  if (!eventsDir) return undefined;
  try {
    mkdirSync(eventsDir, { recursive: true });
  } catch {
    return undefined;
  }
  const stamp = now.toISOString().replace(/[:.]/g, "-");
  return path.join(
    eventsDir,
    `${stamp}-${randomBytes(4).toString("hex")}.jsonl`,
  );
}

// pruneEventsFiles removes all but the newest KEEP_FILES streams and those
// older than KEEP_MS; their names sort by when they were made.
export function pruneEventsFiles(now = Date.now()): void {
  if (!eventsDir) return;
  let names: string[];
  try {
    names = readdirSync(eventsDir)
      .filter((n) => n.endsWith(".jsonl"))
      .sort();
  } catch {
    return;
  }
  names.forEach((name, i) => {
    const file = path.join(eventsDir!, name);
    let old = false;
    try {
      old = now - statSync(file).mtimeMs > KEEP_MS;
    } catch {
      return;
    }
    if (i < names.length - KEEP_FILES || old) {
      try {
        rmSync(file, { force: true });
      } catch {
        // gone already, or in use: the next prune tries again
      }
    }
  });
}
