import * as vscode from "vscode";
import * as path from "node:path";
import { spawn, execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import type { CoverageStore } from "./coverageStore.js";
import type { OverlayOutput } from "./types.js";
import {
  parseTestEvents,
  extractTestMessages,
  type TestEvent,
} from "./outputParser.js";
import { buildCliCommand, formatCliCommand } from "./cli.js";

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
    token.onCancellationRequested(() => cts.cancel());
    const effectiveToken = cts.token;

    const run = this.controller.createTestRun(request, "Go Test Coverage");

    try {
      const items = this.collectItems(request);
      if (items.length === 0) {
        run.end();
        return;
      }

      for (const item of items) {
        run.started(item);
      }

      const groups = this.groupByPackage(items);
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

        const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
        if (!workspaceDir) {
          continue;
        }

        const testFlags =
          vscode.workspace.getConfiguration("gotest").get<string[]>("testFlags") ?? [];

        let overlayDir: string | undefined;
        let coverFile: string | undefined;

        try {
          const overlayCmd = await buildCliCommand(["overlay", importPath]);
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

          const filter = this.buildRunFilter(groupItems);

          const args = [
            "test",
            `-overlay=${overlay.overlayFile}`,
            `-coverprofile=${coverFile}`,
            "-count=1",
            "-json",
            importPath,
          ];
          if (filter) {
            args.push("-run", filter);
          }
          args.push(...testFlags);

          this.outputChannel.appendLine(`[coverage] go ${args.join(" ")}`);
          const stdout = await this.spawnGoTest(args, workspaceDir, effectiveToken);
          allJsonOutput += stdout;

          if (effectiveToken.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          const events = parseTestEvents(stdout);
          this.applyResults(run, events, importPath, pkg.dir);

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

    const workspaceDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? "";
    const rows: { file: string; covered: number; total: number }[] = [];

    for (const fc of coverages) {
      let filePath = fc.uri.fsPath;
      if (workspaceDir && filePath.startsWith(workspaceDir)) {
        filePath = filePath.slice(workspaceDir.length + 1);
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

  dispose(): void {}

  private collectItems(request: vscode.TestRunRequest): vscode.TestItem[] {
    const items: vscode.TestItem[] = [];
    if (request.include && request.include.length > 0) {
      for (const item of request.include) {
        items.push(item);
      }
    } else {
      this.controller.testController.items.forEach((item) => {
        items.push(item);
      });
    }
    return items;
  }

  private groupByPackage(items: vscode.TestItem[]): Map<string, vscode.TestItem[]> {
    const groups = new Map<string, vscode.TestItem[]>();
    for (const item of items) {
      const root = this.getRootItem(item);
      let group = groups.get(root.id);
      if (!group) {
        group = [];
        groups.set(root.id, group);
      }
      group.push(item);
    }
    return groups;
  }

  private getRootItem(item: vscode.TestItem): vscode.TestItem {
    let current = item;
    while (current.parent) {
      current = current.parent;
    }
    return current;
  }

  private getItemDepth(item: vscode.TestItem): number {
    let depth = 0;
    let current = item;
    while (current.parent) {
      current = current.parent;
      depth++;
    }
    return depth;
  }

  private buildRunFilter(items: vscode.TestItem[]): string | undefined {
    if (items.some((item) => this.getItemDepth(item) === 0)) {
      return undefined;
    }

    const filters: string[] = [];
    for (const item of items) {
      const depth = this.getItemDepth(item);
      if (depth === 1) {
        filters.push(`^Test${item.label}$`);
      } else if (depth === 2) {
        filters.push(`^Test${item.parent!.label}$/^${item.label}$`);
      }
    }

    return filters.length === 0 ? undefined : filters.length === 1 ? filters[0] : filters.join("|");
  }

  private spawnGoTest(
    args: string[],
    cwd: string,
    token: vscode.CancellationToken,
  ): Promise<string> {
    return new Promise<string>((resolve) => {
      const child = spawn("go", args, { cwd });
      let stdout = "";
      let stderr = "";

      child.stdout.on("data", (data: Buffer) => {
        stdout += data.toString();
      });

      child.stderr.on("data", (data: Buffer) => {
        stderr += data.toString();
      });

      const cancelListener = token.onCancellationRequested(() => {
        child.kill("SIGTERM");
      });

      child.on("close", () => {
        cancelListener.dispose();
        if (stderr) {
          this.outputChannel.appendLine(`[coverage] stderr: ${stderr}`);
        }
        resolve(stdout);
      });

      child.on("error", (err: Error) => {
        cancelListener.dispose();
        this.outputChannel.appendLine(`[coverage] error: ${err.message}`);
        resolve(stdout);
      });
    });
  }

  private applyResults(
    run: vscode.TestRun,
    events: TestEvent[],
    importPath: string,
    pkgDir: string,
  ): void {
    const outputMap = new Map<string, string>();

    for (const event of events) {
      if (event.Action === "output" && event.Test) {
        const existing = outputMap.get(event.Test) ?? "";
        outputMap.set(event.Test, existing + (event.Output ?? ""));
      }
    }

    for (const event of events) {
      if (!event.Test) {
        continue;
      }

      const item = this.resolveTestItem(event.Test, importPath);
      if (!item) {
        continue;
      }

      const duration =
        event.Elapsed !== undefined ? event.Elapsed * 1000 : undefined;

      switch (event.Action) {
        case "pass":
          run.passed(item, duration);
          break;
        case "fail": {
          const output = outputMap.get(event.Test) ?? "";
          const testMessages = extractTestMessages(output, pkgDir);
          const vscodeMessages = testMessages.map((msg) => {
            const message = new vscode.TestMessage(msg.message);
            message.location = new vscode.Location(
              vscode.Uri.file(msg.file),
              new vscode.Position(msg.line - 1, 0),
            );
            return message;
          });
          if (vscodeMessages.length === 0) {
            vscodeMessages.push(new vscode.TestMessage(output || "Test failed"));
          }
          run.failed(item, vscodeMessages, duration);
          break;
        }
        case "skip":
          run.skipped(item);
          break;
        case "run":
          run.started(item);
          break;
      }
    }
  }

  private resolveTestItem(
    testPath: string,
    importPath: string,
  ): vscode.TestItem | undefined {
    const segments = testPath.split("/");
    if (segments.length === 0) {
      return undefined;
    }

    const firstSegment = segments[0];
    const suiteName = firstSegment.startsWith("Test")
      ? firstSegment.slice(4)
      : firstSegment;

    const suiteId = `${importPath}/${suiteName}`;
    const suiteItem = this.controller.findItem(suiteId);
    if (!suiteItem) {
      return undefined;
    }

    if (segments.length === 1) {
      return suiteItem;
    }

    const methodName = segments[1];
    const methodId = `${suiteId}/${methodName}`;
    const methodItem = this.controller.findItem(methodId);
    if (!methodItem) {
      return undefined;
    }

    if (segments.length === 2) {
      return methodItem;
    }

    let parentItem = methodItem;
    for (let i = 2; i < segments.length; i++) {
      const subtestLabel = segments[i];
      const subtestPath = segments.slice(2, i + 1).join("/");
      parentItem = this.controller.createDynamicSubtest(
        parentItem,
        subtestPath,
        subtestLabel,
      );
    }

    return parentItem;
  }
}
