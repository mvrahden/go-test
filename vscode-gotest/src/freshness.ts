// Whether the verdicts the panel shows still belong to the working tree,
// as `gotest spec --check-fresh` judges them by each package's input hash.
// The CLI decides; this reads its answer and words it for the panel.

export type Freshness = "current" | "stale" | "unknown";

// The spec flags that ask the CLI to judge a stream's packages and print
// nothing else of note: every status hidden, so no rows are written.
export const CHECK_FRESH_ARGS = [
  "spec",
  "--input=-",
  "--render-only",
  "--check-fresh",
  "--format=json",
  "--hide=fail,none,pass,skip",
];

// The label a stale package carries, as the CLI's own mark says it.
export const STALE_LABEL = "stale: inputs changed since the run";

export const UNKNOWN_TITLE =
  "Freshness unknown: the run stamped no input hash on this package's verdict";

// parseFreshness reads what --check-fresh printed: each package's
// freshness. Undefined when stdout is not the document.
export function parseFreshness(
  stdout: string,
): Map<string, Freshness> | undefined {
  let doc: unknown;
  try {
    doc = JSON.parse(stdout.trim());
  } catch {
    return undefined;
  }
  const packages = (doc as { packages?: unknown })?.packages;
  if (!Array.isArray(packages)) return undefined;
  const out = new Map<string, Freshness>();
  for (const p of packages) {
    if (!p || typeof p !== "object") continue;
    const { path, freshness } = p as { path?: unknown; freshness?: unknown };
    if (typeof path !== "string") continue;
    if (
      freshness === "current" ||
      freshness === "stale" ||
      freshness === "unknown"
    )
      out.set(path, freshness);
  }
  return out;
}

// staleHeadline is the panel's count of stale packages among those the CLI
// judged by a hash; undefined when none is stale.
export function staleHeadline(
  paths: string[],
  freshnessOf: (pkg: string) => Freshness | undefined,
): string | undefined {
  let stale = 0;
  let judged = 0;
  for (const p of paths) {
    const f = freshnessOf(p);
    if (f === "stale") stale++;
    if (f === "stale" || f === "current") judged++;
  }
  if (stale === 0) return undefined;
  const noun = judged === 1 ? "package" : "packages";
  return `${stale} of ${judged} ${noun} stale: their inputs changed since the run`;
}
