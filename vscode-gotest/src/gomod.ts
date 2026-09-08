import * as path from "node:path";
import { readFile } from "node:fs/promises";

export async function readModulePath(dir: string): Promise<string | undefined> {
  try {
    const content = await readFile(path.join(dir, "go.mod"), "utf-8");
    const match = /^\s*module\s+(\S+)/m.exec(content);
    return match?.[1];
  } catch {
    return undefined;
  }
}

// A module root the resolver reasons about: its directory and go.mod text.
export interface ModuleRoot {
  dir: string;
  goMod: string;
}

// readModuleRoots returns the go.work `use` modules, else the directory's own
// go.mod. Entries without a readable go.mod are dropped.
export async function readModuleRoots(dir: string): Promise<ModuleRoot[]> {
  let dirs = [dir];
  try {
    const work = await readFile(path.join(dir, "go.work"), "utf-8");
    dirs = parseUseDirectives(work).map((entry) => path.resolve(dir, entry));
  } catch {
    // no go.work
  }
  const roots: ModuleRoot[] = [];
  for (const moduleDir of dirs) {
    try {
      const goMod = await readFile(path.join(moduleDir, "go.mod"), "utf-8");
      roots.push({ dir: moduleDir, goMod });
    } catch {
      // not a module
    }
  }
  return roots;
}

// parseUseDirectives lists a go.work's `use` entries as written.
export function parseUseDirectives(content: string): string[] {
  return parseDirective(content, "use");
}

// parseToolDirectives lists the package paths a go.mod declares as tools.
export function parseToolDirectives(content: string): string[] {
  return parseDirective(content, "tool");
}

function parseDirective(content: string, keyword: string): string[] {
  const entries: string[] = [];
  const block = new RegExp(`^\\s*${keyword}\\s*\\(([\\s\\S]*?)^\\s*\\)`, "gm");
  for (const m of content.matchAll(block)) {
    for (const line of m[1].split("\n")) {
      const entry = line.replace(/\/\/.*$/, "").trim();
      if (entry) entries.push(entry.split(/\s+/)[0]);
    }
  }
  const single = new RegExp(`^\\s*${keyword}\\s+([^\\s(]\\S*)`, "gm");
  for (const m of content.matchAll(single)) {
    entries.push(m[1]);
  }
  return entries;
}

// requireVersion returns the required version of the module providing
// `packagePath`, walking up the path to find the module.
export function requireVersion(
  content: string,
  packagePath: string,
): string | undefined {
  for (const candidate of modulePathCandidates(packagePath)) {
    const escaped = escapeRegExp(candidate);
    const patterns = [
      new RegExp(`^[ \\t]*${escaped}[ \\t]+(v\\S+)`, "m"),
      new RegExp(`^[ \\t]*require[ \\t]+${escaped}[ \\t]+(v\\S+)`, "m"),
    ];
    for (const pattern of patterns) {
      const match = pattern.exec(content);
      if (match) return match[1];
    }
  }
  return undefined;
}

// hasReplaceDirective reports whether go.mod replaces the module providing
// `packagePath`.
export function hasReplaceDirective(
  content: string,
  packagePath: string,
): boolean {
  for (const candidate of modulePathCandidates(packagePath)) {
    const escaped = escapeRegExp(candidate);
    if (new RegExp(`^\\s*replace\\s+${escaped}(?:\\s|$)`, "m").test(content)) {
      return true;
    }
    const entryPattern = new RegExp(`^\\s*${escaped}(?:\\s|$)`, "m");
    for (const block of content.matchAll(/^\s*replace\s*\(([\s\S]*?)\)/gm)) {
      if (entryPattern.test(block[1])) {
        return true;
      }
    }
  }
  return false;
}

function modulePathCandidates(packagePath: string): string[] {
  const candidates: string[] = [];
  let candidate = packagePath;
  while (candidate) {
    candidates.push(candidate);
    const lastSlash = candidate.lastIndexOf("/");
    if (lastSlash <= 0) break;
    candidate = candidate.substring(0, lastSlash);
  }
  return candidates;
}

export function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
