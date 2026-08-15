import * as path from "node:path";
import { readFile, writeFile, mkdir } from "node:fs/promises";

export interface TestResult {
  status: "pass" | "fail" | "skip";
  duration?: number;
}

interface StoredTestResult extends TestResult {
  timestamp: number;
}

interface StoredData {
  version: 2;
  results: Record<string, StoredTestResult>;
}

const DEFAULT_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000; // 7 days

export class TestResultStore {
  private results = new Map<string, StoredTestResult>();
  private readonly storagePath: string | undefined;
  private saveChain = Promise.resolve();
  private debounceTimer: ReturnType<typeof setTimeout> | undefined;
  private readonly maxAge: number;

  constructor(
    storageUri: { fsPath: string } | undefined,
    maxAge = DEFAULT_MAX_AGE_MS,
  ) {
    this.maxAge = maxAge;
    if (storageUri) {
      this.storagePath = path.join(storageUri.fsPath, "testResults.json");
    }
  }

  get size(): number {
    return this.results.size;
  }

  record(
    itemId: string,
    status: TestResult["status"],
    duration?: number,
  ): void {
    this.results.set(itemId, { status, duration, timestamp: Date.now() });
  }

  get(itemId: string): TestResult | undefined {
    return this.results.get(itemId);
  }

  delete(itemId: string): void {
    this.results.delete(itemId);
  }

  forEach(callback: (result: TestResult, itemId: string) => void): void {
    this.results.forEach((result, id) => callback(result, id));
  }

  private evictStale(): void {
    const cutoff = Date.now() - this.maxAge;
    for (const [id, result] of this.results) {
      if (result.timestamp < cutoff) {
        this.results.delete(id);
      }
    }
  }

  async load(): Promise<void> {
    if (!this.storagePath) return;
    try {
      const content = await readFile(this.storagePath, "utf-8");
      const data = JSON.parse(content) as StoredData;
      if (data.version !== 2) return;
      this.results.clear();
      for (const [id, result] of Object.entries(data.results)) {
        this.results.set(id, result);
      }
      this.evictStale();
    } catch {
      /* No stored data or corrupt — start fresh */
    }
  }

  save(): void {
    if (this.debounceTimer !== undefined) {
      clearTimeout(this.debounceTimer);
    }
    this.debounceTimer = setTimeout(() => {
      this.debounceTimer = undefined;
      this.saveChain = this.saveChain
        .then(() => this.writeToDisk())
        .catch(() => {});
    }, 500);
  }

  flush(): Promise<void> {
    if (this.debounceTimer !== undefined) {
      clearTimeout(this.debounceTimer);
      this.debounceTimer = undefined;
      this.saveChain = this.saveChain
        .then(() => this.writeToDisk())
        .catch(() => {});
    }
    return this.saveChain;
  }

  private async writeToDisk(): Promise<void> {
    if (!this.storagePath) return;
    this.evictStale();
    const data: StoredData = {
      version: 2,
      results: Object.fromEntries(this.results),
    };
    await mkdir(path.dirname(this.storagePath), { recursive: true });
    await writeFile(this.storagePath, JSON.stringify(data), "utf-8");
  }

  // clear drops every result and schedules the empty state to disk. Clearing
  // memory alone would leave the persisted copy intact, and the next window
  // reload would restore exactly the results the developer just dismissed.
  clear(): void {
    this.results.clear();
    this.save();
  }

  dispose(): void {
    if (this.debounceTimer !== undefined) {
      clearTimeout(this.debounceTimer);
      this.debounceTimer = undefined;
    }
    this.results.clear();
  }
}
