// A working stand-in for the slice of the VS Code testing API the extension
// writes results into. Everything here is real data structures rather than
// spies: TestItems really are added to collections, and a run's verdicts are
// recorded per item id, so tests can assert on the tree the Test Explorer would
// have shown rather than on the fact that a method was called.

export class FakeTestItemCollection {
  private items = new Map<string, FakeTestItem>();

  // The real API sets `parent` when an item joins a collection, and code that
  // walks up the tree (run filters, package depth) depends on it. Without the
  // owner link the double silently reports every item as parentless.
  constructor(private readonly owner?: FakeTestItem) {}

  get size(): number {
    return this.items.size;
  }

  add(item: FakeTestItem): void {
    item.parent = this.owner;
    this.items.set(item.id, item);
  }

  get(id: string): FakeTestItem | undefined {
    return this.items.get(id);
  }

  delete(id: string): void {
    this.items.delete(id);
  }

  replace(items: FakeTestItem[]): void {
    this.items.clear();
    for (const item of items) {
      item.parent = this.owner;
      this.items.set(item.id, item);
    }
  }

  forEach(callback: (item: FakeTestItem) => void): void {
    for (const item of this.items.values()) callback(item);
  }

  [Symbol.iterator](): IterableIterator<[string, FakeTestItem]> {
    return this.items.entries();
  }
}

export class FakeTestItem {
  children: FakeTestItemCollection = new FakeTestItemCollection(this);
  parent: FakeTestItem | undefined;
  range: unknown;
  tags: unknown[] = [];
  canResolveChildren = false;
  busy = false;
  error: string | undefined;
  description: string | undefined;
  sortText: string | undefined;

  constructor(
    public id: string,
    public label: string,
    public uri?: unknown,
  ) {}
}

export type Verdict =
  | "enqueued"
  | "started"
  | "passed"
  | "failed"
  | "errored"
  | "skipped";

// Records what a run reported, keyed by test item id. Later verdicts overwrite
// earlier ones, which mirrors how the Test Explorer displays an item.
export class FakeTestRun {
  readonly verdicts = new Map<string, Verdict>();
  readonly messages = new Map<string, string[]>();
  readonly output: string[] = [];
  readonly coverage: unknown[] = [];
  ended = false;

  private record(item: FakeTestItem, verdict: Verdict): void {
    this.verdicts.set(item.id, verdict);
  }

  enqueued(item: FakeTestItem): void {
    this.record(item, "enqueued");
  }
  started(item: FakeTestItem): void {
    this.record(item, "started");
  }
  passed(item: FakeTestItem): void {
    this.record(item, "passed");
  }
  skipped(item: FakeTestItem): void {
    this.record(item, "skipped");
  }
  failed(item: FakeTestItem, message?: unknown): void {
    this.record(item, "failed");
    this.addMessage(item, message);
  }
  errored(item: FakeTestItem, message?: unknown): void {
    this.record(item, "errored");
    this.addMessage(item, message);
  }
  appendOutput(text: string): void {
    this.output.push(text);
  }
  addCoverage(c: unknown): void {
    this.coverage.push(c);
  }
  end(): void {
    this.ended = true;
  }

  // The real API accepts a TestMessage, an array of them, or a string, and the
  // extension uses all three. Flattening here keeps assertions about what a
  // developer would actually read on the item.
  private addMessage(item: FakeTestItem, message: unknown): void {
    if (message === undefined) return;
    const list = this.messages.get(item.id) ?? [];
    for (const entry of Array.isArray(message) ? message : [message]) {
      if (entry === undefined || entry === null) continue;
      list.push(
        typeof entry === "string"
          ? entry
          : ((entry as { message?: string }).message ?? String(entry)),
      );
    }
    this.messages.set(item.id, list);
  }

  verdictsMatching(verdict: Verdict): string[] {
    return [...this.verdicts.entries()]
      .filter(([, v]) => v === verdict)
      .map(([id]) => id)
      .sort();
  }
}

export class FakeTestController {
  items = new FakeTestItemCollection();
  refreshHandler: unknown;
  resolveHandler: unknown;
  readonly runs: FakeTestRun[] = [];
  readonly profiles: { label: string; kind: number }[] = [];

  constructor(
    public id: string,
    public label: string,
  ) {}

  createTestItem(id: string, label: string, uri?: unknown): FakeTestItem {
    return new FakeTestItem(id, label, uri);
  }

  createRunProfile(
    label: string,
    kind: number,
  ): { label: string; kind: number; dispose: () => void } {
    const profile = { label, kind, dispose: () => {} };
    this.profiles.push(profile);
    return profile;
  }

  createTestRun(): FakeTestRun {
    const run = new FakeTestRun();
    this.runs.push(run);
    return run;
  }

  invalidateTestResults(): void {}

  dispose(): void {}

  // Depth-first walk, so a test can assert on the whole tree the extension built.
  allItems(): FakeTestItem[] {
    const out: FakeTestItem[] = [];
    const walk = (collection: FakeTestItemCollection) => {
      collection.forEach((item) => {
        out.push(item);
        walk(item.children);
      });
    };
    walk(this.items);
    return out;
  }
}

export class FakeEventEmitter<T> {
  private listeners: ((value: T) => void)[] = [];
  readonly event = (listener: (value: T) => void) => {
    this.listeners.push(listener);
    return { dispose: () => {} };
  };
  fire(value: T): void {
    for (const listener of this.listeners) listener(value);
  }
  dispose(): void {
    this.listeners = [];
  }
}

export class FakeCancellationTokenSource {
  private listeners: (() => void)[] = [];
  token = {
    isCancellationRequested: false,
    onCancellationRequested: (listener: () => void) => {
      this.listeners.push(listener);
      return { dispose: () => {} };
    },
  };
  cancel(): void {
    this.token.isCancellationRequested = true;
    for (const listener of this.listeners) listener();
  }
  dispose(): void {
    this.listeners = [];
  }
}
