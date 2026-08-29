import * as vscode from "vscode";
import * as path from "node:path";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import type { DiscoverPackage, DiscoverWarning } from "./types.js";

export interface DiscoverySnapshot {
  packages: DiscoverPackage[];
  warnings: DiscoverWarning[];
}

interface StoredData {
  version: 1;
  workspaces: Record<string, DiscoverySnapshot>;
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
export class DiscoverySnapshotStore {
  private workspaces = new Map<string, DiscoverySnapshot>();
  private readonly storagePath: string | undefined;
  private saveChain = Promise.resolve();
  private debounceTimer: ReturnType<typeof setTimeout> | undefined;
  // Set by the first update(). Discovery can run before load() resolves —
  // activate() registers the file watchers before initializeAsync awaits the
  // load — and what discovery just found beats what the last session stored.
  private dirty = false;

  // A whole-workspace tree is hundreds of kilobytes to serialize, and every
  // save of a _test.go file triggers a package rediscovery. Coalesce, the way
  // TestResultStore does, rather than rewriting the file per keystroke-save.
  private static readonly SAVE_DEBOUNCE_MS = 500;

  constructor(storageUri: vscode.Uri | undefined) {
    if (storageUri) {
      this.storagePath = path.join(storageUri.fsPath, "discovery.json");
    }
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
    this.dirty = true;
  }

  async load(): Promise<void> {
    if (!this.storagePath || this.dirty) {
      return;
    }
    try {
      const content = await readFile(this.storagePath, "utf-8");
      const data = JSON.parse(content) as StoredData;
      if (data.version !== 1) {
        return;
      }
      this.workspaces.clear();
      for (const [dir, snapshot] of Object.entries(data.workspaces ?? {})) {
        this.workspaces.set(dir, {
          packages: snapshot.packages ?? [],
          warnings: snapshot.warnings ?? [],
        });
      }
    } catch {
      // No stored data or corrupt — start fresh
    }
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
    this.saveChain = this.saveChain
      .then(() => this.writeToDisk())
      .catch(() => {
        // A snapshot is a cache; a failed write costs the next activation its
        // head start and nothing else.
      });
  }

  private async writeToDisk(): Promise<void> {
    if (!this.storagePath) return;
    const data: StoredData = {
      version: 1,
      workspaces: Object.fromEntries(this.workspaces),
    };
    await mkdir(path.dirname(this.storagePath), { recursive: true });
    await writeFile(this.storagePath, JSON.stringify(data), "utf-8");
  }
}
