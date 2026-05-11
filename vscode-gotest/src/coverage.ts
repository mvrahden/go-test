import * as vscode from "vscode";
import * as path from "node:path";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { CoverageStore } from "./coverageStore.js";
import { scopedConfig } from "./cli.js";
import { collectItems, groupByPackage, buildRunFilter, resolvePackageItems } from "./runnerUtils.js";
import { executeBatch } from "./batchRunner.js";

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

    const fc = new vscode.FileCoverage(uri, stmtCount);
    coverages.push(fc);

    const fileDetails: vscode.FileCoverageDetail[] = [...entry.statements];
    if (decls && decls.length > 0) {
      fileDetails.push(...decls);
    }
    details.set(entry.absPath, fileDetails);
  }

  return { coverages, details };
}

export function filterSupplementaryProfiles(
  primary: ParsedFileCoverage[],
  supplementary: ParsedFileCoverage[],
): ParsedFileCoverage[] {
  if (primary.length === 0) return supplementary;
  const scope = new Set(primary.map((p) => p.absPath));
  return supplementary.filter((p) => scope.has(p.absPath));
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
        const r = stmt.location as vscode.Range;
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

export class CoverageRunner implements vscode.Disposable {
  private activeRun: vscode.CancellationTokenSource | undefined;

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

      resolvePackageItems(run, items, this.controller);

      const { coverages: allCoverages } = this.store.buildFileCoverages(this.cache);
      for (const fc of allCoverages) {
        run.addCoverage(fc);
      }
      await this.store.save();
      await this.controller.saveResults();

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
    const result = await executeBatch({
      pkgInfos,
      filter,
      workspaceDir,
      testFlags,
      run,
      token,
      controller: this.controller,
      outputChannel: this.outputChannel,
      label: "coverage",
      coverage: { store: this.store, testOnly },
      onResults: (applied) => {
        for (const r of applied) {
          this.controller.recordResult(r.itemId, r.status, r.duration);
        }
      },
    });
    return result.stdout;
  }

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
  }
}
