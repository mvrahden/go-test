// The little git the Change scope needs: whether a folder is in a checkout,
// a default base to compare with, the refs to choose from, and a file as a
// commit holds it. The CLI obtains the base tree itself; this only names it.
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const GIT_TIMEOUT_MS = 15_000;

async function git(
  cwd: string,
  args: string[],
  maxBuffer = 1024 * 1024,
): Promise<string | undefined> {
  try {
    const { stdout } = await execFileAsync("git", args, {
      cwd,
      timeout: GIT_TIMEOUT_MS,
      maxBuffer,
    });
    return stdout;
  } catch {
    return undefined;
  }
}

// gitToplevel is the checkout's root, undefined outside one.
export async function gitToplevel(dir: string): Promise<string | undefined> {
  return (
    (await git(dir, ["rev-parse", "--show-toplevel"]))?.trim() || undefined
  );
}

// currentBranch is the branch HEAD is on, undefined when detached.
export async function currentBranch(dir: string): Promise<string | undefined> {
  const name = (await git(dir, ["rev-parse", "--abbrev-ref", "HEAD"]))?.trim();
  return name && name !== "HEAD" ? name : undefined;
}

// defaultBranch is the remote default branch, as origin/HEAD names it, else
// a remote's main or master; origin is asked first.
export async function defaultBranch(dir: string): Promise<string | undefined> {
  const remotes = ((await git(dir, ["remote"])) ?? "")
    .split("\n")
    .map((r) => r.trim())
    .filter(Boolean)
    .sort((a, b) => (a === "origin" ? -1 : b === "origin" ? 1 : 0));
  for (const remote of remotes) {
    const head = (
      await git(dir, [
        "symbolic-ref",
        "--quiet",
        "--short",
        `refs/remotes/${remote}/HEAD`,
      ])
    )?.trim();
    if (head) return head;
    for (const branch of ["main", "master"]) {
      const ref = `${remote}/${branch}`;
      if (await git(dir, ["rev-parse", "--verify", "--quiet", ref])) return ref;
    }
  }
  return undefined;
}

// mergeBase is the commit HEAD and ref share last, abbreviated as git
// abbreviates it, which is how the CLI names a commit too.
export async function mergeBase(
  dir: string,
  ref: string,
): Promise<string | undefined> {
  const full = (await git(dir, ["merge-base", "HEAD", ref]))?.trim();
  if (!full) return undefined;
  return (await git(dir, ["rev-parse", "--short", full]))?.trim() || full;
}

// listRefs names the branches and tags, the most recently committed first.
export async function listRefs(dir: string, limit = 200): Promise<string[]> {
  const out = await git(dir, [
    "for-each-ref",
    "--sort=-committerdate",
    `--count=${limit}`,
    "--format=%(refname:short)",
    "refs/heads",
    "refs/remotes",
    "refs/tags",
  ]);
  return (out ?? "")
    .split("\n")
    .map((r) => r.trim())
    .filter((r) => r && !r.endsWith("/HEAD"));
}

// showFile is a file as commit holds it, by its path from the checkout's
// root; undefined when the commit has no such file.
export async function showFile(
  toplevel: string,
  commit: string,
  relPath: string,
): Promise<string | undefined> {
  return git(toplevel, ["show", `${commit}:${relPath}`], 64 * 1024 * 1024);
}

// BASE_SCHEME names documents that hold a file as a base commit has it.
export const BASE_SCHEME = "gotest-base";

export interface BaseFile {
  toplevel: string;
  commit: string;
  // Slash-separated, from the checkout's root.
  path: string;
}

// baseQuery and parseBaseQuery carry a base file in a URI's query.
export function baseQuery(f: BaseFile): string {
  return JSON.stringify(f);
}

export function parseBaseQuery(query: string): BaseFile | undefined {
  try {
    const v = JSON.parse(query) as Partial<BaseFile>;
    if (
      typeof v.toplevel === "string" &&
      typeof v.commit === "string" &&
      typeof v.path === "string"
    ) {
      return { toplevel: v.toplevel, commit: v.commit, path: v.path };
    }
  } catch {
    // not ours
  }
  return undefined;
}
