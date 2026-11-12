// The Spec View's memory of runs: the newest go test -json lines for every
// test of every package it has seen, kept as the raw stream so the CLI can
// render it. A package is one record no matter how many producers reported
// it, and inside a package every test path of every suite instance keeps the
// lines of the newest run that reported it, so a rerun of one method leaves
// its siblings, and its twin's, as they were.

export interface RunPiece {
  // The -json stdout of one go test invocation.
  stream: string;
  // The workspace folder the run belongs to; its CLI renders the packages.
  folder: string;
  // Packages run with a -run filter, and the test paths the filter asked
  // for. A package the stream names that is absent here ran whole, and its
  // newest stream replaces what was kept for it.
  partial?: Map<string, (testPath: string) => boolean>;
}

export interface PackageRecord {
  folder: string;
  // Lines without a test: the package's start, output and verdict, and the
  // build lines its verdict names.
  pkgLines: string[];
  // testKey(instance, test path) to its lines.
  tests: Map<string, KeptLine[]>;
  // Booked by the CLI for the run itself rather than for an import path (a
  // run that was interrupted, shared fixtures that did not come up): its
  // events carry RunLevel. It belongs to the folder's newest run, not to a
  // package.
  runLevel: boolean;
}

// A kept line and its place in the order lines were merged, across runs.
export interface KeptLine {
  seq: number;
  line: string;
}

export interface StoredRecord {
  path: string;
  folder: string;
  pkgLines: string[];
  tests: [string, KeptLine[]][];
  runLevel: boolean;
}

// testKey names a test path within one instance of its top-level name: a
// suite in both test packages runs twice under one name.
export function testKey(instance: number, test: string): string {
  return `${instance}\u0000${test}`;
}

const testOf = (key: string) => key.slice(key.indexOf("\u0000") + 1);

interface Parsed {
  Action?: string;
  Package?: string;
  Test?: string;
  ImportPath?: string;
  FailedBuild?: string;
  RunLevel?: boolean;
  Gotest?: { Instance?: number };
}

interface PieceGroup {
  pkgLines: string[];
  tests: Map<string, KeptLine[]>;
  failedBuilds: string[];
  // Whether any of its lines carried the CLI's RunLevel marker.
  runLevel: boolean;
}

export class SpecStore {
  private records = new Map<string, PackageRecord>();
  // Advanced by every merged line, so lines of later runs sort after.
  private seq = 0;

  get size(): number {
    return this.records.size;
  }

  get packages(): string[] {
    return Array.from(this.records.keys());
  }

  clear(): void {
    this.records.clear();
    this.seq = 0;
  }

  // merge folds a run into what is kept. It reports the packages it touched.
  merge(piece: RunPiece): string[] {
    const builds = new Map<string, string[]>();
    const groups = new Map<string, PieceGroup>();

    for (const raw of piece.stream.split("\n")) {
      const line = raw.trim();
      if (!line) continue;
      let ev: Parsed;
      try {
        ev = JSON.parse(line) as Parsed;
      } catch {
        continue;
      }
      if (!ev || typeof ev.Action !== "string") continue;
      // gotest watch's cycle markers are not test events.
      if (ev.Action.startsWith("watch-")) continue;
      if (ev.Action === "build-output" || ev.Action === "build-fail") {
        const key = ev.ImportPath ?? "";
        let lines = builds.get(key);
        if (!lines) {
          lines = [];
          builds.set(key, lines);
        }
        lines.push(line);
        continue;
      }
      if (typeof ev.Package !== "string") continue;
      let group = groups.get(ev.Package);
      if (!group) {
        group = {
          pkgLines: [],
          tests: new Map(),
          failedBuilds: [],
          runLevel: false,
        };
        groups.set(ev.Package, group);
      }
      if (ev.RunLevel === true) group.runLevel = true;
      if (!ev.Test) {
        group.pkgLines.push(line);
        if (ev.FailedBuild) group.failedBuilds.push(ev.FailedBuild);
        continue;
      }
      // An event without the stamp's Instance is of the first instance.
      const instance = ev.Gotest?.Instance;
      const key = testKey(
        typeof instance === "number" && instance >= 2 ? instance : 1,
        ev.Test,
      );
      let lines = group.tests.get(key);
      if (!lines) {
        lines = [];
        group.tests.set(key, lines);
      }
      lines.push({ seq: this.seq++, line });
    }

    // The newest run of a folder speaks for its run-level bookings: one it
    // no longer reports is over.
    const reported = new Set(groups.keys());
    for (const [path, record] of this.records) {
      if (
        record.runLevel &&
        record.folder === piece.folder &&
        !reported.has(path)
      ) {
        this.records.delete(path);
      }
    }

    for (const [path, group] of groups) {
      const buildLines = group.failedBuilds.flatMap((b) => builds.get(b) ?? []);
      const pkgLines = [...buildLines, ...group.pkgLines];
      const covered = piece.partial?.get(path);
      const kept = this.records.get(path);
      if (!kept || !covered) {
        this.records.set(path, {
          folder: piece.folder,
          pkgLines,
          tests: group.tests,
          runLevel: group.runLevel,
        });
        continue;
      }
      this.records.set(path, {
        folder: piece.folder,
        pkgLines,
        tests: mergeTests(kept.tests, group, covered),
        runLevel: group.runLevel,
      });
    }
    return Array.from(groups.keys());
  }

  // folderOf is the folder whose CLI renders a package.
  folderOf(pkg: string): string | undefined {
    return this.records.get(pkg)?.folder;
  }

  // folders lists the folders whose CLI renders something.
  folders(): string[] {
    return Array.from(
      new Set(Array.from(this.records.values(), (r) => r.folder)),
    );
  }

  // compose writes the kept lines of a folder's packages as one stream: a
  // package's test lines in the order they were merged, then its own lines.
  // A single run's test lines thus reach the CLI as written (its package
  // lines go last); lines of several runs are stamped, and the CLI places
  // those by their stamps.
  compose(folder: string): string {
    const out: string[] = [];
    for (const record of this.records.values()) {
      if (record.folder !== folder) continue;
      const kept = Array.from(record.tests.values()).flat();
      kept.sort((a, b) => a.seq - b.seq);
      for (const k of kept) out.push(k.line);
      out.push(...record.pkgLines);
    }
    return out.length > 0 ? out.join("\n") + "\n" : "";
  }

  serialize(): StoredRecord[] {
    return Array.from(this.records, ([path, r]) => ({
      path,
      folder: r.folder,
      pkgLines: r.pkgLines,
      tests: Array.from(r.tests),
      runLevel: r.runLevel,
    }));
  }

  // restore takes what serialize wrote. A file of any other shape — written
  // by another build, or damaged — restores nothing.
  restore(stored: unknown): void {
    this.records.clear();
    this.seq = 0;
    if (!Array.isArray(stored) || !stored.every(isStoredRecord)) return;
    for (const r of stored) {
      for (const [, lines] of r.tests) {
        for (const k of lines) this.seq = Math.max(this.seq, k.seq + 1);
      }
      this.records.set(r.path, {
        folder: r.folder,
        pkgLines: r.pkgLines,
        tests: new Map(r.tests),
        runLevel: r.runLevel,
      });
    }
  }

  // prune drops a folder's packages that its discovery no longer reports:
  // renamed or deleted. Run-level bookings are not packages and stay.
  prune(folder: string, discovered: Set<string>): string[] {
    const removed: string[] = [];
    for (const [path, record] of this.records) {
      if (record.folder !== folder || record.runLevel) continue;
      if (discovered.has(path)) continue;
      this.records.delete(path);
      removed.push(path);
    }
    return removed;
  }
}

const isStrings = (v: unknown): v is string[] =>
  Array.isArray(v) && v.every((s) => typeof s === "string");

const isKeptLines = (v: unknown): v is KeptLine[] =>
  Array.isArray(v) &&
  v.every(
    (k) =>
      !!k &&
      typeof k === "object" &&
      typeof (k as KeptLine).seq === "number" &&
      typeof (k as KeptLine).line === "string",
  );

function isStoredRecord(v: unknown): v is StoredRecord {
  if (!v || typeof v !== "object") return false;
  const r = v as Record<string, unknown>;
  return (
    typeof r.path === "string" &&
    typeof r.folder === "string" &&
    typeof r.runLevel === "boolean" &&
    isStrings(r.pkgLines) &&
    Array.isArray(r.tests) &&
    r.tests.every(
      (t) =>
        Array.isArray(t) &&
        t.length === 2 &&
        typeof t[0] === "string" &&
        isKeptLines(t[1]),
    )
  );
}

// mergeTests keeps every test path of every instance the newer run neither
// reported nor asked for, and takes the newer lines for the rest.
function mergeTests(
  kept: Map<string, KeptLine[]>,
  newer: PieceGroup,
  covered: (testPath: string) => boolean,
): Map<string, KeptLine[]> {
  const merged = new Map<string, KeptLine[]>();
  for (const [key, lines] of kept) {
    const fresh = newer.tests.get(key);
    if (fresh) merged.set(key, fresh);
    else if (!covered(testOf(key))) merged.set(key, lines);
  }
  for (const [key, lines] of newer.tests) {
    if (!merged.has(key)) merged.set(key, lines);
  }
  return merged;
}

// coverOf answers whether a -run filter built from these test paths asked
// for testPath: the path itself and everything under it. A suite's path
// also covers the fuzz wrappers the filter widens it to.
export function coverOf(paths: string[]): (testPath: string) => boolean {
  return (testPath: string) =>
    paths.some((p) => {
      if (testPath === p || testPath.startsWith(p + "/")) return true;
      if (p.startsWith("Test") && !p.includes("/")) {
        return testPath.startsWith(`Fuzz${p.slice(4)}_`);
      }
      return false;
    });
}
