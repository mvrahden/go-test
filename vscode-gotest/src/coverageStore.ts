import * as vscode from "vscode";
import { JsonStore } from "./jsonStore.js";
import {
  type ParsedFileCoverage,
  type CoverageResult,
  parseCoverProfile,
  parseFuncCoverage,
  buildFileCoverages,
  deduplicateProfiles,
  filterSupplementaryProfiles,
} from "./coverage.js";
import type { DiscoveryCache } from "./discovery.js";

interface StoredPackageCoverage {
  coverprofile: string;
  funcCoverage?: string;
  timestamp: number;
  supplementary?: boolean;
}

type StoredData = Record<string, StoredPackageCoverage>;

// 2, not 1: the on-disk envelope moved to the shared { version, data } shape.
// A mismatch purges, which is right for a cache.
const STORE_VERSION = 2;

interface ParsedPackageCache {
  profiles: ParsedFileCoverage[];
  declarations: Map<string, vscode.DeclarationCoverage[]>;
}

export class CoverageStore implements vscode.Disposable {
  private packages = new Map<string, StoredPackageCoverage>();
  private parsed = new Map<string, ParsedPackageCache>();
  private cachedDetails = new Map<string, vscode.FileCoverageDetail[]>();
  private readonly store: JsonStore<StoredData>;

  constructor(storageUri: vscode.Uri | undefined) {
    this.store = new JsonStore<StoredData>(
      storageUri?.fsPath,
      "coverage.json",
      STORE_VERSION,
    );
  }

  get size(): number {
    return this.packages.size;
  }

  getDetails(absPath: string): vscode.FileCoverageDetail[] {
    return this.cachedDetails.get(absPath) ?? [];
  }

  update(
    importPath: string,
    coverprofile: string,
    funcCoverage?: string,
    supplementary?: boolean,
  ): void {
    this.packages.set(importPath, {
      coverprofile,
      funcCoverage,
      timestamp: Date.now(),
      supplementary: supplementary || undefined,
    });
    this.parsed.delete(importPath);
    this.cachedDetails.clear();
    this.store.markMutated();
  }

  invalidate(importPath: string): boolean {
    const deleted = this.packages.delete(importPath);
    if (deleted) {
      this.parsed.delete(importPath);
      this.cachedDetails.clear();
      this.store.markMutated();
    }
    return deleted;
  }

  // clear drops every profile and persists the empty state, so a window reload
  // cannot restore coverage the developer has dismissed. Memory-only clearing
  // would leave the stored copy to come back on the next activation.
  clear(): Promise<void> {
    this.packages.clear();
    this.parsed.clear();
    this.cachedDetails.clear();
    this.store.markMutated();
    this.save();
    // The one caller that must not race the write: the developer dismissed
    // these results and a reload before the flush would bring them back.
    return this.flush();
  }

  buildFileCoverages(cache: DiscoveryCache): CoverageResult {
    const moduleToDir = (importPath: string) =>
      cache.resolveImportPath(importPath);
    const allDeclarations = new Map<string, vscode.DeclarationCoverage[]>();

    const primaryProfiles: ParsedFileCoverage[] = [];
    const supplementaryProfiles: ParsedFileCoverage[] = [];

    for (const [importPath, pkg] of this.packages) {
      let entry = this.parsed.get(importPath);
      if (!entry) {
        entry = {
          profiles: parseCoverProfile(pkg.coverprofile, moduleToDir),
          declarations: pkg.funcCoverage
            ? parseFuncCoverage(pkg.funcCoverage, moduleToDir)
            : new Map(),
        };
        this.parsed.set(importPath, entry);
      }

      if (pkg.supplementary) {
        supplementaryProfiles.push(...entry.profiles);
      } else {
        primaryProfiles.push(...entry.profiles);
      }

      for (const [filePath, declarations] of entry.declarations) {
        const existing = allDeclarations.get(filePath) ?? [];
        existing.push(...declarations);
        allDeclarations.set(filePath, existing);
      }
    }

    const filtered = filterSupplementaryProfiles(
      primaryProfiles,
      supplementaryProfiles,
    );
    const allProfiles = [...primaryProfiles, ...filtered];

    const result = buildFileCoverages(
      deduplicateProfiles(allProfiles),
      allDeclarations,
    );
    this.cachedDetails = result.details;
    return result;
  }

  async load(): Promise<void> {
    const data = await this.store.read();
    if (!data) return;
    this.packages.clear();
    this.parsed.clear();
    for (const [importPath, pkg] of Object.entries(data)) {
      this.packages.set(importPath, pkg);
    }
  }

  // Void and debounced, like every other store: no call site has to know
  // whether this particular one returns something to await.
  save(): void {
    this.store.save(() => Object.fromEntries(this.packages));
  }

  flush(): Promise<void> {
    return this.store.flush();
  }

  dispose(): void {
    this.cachedDetails.clear();
  }
}
