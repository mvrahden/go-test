// benchHover renders a benchmark method's run-over-run trend on hover: the
// stored history for this host's platform, drawn as a unicode sparkline with
// the endpoints spelled out. Pure display — every number shown was measured
// by the CLI and recorded verbatim.

import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import {
  hostPlatform,
  type BenchResultStore,
  type BenchEntry,
  type BenchHistoryPoint,
} from "./benchResultStore.js";
import { formatNsPerOp, formatAge } from "./benchReport.js";

const SPARK_BLOCKS = "▁▂▃▄▅▆▇█";
const SPARK_POINTS = 20;

/** sparkline draws values (oldest first) scaled to their own min..max. */
export function sparkline(values: number[]): string {
  if (values.length === 0) return "";
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min;
  return values
    .map((v) => {
      const idx =
        span === 0
          ? 0
          : Math.min(
              SPARK_BLOCKS.length - 1,
              Math.floor(((v - min) / span) * SPARK_BLOCKS.length),
            );
      return SPARK_BLOCKS[idx];
    })
    .join("");
}

/**
 * buildBenchHoverMarkdown renders the hover body, or undefined when there is
 * nothing recorded for this method on this platform.
 */
export function buildBenchHoverMarkdown(
  methodName: string,
  entry: BenchEntry | undefined,
  history: BenchHistoryPoint[],
  now: number = Date.now(),
): string | undefined {
  if (!entry) return undefined;

  const lines: string[] = [];
  const spread =
    entry.sampleCount > 1 && entry.nsPerOp > 0
      ? ` ±${((((entry.maxNsPerOp - entry.minNsPerOp) / 2) * 100) / entry.nsPerOp).toFixed(1)}%`
      : "";
  lines.push(
    `**${methodName}** — ${formatNsPerOp(entry.nsPerOp)}${spread}` +
      (entry.sampleCount > 1 ? ` (mean of ${entry.sampleCount}×)` : "") +
      ` · ${formatAge(entry.recordedAt, now)}`,
  );
  lines.push("");
  lines.push(
    `${entry.bytesPerOp} B/op · ${entry.allocsPerOp} allocs/op · ${entry.goos}/${entry.goarch}`,
  );

  if (history.length > 1) {
    const recent = history.slice(-SPARK_POINTS);
    const values = recent.map((p) => p.nsPerOp);
    lines.push("");
    lines.push(`Trend (last ${recent.length} runs): \`${sparkline(values)}\``);
    lines.push(
      `${formatNsPerOp(values[0])} → ${formatNsPerOp(values[values.length - 1])}`,
    );
  }

  return lines.join("\n");
}

export class BenchHoverProvider implements vscode.HoverProvider {
  constructor(
    private readonly cache: DiscoveryCache,
    private readonly store: BenchResultStore,
  ) {}

  provideHover(
    document: vscode.TextDocument,
    position: vscode.Position,
  ): vscode.Hover | undefined {
    const importPath = this.cache.resolveFileToPackage(document.fileName);
    if (!importPath) return undefined;
    const pkg = this.cache.getPackage(importPath);
    if (!pkg) return undefined;

    const platform = hostPlatform();
    for (const suite of pkg.suites) {
      for (const bench of suite.benchmarks ?? []) {
        if (
          path.join(pkg.dir, bench.file) !== document.fileName ||
          bench.line - 1 !== position.line
        ) {
          continue;
        }
        const markdown = buildBenchHoverMarkdown(
          bench.name,
          this.store.getLatest(importPath, suite.name, bench.name, platform),
          this.store.getHistory(importPath, suite.name, bench.name, platform),
        );
        if (!markdown) return undefined;
        return new vscode.Hover(new vscode.MarkdownString(markdown));
      }
    }
    return undefined;
  }
}
