// The runs chosen, per folder, as evidence for the Change scope's audits:
// their events files go to `gotest audit` as --evidence. Kept in the
// workspace's state; the runs themselves are the CLI's records.
import type { Memento } from "./acceptance.js";
import type { EvidencePick } from "./runsModel.js";

const KEY = "gotest.runs.evidence";

export class EvidenceSelection {
  private readonly listeners: (() => void)[] = [];

  constructor(private readonly memento: Memento) {}

  private all(): Record<string, EvidencePick[]> {
    const v = this.memento.get<Record<string, EvidencePick[]>>(KEY);
    return v && typeof v === "object" ? v : {};
  }

  list(folder: string): EvidencePick[] {
    const picks = this.all()[folder];
    return Array.isArray(picks)
      ? picks.filter(
          (p) =>
            p &&
            typeof p.id === "string" &&
            typeof p.file === "string" &&
            typeof p.label === "string",
        )
      : [];
  }

  has(folder: string, id: string): boolean {
    return this.list(folder).some((p) => p.id === id);
  }

  // toggle takes a run as evidence, or drops it when it is taken.
  async toggle(folder: string, pick: EvidencePick): Promise<void> {
    const list = this.list(folder);
    const next = list.some((p) => p.id === pick.id)
      ? list.filter((p) => p.id !== pick.id)
      : [...list, pick];
    await this.write(folder, next);
  }

  async remove(folder: string, id: string): Promise<void> {
    await this.write(
      folder,
      this.list(folder).filter((p) => p.id !== id),
    );
  }

  onChange(fn: () => void): void {
    this.listeners.push(fn);
  }

  private async write(folder: string, picks: EvidencePick[]): Promise<void> {
    const all = this.all();
    if (picks.length > 0) all[folder] = picks;
    else delete all[folder];
    await this.memento.update(KEY, all);
    for (const fn of this.listeners) fn();
  }
}
