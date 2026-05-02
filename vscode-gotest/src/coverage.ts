import * as vscode from "vscode";
import * as path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { CoverageStore } from "./coverageStore.js";
import type { OverlayOutput } from "./types.js";
import { parseTestEvents } from "./outputParser.js";
import { buildCliCommand, formatCliCommand, resolveGoBinary, scopedConfig } from "./cli.js";
import {
  collectItems,
  groupByPackage,
  getItemDepth,
  applyResults,
  spawnTestProcess,
} from "./runnerUtils.js";

const execFileAsync = promisify(execFile);

export function parseCoverProfile(
  content: string,
  moduleToDir: (importPath: string) => string | undefined,
): vscode.FileCoverage[] {
  const lines = content.split("\n");
  const fileEntries = new Map<string, vscode.StatementCoverage[]>();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("mode:")) {
      continue;
    }

    const match = /^(.+):(\d+)\.(\d+),(\d+)\.(\d+)\s+(\d+)\s+(\d+)$/.exec(trimmed);
    if (!match) {
      continue;
    }

    const filePath = match[1];
    const startLine = parseInt(match[2], 10) - 1;
    const startCol = parseInt(match[3], 10) - 1;
    const endLine = parseInt(match[4], 10) - 1;
    const endCol = parseInt(match[5], 10) - 1;
    const count = parseInt(match[7], 10);

    let statements = fileEntries.get(filePath);
    if (!statements) {
      statements = [];
      fileEntries.set(filePath, statements);
    }

    const range = new vscode.Range(
      new vscode.Position(startLine, startCol),
      new vscode.Position(endLine, endCol),
    );
    statements.push(new vscode.StatementCoverage(count > 0 ? count : false, range));
  }

  const result: vscode.FileCoverage[] = [];
  for (const [importFilePath, statements] of fileEntries) {
    const lastSlash = importFilePath.lastIndexOf("/");
    const fileName = importFilePath.slice(lastSlash + 1);
    const importDir = importFilePath.slice(0, lastSlash);
    const absDir = moduleToDir(importDir);
    if (!absDir) {
      continue;
    }
    const absPath = path.join(absDir, fileName);
    const uri = vscode.Uri.file(absPath);

    const fileCoverage = vscode.FileCoverage.fromDetails(uri, statements);
    result.push(fileCoverage);
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
        run.end();
        return;
      }

      for (const item of items) {
        run.started(item);
      }

      const groups = groupByPackage(items);
      let allJsonOutput = "";

      for (const [importPath, groupItems] of groups) {
        if (effectiveToken.isCancellationRequested) {
          for (const item of groupItems) {
            run.skipped(item);
          }
          continue;
        }

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
              new vscode.TestMessage(`Workspace folder not found for: ${importPath}`),
            );
          }
          continue;
        }

        const config = scopedConfig(workspaceDir);
        const testFlags = config.get<string[]>("testFlags") ?? [];

        let overlayDir: string | undefined;
        let coverFile: string | undefined;

        try {
          const overlayCmd = await buildCliCommand(["overlay", importPath], workspaceDir, this.outputChannel);
          this.outputChannel.appendLine(`[coverage] ${formatCliCommand(overlayCmd)}`);
          const { stdout: overlayStdout } = await execFileAsync(
            overlayCmd.bin,
            overlayCmd.args,
            { cwd: workspaceDir },
          );
          const overlay = JSON.parse(overlayStdout) as OverlayOutput;
          overlayDir = overlay.dir;

          const tmpDir = await mkdtemp(path.join(tmpdir(), "gotest-cov-"));
          coverFile = path.join(tmpDir, "cover.out");

          const filter = this.buildRunFilter(groupItems, importPath);

          const buildTags = config.get<string>("buildTags", "").trim();
          const args = [
            "test",
            `-overlay=${overlay.overlayFile}`,
            `-coverprofile=${coverFile}`,
            "-count=1",
            "-json",
            importPath,
          ];
          if (buildTags) {
            args.push(`-tags=${buildTags}`);
          }
          if (filter) {
            args.push("-run", filter);
          }
          args.push(...testFlags);

          const goBin = await resolveGoBinary(this.outputChannel, workspaceDir);
          this.outputChannel.appendLine(`[coverage] ${goBin} ${args.join(" ")}`);
          const stdout = await spawnTestProcess(goBin, args, workspaceDir, effectiveToken, this.outputChannel, "coverage");
          allJsonOutput += stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          const events = parseTestEvents(stdout);
          applyResults(this.controller, run, events, importPath, pkg.dir);

          try {
            const coverContent = await readFile(coverFile, "utf-8");
            this.store.update(importPath, coverContent);
          } catch {
            this.outputChannel.appendLine("[coverage] no coverprofile generated");
          }
        } finally {
          if (overlayDir) {
            rm(overlayDir, { recursive: true, force: true }).catch(() => {});
          }
          if (coverFile) {
            const coverDir = path.dirname(coverFile);
            rm(coverDir, { recursive: true, force: true }).catch(() => {});
          }
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

  async copyCoverageSummary(): Promise<void> {
    const coverages = this.store.buildFileCoverages(this.cache);
    if (coverages.length === 0) {
      vscode.window.showInformationMessage("No coverage data available. Run tests with coverage first.");
      return;
    }

    const rows: { file: string; covered: number; total: number }[] = [];

    for (const fc of coverages) {
      let filePath = fc.uri.fsPath;
      const folder = vscode.workspace.getWorkspaceFolder(fc.uri);
      if (folder && filePath.startsWith(folder.uri.fsPath)) {
        filePath = filePath.slice(folder.uri.fsPath.length + 1);
      }
      rows.push({
        file: filePath,
        covered: fc.statementCoverage.covered,
        total: fc.statementCoverage.total,
      });
    }

    rows.sort((a, b) => a.file.localeCompare(b.file));

    const maxFileLen = Math.max(4, ...rows.map((r) => r.file.length));
    const header = `${"File".padEnd(maxFileLen)}  Stmts      Cover`;
    const separator = "-".repeat(header.length);

    const lines = [header, separator];
    let totalCovered = 0;
    let totalStmts = 0;

    for (const row of rows) {
      totalCovered += row.covered;
      totalStmts += row.total;
      const pct = row.total > 0 ? ((row.covered / row.total) * 100).toFixed(1) + "%" : "N/A";
      const stmts = `${row.covered}/${row.total}`;
      lines.push(`${row.file.padEnd(maxFileLen)}  ${stmts.padEnd(9)}  ${pct}`);
    }

    lines.push(separator);
    const totalPct = totalStmts > 0 ? ((totalCovered / totalStmts) * 100).toFixed(1) + "%" : "N/A";
    const totalStmtsStr = `${totalCovered}/${totalStmts}`;
    lines.push(`${"Total".padEnd(maxFileLen)}  ${totalStmtsStr.padEnd(9)}  ${totalPct}`);

    const text = lines.join("\n");
    await vscode.env.clipboard.writeText(text);
    vscode.window.showInformationMessage("Coverage summary copied to clipboard.");
  }

  dispose(): void {
    this.activeRun?.cancel();
    this.activeRun = undefined;
  }

  private suiteHasFixtures(suiteName: string, importPath: string): boolean {
    const pkg = this.cache.getPackage(importPath);
    if (!pkg) {
      return false;
    }
    const suite = pkg.suites.find((s) => s.name === suiteName);
    return suite !== undefined && suite.fixtures.length > 0;
  }

  private buildRunFilter(items: vscode.TestItem[], importPath: string): string | undefined {
    if (items.some((item) => getItemDepth(item) === 0)) {
      return undefined;
    }

    const suiteGroups = new Map<
      string,
      { wholeSuite: boolean; methods: string[]; subtests: string[] }
    >();

    for (const item of items) {
      const depth = getItemDepth(item);

      if (depth === 1) {
        const suiteName = item.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteGroups.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteGroups.set(suiteName, group);
        }
        group.wholeSuite = true;
      } else if (depth === 2) {
        const suiteName = item.parent!.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteGroups.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteGroups.set(suiteName, group);
        }
        group.methods.push(item.label);
      } else if (depth >= 3) {
        let current = item;
        const subtestParts: string[] = [];
        while (getItemDepth(current) > 2) {
          subtestParts.unshift(current.label);
          current = current.parent!;
        }
        const methodName = current.label;
        const suiteName = current.parent!.label;
        if (this.suiteHasFixtures(suiteName, importPath)) {
          return undefined;
        }
        let group = suiteGroups.get(suiteName);
        if (!group) {
          group = { wholeSuite: false, methods: [], subtests: [] };
          suiteGroups.set(suiteName, group);
        }
        group.subtests.push(`^Test${suiteName}$/^${methodName}$/^${subtestParts.join("/")}$`);
      }
    }

    const filters: string[] = [];
    for (const [suiteName, group] of suiteGroups) {
      if (group.wholeSuite) {
        filters.push(`^Test${suiteName}$`);
      } else if (group.subtests.length > 0) {
        filters.push(...group.subtests);
      } else if (group.methods.length === 1) {
        filters.push(`^Test${suiteName}$/^${group.methods[0]}$`);
      } else if (group.methods.length > 1) {
        filters.push(`^Test${suiteName}$/^(${group.methods.join("|")})$`);
      }
    }

    return filters.length === 0 ? undefined : filters.length === 1 ? filters[0] : filters.join("|");
  }
}
