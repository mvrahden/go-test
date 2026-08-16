import * as vscode from "vscode";
import type { CoverageStore } from "./coverageStore.js";
import type { DiscoveryCache } from "./discovery.js";
import type { TestResult, TestResultStore } from "./testResultStore.js";

export async function copyCoverageSummary(
  store: CoverageStore,
  cache: DiscoveryCache,
): Promise<void> {
  const { coverages } = store.buildFileCoverages(cache);
  const sourceUris = await vscode.workspace.findFiles(
    "**/*.go",
    "**/*_test.go",
  );

  if (coverages.length === 0 && sourceUris.length === 0) {
    vscode.window.showInformationMessage(
      "No coverage data available. Run tests with coverage first.",
    );
    return;
  }

  const profileAbsPaths = new Set(coverages.map((fc) => fc.uri.fsPath));

  type Node = {
    children: Map<string, Node>;
    covered: number;
    total: number;
    isFile: boolean;
    sourceFiles: number;
    profileFiles: number;
  };

  const mkNode = (isFile = false): Node => ({
    children: new Map(),
    covered: 0,
    total: 0,
    isFile,
    sourceFiles: 0,
    profileFiles: 0,
  });

  const root = mkNode();

  const ensureDir = (parts: string[]): Node => {
    let node = root;
    for (const part of parts) {
      if (!node.children.has(part)) {
        node.children.set(part, mkNode());
      }
      node = node.children.get(part)!;
    }
    return node;
  };

  const isMultiFolder = (vscode.workspace.workspaceFolders?.length ?? 0) > 1;

  const relativize = (fsPath: string): string => {
    const uri = vscode.Uri.file(fsPath);
    const folder = vscode.workspace.getWorkspaceFolder(uri);
    if (folder && fsPath.startsWith(folder.uri.fsPath)) {
      const rel = fsPath.slice(folder.uri.fsPath.length + 1);
      return isMultiFolder ? `${folder.name}/${rel}` : rel;
    }
    return fsPath;
  };

  for (const uri of sourceUris) {
    const relPath = relativize(uri.fsPath);
    const parts = relPath.split("/");
    parts.pop();
    const dir = ensureDir(parts);
    dir.sourceFiles++;
    if (profileAbsPaths.has(uri.fsPath)) {
      dir.profileFiles++;
    }
  }

  for (const fc of coverages) {
    const relPath = relativize(fc.uri.fsPath);
    const parts = relPath.split("/");
    const fileName = parts.pop()!;
    const dir = ensureDir(parts);
    dir.children.set(fileName, {
      ...mkNode(true),
      covered: fc.statementCoverage.covered,
      total: fc.statementCoverage.total,
    });
  }

  const computeAggregates = (node: Node): void => {
    if (node.isFile) return;
    let covered = 0;
    let total = 0;
    let srcFiles = node.sourceFiles;
    let profFiles = node.profileFiles;
    for (const child of node.children.values()) {
      computeAggregates(child);
      covered += child.covered;
      total += child.total;
      if (!child.isFile) {
        srcFiles += child.sourceFiles;
        profFiles += child.profileFiles;
      }
    }
    node.covered = covered;
    node.total = total;
    node.sourceFiles = srcFiles;
    node.profileFiles = profFiles;
  };
  computeAggregates(root);

  const compress = (node: Node): void => {
    let changed = true;
    while (changed) {
      changed = false;
      for (const [key, child] of [...node.children]) {
        if (!child.isFile && child.children.size === 1) {
          const [gKey, grandchild] = [...child.children][0];
          if (!grandchild.isFile) {
            node.children.delete(key);
            grandchild.covered = child.covered;
            grandchild.total = child.total;
            grandchild.sourceFiles = child.sourceFiles;
            grandchild.profileFiles = child.profileFiles;
            node.children.set(key + "/" + gKey, grandchild);
            changed = true;
            break;
          }
        }
      }
    }
    for (const child of node.children.values()) {
      if (!child.isFile) compress(child);
    }
  };
  compress(root);

  type OutputRow = {
    label: string;
    stmts: string;
    pct: string;
    files: string;
  };
  const outputRows: OutputRow[] = [];
  const fmtPct = (covered: number, total: number): string =>
    total > 0 ? ((covered / total) * 100).toFixed(1) + "%" : "—";
  const fmtFiles = (profile: number, source: number): string =>
    source > 0 ? `(${profile} of ${source} files)` : "";

  const renderNode = (node: Node, indent: number): void => {
    const sorted = [...node.children.entries()].sort((a, b) => {
      if (a[1].isFile !== b[1].isFile) return a[1].isFile ? 1 : -1;
      return a[0].localeCompare(b[0]);
    });
    for (const [name, child] of sorted) {
      outputRows.push({
        label: "  ".repeat(indent) + name,
        stmts: `${child.covered}/${child.total}`,
        pct: fmtPct(child.covered, child.total),
        files: child.isFile
          ? ""
          : fmtFiles(child.profileFiles, child.sourceFiles),
      });
      if (!child.isFile) {
        renderNode(child, indent + 1);
      }
    }
  };
  renderNode(root, 0);

  const maxLabelLen = Math.max(4, ...outputRows.map((r) => r.label.length));
  const maxStmtsLen = Math.max(5, ...outputRows.map((r) => r.stmts.length));
  const maxPctLen = Math.max(5, ...outputRows.map((r) => r.pct.length));
  const header = `${"File".padEnd(maxLabelLen)}  ${"Stmts".padEnd(maxStmtsLen)}  ${"Cover".padEnd(maxPctLen)}  Files`;
  const separator = "-".repeat(header.length);

  const lines = [header, separator];
  for (const row of outputRows) {
    let line = `${row.label.padEnd(maxLabelLen)}  ${row.stmts.padEnd(maxStmtsLen)}  ${row.pct.padEnd(maxPctLen)}`;
    if (row.files) line += `  ${row.files}`;
    lines.push(line);
  }

  lines.push(separator);
  const totalFiles =
    root.sourceFiles > 0 ? fmtFiles(root.profileFiles, root.sourceFiles) : "";
  let totalLine = `${"Total".padEnd(maxLabelLen)}  ${`${root.covered}/${root.total}`.padEnd(maxStmtsLen)}  ${fmtPct(root.covered, root.total).padEnd(maxPctLen)}`;
  if (totalFiles) totalLine += `  ${totalFiles}`;
  lines.push(totalLine);

  const text = lines.join("\n");
  await vscode.env.clipboard.writeText(text);
  vscode.window.showInformationMessage("Coverage summary copied to clipboard.");
}

/** Every duration in the table is printed in seconds, to three decimals. */
function fmtSeconds(ms: number): string {
  return `${(ms / 1000).toFixed(3)}s`;
}

interface Window {
  start: number;
  end: number;
}

/**
 * Where a node sat on the clock. A parked node declines: its measure says how
 * long it ran but neither endpoint says when, so placing it would put a window
 * in the wrong part of the timeline.
 */
function windowOf(result: TestResult | undefined): Window | undefined {
  if (!result || result.paused) return undefined;
  if (result.startedAt === undefined || result.endedAt === undefined) {
    return undefined;
  }
  if (result.endedAt < result.startedAt) return undefined;
  return { start: result.startedAt, end: result.endedAt };
}

/**
 * What a node cost. Which of its two measures to believe follows from why each
 * one lies. A node that called t.Parallel reports what go test measured, because
 * its bracket carries both the wait for a slot and go test's habit of flushing a
 * parked test's report through its parent — which can delay the end by however
 * long a slower sibling runs. Everything else reports its bracket, because a
 * measure that stops when the function returns cannot see parallel children,
 * while the bracket encloses the whole subtree.
 */
function effectiveOf(result: TestResult | undefined): number | undefined {
  if (!result) return undefined;
  if (result.paused) return result.duration;
  const own = windowOf(result);
  return own ? own.end - own.start : result.duration;
}

/**
 * Total clock covered by the windows, counting overlap once. This is what makes
 * a row assembled out of several nodes honest: two suites that ran side by side
 * for a second cost a second, not two. It also drops the gap between windows,
 * so a tree still holding results from an earlier run reports the time that run
 * spent rather than the hours since.
 */
function occupied(windows: Window[]): number {
  if (windows.length === 0) return 0;
  const sorted = [...windows].sort((a, b) => a.start - b.start);
  let total = 0;
  let cur = { ...sorted[0] };
  for (const w of sorted.slice(1)) {
    if (w.start > cur.end) {
      total += cur.end - cur.start;
      cur = { ...w };
      continue;
    }
    if (w.end > cur.end) cur.end = w.end;
  }
  return total + (cur.end - cur.start);
}

// Exported for tests; copyTestResults is the thin clipboard wrapper.
export function buildTestResultsTable(
  controllerItems: vscode.TestItemCollection,
  resultStore: TestResultStore,
  findItem: (id: string) => vscode.TestItem | undefined,
  rootItem?: vscode.TestItem,
): string | undefined {
  type Agg = {
    passed: number;
    failed: number;
    skipped: number;
    // What this row displayed, so a parent without timestamps can still fall
    // back to summing; the longest single child, and the windows this row
    // contributes upward.
    duration: number;
    maxChild: number;
    windows: Window[];
  };
  type Row = {
    label: string;
    duration?: number;
    status?: string;
    agg?: Agg;
  };
  const rows: Row[] = [];

  const walkItem = (item: vscode.TestItem, indent: number): Agg => {
    const structural =
      item.id.startsWith("dir:") ||
      item.id.startsWith("wsFolder:") ||
      item.tags.some((t) => t.id === "package");
    const result = structural ? undefined : resultStore.get(item.id);

    const rowIdx = rows.length;
    rows.push({
      label: "  ".repeat(indent) + item.label,
      duration: effectiveOf(result),
      status: result?.status,
    });

    const childAgg: Agg = {
      passed: 0,
      failed: 0,
      skipped: 0,
      duration: 0,
      maxChild: 0,
      windows: [],
    };
    item.children.forEach((child) => {
      const ca = walkItem(child, indent + 1);
      childAgg.passed += ca.passed;
      childAgg.failed += ca.failed;
      childAgg.skipped += ca.skipped;
      childAgg.duration += ca.duration;
      childAgg.maxChild = Math.max(childAgg.maxChild, ca.duration);
      childAgg.windows.push(...ca.windows);
    });

    const isLeaf = item.children.size === 0;
    const own = windowOf(result);

    if (!isLeaf) {
      // A directory or a package is not something that executes and has no
      // bracket of its own, so it is worth the clock during which something
      // under it was running. A node that does execute reports what it cost,
      // including any idle it spent waiting — that idle is its cost.
      let displayed = effectiveOf(result);
      if (displayed === undefined) {
        if (childAgg.windows.length > 0) {
          displayed = occupied(childAgg.windows);
        } else {
          // No timestamps anywhere: a hand-built tree, or results recorded from
          // a stream without them. Bound the subtree by both measures.
          const bound = Math.max(result?.duration ?? 0, childAgg.duration);
          displayed = bound > 0 ? bound : undefined;
        }
      }

      // A child runs inside its parent, so the parent is at least as long as
      // any single one of them and as the clock they occupied between them.
      // Flooring by that repairs the rounding go test applies to a parked
      // node's measure — it reports to 10ms, while a child's bracket is exact
      // — and can never invent time, since neither bound can exceed what the
      // parent actually ran for.
      const floor = Math.max(childAgg.maxChild, occupied(childAgg.windows));
      if (displayed !== undefined && floor > displayed) displayed = floor;

      rows[rowIdx].agg = childAgg;
      rows[rowIdx].duration = displayed;
      return {
        ...childAgg,
        duration: displayed ?? 0,
        // Pass the real nodes' brackets up rather than this row's total: a
        // union can have holes, and an ancestor has to union the pieces itself.
        // A parked node has no window to pass, so its children's stand in.
        windows: own ? [own] : childAgg.windows,
      };
    }

    const leafAgg: Agg = {
      passed: 0,
      failed: 0,
      skipped: 0,
      duration: effectiveOf(result) ?? 0,
      maxChild: 0,
      windows: own ? [own] : [],
    };
    if (result?.status === "pass") leafAgg.passed = 1;
    else if (result?.status === "fail") leafAgg.failed = 1;
    else if (result?.status === "skip") leafAgg.skipped = 1;
    return leafAgg;
  };

  const roots: Agg[] = [];
  const resolved = rootItem ? findItem(rootItem.id) : undefined;
  if (resolved) {
    roots.push(walkItem(resolved, 0));
  } else {
    controllerItems.forEach((item) => roots.push(walkItem(item, 0)));
  }

  // The total is the clock the run occupied, not the sum of the rows: rows
  // overlap whenever anything ran in parallel, and summing them would report
  // more time than passed.
  const rootWindows = roots.flatMap((r) => r.windows);
  const rootDuration =
    rootWindows.length > 0
      ? occupied(rootWindows)
      : roots.reduce((a, r) => a + r.duration, 0);

  if (rows.length === 0) {
    return undefined;
  }

  const maxLabelLen = Math.max(4, ...rows.map((r) => r.label.length));
  const header = `${"Test".padEnd(maxLabelLen)}  Time       Result`;
  const separator = "-".repeat(header.length);

  const lines = [header, separator];
  let totalPassed = 0;
  let totalFailed = 0;
  let totalSkipped = 0;

  for (const row of rows) {
    const time = row.duration !== undefined ? fmtSeconds(row.duration) : "-";

    if (row.agg) {
      const a = row.agg;
      const parts: string[] = [];
      if (a.passed > 0) parts.push(`${a.passed} passed`);
      if (a.failed > 0) parts.push(`${a.failed} failed`);
      if (a.skipped > 0) parts.push(`${a.skipped} skipped`);
      const aggSummary = parts.length > 0 ? parts.join(", ") : "-";
      lines.push(
        `${row.label.padEnd(maxLabelLen)}  ${time.padEnd(9)}  ${aggSummary}`,
      );
      continue;
    }

    const status = row.status ?? "-";
    lines.push(
      `${row.label.padEnd(maxLabelLen)}  ${time.padEnd(9)}  ${status}`,
    );

    if (row.status === "pass") totalPassed++;
    else if (row.status === "fail") totalFailed++;
    else if (row.status === "skip") totalSkipped++;
  }

  lines.push(separator);
  const hasResults = totalPassed + totalFailed + totalSkipped > 0;
  if (hasResults) {
    const parts: string[] = [];
    if (totalPassed > 0) parts.push(`${totalPassed} passed`);
    if (totalFailed > 0) parts.push(`${totalFailed} failed`);
    if (totalSkipped > 0) parts.push(`${totalSkipped} skipped`);
    lines.push(`Total: ${parts.join(", ")} (${fmtSeconds(rootDuration)})`);
  } else {
    lines.push("Total: no results");
  }

  return lines.join("\n");
}

export async function copyTestResults(
  controllerItems: vscode.TestItemCollection,
  resultStore: TestResultStore,
  findItem: (id: string) => vscode.TestItem | undefined,
  rootItem?: vscode.TestItem,
): Promise<void> {
  const text = buildTestResultsTable(
    controllerItems,
    resultStore,
    findItem,
    rootItem,
  );
  if (text === undefined) {
    vscode.window.showInformationMessage(
      "No test items available. Run discovery first.",
    );
    return;
  }

  await vscode.env.clipboard.writeText(text);
  vscode.window.showInformationMessage("Test results copied to clipboard.");
}
