import * as vscode from "vscode";
import * as path from "node:path";
import { createHash } from "node:crypto";
import { readFile, readdir, stat, unlink } from "node:fs/promises";
import { atomicWrite, LoadOnce, reportStoreError } from "./jsonStore.js";
import type { DiscoverPackage, DiscoverWarning } from "./types.js";

export interface DiscoverySnapshot {
  packages: DiscoverPackage[];
  warnings: DiscoverWarning[];
}

interface StoredData {
  version: 2;
  workspaceDir: string;
  snapshot: DiscoverySnapshot;
}

const PREFIX = "discovery-";
const SUFFIX = ".json";

// Snapshots for workspaces nobody has opened in this long are dropped. Without
// it, one file per workspace only trades an unbounded number of entries in one
// file for an unbounded number of files in a directory.
const MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000;

function fileFor(workspaceDir: string): string {
  const digest = createHash("sha256").update(workspaceDir).digest("hex");
  return `${PREFIX}${digest.slice(0, 16)}${SUFFIX}`;
}

// DiscoverySnapshotStore persists the last known test tree.
//
// Discovery's cost is the Go toolchain's, not ours: loading a workspace with
// type information means compiling its dependency graph for export data, which
// is seconds warm and tens of seconds on a cold build cache. Paying that before
// the explorer shows anything made every activation as slow as the toolchain's
// worst case. The snapshot decouples the two — the tree comes back from disk,
// and discovery corrects it in the background.
//
// The snapshot is a cache, never a source of truth: it is whatever was true
// when the window last closed, and the discovery that follows it replaces it
// wholesale.
//
// One file per workspace. A single shared file meant every save of a _test.go
// file rewrote the tree of every workspace ever opened to update one of them,
// and one corrupt file cost all of them their head start.
export class DiscoverySnapshotStore {
  private workspaces = new Map<string, DiscoverySnapshot>();
  private readonly storageDir: string | undefined;
  private saveChain = Promise.resolve();
  private debounceTimer: ReturnType<typeof setTimeout> | undefined;
  private pendingWrites = new Set<string>();
  // Discovery can run before load() resolves — activate() registers the file
  // watchers before initializeAsync awaits the load — and what discovery just
  // found beats what the last session stored. Same latch as JsonStore's, so the
  // rule has one implementation rather than one per store.
  private readonly latch = new LoadOnce();

  // A tree is hundreds of kilobytes to serialize, and every save of a _test.go
  // file triggers a package rediscovery. Coalesce, the way TestResultStore does,
  // rather than rewriting on every keystroke-save.
  private static readonly SAVE_DEBOUNCE_MS = 500;

  constructor(storageUri: vscode.Uri | undefined) {
    this.storageDir = storageUri?.fsPath;
  }

  get size(): number {
    return this.workspaces.size;
  }

  get(workspaceDir: string): DiscoverySnapshot | undefined {
    return this.workspaces.get(workspaceDir);
  }

  // update replaces a workspace's snapshot. Replacing rather than merging is
  // what lets a deleted package leave the tree: a merge would keep resurrecting
  // it on every activation.
  update(
    workspaceDir: string,
    packages: DiscoverPackage[],
    warnings: DiscoverWarning[],
  ): void {
    this.workspaces.set(workspaceDir, { packages, warnings });
    this.pendingWrites.add(workspaceDir);
    this.latch.markMutated();
  }

  // load reads the snapshots for the workspaces currently open, and prunes
  // whatever has gone stale. Only the open workspaces are read: the rest are
  // files on disk that cost nothing until someone opens them again.
  async load(workspaceDirs: string[]): Promise<void> {
    if (!this.storageDir || this.latch.blocked) {
      return;
    }
    for (const dir of workspaceDirs) {
      try {
        const content = await readFile(
          path.join(this.storageDir, fileFor(dir)),
          "utf-8",
        );
        const data = JSON.parse(content) as StoredData;
        if (data.version !== 2 || data.workspaceDir !== dir) continue;
        this.workspaces.set(dir, {
          packages: data.snapshot?.packages ?? [],
          warnings: data.snapshot?.warnings ?? [],
        });
      } catch {
        // No stored data or corrupt — this workspace starts without a head
        // start, and the others are unaffected.
      }
    }
    await this.prune();
  }

  save(): void {
    if (this.debounceTimer !== undefined) {
      clearTimeout(this.debounceTimer);
    }
    this.debounceTimer = setTimeout(() => {
      this.debounceTimer = undefined;
      this.enqueueWrite();
    }, DiscoverySnapshotStore.SAVE_DEBOUNCE_MS);
  }

  flush(): Promise<void> {
    if (this.debounceTimer !== undefined) {
      clearTimeout(this.debounceTimer);
      this.debounceTimer = undefined;
      this.enqueueWrite();
    }
    return this.saveChain;
  }

  private enqueueWrite(): void {
    const dirs = [...this.pendingWrites];
    this.pendingWrites.clear();
    this.saveChain = this.saveChain
      .then(() => this.writeToDisk(dirs))
      .catch((err) => {
        // A snapshot is a cache; a failed write costs the next activation its
        // head start and nothing else — but it is said out loud.
        reportStoreError("write discovery snapshot", err);
      });
  }

  private async writeToDisk(dirs: string[]): Promise<void> {
    if (!this.storageDir) return;
    for (const dir of dirs) {
      const snapshot = this.workspaces.get(dir);
      if (!snapshot) continue;
      const data: StoredData = { version: 2, workspaceDir: dir, snapshot };
      await atomicWrite(
        path.join(this.storageDir, fileFor(dir)),
        JSON.stringify(data),
      );
    }
  }

  private async prune(): Promise<void> {
    if (!this.storageDir) return;
    try {
      const entries = await readdir(this.storageDir);
      const now = Date.now();
      for (const entry of entries) {
        if (!entry.startsWith(PREFIX) || !entry.endsWith(SUFFIX)) continue;
        const full = path.join(this.storageDir, entry);
        const info = await stat(full);
        if (now - info.mtimeMs > MAX_AGE_MS) {
          await unlink(full);
        }
      }
    } catch {
      // Pruning is housekeeping; failing it must not fail activation.
    }
  }
}
