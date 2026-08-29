import * as path from "node:path";
import { readFile } from "node:fs/promises";
import { atomicWrite } from "./jsonStore.js";

export type RunKind = "test" | "coverage" | "watch" | "prepare";
export type RunStatus = "running" | "completed" | "cancelled" | "crashed";

export interface RunRecord {
  id: string;
  kind: RunKind;
  status: RunStatus;
  startedAt: number;
  endedAt?: number;
  packages: string[];
}

export interface RegisterInput {
  kind: RunKind;
  packages: string[];
}

const TTL_MS = 15 * 60 * 1000;
const REGISTRY_FILE = "run-registry.json";

// Bump when RunRecord's shape changes. Without this the file was a bare array,
// so a shape change had no way to invalidate records written by an older build
// — they would be read back and reasoned about as if they were current.
const STORE_VERSION = 1;

interface StoredData {
  version: number;
  records: RunRecord[];
}

export class RunRegistry {
  private records = new Map<string, RunRecord>();

  constructor(private readonly storageDir: string) {}

  register(input: RegisterInput): RunRecord {
    const record: RunRecord = {
      id: crypto.randomUUID(),
      kind: input.kind,
      status: "running",
      startedAt: Date.now(),
      packages: input.packages,
    };
    this.records.set(record.id, record);
    return record;
  }

  get(id: string): RunRecord | undefined {
    return this.records.get(id);
  }

  complete(id: string): void {
    this.transition(id, "completed");
  }

  cancel(id: string): void {
    this.transition(id, "cancelled");
  }

  crash(id: string): void {
    this.transition(id, "crashed");
  }

  active(): RunRecord[] {
    return [...this.records.values()].filter((r) => r.status === "running");
  }

  all(): RunRecord[] {
    return [...this.records.values()];
  }

  sweepStale(): RunRecord[] {
    const crashed: RunRecord[] = [];
    for (const record of this.records.values()) {
      if (record.status !== "running") continue;
      record.status = "crashed";
      record.endedAt = Date.now();
      crashed.push(record);
    }
    return crashed;
  }

  sweep(): void {
    const now = Date.now();
    for (const [id, record] of this.records) {
      if (
        record.status !== "running" &&
        record.endedAt &&
        now - record.endedAt > TTL_MS
      ) {
        this.records.delete(id);
      }
    }
  }

  async save(): Promise<void> {
    const payload: StoredData = {
      version: STORE_VERSION,
      records: [...this.records.values()],
    };
    await atomicWrite(
      path.join(this.storageDir, REGISTRY_FILE),
      JSON.stringify(payload),
    );
  }

  async load(): Promise<void> {
    try {
      const data = await readFile(
        path.join(this.storageDir, REGISTRY_FILE),
        "utf-8",
      );
      const parsed = JSON.parse(data) as StoredData | RunRecord[];
      // A bare array is the pre-versioning file. Its records are 15-minute
      // bookkeeping, so dropping them costs nothing and beats guessing.
      if (Array.isArray(parsed) || parsed.version !== STORE_VERSION) return;
      this.records.clear();
      for (const r of parsed.records ?? []) {
        this.records.set(r.id, r);
      }
    } catch {
      // No file or invalid — start fresh
    }
  }

  private transition(id: string, status: RunStatus): void {
    const record = this.records.get(id);
    if (record) {
      record.status = status;
      record.endedAt = Date.now();
    }
  }
}
