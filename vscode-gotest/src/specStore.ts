// The Spec View's memory of runs: the newest go test -json lines for every
// test of every package it has seen, kept as the raw stream so the CLI can
// render it. A package is one record no matter how many producers reported
// it, and inside a package every test path keeps the lines of the newest run
// that reported it, so a rerun of one method leaves its siblings as they were.

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
  // Test path to its lines, in the order the paths were first seen.
  tests: Map<string, string[]>;
  // Booked by the CLI under a name that is not a package (a run that was
  // interrupted, shared fixtures that did not come up): it belongs to the
  // folder's newest run, not to a package.
  runLevel: boolean;
}

export interface StoredRecord {
  path: string;
  folder: string;
  pkgLines: string[];
  tests: [string, string[]][];
  runLevel: boolean;
}

interface Parsed {
  Action?: string;
  Package?: string;
  Test?: string;
  ImportPath?: string;
  FailedBuild?: string;
}

interface PieceGroup {
  pkgLines: string[];
  tests: Map<string, string[]>;
  topRuns: Map<string, number>;
  failedBuilds: string[];
}

// isRunLevelPackage reports whether the CLI booked a run's failure under a
// synthetic name: one with a space can never be an import path, and a stopped
// run is booked as "interrupted".
export function isRunLevelPackage(name: string): boolean {
  return /\s/.test(name) || name === "interrupted";
}

export class SpecStore {
  private records = new Map<string, PackageRecord>();

  get size(): number {
    return this.records.size;
  }

  get packages(): string[] {
    return Array.from(this.records.keys());
  }

  clear(): void {
    this.records.clear();
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
          topRuns: new Map(),
          failedBuilds: [],
        };
        groups.set(ev.Package, group);
      }
      if (!ev.Test) {
        group.pkgLines.push(line);
        if (ev.FailedBuild) group.failedBuilds.push(ev.FailedBuild);
        continue;
      }
      let lines = group.tests.get(ev.Test);
      if (!lines) {
        lines = [];
        group.tests.set(ev.Test, lines);
      }
      lines.push(line);
      if (ev.Action === "run" && !ev.Test.includes("/")) {
        group.topRuns.set(ev.Test, (group.topRuns.get(ev.Test) ?? 0) + 1);
      }
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
          runLevel: isRunLevelPackage(path),
        });
        continue;
      }
      this.records.set(path, {
        folder: piece.folder,
        pkgLines,
        tests: mergeTests(kept.tests, group, covered),
        runLevel: kept.runLevel,
      });
    }
    return Array.from(groups.keys());
  }

  // folders lists the folders whose CLI renders something.
  folders(): string[] {
    return Array.from(
      new Set(Array.from(this.records.values(), (r) => r.folder)),
    );
  }

  // compose writes the kept lines of a folder's packages as one stream, each
  // package's tests before its verdict, as go test orders them.
  compose(folder: string): string {
    const out: string[] = [];
    for (const record of this.records.values()) {
      if (record.folder !== folder) continue;
      for (const lines of record.tests.values()) out.push(...lines);
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
    if (!Array.isArray(stored) || !stored.every(isStoredRecord)) return;
    for (const r of stored) {
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
        isStrings(t[1]),
    )
  );
}

// mergeTests keeps every path the newer run neither reported nor asked for,
// in its place, and takes the newer lines for the rest. A suite that ran
// twice in one package (its internal and external test files) is one unit:
// its runs cannot be told apart path by path.
function mergeTests(
  kept: Map<string, string[]>,
  newer: PieceGroup,
  covered: (testPath: string) => boolean,
): Map<string, string[]> {
  const twice = new Set<string>();
  for (const [top, n] of newer.topRuns) if (n > 1) twice.add(top);
  const keptTwice = topsRunTwice(kept);
  for (const top of keptTwice) if (newer.tests.has(top)) twice.add(top);
  const underTwice = (p: string) => twice.has(topOf(p));

  const merged = new Map<string, string[]>();
  for (const [path, lines] of kept) {
    const fresh = newer.tests.get(path);
    if (fresh && !underTwice(path)) {
      merged.set(path, fresh);
      continue;
    }
    if (underTwice(path) || covered(path)) continue;
    merged.set(path, lines);
  }
  for (const [path, lines] of newer.tests) {
    if (!merged.has(path)) merged.set(path, lines);
  }
  return merged;
}

function topOf(path: string): string {
  const i = path.indexOf("/");
  return i < 0 ? path : path.slice(0, i);
}

function topsRunTwice(tests: Map<string, string[]>): Set<string> {
  const twice = new Set<string>();
  for (const [path, lines] of tests) {
    if (path.includes("/")) continue;
    let runs = 0;
    for (const line of lines) if (line.includes('"Action":"run"')) runs++;
    if (runs > 1) twice.add(path);
  }
  return twice;
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
