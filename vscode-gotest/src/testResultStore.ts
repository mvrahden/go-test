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
  version: 1;
  results: Record<string, StoredTestResult>;
}

export class TestResultStore {
  private results = new Map<string, StoredTestResult>();
  private readonly storagePath: string | undefined;
  private saveChain = Promise.resolve();

  constructor(storageUri: { fsPath: string } | undefined) {
    if (storageUri) {
      this.storagePath = path.join(storageUri.fsPath, "testResults.json");
    }
  }

  get size(): number {
    return this.results.size;
  }

  record(itemId: string, status: TestResult["status"], duration?: number): void {
    this.results.set(itemId, { status, duration, timestamp: Date.now() });
  }

  get(itemId: string): TestResult | undefined {
    return this.results.get(itemId);
  }

  delete(itemId: string): void {
    this.results.delete(itemId);
  }

  clear(): void {
    this.results.clear();
  }

  forEach(callback: (result: TestResult, itemId: string) => void): void {
    this.results.forEach((result, id) => callback(result, id));
  }

  async load(): Promise<void> {
    if (!this.storagePath) return;
    try {
      const content = await readFile(this.storagePath, "utf-8");
      const data = JSON.parse(content) as StoredData;
      if (data.version !== 1) return;
      this.results.clear();
      for (const [id, result] of Object.entries(data.results)) {
        this.results.set(id, result);
      }
    } catch { /* No stored data or corrupt — start fresh */ }
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
      results: Object.fromEntries(this.results),
    };
    await mkdir(path.dirname(this.storagePath), { recursive: true });
    await writeFile(this.storagePath, JSON.stringify(data), "utf-8");
  }

  dispose(): void {
    this.results.clear();
  }
}
