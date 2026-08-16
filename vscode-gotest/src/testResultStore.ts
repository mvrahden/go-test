import * as path from "node:path";
import { readFile, writeFile, mkdir } from "node:fs/promises";

export interface TestResult {
  status: "pass" | "fail" | "skip";
  /**
   * What go test measured for this node alone. For a leaf that is its execution
   * time; for a node with children it is close to useless, because go test
   * stops a parent's clock when the parent function returns.
   */
  duration?: number;
  /**
   * The node's bracket in the event stream — when go test registered it and
   * when it reported. It encloses every descendant, so for a node that was not
   * parked it is what that node cost.
   */
  startedAt?: number;
  endedAt?: number;
  /**
   * The node called t.Parallel, so it was parked before it ran. Its bracket is
   * then worthless in both halves: it opens at registration rather than at the
   * start of work, and go test flushes a parked test's report through its
   * parent, which can delay the close by however long a slower sibling runs.
   * Its duration stays exact, because go test does not count parked time.
   */
  paused?: boolean;
}

interface StoredTestResult extends TestResult {
  timestamp: number;
}

// Version 2 is the shape v1.27 ships: a verdict, the wall-clock bracket, and
// whether the node was parked. Version 1 — every release through v1.26 —
// carries none of the last three and cannot be migrated: an upgraded row would
// keep reporting the old number, unlabelled, in a table that claims to show
// intervals. This is a cache of a run that repeats in one command and expires
// in a week regardless, so a mismatch drops it and starts clean. The number
// cannot catch a store left by an unreleased v1.27 build, which predates these
// fields without saying so; that one falls back to the pre-bracket numbers
// until the next run replaces it.
const STORE_VERSION = 2;

interface StoredData {
  version: number;
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
    bracket?: { startedAt?: number; endedAt?: number; paused?: boolean },
  ): void {
    this.results.set(itemId, {
      status,
      duration,
      startedAt: bracket?.startedAt,
      endedAt: bracket?.endedAt,
      paused: bracket?.paused,
      timestamp: Date.now(),
    });
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
      if (data.version !== STORE_VERSION) return;
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
      version: STORE_VERSION,
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
