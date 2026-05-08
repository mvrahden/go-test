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
  private readonly storagePath: string | undefined;
  private readonly _onDidChange = new vscode.EventEmitter<void>();
  readonly onDidChange: vscode.Event<void> = this._onDidChange.event;

  constructor(storageUri: vscode.Uri | undefined) {
    if (storageUri) {
      this.storagePath = path.join(storageUri.fsPath, "coverage.json");
    }
  }

  get size(): number {
    return this.packages.size;
  }

  has(importPath: string): boolean {
    return this.packages.has(importPath);
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
    this._onDidChange.fire();
  }

  invalidate(importPath: string): boolean {
    const deleted = this.packages.delete(importPath);
    if (deleted) {
      this.parsed.delete(importPath);
      this._onDidChange.fire();
    }
    return deleted;
  }

  clear(): void {
    if (this.packages.size === 0) {
      return;
    }
    this.packages.clear();
    this.parsed.clear();
    this._onDidChange.fire();
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

    return buildFileCoverages(
      deduplicateProfiles(allProfiles),
      allDeclarations,
    );
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

  async save(): Promise<void> {
    if (!this.storagePath) {
      return;
    }
    const data: StoredData = {
      version: 1,
      packages: Object.fromEntries(this.packages),
    };
    try {
      await mkdir(path.dirname(this.storagePath), { recursive: true });
      await writeFile(this.storagePath, JSON.stringify(data), "utf-8");
    } catch {
      // Best-effort persistence
    }
  }

  dispose(): void {
    this._onDidChange.dispose();
  }
}
