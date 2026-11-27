// The reviewer's acknowledgements in the Change scope: local editor state,
// kept in the workspace's memento and never sent anywhere. Each folder keeps
// the rows accepted against one head commit; a new head starts empty.

export interface Memento {
  get<T>(key: string): T | undefined;
  update(key: string, value: unknown): Thenable<void>;
}

const KEY = "gotest.audit.accepted";

interface Entry {
  head: string;
  keys: string[];
}

export class AcceptanceStore {
  constructor(private readonly memento: Memento) {}

  private all(): Record<string, Entry> {
    const v = this.memento.get<Record<string, Entry>>(KEY);
    return v && typeof v === "object" ? v : {};
  }

  // accepted lists the rows accepted in folder against head.
  accepted(folder: string, head: string): Set<string> {
    const e = this.all()[folder];
    if (!e || e.head !== head || !Array.isArray(e.keys)) return new Set();
    return new Set(e.keys);
  }

  // toggle accepts a row, or takes its acceptance back; it reports whether
  // the row is accepted now. Accepting against a new head forgets the old.
  async toggle(folder: string, head: string, key: string): Promise<boolean> {
    const all = this.all();
    const keys = this.accepted(folder, head);
    const now = !keys.delete(key);
    if (now) keys.add(key);
    all[folder] = { head, keys: Array.from(keys) };
    await this.memento.update(KEY, all);
    return now;
  }
}
