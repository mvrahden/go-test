// Which workspace folders use gotest. Elsewhere the extension stays idle:
// no discovery, no CLI, no git setting.
import type * as vscode from "vscode";
import { execFile } from "node:child_process";
import { cliPackagePath } from "./cli.js";
import { folderUsesGotest } from "./gomod.js";
import { ensureGitExclude } from "./gitExclude.js";

// ActiveFolders knows which workspace folders use gotest.
export class ActiveFolders {
  private readonly active = new Set<string>();

  has(folder: string): boolean {
    return this.active.has(folder);
  }

  get size(): number {
    return this.active.size;
  }

  // refresh re-reads the folder's go.work and go.mod files.
  async refresh(
    folder: string,
  ): Promise<"activated" | "deactivated" | "unchanged"> {
    const uses = await folderUsesGotest(folder, cliPackagePath(folder));
    const was = this.active.has(folder);
    if (uses === was) return "unchanged";
    if (uses) {
      this.active.add(folder);
      return "activated";
    }
    this.active.delete(folder);
    return "deactivated";
  }
}

// writeGitSettings names snapshot sections in git diffs and keeps generated
// harness files out of git status, in a folder that uses gotest.
export function writeGitSettings(
  folder: string,
  log: vscode.LogOutputChannel,
): void {
  execFile(
    "git",
    ["config", "diff.snapshot.xfuncname", "^=== SNAP .+ ===$"],
    { cwd: folder },
    (err) => {
      if (err) {
        log.debug(
          `[activate] git config diff.snapshot.xfuncname failed: ${err.message}`,
        );
      }
    },
  );
  ensureGitExclude(folder, log);
}

// moduleFileChanged follows a change to a folder's go.mod or go.work: the CLI
// is resolved afresh, and the folder rediscovered, which empties the tree of
// a folder that stopped using gotest.
export async function moduleFileChanged(
  folder: string,
  deps: {
    active: ActiveFolders;
    discover: (folder: string) => Promise<void>;
    resetCli: () => void;
    setActiveContext: (anyActive: boolean) => void;
    writeGitSettings: (folder: string) => void;
  },
): Promise<void> {
  deps.resetCli();
  if ((await deps.active.refresh(folder)) === "activated") {
    deps.writeGitSettings(folder);
  }
  deps.setActiveContext(deps.active.size > 0);
  await deps.discover(folder);
}
