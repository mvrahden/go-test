import * as vscode from "vscode";
import * as path from "node:path";
import { spawn, execFile } from "node:child_process";
import { promisify } from "node:util";
import { rm, mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import type { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
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
  constructor(
    private readonly controller: GoTestController,
    private readonly cache: DiscoveryCache,
    private readonly outputChannel: vscode.OutputChannel,
    private readonly onJsonOutput: (json: string) => void,
  ) {}

  async run(
    request: vscode.TestRunRequest,
    token: vscode.CancellationToken,
  ): Promise<void> {
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
          const overlayCmd = buildCliCommand(["overlay", pkg.dir]);
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
            "-json",
            pkg.dir,
          ];
          if (filter) {
            args.push("-run", filter);
          }
          args.push(...testFlags);

          this.outputChannel.appendLine(`[coverage] go ${args.join(" ")}`);
          const stdout = await this.spawnGoTest(args, workspaceDir, token);
          allJsonOutput += stdout;

          if (token.isCancellationRequested) {
            for (const item of groupItems) {
              run.skipped(item);
            }
            continue;
          }

          const events = parseTestEvents(stdout);
          this.applyResults(run, events, importPath, pkg.dir);

          try {
            const coverContent = await readFile(coverFile, "utf-8");
            const moduleToDir = (importDir: string) => {
              return this.cache.resolveImportPath(importDir);
            };
            const fileCoverages = parseCoverProfile(coverContent, moduleToDir);
            for (const fc of fileCoverages) {
              run.addCoverage(fc);
            }
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

      if (allJsonOutput) {
        this.onJsonOutput(allJsonOutput);
      }
    } finally {
      run.end();
    }
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
    for (const item of items) {
      if (this.getItemDepth(item) === 0) {
        return undefined;
      }
    }

    const item = items[0];
    const depth = this.getItemDepth(item);
    if (depth === 1) {
      return `^Test${item.label}$`;
    }
    if (depth === 2) {
      const suite = item.parent!;
      return `^Test${suite.label}$/^${item.label}$`;
    }
    return undefined;
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
