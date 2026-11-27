// Poster coalesces what the host tells the webview: a channel marked again
// and again posts at most once per COALESCE_MS, a hidden panel gets nothing
// until it is seen, and a tab not on show waits for its turn; the badges
// always go.
import type { HostMessage, Tab } from "./protocol.js";

export const COALESCE_MS = 250;

export type Channel = Tab | "badges" | "running";

export interface PosterTarget {
  visible(): boolean;
  tab(): Tab;
  post(msg: HostMessage): void;
}

export class Poster {
  private readonly dirty = new Set<Channel>();
  private readonly last = new Map<Channel, number>();
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private readonly target: PosterTarget,
    // The message a channel stands for, built when it is posted.
    private readonly build: (ch: Channel) => HostMessage | undefined,
    private readonly now: () => number = Date.now,
  ) {}

  // mark says a channel's message changed.
  mark(...channels: Channel[]): void {
    for (const ch of channels) this.dirty.add(ch);
    this.schedule();
  }

  isDirty(ch: Channel): boolean {
    return this.dirty.has(ch);
  }

  // due says whether a channel may go now: the panel is seen, and a tab's
  // channel is the tab on show; Running belongs to Runs.
  private due(ch: Channel): boolean {
    if (!this.target.visible()) return false;
    if (ch === "badges") return true;
    if (ch === "running") return this.target.tab() === "runs";
    return this.target.tab() === ch;
  }

  private schedule(): void {
    if (this.timer) return;
    const t = this.now();
    let wait = 0;
    for (const ch of this.dirty) {
      if (!this.due(ch)) continue;
      const since = t - (this.last.get(ch) ?? -Infinity);
      wait = Math.max(wait, Math.max(0, COALESCE_MS - since));
    }
    this.timer = setTimeout(() => {
      this.timer = undefined;
      this.flush();
    }, wait);
  }

  // flush posts every due channel that waited its interval; the rest wait.
  flush(): void {
    const t = this.now();
    let waiting = false;
    for (const ch of Array.from(this.dirty)) {
      if (!this.due(ch)) continue;
      if (t - (this.last.get(ch) ?? -Infinity) < COALESCE_MS) {
        waiting = true;
        continue;
      }
      this.dirty.delete(ch);
      this.last.set(ch, t);
      const msg = this.build(ch);
      if (msg) this.target.post(msg);
    }
    if (waiting) this.schedule();
  }

  // seen posts what waited while the panel was hidden or a tab not shown.
  seen(): void {
    this.schedule();
  }

  dispose(): void {
    clearTimeout(this.timer);
    this.timer = undefined;
  }
}
