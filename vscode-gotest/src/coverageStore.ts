import * as vscode from "vscode";
import * as path from "node:path";
import { readFile, writeFile, mkdir } from "node:fs/promises";
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

interface StoredData {
  version: 1;
  packages: Record<string, StoredPackageCoverage>;
}

interface ParsedPackageCache {
  profiles: ParsedFileCoverage[];
  declarations: Map<string, vscode.DeclarationCoverage[]>;
}

export class CoverageStore implements vscode.Disposable {
  private packages = new Map<string, StoredPackageCoverage>();
  private parsed = new Map<string, ParsedPackageCache>();
  private cachedDetails = new Map<string, vscode.FileCoverageDetail[]>();
  private readonly storagePath: string | undefined;
  private saveChain = Promise.resolve();

  constructor(storageUri: vscode.Uri | undefined) {
    if (storageUri) {
      this.storagePath = path.join(storageUri.fsPath, "coverage.json");
    }
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
  }

  invalidate(importPath: string): boolean {
    const deleted = this.packages.delete(importPath);
    if (deleted) {
      this.parsed.delete(importPath);
      this.cachedDetails.clear();
    }
    return deleted;
  }

  clear(): void {
    if (this.packages.size === 0) {
      return;
    }
    this.packages.clear();
    this.parsed.clear();
    this.cachedDetails.clear();
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
    if (!this.storagePath) {
      return;
    }
    try {
      const content = await readFile(this.storagePath, "utf-8");
      const data = JSON.parse(content) as StoredData;
      if (data.version !== 1) {
        return;
      }
      this.packages.clear();
      this.parsed.clear();
      for (const [importPath, pkg] of Object.entries(data.packages)) {
        this.packages.set(importPath, pkg);
      }
    } catch {
      // No stored data or corrupt — start fresh
    }
  }

  save(): Promise<void> {
    const op = this.saveChain.then(() => this.writeToDisk());
    this.saveChain = op.catch(() => {});
    return op;
  }

  flush(): Promise<void> {
    return this.saveChain;
  }

  private async writeToDisk(): Promise<void> {
    if (!this.storagePath) return;
    const data: StoredData = {
      version: 1,
      packages: Object.fromEntries(this.packages),
    };
    await mkdir(path.dirname(this.storagePath), { recursive: true });
    await writeFile(this.storagePath, JSON.stringify(data), "utf-8");
  }

  dispose(): void {
    this.cachedDetails.clear();
  }
}
