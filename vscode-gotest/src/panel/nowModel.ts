// The Now tab's data, built on the host from the spec document it keeps:
// a summary small enough to send on every change, and one package's rows
// when that package is opened. Verdicts and counts are the CLI's; a run in
// progress adds what its events said so far, until its end renders again.
import { staleHeadline, type Freshness } from "../freshness.js";
import type { LivePackage } from "../liveProgress.js";
import { agentName, fromEditor, levelLabel } from "../runsModel.js";
import type {
  Card,
  Chip,
  FilterStatus,
  NowModel,
  OutputLine,
  PackageRow,
  Tone,
  TreeNode,
} from "./protocol.js";
import {
  countNoun,
  formatDuration,
  formatRanAt,
  groupText,
  itemIdOf,
  ownOutput,
  provenanceText,
  sourceOf,
  sourceSegments,
  type SpecData,
  type SpecNode,
  type SpecPackage,
  type SpecRun,
} from "./specData.js";

// How many cards of each kind the tab shows before it counts the rest.
export const CARD_LIMIT: Record<Card["kind"], number> = {
  failed: 30,
  stale: 10,
  vacuous: 10,
};

// How many lines of a failure a card or a row shows; Full output has all.
export const CARD_LINES = 4;
export const ROW_LINES = 40;
export const FULL_OUTPUT_LINES = 2000;

export interface SourceOption {
  id: string;
  label: string;
  selected: boolean;
}

// What the tab is built against.
export interface NowEnv {
  rev: number;
  doc: SpecData | undefined;
  error?: string;
  modulePaths: string[];
  dirOf: (pkg: string) => string | undefined;
  freshnessOf: (pkg: string) => Freshness | undefined;
  ranAtOf: (pkg: string) => number | undefined;
  neverRan: (pkg: string) => boolean;
  now: number;
  branch?: string;
  sources: SourceOption[];
  // The label of the imported stream on show; undefined for the workspace's.
  imported?: string;
  // Runs in progress: a line each and what their events said per package.
  live: { line: string; packages: ReadonlyMap<string, LivePackage> }[];
  query: string;
  filters: ReadonlySet<FilterStatus>;
  // The flaky hint for a failed test, when the runs document can say.
  flakyOf?: (
    pkg: string,
    test: string,
    inputHash?: string,
  ) => string | undefined;
}

const num = (n: number) => n.toLocaleString("en-US");

// shortName is a package's path within its module; a package of another
// module than the one with the most packages keeps that module's last name.
export function shortNames(
  packages: string[],
  modulePaths: string[],
): Map<string, string> {
  const mods = [...modulePaths].sort((a, b) => b.length - a.length);
  const modOf = (p: string) =>
    mods.find((m) => p === m || p.startsWith(m + "/")) ?? "";
  const counts = new Map<string, number>();
  for (const p of packages) {
    const m = modOf(p);
    counts.set(m, (counts.get(m) ?? 0) + 1);
  }
  let primary = "";
  let most = -1;
  for (const [m, n] of counts) if (n > most) [primary, most] = [m, n];
  const out = new Map<string, string>();
  for (const p of packages) {
    const m = modOf(p);
    if (!m) {
      out.set(p, p);
      continue;
    }
    const tail = m.slice(m.lastIndexOf("/") + 1);
    const rest = p === m ? "" : p.slice(m.length + 1);
    if (m === primary) out.set(p, rest || tail);
    else out.set(p, rest ? `${tail}/${rest}` : tail);
  }
  return out;
}

// shortDuration is a package's time as a row shows it.
export function shortDuration(seconds: number): string {
  if (!(seconds > 0)) return "";
  if (seconds < 1) return "<1s";
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${String(s % 60).padStart(2, "0")}s`;
}

// countsText words a package's counts, failures first.
export function countsText(p: SpecPackage): { text: string; tone: Tone } {
  const s = p.stats;
  if (!s)
    return { text: p.status === "none" ? "not run" : p.status, tone: "none" };
  const parts: string[] = [];
  const failedPkg = s.failedPackages ?? 0;
  if (failedPkg > 0) parts.push("package failed");
  if (s.failed > 0) parts.push(`${num(s.failed)} failed`);
  if (s.passed > 0) parts.push(`${num(s.passed)} passed`);
  if (s.skipped > 0) parts.push(`${num(s.skipped)} skipped`);
  if ((s.notRun ?? 0) > 0) parts.push(`${num(s.notRun!)} not run`);
  if ((s.vacuous ?? 0) > 0) parts.push(`${num(s.vacuous!)} vacuous`);
  const tone: Tone =
    s.failed > 0 || failedPkg > 0
      ? "fail"
      : (s.vacuous ?? 0) > 0
        ? "warn"
        : s.passed > 0
          ? "pass"
          : "none";
  return { text: parts.join(" · ") || "nothing ran", tone };
}

// whoRan says who started a run, from its command line.
export function whoRan(args: string[] | undefined, agent?: string): string {
  if (fromEditor(args)) return "from this editor";
  if (agent) return `by agent ${agentName(agent)}`;
  return "from a terminal";
}

// lastRunText is the context strip's sentence on the run on show.
export function lastRunText(
  run: SpecRun | undefined,
  at: number | undefined,
  now: number,
  imported?: string,
): string | undefined {
  if (!run && at === undefined) return undefined;
  const when = at !== undefined ? ` ${formatRanAt(at, now)}` : "";
  const who = imported
    ? `imported from ${imported}`
    : whoRan(run?.args, run?.agent);
  let text = `Last run${when}, ${who}.`;
  const c = run?.changed;
  if (c?.noRecord)
    text += ` No earlier record, so all ${countNoun(c.ran, "package")} ran.`;
  else if (c)
    text += ` Run changed ran ${countNoun(c.ran, "package")} and left out ${c.unchanged} unchanged.`;
  if (run && run.exitCode === undefined) text += " It did not finish.";
  return text;
}

// goName is a node's go test name below its ancestors'.
function goName(parent: string, n: SpecNode): string {
  const own = n.reported ?? n.name;
  return parent ? `${parent}/${own}` : own;
}

// failureOf reads a failure's output: the first file:line it names, and the
// lines after it up to a stack trace.
export function failureOf(
  lines: string[],
  dir: string | undefined,
): { message: OutputLine[]; at?: OutputLine; more: boolean } {
  const own = lines.map((l) => l.trim()).filter(Boolean);
  let start = own.findIndex((l) => /^[\w.\-/\\]+\.go:\d+: /.test(l));
  let at: OutputLine | undefined;
  const msg: string[] = [];
  if (start >= 0) {
    const m = /^([\w.\-/\\]+\.go:\d+): (.*)$/.exec(own[start])!;
    at = sourceSegments(m[1], dir);
    if (m[2]) msg.push(m[2]);
    start++;
  } else start = 0;
  for (let i = start; i < own.length && msg.length < CARD_LINES; i++) {
    const l = own[i];
    if (/goroutine \d+ \[/.test(l) || /\+0x[0-9a-f]+$/.test(l)) break;
    msg.push(l);
  }
  return {
    message: msg.map((l) => sourceSegments(l, dir)),
    at,
    more: own.length > msg.length + (at ? 1 : 0),
  };
}

// The rows above a node, as a card's path names them.
interface Walk {
  pkg: SpecPackage;
  short: string;
  dir: string | undefined;
}

function walkNodes(
  nodes: SpecNode[],
  visit: (
    n: SpecNode,
    ctx: {
      pos: number[];
      id: string | undefined;
      ancestors: string[];
      labels: string[];
      test: string;
      hasFailedChild: boolean;
    },
  ) => void,
  pkgPath: string,
): void {
  const rec = (
    list: SpecNode[],
    parentBase: string | null | undefined,
    pos: number[],
    ancestors: string[],
    labels: string[],
    parentTest: string,
  ) => {
    list.forEach((n, i) => {
      const base = itemIdOf(pkgPath, parentBase, n);
      const id = n.kind === "fixture" ? undefined : base;
      const here = [...pos, i];
      const test = goName(parentTest, n);
      visit(n, {
        pos: here,
        id,
        ancestors,
        labels,
        test,
        hasFailedChild: n.children.some((c) => c.status === "fail"),
      });
      rec(
        n.children,
        base,
        here,
        id ? [...ancestors, id] : ancestors,
        [...labels, n.display],
        test,
      );
    });
  };
  rec(nodes, null, [], [pkgPath], [], "");
}

// isFailedRow says whether a row's own failure is one to act on: a failed
// leaf, or a failed branch none of whose rows failed (a failed AfterAll).
const isFailedRow = (n: SpecNode, hasFailedChild: boolean) =>
  n.status === "fail" && !hasFailedChild;

function leafMatches(
  n: SpecNode,
  query: string,
  filters: ReadonlySet<FilterStatus>,
  stale: boolean,
  hasFailedChild: boolean,
): boolean {
  if (query && !n.display.toLowerCase().includes(query)) return false;
  if (filters.size === 0) return true;
  if (filters.has("stale") && stale) return true;
  if (filters.has("vacuous") && n.vacuous) return true;
  if (filters.has("fail") && isFailedRow(n, hasFailedChild)) return true;
  if (filters.has("pass") && n.status === "pass") return true;
  if (filters.has("skip") && n.status === "skip") return true;
  if (filters.has("none") && n.status === "none") return true;
  return false;
}

// matchCount counts a package's leaves a search and the chips leave.
export function matchCount(
  pkg: SpecPackage,
  query: string,
  filters: ReadonlySet<FilterStatus>,
  stale: boolean,
): number {
  let n = 0;
  walkNodes(
    pkg.nodes,
    (node, ctx) => {
      const leaf = node.children.length === 0;
      const own = !leaf && isFailedRow(node, ctx.hasFailedChild);
      if (
        (leaf || own) &&
        leafMatches(node, query, filters, stale, ctx.hasFailedChild)
      )
        n++;
    },
    pkg.path,
  );
  return n;
}

// buildNowModel lays out the Now tab.
export function buildNowModel(env: NowEnv): NowModel {
  const doc = env.doc;
  const query = env.query.trim().toLowerCase();
  const filtering = query !== "" || env.filters.size > 0;
  const packages = doc?.packages ?? [];
  const livePkgs = new Map<string, LivePackage>();
  for (const run of env.live)
    for (const [p, lp] of run.packages) livePkgs.set(p, lp);
  const names = shortNames(
    Array.from(new Set([...packages.map((p) => p.path), ...livePkgs.keys()])),
    env.modulePaths,
  );
  const shortOf = (p: string) => names.get(p) ?? p;

  const cards: Card[] = [];
  const counted: Record<Card["kind"], number> = {
    failed: 0,
    stale: 0,
    vacuous: 0,
  };
  const add = (c: Card) => {
    counted[c.kind]++;
    if (counted[c.kind] <= CARD_LIMIT[c.kind]) cards.push(c);
  };
  // Tests a run in progress judged again: their old card gives way.
  const liveVerdict = (pkg: string, test: string) =>
    livePkgs.get(pkg)?.tests.get(test)?.status;

  const rows: PackageRow[] = [];
  const noSuites: string[] = [];
  let failed = 0;
  let staleBehaviors = 0;
  for (const p of packages) {
    const stale = env.freshnessOf(p.path) === "stale";
    const fresh = env.freshnessOf(p.path);
    const walk: Walk = {
      pkg: p,
      short: shortOf(p.path),
      dir: p.dir ?? env.dirOf(p.path),
    };
    if (stale) staleBehaviors += p.stats?.behaviors ?? 0;
    failed += (p.stats?.failed ?? 0) + (p.stats?.failedPackages ?? 0);
    if (
      (p.stats?.failedPackages ?? 0) > 0 &&
      (p.output ?? []).some((l) => l.trim())
    ) {
      const f = failureOf(p.output ?? [], walk.dir);
      add({
        key: `${p.path}\u0000`,
        kind: "failed",
        pkg: p.path,
        where: walk.short,
        name: "The package failed outside any test",
        message: f.message,
        at: f.at,
        id: p.path,
        pos: [],
        more: f.more,
      });
    }
    if (stale)
      add({
        key: `${p.path}\u0000stale`,
        kind: "stale",
        pkg: p.path,
        where: walk.short,
        name: `${countNoun(p.stats?.behaviors ?? 0, "verdict")} from before its inputs changed`,
        id: p.path,
        pos: [],
      });
    walkNodes(
      p.nodes,
      (n, ctx) => {
        const where = [walk.short, ...ctx.labels].join(" › ");
        if (isFailedRow(n, ctx.hasFailedChild)) {
          const again = liveVerdict(p.path, ctx.test);
          if (again && again !== "fail") return;
          const f = failureOf(ownOutput(n), walk.dir);
          add({
            key: `${p.path}\u0000${ctx.pos.join(".")}`,
            kind: "failed",
            pkg: p.path,
            where,
            name: n.display,
            message: f.message,
            at: f.at,
            loc: sourceOf(n, p.dir),
            flaky: env.flakyOf?.(p.path, ctx.test, p.inputHash),
            id: ctx.id,
            pos: ctx.pos,
            more: f.more,
          });
        } else if (n.vacuous && n.children.length === 0) {
          add({
            key: `${p.path}\u0000${ctx.pos.join(".")}`,
            kind: "vacuous",
            pkg: p.path,
            where,
            name: n.display,
            message: [
              ["Passed without an assertion, and declares no other means."],
            ],
            loc: sourceOf(n, p.dir),
            id: ctx.id,
            pos: ctx.pos,
          });
        }
      },
      p.path,
    );
    if (p.nodes.length === 0 && (p.stats?.failedPackages ?? 0) === 0) {
      noSuites.push(walk.short);
      continue;
    }
    const c = countsText(p);
    const at = env.ranAtOf(p.path);
    const row: PackageRow = {
      pkg: p.path,
      name: walk.short,
      counts: c.text,
      tone: stale ? "warn" : c.tone,
      time: shortDuration(p.duration),
      title:
        at !== undefined ? `last run ${formatRanAt(at, env.now)}` : undefined,
      stale: stale || undefined,
      note: stale
        ? "stale"
        : env.neverRan(p.path)
          ? "declared, not run yet"
          : fresh === "unknown"
            ? "freshness unknown"
            : undefined,
    };
    if (filtering) row.matches = matchCount(p, query, env.filters, stale);
    rows.push(row);
  }

  // A run in progress: its packages' counts so far, and its failures.
  let liveFailed = 0;
  for (const [pkgPath, lp] of livePkgs) {
    const n = liveCounts(lp);
    const parts = [
      n.failed > 0 ? `${num(n.failed)} failed` : "",
      `${num(n.passed)} passed`,
      n.skipped > 0 ? `${num(n.skipped)} skipped` : "",
    ].filter(Boolean);
    const text = `${lp.done ? "ran" : "running"} · ${parts.join(" · ")}`;
    const tone: Tone = n.failed > 0 || lp.done === "fail" ? "fail" : "info";
    const row = rows.find((r) => r.pkg === pkgPath);
    if (row)
      Object.assign(row, { counts: text, tone, live: true, note: undefined });
    else
      rows.push({
        pkg: pkgPath,
        name: shortOf(pkgPath),
        counts: text,
        tone,
        time: "",
        live: true,
      });
    for (const t of liveFailures(lp)) {
      const segs = t.test.split("/");
      const labels = segs.slice(0, -1).map(levelLabel);
      const f = failureOf(t.output, pkgDirOf(packages, pkgPath, env));
      const ids = liveIds(pkgPath, segs);
      liveFailed++;
      if (cards.some((c) => c.key === liveKey(pkgPath, t.test))) continue;
      add({
        key: liveKey(pkgPath, t.test),
        kind: "failed",
        pkg: pkgPath,
        where: [shortOf(pkgPath), ...labels].join(" › "),
        name: levelLabel(segs[segs.length - 1]),
        message: f.message,
        at: f.at,
        id: ids.id,
        live: true,
        more: f.more,
      });
    }
  }
  rows.sort(
    (a, b) =>
      Number(b.tone === "fail") - Number(a.tone === "fail") ||
      behaviorsOf(packages, b.pkg) - behaviorsOf(packages, a.pkg) ||
      (a.name < b.name ? -1 : a.name > b.name ? 1 : 0),
  );

  const stats = doc?.stats;
  const chips: Chip[] = stats
    ? [
        { status: "fail", label: "Failed", count: failed },
        { status: "stale", label: "Stale", count: staleBehaviors },
        { status: "vacuous", label: "Vacuous", count: stats.vacuous ?? 0 },
        { status: "none", label: "Not run", count: stats.notRun ?? 0 },
        ...(stats.skipped > 0
          ? [
              {
                status: "skip" as const,
                label: "Skipped",
                count: stats.skipped,
              },
            ]
          : []),
        { status: "pass", label: "Passed", count: stats.passed },
      ]
    : [];

  const more = (Object.keys(counted) as Card["kind"][])
    .filter((k) => counted[k] > CARD_LIMIT[k])
    .map((k) => `${num(counted[k] - CARD_LIMIT[k])} more ${k}`);

  const run = doc?.runs?.[0] ?? doc?.run;
  const lastAt = packages.reduce<number | undefined>((m, p) => {
    const at = env.ranAtOf(p.path);
    return at !== undefined && (m === undefined || at > m) ? at : m;
  }, undefined);

  return {
    rev: env.rev,
    empty:
      !doc && livePkgs.size === 0
        ? {
            title: "Nothing has run yet",
            text: "Run the tests, or Run changed to run only what changed since the last record. Every suite, method and behavior then shows here with its newest verdict.",
          }
        : undefined,
    error: env.error,
    context: {
      branch: env.branch,
      commit: run?.commit
        ? run.commit + (run.dirty ? " with uncommitted changes" : "")
        : undefined,
      fresh: freshLine(
        packages.map((p) => p.path),
        env.freshnessOf,
      ),
      lastRun: lastRunText(run, lastAt, env.now, env.imported),
      runTitle: run
        ? provenanceText(run) +
          (run.args ? `\ngotest ${run.args.join(" ")}` : "")
        : undefined,
      live: env.live.length > 0 ? env.live.map((l) => l.line) : undefined,
      sources: env.sources.length > 0 ? env.sources : undefined,
    },
    chips,
    cards,
    moreCards:
      more.length > 0
        ? `And ${more.join(", ")}; the chips narrow the rows to them.`
        : undefined,
    noSuites:
      noSuites.length > 0
        ? {
            text: `${countNoun(noSuites.length, "package")} ${noSuites.length === 1 ? "has" : "have"} no suites`,
            names: noSuites,
          }
        : undefined,
    total: stats ? totalText(stats, packages.length) : "",
    groups: [
      ...(stats ? moreTotals(stats) : []),
      ...(doc?.groups ?? []).map(groupText),
    ],
    packages: rows,
    filtering,
    failed: failed + liveFailed,
  };
}

function behaviorsOf(packages: SpecPackage[], pkg: string): number {
  return packages.find((p) => p.path === pkg)?.stats?.behaviors ?? 0;
}

function pkgDirOf(
  packages: SpecPackage[],
  pkg: string,
  env: NowEnv,
): string | undefined {
  return packages.find((p) => p.path === pkg)?.dir ?? env.dirOf(pkg);
}

const liveKey = (pkg: string, test: string) => `${pkg}\u0000live\u0000${test}`;

// liveIds names the Test Explorer items of a test's levels.
function liveIds(
  pkg: string,
  segs: string[],
): { id?: string; ancestors: string[] } {
  let parent: string | null | undefined = null;
  const ancestors: string[] = [];
  for (const name of segs) {
    const id = itemIdOf(pkg, parent, {
      name,
      reported: undefined,
      external: false,
    });
    if (id === undefined) return { ancestors };
    ancestors.push(id);
    parent = id;
  }
  const id = ancestors.pop();
  return { id, ancestors };
}

// liveCounts counts a package's leaves by what a run said of them so far.
export function liveCounts(lp: LivePackage): {
  passed: number;
  failed: number;
  skipped: number;
  running: number;
} {
  const n = { passed: 0, failed: 0, skipped: 0, running: 0 };
  for (const [test, t] of lp.tests) {
    if (lp.parents.has(test)) continue;
    if (t.status === "pass") n.passed++;
    else if (t.status === "fail") n.failed++;
    else if (t.status === "skip") n.skipped++;
    else n.running++;
  }
  return n;
}

// liveFailures are the tests of a package a run failed so far and none of
// whose own tests failed.
export function liveFailures(
  lp: LivePackage,
): { test: string; output: string[] }[] {
  const out: { test: string; output: string[] }[] = [];
  for (const [test, t] of lp.tests)
    if (t.status === "fail" && !lp.failedParents.has(test))
      out.push({ test, output: t.output });
  return out;
}

function totalText(
  s: NonNullable<SpecData["stats"]>,
  packages: number,
): string {
  return `${num(s.behaviors)} in ${countNoun(s.suites, "suite")}, ${countNoun(packages, "package")}`;
}

// moreTotals are the document's other counts, said under the total.
function moreTotals(s: NonNullable<SpecData["stats"]>): string[] {
  const parts: string[] = [];
  if ((s.fuzzers ?? 0) > 0) parts.push(countNoun(s.fuzzers!, "fuzz target"));
  if (s.tests > 0) parts.push(`${s.tests} stdlib tests`);
  if ((s.benchmarks ?? 0) > 0)
    parts.push(countNoun(s.benchmarks!, "benchmark"));
  if ((s.assertsBy ?? 0) > 0)
    parts.push(`${num(s.assertsBy!)} asserting by other means`);
  if ((s.excluded ?? 0) > 0) parts.push(`${s.excluded} excluded`);
  return parts;
}

// freshLine says whether the verdicts on show belong to the working tree.
export function freshLine(
  paths: string[],
  freshnessOf: (pkg: string) => Freshness | undefined,
): { text: string; tone: Tone } | undefined {
  if (paths.length === 0) return undefined;
  const stale = staleHeadline(paths, freshnessOf);
  if (stale) return { text: stale, tone: "warn" };
  const judged = paths.filter((p) => freshnessOf(p) === "current").length;
  if (judged > 0 && judged === paths.length)
    return { text: "verdicts match this tree", tone: "pass" };
  if (judged > 0)
    return {
      text: `verdicts match this tree where an input hash says so (${judged} of ${paths.length} packages)`,
      tone: "pass",
    };
  if (paths.some((p) => freshnessOf(p) === "unknown"))
    return {
      text: "freshness unknown: the run stamped no input hashes",
      tone: "none",
    };
  return undefined;
}

// --- one package's rows ---

// The kinds whose rows show their time at any length, as the CLI's spec does.
const TIMED_KINDS = new Set(["suite", "method", "test", "fuzz"]);

function durationText(n: SpecNode): string | undefined {
  if (!["pass", "fail", "skip"].includes(n.status)) return undefined;
  if (!TIMED_KINDS.has(n.kind) && n.duration < 1) return undefined;
  return formatDuration(n.duration);
}

function tagsOf(n: SpecNode): TreeNode["tags"] {
  const tags: NonNullable<TreeNode["tags"]> = [];
  if (n.kind === "fuzz")
    tags.push({
      text: `FUZZ · ${countNoun(n.children.length, "seed")}`,
      tone: "info",
    });
  if (n.external) tags.push({ text: "EXT." });
  if (n.runs && n.run) tags.push({ text: `RUN ${n.run} OF ${n.runs}` });
  if (n.focused) tags.push({ text: "FOCUSED", tone: "warn" });
  else if (n.excluded || n.status === "skip")
    tags.push({
      text: `SKIPPED${n.status === "skip" && n.skipReason ? `: ${n.skipReason}` : ""}`,
      tone: "warn",
    });
  if (n.incomplete)
    tags.push({ text: "INCOMPLETE: behaviors known only at run time" });
  if (n.children.length === 0) {
    if (n.vacuous)
      tags.push({ text: "VACUOUS: passed without an assertion", tone: "warn" });
    else if (n.assertsBy) tags.push({ text: `ASSERTS BY: ${n.assertsBy}` });
    else if (n.assertions !== undefined)
      tags.push({ text: countNoun(n.assertions, "assertion") });
  }
  if (n.status === "none") tags.push({ text: "NOT RUN" });
  return tags.length > 0 ? tags : undefined;
}

function outputLines(
  lines: string[],
  dir: string | undefined,
  limit: number,
): OutputLine[] {
  return lines.slice(0, limit).map((l) => sourceSegments(l, dir));
}

// packageTree is one package's rows as the tab draws them, the rows a search
// and the chips leave; undefined for a package the document lacks.
export function packageTree(
  doc: SpecData | undefined,
  pkgPath: string,
  opts: {
    dirOf: (pkg: string) => string | undefined;
    query: string;
    filters: ReadonlySet<FilterStatus>;
    stale: boolean;
  },
): { nodes: TreeNode[]; failure?: OutputLine[] } | undefined {
  const pkg = doc?.packages.find((p) => p.path === pkgPath);
  if (!pkg) return undefined;
  const query = opts.query.trim().toLowerCase();
  const outDir = pkg.dir ?? opts.dirOf(pkg.path);
  const narrow = query !== "" || opts.filters.size > 0;
  const rec = (
    list: SpecNode[],
    parentBase: string | null | undefined,
    pos: number[],
  ): TreeNode[] => {
    const out: TreeNode[] = [];
    list.forEach((n, i) => {
      const base = itemIdOf(pkg.path, parentBase, n);
      const here = [...pos, i];
      const kids = rec(n.children, base, here);
      const hasFailedChild = n.children.some((c) => c.status === "fail");
      const self =
        n.children.length === 0 || isFailedRow(n, hasFailedChild)
          ? leafMatches(n, query, opts.filters, opts.stale, hasFailedChild)
          : false;
      if (narrow && kids.length === 0 && !self) return;
      const loc = sourceOf(n, pkg.dir);
      const t: TreeNode = {
        d: n.display,
        k: n.kind,
        s: n.status,
        pos: here.join("."),
      };
      const dur = durationText(n);
      if (dur) t.t = dur;
      if (n.kind !== "fixture" && base) t.id = base;
      if (loc) t.loc = [loc.file, loc.line];
      const tags = tagsOf(n);
      if (tags) t.tags = tags;
      if (n.status === "fail") {
        const own = ownOutput(n);
        if (own.length > 0) t.out = outputLines(own, outDir, ROW_LINES);
      }
      if (kids.length > 0) t.c = kids;
      out.push(t);
    });
    return out;
  };
  const nodes = rec(pkg.nodes, null, []);
  const failure =
    (pkg.stats?.failedPackages ?? 0) > 0
      ? outputLines(
          (pkg.output ?? []).map((l) => l.trimEnd()).filter(Boolean),
          outDir,
          ROW_LINES,
        )
      : undefined;
  return {
    nodes,
    failure: failure && failure.length > 0 ? failure : undefined,
  };
}

// nodeAt is the row at a place in a package, with its item id and the ids
// above it.
export function nodeAt(
  doc: SpecData | undefined,
  pkgPath: string,
  pos: number[],
):
  | { node?: SpecNode; pkg: SpecPackage; id?: string; ancestors: string[] }
  | undefined {
  const pkg = doc?.packages.find((p) => p.path === pkgPath);
  if (!pkg) return undefined;
  let list = pkg.nodes;
  let parent: string | null | undefined = null;
  // The package's own item is above every row.
  const ancestors: string[] = [pkg.path];
  let node: SpecNode | undefined;
  let id: string | undefined;
  for (const i of pos) {
    node = list[i];
    if (!node) return undefined;
    if (id) ancestors.push(id);
    const base = itemIdOf(pkg.path, parent, node);
    id = node.kind === "fixture" ? undefined : base;
    parent = base;
    list = node.children;
  }
  return { node, pkg, id, ancestors };
}

// fullOutput is a row's whole output, or a package's own, linked.
export function fullOutput(
  doc: SpecData | undefined,
  pkgPath: string,
  pos: number[],
  dirOf: (pkg: string) => string | undefined,
): { lines: OutputLine[]; more: number } | undefined {
  const at = nodeAt(doc, pkgPath, pos);
  if (!at) return undefined;
  const raw = at.node ? at.node.output : (at.pkg.output ?? []);
  const lines = raw
    .map((l) => l.replace(/\n$/, ""))
    .filter((l) => l.trim() !== "");
  const dir = at.pkg.dir ?? dirOf(pkgPath);
  return {
    lines: outputLines(lines, dir, FULL_OUTPUT_LINES),
    more: Math.max(0, lines.length - FULL_OUTPUT_LINES),
  };
}

// failedIds are the narrowest failed rows' items, with the ids above each.
export function failedIds(doc: SpecData | undefined): {
  ids: string[];
  ancestors: Record<string, string[]>;
} {
  const ids: string[] = [];
  const ancestors: Record<string, string[]> = {};
  for (const p of doc?.packages ?? []) {
    walkNodes(
      p.nodes,
      (n, ctx) => {
        if (!isFailedRow(n, ctx.hasFailedChild) || !ctx.id) return;
        ids.push(ctx.id);
        ancestors[ctx.id] = ctx.ancestors;
      },
      p.path,
    );
  }
  return { ids, ancestors };
}
