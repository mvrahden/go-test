import * as fs from "node:fs";
import * as path from "node:path";
import type * as vscode from "vscode";

const GENERATED_FILE_PATTERNS = [
  "gotest_psuite_test.go",
  "gotest_pxsuite_test.go",
  // The names older gotest versions generated into.
  "ƒƒ_psuite_test.go",
  "ƒƒ_pxsuite_test.go",
];

const MARKER = "# gotest generated files";

export function ensureGitExclude(
  workspaceDir: string,
  outputChannel: vscode.LogOutputChannel,
): void {
  const excludePath = path.join(workspaceDir, ".git", "info", "exclude");

  const gitDir = path.join(workspaceDir, ".git");
  if (!fs.existsSync(gitDir)) {
    return;
  }

  const infoDir = path.dirname(excludePath);
  let content: string;
  try {
    content = fs.readFileSync(excludePath, "utf-8");
  } catch {
    try {
      fs.mkdirSync(infoDir, { recursive: true });
    } catch {
      return;
    }
    content = "";
  }

  const lines = new Set(content.split(/\r?\n/).map((l) => l.trim()));
  const missing = GENERATED_FILE_PATTERNS.filter((p) => !lines.has(p));
  if (missing.length === 0) {
    return;
  }

  // A block written before some names were covered gets only those.
  const head = lines.has(MARKER) ? [] : ["", MARKER];
  const sep = content === "" || content.endsWith("\n") ? "" : "\n";
  const block = sep + [...head, ...missing].join("\n") + "\n";

  try {
    fs.appendFileSync(excludePath, block);
    outputChannel.debug(
      "[activate] added generated file patterns to .git/info/exclude",
    );
  } catch (err) {
    outputChannel.debug(
      `[activate] could not update .git/info/exclude: ${err}`,
    );
  }
}
