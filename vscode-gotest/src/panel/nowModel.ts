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
  TreeEntry,
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
  type SpecStats,
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
  // A row above it matched the search: it stands for its whole subtree.
  above = false,
): boolean {
  if (query && !above && !n.display.toLowerCase().includes(query)) return false;
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
      const above =
        query !== "" && ctx.labels.some((l) => l.toLowerCase().includes(query));
      if (
        (leaf || own) &&
        leafMatches(node, query, filters, stale, ctx.hasFailedChild, above)
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
  const filtering = env.filters.size > 0;
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
    if (filtering) row.matches = matchCount(p, "", env.filters, stale);
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
  const tree = dirTree(rows, packages, env.modulePaths);

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
    tree,
    filtering,
    failed: failed + liveFailed,
  };
}

// hot is a row that needs looking at: failed or stale.
const hot = (r: PackageRow) => r.tone === "fail" || !!r.stale;

const byName = (a: { name: string }, b: { name: string }) =>
  a.name < b.name ? -1 : a.name > b.name ? 1 : 0;

// sumStats adds packages' counts for their folder.
function sumStats(list: (SpecStats | undefined)[]): SpecStats {
  const sum: SpecStats = {
    suites: 0,
    behaviors: 0,
    tests: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    notRun: 0,
    vacuous: 0,
    failedPackages: 0,
  };
  for (const s of list) {
    if (!s) continue;
    sum.suites += s.suites;
    sum.behaviors += s.behaviors;
    sum.passed += s.passed;
    sum.failed += s.failed;
    sum.skipped += s.skipped;
    sum.notRun! += s.notRun ?? 0;
    sum.vacuous! += s.vacuous ?? 0;
    sum.failedPackages! += s.failedPackages ?? 0;
  }
  return sum;
}

// A directory of the tree as it is built: its subdirectories by name, and
// the package that sits at it, if one does.
interface Trie {
  dirs: Map<string, Trie>;
  pkg?: PackageRow;
}

// dirTree lays the packages out as their directories, per module: the
// module with the most packages at the top, another module a directory of
// its own. A chain of directories holding only one directory each is one
// row ("gotest/internal"); a package's subdirectories sit beside it, named
// from it. Failed or stale entries come first within a directory, the rest
// in path order.
export function dirTree(
  rows: PackageRow[],
  packages: SpecPackage[],
  modulePaths: string[],
): TreeEntry[] {
  const mods = [...modulePaths].sort((a, b) => b.length - a.length);
  const modOf = (p: string) =>
    mods.find((m) => p === m || p.startsWith(m + "/")) ?? "";
  const counts = new Map<string, number>();
  for (const r of rows)
    counts.set(modOf(r.pkg), (counts.get(modOf(r.pkg)) ?? 0) + 1);
  let primary = "";
  let most = -1;
  for (const [m, n] of counts) if (n > most) [primary, most] = [m, n];
  const tail = (m: string) => m.slice(m.lastIndexOf("/") + 1);
  const segsOf = (pkg: string): string[] => {
    const m = modOf(pkg);
    if (!m) return pkg.split("/");
    const rel = pkg === m ? [] : pkg.slice(m.length + 1).split("/");
    if (m === primary) return rel.length > 0 ? rel : [tail(m)];
    const base =
      primary && m.startsWith(primary + "/")
        ? m.slice(primary.length + 1).split("/")
        : [tail(m)];
    return [...base, ...rel];
  };
  const top: Trie = { dirs: new Map() };
  for (const r of rows) {
    let at = top;
    for (const seg of segsOf(r.pkg)) {
      let next = at.dirs.get(seg);
      if (!next) {
        next = { dirs: new Map() };
        at.dirs.set(seg, next);
      }
      at = next;
    }
    at.pkg = r;
  }
  const stats = new Map(packages.map((p) => [p.path, p.stats]));
  const order = (list: TreeEntry[]) =>
    list.sort(
      (a, b) =>
        Number(entryHot(b)) - Number(entryHot(a)) ||
        (a.name < b.name ? -1 : a.name > b.name ? 1 : 0),
    );
  const below = (e: TreeEntry): PackageRow[] =>
    e.kind === "pkg" ? [e] : e.children.flatMap(below);
  const emit = (t: Trie, name: string, path: string, out: TreeEntry[]) => {
    if (t.pkg) {
      out.push({ ...t.pkg, kind: "pkg", name });
      for (const [seg, c] of t.dirs)
        emit(c, `${name}/${seg}`, `${path}/${seg}`, out);
      return;
    }
    // A directory holding one directory only is one row with it.
    if (t.dirs.size === 1) {
      const [[seg, c]] = Array.from(t.dirs);
      if (!c.pkg) {
        emit(c, `${name}/${seg}`, `${path}/${seg}`, out);
        return;
      }
    }
    const children: TreeEntry[] = [];
    for (const [seg, c] of t.dirs) emit(c, seg, `${path}/${seg}`, children);
    const pkgs = children.flatMap(below);
    const sum = countsText({
      path,
      status: "",
      duration: 0,
      nodes: [],
      stats: sumStats(pkgs.map((r) => stats.get(r.pkg))),
    });
    const anyHot = pkgs.some(hot);
    out.push({
      kind: "dir",
      key: path,
      name,
      packages: pkgs.length,
      counts: sum.text,
      tone: pkgs.some((r) => r.tone === "fail")
        ? "fail"
        : anyHot
          ? "warn"
          : sum.tone,
      hot: anyHot,
      children: order(children),
    });
  };
  const out: TreeEntry[] = [];
  for (const [seg, c] of top.dirs) emit(c, seg, seg, out);
  return order(out);
}

const entryHot = (e: TreeEntry): boolean => (e.kind === "dir" ? e.hot : hot(e));

// packagesIn lists the packages of a tree in the order it shows them.
export function packagesIn(tree: TreeEntry[]): string[] {
  return tree.flatMap((e) =>
    e.kind === "pkg" ? [e.pkg] : packagesIn(e.children),
  );
}

// leafCounts words the leaves under a row by status.
function leafCounts(n: SpecNode): { text: string; tone: Tone } {
  const c = { pass: 0, fail: 0, skip: 0, none: 0 } as Record<string, number>;
  const walk = (x: SpecNode) => {
    if (x.children.length === 0 || x.kind === "fuzz")
      c[x.status in c ? x.status : "none"]++;
    else x.children.forEach(walk);
  };
  walk(n);
  const parts = [
    c.fail ? `${num(c.fail)} failed` : "",
    c.pass ? `${num(c.pass)} passed` : "",
    c.skip ? `${num(c.skip)} skipped` : "",
    c.none ? `${num(c.none)} not run` : "",
  ].filter(Boolean);
  return {
    text: parts.join(" · "),
    tone: c.fail ? "fail" : c.pass ? "pass" : "none",
  };
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

// tagsOf lists a row's marks; EXT. only in a package whose suites sit in
// both its test packages, where it tells the twins apart.
function tagsOf(n: SpecNode, twins: boolean): TreeNode["tags"] {
  const tags: NonNullable<TreeNode["tags"]> = [];
  if (n.kind === "fuzz")
    tags.push({
      text: `FUZZ · ${countNoun(n.children.length, "seed")}`,
      tone: "info",
    });
  if (n.external && twins) tags.push({ text: "EXT." });
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
  // A leaf says it in its status word.
  if (n.status === "none" && n.children.length > 0)
    tags.push({ text: "NOT RUN" });
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
  const twins =
    pkg.nodes.some((n) => n.external) && pkg.nodes.some((n) => !n.external);
  const narrow = query !== "" || opts.filters.size > 0;
  const rec = (
    list: SpecNode[],
    parentBase: string | null | undefined,
    pos: number[],
    above: boolean,
  ): TreeNode[] => {
    const out: TreeNode[] = [];
    list.forEach((n, i) => {
      const base = itemIdOf(pkg.path, parentBase, n);
      const here = [...pos, i];
      const matched =
        above || (query !== "" && n.display.toLowerCase().includes(query));
      const kids = rec(n.children, base, here, matched);
      const hasFailedChild = n.children.some((c) => c.status === "fail");
      const self =
        n.children.length === 0 || isFailedRow(n, hasFailedChild)
          ? leafMatches(
              n,
              query,
              opts.filters,
              opts.stale,
              hasFailedChild,
              above,
            )
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
      const tags = tagsOf(n, twins);
      if (tags) t.tags = tags;
      if (n.status === "fail") {
        const own = ownOutput(n);
        if (own.length > 0) t.out = outputLines(own, outDir, ROW_LINES);
      }
      if (kids.length > 0) t.c = kids;
      if (pos.length === 0 && n.children.length > 0) {
        const c = leafCounts(n);
        if (c.text) [t.n, t.nt] = [c.text, c.tone];
      }
      out.push(t);
    });
    return out;
  };
  const nodes = rec(pkg.nodes, null, [], false);
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

// The most a search answer carries: leaves, and bytes of rows.
export const FOUND_LEAVES = 300;
export const FOUND_BYTES = 150_000;

const leavesIn = (nodes: TreeNode[]): number =>
  nodes.reduce((n, x) => n + (x.c ? leavesIn(x.c) : 1), 0);

// searchFound is what a search finds: per package, in the order the tab
// lists them, the rows that match with the rows above them, until the
// leaves or the bytes would pass their bound; total counts every match.
export function searchFound(
  doc: SpecData | undefined,
  query: string,
  order: string[],
  opts: {
    dirOf: (pkg: string) => string | undefined;
    filters: ReadonlySet<FilterStatus>;
    staleOf: (pkg: string) => boolean;
  },
  limits = { leaves: FOUND_LEAVES, bytes: FOUND_BYTES },
): {
  results: { pkg: string; nodes: TreeNode[] }[];
  total: number;
  shown: number;
} {
  const q = query.trim().toLowerCase();
  const results: { pkg: string; nodes: TreeNode[] }[] = [];
  let total = 0;
  let shown = 0;
  let bytes = 0;
  let full = false;
  if (!q || !doc) return { results, total, shown };
  for (const pkg of order) {
    const p = doc.packages.find((x) => x.path === pkg);
    if (!p) continue;
    const n = matchCount(p, q, opts.filters, opts.staleOf(pkg));
    if (n === 0) continue;
    total += n;
    if (full) continue;
    const tree = packageTree(doc, pkg, {
      dirOf: opts.dirOf,
      query: q,
      filters: opts.filters,
      stale: opts.staleOf(pkg),
    });
    if (!tree) continue;
    const size = JSON.stringify(tree.nodes).length;
    const leaves = leavesIn(tree.nodes);
    if (
      shown > 0 &&
      (shown + leaves > limits.leaves || bytes + size > limits.bytes)
    ) {
      full = true;
      continue;
    }
    results.push({ pkg, nodes: tree.nodes });
    shown += leaves;
    bytes += size;
  }
  return { results, total, shown };
}

// The names that pick a row out of its siblings.
type RowName = Pick<
  SpecNode,
  "name" | "reported" | "external" | "variant" | "run"
>;

const sameRow = (a: RowName, b: RowName) =>
  a.name === b.name &&
  a.reported === b.reported &&
  !!a.external === !!b.external &&
  a.variant === b.variant &&
  a.run === b.run;

// namesAt is the names of the rows down to a place in a package.
export function namesAt(
  doc: SpecData | undefined,
  pkgPath: string,
  pos: number[],
): RowName[] | undefined {
  let list = doc?.packages.find((p) => p.path === pkgPath)?.nodes;
  const out: RowName[] = [];
  for (const i of pos) {
    const n = list?.[i];
    if (!n) return undefined;
    out.push({
      name: n.name,
      reported: n.reported,
      external: n.external,
      variant: n.variant,
      run: n.run,
    });
    list = n.children;
  }
  return out;
}

// findByNames finds a row by its names in another rendering of its
// package, with its place there.
export function findByNames(
  doc: SpecData,
  pkgPath: string,
  names: RowName[],
): { node: SpecNode; pos: number[] } | undefined {
  let list = doc.packages.find((p) => p.path === pkgPath)?.nodes;
  let node: SpecNode | undefined;
  const pos: number[] = [];
  for (const want of names) {
    const i = (list ?? []).findIndex((n) => sameRow(n, want));
    if (i < 0) return undefined;
    node = list![i];
    pos.push(i);
    list = node.children;
  }
  return node ? { node, pos } : undefined;
}

// leavesUnder counts the behaviors a row holds; a fuzz target is one.
export function leavesUnder(n: SpecNode): number {
  if (n.children.length === 0 || n.kind === "fuzz") return 1;
  return n.children.reduce((s, c) => s + leavesUnder(c), 0);
}

// rowOfItem finds the row a Test Explorer item stands for.
export function rowOfItem(
  doc: SpecData | undefined,
  itemId: string,
): { pkg: string; pos: number[] } | undefined {
  for (const p of doc?.packages ?? []) {
    if (itemId === p.path) return { pkg: p.path, pos: [] };
    if (!itemId.startsWith(p.path + "/")) continue;
    let hit: number[] | undefined;
    walkNodes(
      p.nodes,
      (_n, ctx) => {
        if (!hit && ctx.id === itemId) hit = ctx.pos;
      },
      p.path,
    );
    if (hit) return { pkg: p.path, pos: hit };
  }
  return undefined;
}
