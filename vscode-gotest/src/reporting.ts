import * as vscode from "vscode";
import type { CoverageStore } from "./coverageStore.js";
import type { DiscoveryCache } from "./discovery.js";
import type { TestResultStore } from "./testResultStore.js";

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

export async function copyTestResults(
  controllerItems: vscode.TestItemCollection,
  resultStore: TestResultStore,
  findItem: (id: string) => vscode.TestItem | undefined,
  rootItem?: vscode.TestItem,
): Promise<void> {
  type Agg = {
    passed: number;
    failed: number;
    skipped: number;
    duration: number;
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
      item.id.startsWith("dir:") || item.id.startsWith("wsFolder:") || item.tags.some((t) => t.id === "package");
    const result = structural ? undefined : resultStore.get(item.id);

    const rowIdx = rows.length;
    rows.push({
      label: "  ".repeat(indent) + item.label,
      duration: result?.duration,
      status: result?.status,
    });

    const childAgg: Agg = { passed: 0, failed: 0, skipped: 0, duration: 0 };
    item.children.forEach((child) => {
      const ca = walkItem(child, indent + 1);
      childAgg.passed += ca.passed;
      childAgg.failed += ca.failed;
      childAgg.skipped += ca.skipped;
      childAgg.duration += ca.duration;
    });

    if (item.children.size > 0) {
      rows[rowIdx].agg = childAgg;
      return childAgg;
    }

    const leafAgg: Agg = { passed: 0, failed: 0, skipped: 0, duration: 0 };
    if (result?.status === "pass") leafAgg.passed = 1;
    else if (result?.status === "fail") leafAgg.failed = 1;
    else if (result?.status === "skip") leafAgg.skipped = 1;
    if (result?.duration) leafAgg.duration = result.duration;
    return leafAgg;
  };

  const resolved = rootItem ? findItem(rootItem.id) : undefined;
  if (resolved) {
    walkItem(resolved, 0);
  } else {
    controllerItems.forEach((item) => walkItem(item, 0));
  }

  if (rows.length === 0) {
    vscode.window.showInformationMessage(
      "No test items available. Run discovery first.",
    );
    return;
  }

  const maxLabelLen = Math.max(4, ...rows.map((r) => r.label.length));
  const header = `${"Test".padEnd(maxLabelLen)}  Time       Result`;
  const separator = "-".repeat(header.length);

  const lines = [header, separator];
  let totalPassed = 0;
  let totalFailed = 0;
  let totalSkipped = 0;
  let totalDuration = 0;

  for (const row of rows) {
    if (row.agg) {
      const a = row.agg;
      const aggTime =
        a.duration > 0 ? (a.duration / 1000).toFixed(3) + "s" : "-";
      const parts: string[] = [];
      if (a.passed > 0) parts.push(`${a.passed} passed`);
      if (a.failed > 0) parts.push(`${a.failed} failed`);
      if (a.skipped > 0) parts.push(`${a.skipped} skipped`);
      const aggSummary = parts.length > 0 ? parts.join(", ") : "-";
      lines.push(
        `${row.label.padEnd(maxLabelLen)}  ${aggTime.padEnd(9)}  ${aggSummary}`,
      );
      continue;
    }

    const time =
      row.duration !== undefined ? (row.duration / 1000).toFixed(3) + "s" : "-";
    const status = row.status ?? "-";
    lines.push(
      `${row.label.padEnd(maxLabelLen)}  ${time.padEnd(9)}  ${status}`,
    );

    if (row.status === "pass") totalPassed++;
    else if (row.status === "fail") totalFailed++;
    else if (row.status === "skip") totalSkipped++;
    if (row.duration) totalDuration += row.duration;
  }

  lines.push(separator);
  const hasResults = totalPassed + totalFailed + totalSkipped > 0;
  if (hasResults) {
    const parts: string[] = [];
    if (totalPassed > 0) parts.push(`${totalPassed} passed`);
    if (totalFailed > 0) parts.push(`${totalFailed} failed`);
    if (totalSkipped > 0) parts.push(`${totalSkipped} skipped`);
    lines.push(
      `Total: ${parts.join(", ")} (${(totalDuration / 1000).toFixed(3)}s)`,
    );
  } else {
    lines.push("Total: no results");
  }

  const text = lines.join("\n");
  await vscode.env.clipboard.writeText(text);
  vscode.window.showInformationMessage("Test results copied to clipboard.");
}
