// The document `gotest spec --format=json` prints, and the pure reading of
// it the panel needs: counts, item ids, sources, the run it renders. The CLI
// judges; this only names and words what the document says.
import * as path from "node:path";
import { existsSync } from "node:fs";
import { stripGoRunExitEcho } from "../cli.js";
import {
  EXTERNAL_ID,
  externalInstance,
  externalTwinNode,
  methodItemId,
  type DiscoverPackage,
} from "../types.js";
import type { LineSegment } from "./protocol.js";

// encodeStateForScript serialises the spec for embedding inside a <script>
// block. JSON.stringify leaves "<" alone, so a test whose output contains
// "</script>" would close the block early and spill the rest of the document
// into the page as live markup. Escaping the angle brackets as unicode
// escapes leaves the parsed value identical while making that impossible.
export function encodeStateForScript(data: unknown): string {
  return JSON.stringify(data)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026");
}

export type SpecExitOutcome =
  { ok: true; stdout: string } | { ok: false; message: string };

// interpretSpecExit decides whether a finished `gotest spec --input=-` left a
// usable spec behind. Exit 1 means the piped stream carried failures — a
// verdict about the tests, not a rendering error — so the spec on stdout still
// stands. Only an exit that produced no spec is an error worth reporting.
export function interpretSpecExit(
  code: number | null,
  stdout: string,
  stderr: string,
): SpecExitOutcome {
  if (code === 0 || (code === 1 && stdout.trim() !== "")) {
    return { ok: true, stdout };
  }
  const detail = stripGoRunExitEcho(stderr);
  const suffix = detail ? `: ${detail}` : "";
  return {
    ok: false,
    message: `gotest spec exited with code ${code}${suffix}`,
  };
}

// combineSpecData joins the documents of several folders' CLIs. Each package
// is rendered by exactly one of them and the CLI's counts are sums over
// packages, so the joined counts are sums too.
export function combineSpecData(docs: SpecData[]): SpecData {
  if (docs.length === 1) return docs[0];
  const stats: SpecStats = {
    suites: 0,
    behaviors: 0,
    tests: 0,
    fuzzers: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    failedPackages: 0,
    notRun: 0,
    vacuous: 0,
    assertsBy: 0,
    excluded: 0,
  };
  const packages: SpecPackage[] = [];
  const groups: SpecGroup[] = [];
  const runs: SpecRun[] = [];
  for (const doc of docs) {
    packages.push(...doc.packages);
    groups.push(...(doc.groups ?? []));
    if (doc.run) runs.push(doc.run);
    for (const key of Object.keys(stats) as (keyof SpecStats)[]) {
      stats[key] = (stats[key] ?? 0) + (doc.stats[key] ?? 0);
    }
  }
  packages.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  return {
    version: docs[0]?.version,
    packages,
    stats,
    ...(groups.length > 0 ? { groups } : {}),
    ...(runs.length > 0 ? { runs } : {}),
  };
}

// --- Types ---

export interface SpecData {
  /** The CLI that produced the document; absent before v1.30.2. */
  version?: string;
  // The run the document renders, as its stream recorded it.
  run?: SpecRun;
  packages: SpecPackage[];
  stats: SpecStats;
  // What each named ExclusiveGroup took.
  groups?: SpecGroup[];
  // The runs of the documents combined into this one, one per folder that
  // had one; set by combineSpecData, not by the CLI.
  runs?: SpecRun[];
}

export interface SpecRun {
  version?: string;
  goVersion?: string;
  args?: string[];
  commit?: string;
  dirty?: boolean;
  agent?: string;
  exitCode?: number;
  // A --changed run: how many packages it ran and left out unchanged.
  changed?: { ran: number; unchanged: number; noRecord?: boolean };
}

export interface SpecGroup {
  name: string;
  suites: number;
  // Seconds from the group's first suite start to its last end.
  wall: number;
}

export interface SpecPackage {
  path: string;
  // The package's directory, when the CLI read its source (--declared).
  dir?: string;
  status: string;
  duration: number;
  nodes: SpecNode[];
  output?: string[];
  // The package's own counts; given with --hide or --declared.
  stats?: SpecStats;
  // A package the run booked for itself (shared fixtures, an interrupt).
  runLevel?: boolean;
  // The hash of the inputs the run read; absent from an older CLI.
  inputHash?: string;
}

export interface SpecNode {
  name: string;
  display: string;
  kind: string;
  // "none" for a declared node the run did not report.
  status: string;
  duration: number;
  focused: boolean;
  excluded: boolean;
  external: boolean;
  // The instance of its top-level name in the run, from 2 on.
  variant?: number;
  // Which run of a repeated suite this is (-count); absent when it ran once
  // and from an older CLI's document.
  run?: number;
  runs?: number;
  // The own level go test reported, when it is not name: a twin's
  // TestCreate#01.
  reported?: string;
  // The children are a floor: more behaviors are known only at run time.
  incomplete?: boolean;
  skipReason?: string;
  // Assertions a leaf made; absent when unknown, which is not zero.
  assertions?: number;
  // Passed without an assertion and declared no other means.
  vacuous?: boolean;
  // The means of asserting a leaf declared instead.
  assertsBy?: string;
  // Where the source declares the node, relative to its package's dir.
  file?: string;
  line?: number;
  output: string[];
  children: SpecNode[];
}

export interface SpecStats {
  suites: number;
  behaviors: number;
  tests: number;
  // Fuzz targets: each is one property under its suite, counted once.
  fuzzers?: number;
  passed: number;
  failed: number;
  skipped: number;
  // Packages whose verdict sits on the package itself — a build failure, a
  // death outside any test. They carry no failing behavior, so they are
  // counted separately from `failed`.
  failedPackages?: number;
  // Declared behaviors the run did not report (--declared).
  notRun?: number;
  vacuous?: number;
  assertsBy?: number;
  excluded?: number;
  benchmarks?: number;
}

// formatRanAt is a run's local time: HH:MM on the day of now, with the date
// on any other day.
export function formatRanAt(at: number, now: number): string {
  const d = new Date(at);
  const n = new Date(now);
  const two = (v: number) => String(v).padStart(2, "0");
  const time = `${two(d.getHours())}:${two(d.getMinutes())}`;
  const sameDay =
    d.getFullYear() === n.getFullYear() &&
    d.getMonth() === n.getMonth() &&
    d.getDate() === n.getDate();
  return sameDay
    ? time
    : `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${time}`;
}

// provenanceText says which run a document renders, from its run header:
// the commit and whether the tree was dirty, the agent, the exit code.
export function provenanceText(run: SpecRun): string {
  const parts: string[] = [];
  if (run.commit) {
    parts.push(run.dirty ? `${run.commit}, uncommitted changes` : run.commit);
  }
  if (run.agent) parts.push(`agent ${run.agent}`);
  if (run.exitCode !== undefined) parts.push(`exit ${run.exitCode}`);
  else parts.push("run did not finish");
  const c = run.changed;
  if (c)
    parts.push(
      c.noRecord
        ? "--changed found no record and ran everything"
        : `narrowed by --changed: ran ${countNoun(c.ran, "package")}, left out ${c.unchanged} unchanged`,
    );
  if (run.version) parts.push(`gotest ${run.version}`);
  return parts.join(" · ");
}

// groupText is a group's line as the CLI's summary words it.
export function groupText(g: SpecGroup): string {
  return `group ${g.name}: ${countNoun(g.suites, "suite")}, ${formatDuration(g.wall)}, run apart`;
}

// statsText words a package's or a module's counts as the CLI's trailer
// does; empty when the CLI gave none.
export function statsText(stats: SpecStats | undefined): string {
  if (!stats) return "";
  const parts: string[] = [];
  if (stats.passed > 0) parts.push(`${stats.passed} passed`);
  if (stats.failed > 0) parts.push(`${stats.failed} failed`);
  if (stats.skipped > 0) parts.push(`${stats.skipped} skipped`);
  if ((stats.notRun ?? 0) > 0) parts.push(`${stats.notRun} not run`);
  if ((stats.vacuous ?? 0) > 0) parts.push(`${stats.vacuous} vacuous`);
  if ((stats.assertsBy ?? 0) > 0)
    parts.push(`${stats.assertsBy} asserting by other means`);
  const failedPackages = stats.failedPackages ?? 0;
  if (failedPackages > 0)
    parts.push(countNoun(failedPackages, "failed package"));
  return parts.join(", ");
}

// addStats adds the CLI's counts of several packages, for a module's row.
export function addStats(
  all: (SpecStats | undefined)[],
): SpecStats | undefined {
  const given = all.filter((s): s is SpecStats => s !== undefined);
  if (given.length === 0) return undefined;
  const sum: SpecStats = {
    suites: 0,
    behaviors: 0,
    tests: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    notRun: 0,
    vacuous: 0,
    assertsBy: 0,
    failedPackages: 0,
  };
  for (const s of given) {
    sum.suites += s.suites;
    sum.behaviors += s.behaviors;
    sum.tests += s.tests;
    sum.passed += s.passed;
    sum.failed += s.failed;
    sum.skipped += s.skipped;
    sum.notRun = (sum.notRun ?? 0) + (s.notRun ?? 0);
    sum.vacuous = (sum.vacuous ?? 0) + (s.vacuous ?? 0);
    sum.assertsBy = (sum.assertsBy ?? 0) + (s.assertsBy ?? 0);
    sum.failedPackages = (sum.failedPackages ?? 0) + (s.failedPackages ?? 0);
  }
  return sum;
}

// sourceSegments splits an output line at its file:line tokens, each token
// that names a file on disk carrying where it opens. go test names test files
// relative to the package, the compiler relative to where it ran.
export function sourceSegments(
  line: string,
  dir: string | undefined,
): LineSegment[] {
  const out: LineSegment[] = [];
  const re = /((?:[A-Za-z]:)?[\w.\-~/\\]+\.go):(\d+)(?::\d+)?/g;
  let last = 0;
  for (let m = re.exec(line); m; m = re.exec(line)) {
    const resolved = resolveSource(m[1], dir);
    if (!resolved) continue;
    if (m.index > last) out.push(line.slice(last, m.index));
    out.push([m[0], resolved, Number(m[2])]);
    last = m.index + m[0].length;
  }
  if (last < line.length || out.length === 0) out.push(line.slice(last));
  return out;
}

function resolveSource(
  file: string,
  dir: string | undefined,
): string | undefined {
  const candidates = path.isAbsolute(file)
    ? [file]
    : dir
      ? [path.join(dir, file), path.join(dir, path.basename(file))]
      : [];
  return candidates.find((c) => existsSync(c));
}

// itemIdOf is the Test Explorer id of a node under its parent's (null at the
// top, undefined when the parent has none): a suite as discovery names it,
// its Test prefix dropped and an external twin's run marked; below it the
// level go test reported, as the router names a subtest's item. A fuzz
// target sits under its suite in both trees.
export function itemIdOf(
  pkgPath: string,
  parentId: string | null | undefined,
  node: Pick<SpecNode, "name" | "reported" | "external" | "variant" | "run">,
): string | undefined {
  if (parentId === undefined) return undefined;
  if (parentId !== null) {
    return methodItemId(parentId, { name: node.name, testName: node.reported });
  }
  if (!node.name.startsWith("Test") || node.name.startsWith("Test_")) {
    return undefined;
  }
  const twin = externalTwinNode(node);
  return `${pkgPath}/${node.name.slice(4)}${twin ? EXTERNAL_ID : ""}`;
}

// nearestItemId is id itself when the tree has it, else its closest
// ancestor that it has (the row's own list of them when it has one, else cut
// at slashes, which a behavior's name may hold): behaviors past the tree's per-method cap are not
// items, and running one runs its method.
export function nearestItemId(
  id: string,
  has: (id: string) => boolean,
  ancestors?: string[],
): string | undefined {
  if (ancestors) {
    if (has(id)) return id;
    for (let i = ancestors.length - 1; i >= 0; i--) {
      if (has(ancestors[i])) return ancestors[i];
    }
    return undefined;
  }
  let current = id;
  for (;;) {
    if (has(current)) return current;
    const cut = current.lastIndexOf("/");
    if (cut <= 0) return undefined;
    current = current.slice(0, cut);
  }
}

// sourceOf is where the CLI says the source declares a node; a node only a
// run revealed has no position.
export function sourceOf(
  node: SpecNode,
  dir: string | undefined,
): { file: string; line: number } | undefined {
  if (!dir || !node.file || !node.line) return undefined;
  return { file: path.join(dir, node.file), line: node.line };
}

// ownOutput is what a node printed, without go test's RUN/PASS/FAIL frames.
export function ownOutput(node: SpecNode): string[] {
  return node.output
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("=== ") && !l.startsWith("--- "));
}

// formatDuration is a time as the CLI's spec writes it.
export function formatDuration(seconds: number): string {
  const ms = Math.round(seconds * 1000);
  if (ms < 1) return "<1ms";
  if (ms < 1000) return `${ms}ms`;
  return `${seconds.toFixed(1)}s`;
}

/** Spells a count with its noun, singular for exactly one, as the CLI trailer does. */
export function countNoun(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`;
}

// declaredTests says whether a kept test path is still declared in pkg: a
// suite's own line and its methods' rows by the suite and method names, a
// fuzz target by its wrapper's name. Paths discovery does not list (plain
// test functions, benchmarks) are kept. A twin's row is matched against
// its own twin's methods. Undefined for a package that does not load: its
// suites are unknown.
export function declaredTests(
  pkg: DiscoverPackage,
): ((instance: number, testPath: string) => boolean) | undefined {
  if (pkg.broken) return undefined;
  const suites = pkg.suites ?? [];
  const hasSuite = (name: string) => suites.some((s) => s.name === name);
  // The suites a row of this instance belongs to, read as the router reads
  // the stream; a lone suite takes every instance.
  const suitesOf = (name: string, instance: number) => {
    const named = suites.filter((s) => s.name === name);
    const twins =
      named.some((s) => s.external) && named.some((s) => !s.external);
    if (!twins) return named;
    const external = externalInstance(instance, true);
    return named.filter((s) => !!s.external === external);
  };
  const hasMethod = (name: string, instance: number, method: string) =>
    suitesOf(name, instance).some((s) =>
      [...(s.methods ?? []), ...(s.fuzzers ?? [])].some(
        (m) => (m.testName ?? m.name) === method,
      ),
    );
  const hasFuzzer = (name: string, fuzzer: string) =>
    suites.some(
      (s) =>
        s.name === name && (s.fuzzers ?? []).some((f) => f.name === fuzzer),
    );
  return (instance, testPath) => {
    const slash = testPath.indexOf("/");
    const top = slash < 0 ? testPath : testPath.slice(0, slash);
    if (top.startsWith("Test")) {
      const name = top.slice(4);
      if (!hasSuite(name)) return !name.endsWith("TestSuite");
      if (slash < 0) return true;
      const rest = testPath.slice(slash + 1);
      const next = rest.indexOf("/");
      const method = next < 0 ? rest : rest.slice(0, next);
      // Under -count a repeat's method carries testing's #NN suffix.
      return (
        hasMethod(name, instance, method) ||
        hasMethod(name, instance, method.replace(/#\d+$/, ""))
      );
    }
    if (top.startsWith("Fuzz")) {
      const body = top.slice(4);
      let suiteLike = false;
      for (
        let idx = body.indexOf("_Fuzz");
        idx >= 0;
        idx = body.indexOf("_Fuzz", idx + 1)
      ) {
        const name = body.slice(0, idx);
        if (hasFuzzer(name, body.slice(idx + 1))) return true;
        if (name.endsWith("TestSuite")) suiteLike = true;
      }
      return !suiteLike;
    }
    return true;
  };
}
