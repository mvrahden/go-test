import * as path from "node:path";
import { readFile, writeFile, mkdir, rename } from "node:fs/promises";

// Where a store reports a write it could not make. These are caches, so a
// failure must never propagate into the operation that triggered it — but
// silence is not the same as harmlessness, and until now nothing anywhere
// logged a failed write.
let reportError: (message: string) => void = () => {};

export function setStoreErrorLogger(
  log: ((message: string) => void) | undefined,
): void {
  reportError = log ?? (() => {});
}

export function reportStoreError(context: string, err: unknown): void {
  const message = err instanceof Error ? err.message : String(err);
  reportError(`[store] ${context}: ${message}`);
}

// Marks a store's memory as ahead of disk.
//
// A store loads exactly once. Reading from disk after anything has been
// recorded in memory discards the newer state — a rule broken three separate
// times (a second load during activation, a snapshot load racing a
// watcher-triggered discovery, a coverage load racing an invalidation) and
// patched three separate ways. One implementation, so a fourth store gets it
// without anyone noticing it needed it.
export class LoadOnce {
  private mutated = false;

  markMutated(): void {
    this.mutated = true;
  }

  get blocked(): boolean {
    return this.mutated;
  }
}

// atomicWrite replaces a file rather than rewriting it in place. Every store
// here wrote straight over its target, so a crash mid-write left truncated
// JSON; each load() catches and starts fresh, so it degraded to cache-loss
// rather than corruption, but losing a cache to a power cut is avoidable for
// two lines. rename(2) is atomic within a filesystem, and the temp file is a
// sibling so it always is one.
export async function atomicWrite(
  filePath: string,
  content: string,
): Promise<void> {
  await mkdir(path.dirname(filePath), { recursive: true });
  // Unique per writer. Two VS Code windows on one folder share workspace
  // storage, so a fixed temp name lets them interleave writes into the same
  // file and rename a mixture of both into place.
  const tmp = `${filePath}.${process.pid}.${tmpSeq++}.tmp`;
  await writeFile(tmp, content, "utf-8");
  await rename(tmp, filePath);
}

let tmpSeq = 0;

interface Envelope<T> {
  version: number;
  data: T;
}

// JsonStore is the persistence half of a store, and only that half: the
// in-memory model, and what `load` means for it, stay with the owner.
//
// It carries the shared LoadOnce latch, so the load-exactly-once rule is the
// same implementation here and in the stores that manage their own files.
export class JsonStore<T> {
  private readonly filePath: string | undefined;
  private saveChain = Promise.resolve();
  private debounceTimer: ReturnType<typeof setTimeout> | undefined;
  private readonly latch = new LoadOnce();
  private snapshot: (() => T) | undefined;

  private static readonly DEBOUNCE_MS = 500;

  constructor(
    storageDir: string | undefined,
    filename: string,
    private readonly version: number,
  ) {
    this.filePath = storageDir ? path.join(storageDir, filename) : undefined;
  }

  // markMutated is the owner telling the store that memory is now ahead of
  // disk. Every mutating method must call it; that is the whole contract.
  markMutated(): void {
    this.latch.markMutated();
  }

  // read returns undefined when there is nothing safe to load: no storage, a
  // version this build does not understand, a missing or corrupt file — or
  // memory that is already ahead of disk.
  async read(): Promise<T | undefined> {
    if (!this.filePath || this.latch.blocked) return undefined;
    try {
      const content = await readFile(this.filePath, "utf-8");
      const envelope = JSON.parse(content) as Envelope<T>;
      if (envelope.version !== this.version) return undefined;
      return envelope.data;
    } catch {
      return undefined;
    }
  }

  // save is always void and always debounced — one contract, so no call site
  // has to know whether this particular store returns something to await.
  // flush() is the only thing that can be awaited.
  save(snapshot: () => T): void {
    this.snapshot = snapshot;
    if (this.debounceTimer !== undefined) clearTimeout(this.debounceTimer);
    this.debounceTimer = setTimeout(() => {
      this.debounceTimer = undefined;
      this.enqueue();
    }, JsonStore.DEBOUNCE_MS);
  }

  flush(): Promise<void> {
    if (this.debounceTimer !== undefined) {
      clearTimeout(this.debounceTimer);
      this.debounceTimer = undefined;
      this.enqueue();
    }
    return this.saveChain;
  }

  private enqueue(): void {
    const snapshot = this.snapshot;
    if (!this.filePath || !snapshot) return;
    const filePath = this.filePath;
    this.saveChain = this.saveChain
      .then(() =>
        atomicWrite(
          filePath,
          JSON.stringify({ version: this.version, data: snapshot() }),
        ),
      )
      .catch((err) => {
        // These are caches. A failed write costs the next session its head
        // start and must never propagate into the operation that triggered it
        // — but it is said out loud.
        reportStoreError(`write ${filePath}`, err);
      });
  }
}
