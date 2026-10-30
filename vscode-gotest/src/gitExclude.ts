import * as fs from "node:fs";
import * as path from "node:path";
import type * as vscode from "vscode";

const GENERATED_FILE_PATTERNS = [
  "gotest_psuite_test.go",
  "gotest_pxsuite_test.go",
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

  if (content.includes(GENERATED_FILE_PATTERNS[0])) {
    return;
  }

  const block = ["", MARKER, ...GENERATED_FILE_PATTERNS].join("\n") + "\n";

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
