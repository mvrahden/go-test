// benchDiagnostics raises warning squiggles on benchmark methods the CLI's
// gate condemned. Which methods breach is decided in Go (gate.breachedKeys)
// — this module only places the verdicts at their source positions, the
// same mechanism the focus warnings use.

import * as vscode from "vscode";
import * as path from "node:path";
import type { DiscoveryCache } from "./discovery.js";
import type { BenchReport } from "./benchReport.js";

/** parseDeltaKey splits the CLI's "pkg Suite/Name" delta key. */
export function parseDeltaKey(
  key: string,
): { importPath: string; suiteName: string; methodName: string } | undefined {
  const space = key.indexOf(" ");
  if (space <= 0) return undefined;
  const importPath = key.slice(0, space);
  const rest = key.slice(space + 1);
  const slash = rest.indexOf("/");
  if (slash <= 0 || slash === rest.length - 1) return undefined;
  return {
    importPath,
    suiteName: rest.slice(0, slash),
    methodName: rest.slice(slash + 1),
  };
}

export class BenchGateDiagnostics implements vscode.Disposable {
  private readonly collection: vscode.DiagnosticCollection;

  constructor(private readonly cache: DiscoveryCache) {
    this.collection =
      vscode.languages.createDiagnosticCollection("gotest-bench-gate");
  }

  /**
   * apply replaces all gate diagnostics with the ones this report carries.
   * A report without a gate — or without breaches — clears the board: the
   * verdict belongs to the latest run, not to history.
   */
  apply(report: BenchReport): void {
    this.collection.clear();
    const breached = report.gate?.breachedKeys ?? [];
    if (breached.length === 0) return;

    const byFile = new Map<string, vscode.Diagnostic[]>();
    const deltaByKey = new Map(
      (report.deltas ?? []).map((d) => [d.key, d] as const),
    );

    for (const key of breached) {
      const parsed = parseDeltaKey(key);
      if (!parsed) continue;
      const pkg = this.cache.getPackage(parsed.importPath);
      if (!pkg) continue;
      const suite = pkg.suites.find((s) => s.name === parsed.suiteName);
      const method = suite?.benchmarks.find(
        (b) => b.name === parsed.methodName,
      );
      if (!suite || !method) continue;

      const delta = deltaByKey.get(key);
      const pct = delta ? `+${Math.abs(delta.percentChange).toFixed(1)}%` : "";
      const threshold = report.gate?.thresholdPct ?? 0;
      const message = `${parsed.methodName} regressed ${pct} vs baseline — exceeds the ${threshold}% bench gate`;

      const line = method.line - 1;
      const col = method.col - 1;
      const diagnostic = new vscode.Diagnostic(
        new vscode.Range(line, col, line, col + parsed.methodName.length),
        message,
        vscode.DiagnosticSeverity.Warning,
      );
      diagnostic.source = "gotest";

      const file = path.join(pkg.dir, method.file);
      const list = byFile.get(file) ?? [];
      list.push(diagnostic);
      byFile.set(file, list);
    }

    for (const [file, diagnostics] of byFile) {
      this.collection.set(vscode.Uri.file(file), diagnostics);
    }
  }

  clear(): void {
    this.collection.clear();
  }

  dispose(): void {
    this.collection.dispose();
  }
}
