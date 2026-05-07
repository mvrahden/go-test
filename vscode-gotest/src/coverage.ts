import * as vscode from "vscode";
import * as path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { CoverageStore } from "./coverageStore.js";
import { parseTestEvents, groupEventsByPackage } from "./outputParser.js";
import {
  buildCliCommand,
  formatCliCommand,
  resolveGoBinary,
  scopedConfig,
} from "./cli.js";
import {
  collectItems,
  groupByPackage,
  applyResults,
  spawnTestProcess,
  buildRunFilter,
  computeWildcard,
} from "./runnerUtils.js";

const execFileAsync = promisify(execFile);

export interface ParsedFileCoverage {
  absPath: string;
  statements: vscode.StatementCoverage[];
  numStatements: number[];
}

export function parseCoverProfile(
  content: string,
  moduleToDir: (importPath: string) => string | undefined,
): ParsedFileCoverage[] {
  const lines = content.split("\n");
  const fileEntries = new Map<
    string,
    { statements: vscode.StatementCoverage[]; numStatements: number[] }
  >();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("mode:")) {
      continue;
    }

    const match = /^(.+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$/.exec(
      trimmed,
    );
    if (!match) {
      continue;
    }

    const filePath = match[1];
    const startLine = parseInt(match[2], 10) - 1;
    const startCol = parseInt(match[3], 10) - 1;
    const endLine = parseInt(match[4], 10) - 1;
    const endCol = parseInt(match[5], 10) - 1;
    const numStatements = parseInt(match[6], 10);
    const count = parseInt(match[7], 10);

    let entry = fileEntries.get(filePath);
    if (!entry) {
      entry = { statements: [], numStatements: [] };
      fileEntries.set(filePath, entry);
    }

    const range = new vscode.Range(
      new vscode.Position(startLine, startCol),
      new vscode.Position(endLine, endCol),
    );
    entry.statements.push(
      new vscode.StatementCoverage(count > 0 ? count : false, range),
    );
    entry.numStatements.push(numStatements);
  }

  const result: ParsedFileCoverage[] = [];
  for (const [importFilePath, entry] of fileEntries) {
    const lastSlash = importFilePath.lastIndexOf("/");
    const fileName = importFilePath.slice(lastSlash + 1);
    const importDir = importFilePath.slice(0, lastSlash);
    const absDir = moduleToDir(importDir);
    if (!absDir) {
      continue;
    }
    const absPath = path.join(absDir, fileName);
    result.push({
      absPath,
      statements: entry.statements,
      numStatements: entry.numStatements,
    });
  }

  return result;
}

export interface CoverageResult {
  coverages: vscode.FileCoverage[];
  details: Map<string, vscode.FileCoverageDetail[]>;
}

export function buildFileCoverages(
  parsed: ParsedFileCoverage[],
  declarations?: Map<string, vscode.DeclarationCoverage[]>,
): CoverageResult {
  const coverages: vscode.FileCoverage[] = [];
  const details = new Map<string, vscode.FileCoverageDetail[]>();

  for (const entry of parsed) {
    const uri = vscode.Uri.file(entry.absPath);
    const decls = declarations?.get(entry.absPath);

    let covered = 0;
    let total = 0;
    for (let i = 0; i < entry.statements.length; i++) {
      const ns = entry.numStatements[i];
      total += ns;
      const exec = entry.statements[i].executed;
      if ((typeof exec === "number" && exec > 0) || exec === true) {
        covered += ns;
      }
    }
    const stmtCount = new vscode.TestCoverageCount(covered, total);

    let declCount: vscode.TestCoverageCount | undefined;
    if (decls && decls.length > 0) {
      let declCovered = 0;
      for (const d of decls) {
        if (
          (typeof d.executed === "number" && d.executed > 0) ||
          d.executed === true
        ) {
          declCovered++;
        }
      }
      declCount = new vscode.TestCoverageCount(declCovered, decls.length);
    }

    const fc = new vscode.FileCoverage(uri, stmtCount, undefined, declCount);
    coverages.push(fc);

    const fileDetails: vscode.FileCoverageDetail[] = [...entry.statements];
    if (decls) {
      fileDetails.push(...decls);
    }
    details.set(entry.absPath, fileDetails);
  }

  return { coverages, details };
}

export function deduplicateProfiles(
  profiles: ParsedFileCoverage[],
): ParsedFileCoverage[] {
  const byFile = new Map<string, ParsedFileCoverage[]>();
  for (const p of profiles) {
    const list = byFile.get(p.absPath) ?? [];
    list.push(p);
    byFile.set(p.absPath, list);
  }

  const result: ParsedFileCoverage[] = [];
  for (const [absPath, entries] of byFile) {
    if (entries.length === 1) {
      result.push(entries[0]);
      continue;
    }

    const blocks = new Map<
      string,
      { stmt: vscode.StatementCoverage; numStmts: number }
    >();
    for (const entry of entries) {
      for (let i = 0; i < entry.statements.length; i++) {
        const stmt = entry.statements[i];
        const ns = entry.numStatements[i];
        const r = stmt.location;
        const key = `${r.start.line}:${r.start.character},${r.end.line}:${r.end.character}`;

        const prev = blocks.get(key);
        if (
          !prev ||
          executedToCount(stmt.executed) > executedToCount(prev.stmt.executed)
        ) {
          blocks.set(key, { stmt, numStmts: ns });
        }
      }
    }

    const statements: vscode.StatementCoverage[] = [];
    const numStatements: number[] = [];
    for (const { stmt, numStmts } of blocks.values()) {
      statements.push(stmt);
      numStatements.push(numStmts);
    }
    result.push({ absPath, statements, numStatements });
  }

  return result;
}

function executedToCount(executed: number | boolean): number {
  if (typeof executed === "number") return executed;
  return executed ? 1 : 0;
}

export function parseFuncCoverage(
  content: string,
  moduleToDir: (importPath: string) => string | undefined,
): Map<string, vscode.DeclarationCoverage[]> {
  const result = new Map<string, vscode.DeclarationCoverage[]>();

  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("total:")) {
      continue;
    }

    const match = /^(.+):(\d+):\s+(\S+)\s+(\d+(?:\.\d+)?)%$/.exec(trimmed);
    if (!match) {
      continue;
    }

    const filePath = match[1];
    const lineNum = parseInt(match[2], 10) - 1;
    const funcName = match[3];
    const pct = parseFloat(match[4]);

    const lastSlash = filePath.lastIndexOf("/");
    const fileName = filePath.slice(lastSlash + 1);
    const importDir = filePath.slice(0, lastSlash);
    const absDir = moduleToDir(importDir);
    if (!absDir) {
      continue;
    }
    const absPath = path.join(absDir, fileName);

    let declarations = result.get(absPath);
    if (!declarations) {
      declarations = [];
      result.set(absPath, declarations);
    }

    const executed = pct > 0 ? pct / 100 : false;
    const position = new vscode.Position(lineNum, 0);
    declarations.push(
      new vscode.DeclarationCoverage(funcName, executed, position),
    );
  }

  return result;
}

export function splitCoverByPackage(
  content: string,
  importPaths: string[],
): Map<string, string> {
  const lines = content.split("\n");
  const buckets = new Map<string, string[]>();
  let modeLine = "";

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    if (trimmed.startsWith("mode:")) {
      modeLine = trimmed;
      continue;
    }
    for (const ip of importPaths) {
      if (trimmed.startsWith(ip + "/")) {
        let bucket = buckets.get(ip);
        if (!bucket) {
          bucket = [];
          buckets.set(ip, bucket);
        }
        bucket.push(trimmed);
        break;
      }
    }
  }

  const result = new Map<string, string>();
  for (const [ip, pkgLines] of buckets) {
    result.set(ip, modeLine + "\n" + pkgLines.join("\n") + "\n");
  }
  return result;
}

export function splitFuncCoverageByPackage(
  content: string,
  importPaths: string[],
): Map<string, string> {
  const lines = content.split("\n");
  const buckets = new Map<string, string[]>();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("total:")) continue;
    for (const ip of importPaths) {
      if (trimmed.startsWith(ip + "/")) {
        let bucket = buckets.get(ip);
        if (!bucket) {
          bucket = [];
          buckets.set(ip, bucket);
        }
        bucket.push(trimmed);
        break;
      }
    }
  }

  const result = new Map<string, string>();
  for (const [ip, pkgLines] of buckets) {
    result.set(ip, pkgLines.join("\n") + "\n");
  }
  return result;
}

export async function runGoToolCoverFunc(
  coverFile: string,
  workspaceDir: string,
): Promise<string> {
  const goBin = await resolveGoBinary(undefined, workspaceDir);
  const { stdout } = await execFileAsync(
    goBin,
    ["tool", "cover", `-func=${coverFile}`],
    {
      cwd: workspaceDir,
      timeout: 10_000,
    },
  );
  return stdout;
}

export class CoverageRunner implements vscode.Disposable {
  private activeRun: vscode.CancellationTokenSource | undefined;
  private activePackageRun: vscode.CancellationTokenSource | undefined;
  private packageRunQueue = Promise.resolve();
  private pendingPackages = new Set<string>();

  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly store: CoverageStore,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onJsonOutput: (json: string) => void,
  ) {}

  async run(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
    if (this.activeRun) {
      this.outputChannel.appendLine("[coverage] cancelling previous run");
      this.activeRun.cancel();
    }
    const cts = new vscode.CancellationTokenSource();
    this.activeRun = cts;
    const cancelSub = token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Coverage");

    try {
      const items = collectItems(this.controller, request);
      if (items.length === 0) {
        run.end();
        return;
      }

      for (const item of items) {
        this.controller.clearResults(item);
        run.started(item);
      }

      const groups = groupByPackage(items);
      let allJsonOutput = "";

      interface PkgInfo {
        importPath: string;
        items: vscode.TestItem[];
        dir: string;
        workspaceDir: string;
        filter: string | undefined;
        testOnly: boolean;
      }

      const validPkgs: PkgInfo[] = [];

      for (const [importPath, groupItems] of groups) {
        const pkg = this.cache.getPackage(importPath);
        if (!pkg) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(`Package not found: ${importPath}`),
            );
          }
          continue;
        }

        const workspaceDir = this.cache.getWorkspaceDir(importPath);
        if (!workspaceDir) {
          for (const item of groupItems) {
            run.errored(
              item,
              new vscode.TestMessage(
                `Workspace folder not found for: ${importPath}`,
              ),
            );
          }
          continue;
        }

        const filter = buildRunFilter(groupItems);
        validPkgs.push({
          importPath,
          items: groupItems,
          dir: pkg.dir,
          workspaceDir,
          filter,
          testOnly: pkg.testOnly ?? false,
        });
      }

      const byWorkspace = new Map<string, PkgInfo[]>();
      for (const info of validPkgs) {
        let list = byWorkspace.get(info.workspaceDir);
        if (!list) {
          list = [];
          byWorkspace.set(info.workspaceDir, list);
        }
        list.push(info);
      }

      for (const [workspaceDir, pkgInfos] of byWorkspace) {
        if (effectiveToken.isCancellationRequested) {
          for (const info of pkgInfos) {
            for (const item of info.items) {
              run.skipped(item);
            }
          }
          continue;
        }

        const config = scopedConfig(workspaceDir);
        const testFlags = config.get<string[]>("testFlags") ?? [];

        const unfiltered = pkgInfos.filter((p) => !p.filter && !p.testOnly);
        const unfilteredTestOnly = pkgInfos.filter(
          (p) => !p.filter && p.testOnly,
        );
        const filtered = pkgInfos.filter((p) => p.filter);

        if (unfiltered.length > 0) {
          allJsonOutput += await this.runCoverageBatch(
            unfiltered,
            undefined,
            workspaceDir,
            testFlags,
            run,
            effectiveToken,
          );
        }

        if (unfilteredTestOnly.length > 0) {
          allJsonOutput += await this.runCoverageBatch(
            unfilteredTestOnly,
            undefined,
            workspaceDir,
            testFlags,
            run,
            effectiveToken,
            true,
          );
        }

        for (const info of filtered) {
          if (effectiveToken.isCancellationRequested) {
            for (const item of info.items) {
              run.skipped(item);
            }
            continue;
          }
          allJsonOutput += await this.runCoverageBatch(
            [info],
            info.filter,
            workspaceDir,
            testFlags,
            run,
            effectiveToken,
            info.testOnly,
          );
        }
      }

      const allCoverages = this.store.buildFileCoverages(this.cache);
      for (const fc of allCoverages) {
        run.addCoverage(fc);
      }
      await this.store.save();

      if (allJsonOutput) {
        this.onJsonOutput(allJsonOutput);
      }
    } finally {
      cancelSub.dispose();
      if (this.activeRun === cts) {
        this.activeRun = undefined;
      }
      cts.dispose();
      run.end();
    }
  }

  private async runCoverageBatch(
    pkgInfos: {
      importPath: string;
      items: vscode.TestItem[];
      dir: string;
    }[],
    filter: string | undefined,
    workspaceDir: string,
    testFlags: string[],
    run: vscode.TestRun,
    token: vscode.CancellationToken,
    testOnly?: boolean,
  ): Promise<string> {
    const importPaths = pkgInfos.map((p) => p.importPath);
    const wildcard = filter ? undefined : computeWildcard(importPaths);
    const cliPkgArgs = wildcard ? [wildcard] : importPaths;
    let coverFile: string | undefined;

    try {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");

      const cliArgs: string[] = [
        "-json",
        "-count=1",
        "-covermode=atomic",
        `-coverprofile=${coverFile}`,
        ...cliPkgArgs,
      ];
      if (testOnly) {
        cliArgs.push("-coverpkg=./...");
      }
      if (filter) {
        cliArgs.push("-run", filter);
      }
      cliArgs.push(...testFlags);

      const cmd = await buildCliCommand(
        cliArgs,
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.appendLine(`[coverage] ${formatCliCommand(cmd)}`);

      const result = await spawnTestProcess(
        cmd.bin,
        cmd.args,
        workspaceDir,
        token,
        this.outputChannel,
        "coverage",
      );

      if (token.isCancellationRequested) {
        for (const info of pkgInfos) {
          for (const item of info.items) {
            run.skipped(item);
          }
        }
        return result.stdout;
      }

      if (result.stdout) {
        const events = parseTestEvents(result.stdout);
        const eventsByPkg = groupEventsByPackage(events);

        for (const info of pkgInfos) {
          const pkgEvents = eventsByPkg.get(info.importPath) ?? [];
          applyResults(
            this.controller,
            run,
            pkgEvents,
            info.importPath,
            info.dir,
          );
        }
      } else if (result.exitCode !== 0) {
        const message =
          result.stderr.trim() || `gotest exited with code ${result.exitCode}`;
        for (const info of pkgInfos) {
          for (const item of info.items) {
            run.errored(item, new vscode.TestMessage(message));
          }
        }
      }
      try {
        const coverContent = await readFile(coverFile, "utf-8");
        let funcOutput: string | undefined;
        try {
          funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
        } catch {
          this.outputChannel.appendLine(
            "[coverage] go tool cover -func failed, skipping declaration coverage",
          );
        }

        const splitPaths = testOnly
          ? this.cache.packages.map((p) => p.importPath)
          : importPaths;
        const coverByPkg = splitCoverByPackage(coverContent, splitPaths);
        const funcByPkg = funcOutput
          ? splitFuncCoverageByPackage(funcOutput, splitPaths)
          : undefined;

        for (const info of pkgInfos) {
          const pkgCover = coverByPkg.get(info.importPath);
          if (pkgCover) {
            this.store.update(
              info.importPath,
              pkgCover,
              funcByPkg?.get(info.importPath),
            );
          }
        }

        if (testOnly) {
          for (const [ip, cover] of coverByPkg) {
            if (!importPaths.includes(ip)) {
              this.store.update(ip, cover, funcByPkg?.get(ip));
            }
          }
        }
      } catch {
        this.outputChannel.appendLine("[coverage] no coverprofile generated");
      }
      return result.stdout;
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[coverage] error: ${message}`);
      for (const info of pkgInfos) {
        for (const item of info.items) {
          run.errored(item, new vscode.TestMessage(message));
        }
      }
      return "";
    } finally {
      if (coverFile) {
        rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
          () => {},
        );
      }
    }
  }

  async copyCoverageSummary(): Promise<void> {
    const coverages = this.store.buildFileCoverages(this.cache);
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

    const relativize = (fsPath: string): string => {
      const uri = vscode.Uri.file(fsPath);
      const folder = vscode.workspace.getWorkspaceFolder(uri);
      if (folder && fsPath.startsWith(folder.uri.fsPath)) {
        return fsPath.slice(folder.uri.fsPath.length + 1);
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
      root.sourceFiles > 0
        ? fmtFiles(root.profileFiles, root.sourceFiles)
        : "";
    let totalLine = `${"Total".padEnd(maxLabelLen)}  ${`${root.covered}/${root.total}`.padEnd(maxStmtsLen)}  ${fmtPct(root.covered, root.total).padEnd(maxPctLen)}`;
    if (totalFiles) totalLine += `  ${totalFiles}`;
    lines.push(totalLine);

    const text = lines.join("\n");
    await vscode.env.clipboard.writeText(text);
    vscode.window.showInformationMessage(
      "Coverage summary copied to clipboard.",
    );
  }

  async runPackage(importPath: string): Promise<void> {
    if (this.pendingPackages.has(importPath)) return;
    this.pendingPackages.add(importPath);
    this.packageRunQueue = this.packageRunQueue.then(async () => {
      this.pendingPackages.delete(importPath);
      await this.executePackageRun(importPath);
    });
    return this.packageRunQueue;
  }

  private async executePackageRun(importPath: string): Promise<void> {
    this.activePackageRun?.cancel();
    const cts = new vscode.CancellationTokenSource();
    this.activePackageRun = cts;

    const pkg = this.cache.getPackage(importPath);
    if (!pkg) return;
    const workspaceDir = this.cache.getWorkspaceDir(importPath);
    if (!workspaceDir) return;

    const config = scopedConfig(workspaceDir);
    const testFlags = config.get<string[]>("testFlags") ?? [];
    let coverFile: string | undefined;

    try {
      const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
      coverFile = path.join(tmpDir, "cover.out");

      const cliArgs: string[] = [
        "-count=1",
        "-covermode=atomic",
        `-coverprofile=${coverFile}`,
        importPath,
      ];
      cliArgs.push(...testFlags);

      const cmd = await buildCliCommand(
        cliArgs,
        workspaceDir,
        this.outputChannel,
      );
      this.outputChannel.appendLine(`[coverage:save] ${formatCliCommand(cmd)}`);

      await spawnTestProcess(
        cmd.bin,
        cmd.args,
        workspaceDir,
        cts.token,
        this.outputChannel,
        "coverage",
      );

      if (cts.token.isCancellationRequested) return;

      const coverContent = await readFile(coverFile, "utf-8");
      let funcOutput: string | undefined;
      try {
        funcOutput = await runGoToolCoverFunc(coverFile, workspaceDir);
      } catch {
        this.outputChannel.appendLine(
          "[coverage:save] go tool cover -func failed",
        );
      }
      this.store.update(importPath, coverContent, funcOutput);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.outputChannel.appendLine(`[coverage:save] failed: ${message}`);
      return;
    } finally {
      if (this.activePackageRun === cts) this.activePackageRun = undefined;
      cts.dispose();
      if (coverFile)
        rm(path.dirname(coverFile), { recursive: true, force: true }).catch(
          () => {},
        );
    }

    const request = new vscode.TestRunRequest();
    const run = this.controller.createTestRun(request, "Cover on Save");
    const allCoverages = this.store.buildFileCoverages(this.cache);
    for (const fc of allCoverages) {
      run.addCoverage(fc);
    }
    run.end();
    await this.store.save();
  }

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
    this.activePackageRun?.cancel();
    this.activePackageRun = undefined;
  }
}
