import * as path from "node:path";
import { randomUUID } from "node:crypto";
import { readFile, readdir, unlink } from "node:fs/promises";
import { atomicWrite, reportStoreError } from "./jsonStore.js";
import { identify, readProcessStartToken } from "./processIdentity.js";

export interface ProcessRecord {
  key: string;
  pid: number;
  kind: string;
  startedAt: number;
  // Proof of identity, so a recycled pid is never mistaken for ours. A record
  // is only written when one can be produced.
  startToken: string;
}

interface SessionFile {
  version: 2;
  sessionId: string;
  // The extension host that owns these children. A session whose host is still
  // alive owns its processes; nobody else may touch them.
  hostPid: number;
  hostToken: string;
  processes: ProcessRecord[];
}

const PREFIX = "child-processes-";
const SUFFIX = ".json";

export interface ReapOptions {
  kill: (pid: number, signal: NodeJS.Signals) => void;
  // How long a signalled orphan gets before SIGKILL. An orphan is a run or a
  // daemon by definition — it can be mid-fixture-teardown — so this is the
  // teardown grace, passed in rather than read here to keep this module free of
  // vscode configuration.
  graceMs: number;
}

export interface ReapResult {
  signalled: ProcessRecord[];
  vanished: ProcessRecord[];
}

// ProcessRegistry is the answer to "what did a session that is no longer
// running leave behind?".
//
// Every child is spawned detached so that termination can signal its whole
// group. That is necessary — the direct child is `go` and the process holding
// the pipes is the binary it compiled — but it is symmetric: a detached child
// also survives *us*. Kill the extension host and a `gotest watch`, a process
// designed to run for hours, keeps running with nothing left to end it.
//
// Each session owns one file, named for itself. That is the whole design, and
// it is what makes two mistakes impossible rather than merely unlikely:
//
//   - A session can never overwrite another's table, because it only ever
//     writes its own file. A shared file had to be read before it could be
//     safely written, and a spawn during activation happens before that read.
//   - A session can never reap another's live children, because a file is only
//     eligible once its owning host is verifiably gone. Process identity
//     answers "is this still the process the record named?"; it cannot answer
//     "is this one of mine?", and conflating the two is how a reaper ends up
//     SIGTERMing the discovery its own activation just started — or the run in
//     a second window open on the same folder.
//
// Deliberately separate from RunRegistry. That tracks logical runs — kind,
// packages, a status a user might see. This tracks operating-system processes,
// a different question with a different lifetime.
export class ProcessRegistry {
  private current = new Map<string, ProcessRecord>();
  // Inherited records, and the file each came from, so a file can be cleaned up
  // once nothing in it is still running.
  private previous = new Map<string, { record: ProcessRecord; file: string }>();
  private saveChain = Promise.resolve();
  private seq = 0;
  private escalation: ReturnType<typeof setTimeout> | undefined;
  private readonly sessionId: string;
  private readonly hostPid: number;
  // Files absorbed from dead sessions, kept so they can be shrunk as their
  // processes are confirmed gone and removed once nothing is left.
  private inheritedFiles = new Set<string>();

  constructor(
    private readonly storageDir: string | undefined,
    identity: { sessionId?: string; hostPid?: number } = {},
  ) {
    this.sessionId = identity.sessionId ?? randomUUID();
    this.hostPid = identity.hostPid ?? process.pid;
  }

  get size(): number {
    return this.current.size + this.previous.size;
  }

  get inheritedSize(): number {
    return this.previous.size;
  }

  // add persists immediately rather than on a debounce: the whole point is to
  // survive a crash that could happen at any moment, and spawns are rare enough
  // that a write each is nothing.
  //
  // A process we could not identify later can never be reaped, so recording it
  // would be pure overhead. That makes this Linux-only today, and free
  // everywhere else, rather than a table nobody can ever act on.
  add(pid: number | undefined, kind: string): string | undefined {
    if (pid === undefined) return undefined;
    const startToken = readProcessStartToken(pid);
    if (!startToken) return undefined;

    const key = `${pid}-${Date.now()}-${this.seq++}`;
    this.current.set(key, {
      key,
      pid,
      kind,
      startedAt: Date.now(),
      startToken,
    });
    this.persist();
    return key;
  }

  remove(key: string | undefined): void {
    if (!key) return;
    if (this.current.delete(key)) this.persist();
  }

  // load absorbs the tables of sessions whose hosts are gone. A file belonging
  // to a live host — this one, or a second window on the same folder — is left
  // strictly alone.
  async load(): Promise<void> {
    if (!this.storageDir) return;
    let entries: string[];
    try {
      entries = await readdir(this.storageDir);
    } catch {
      return;
    }

    for (const entry of entries) {
      if (!entry.startsWith(PREFIX) || !entry.endsWith(SUFFIX)) continue;
      if (entry === this.ownFile()) continue;
      const full = path.join(this.storageDir, entry);
      try {
        const data = JSON.parse(await readFile(full, "utf-8")) as SessionFile;
        if (data.version !== 2) continue;
        if (identify(data.hostPid, data.hostToken) === "same-process") {
          // That session is still running. Its children are its business.
          continue;
        }
        if ((data.processes ?? []).length === 0) {
          await unlink(full).catch(() => {});
          continue;
        }
        this.inheritedFiles.add(full);
        for (const record of data.processes) {
          this.previous.set(record.key, { record, file: full });
        }
      } catch {
        // Unreadable or corrupt: it names processes we cannot verify, so it
        // cannot be acted on. Remove it rather than re-reading it forever.
        await unlink(full).catch(() => {});
      }
    }
  }

  // reapOrphans signals what a dead session left behind and schedules the
  // escalation. It returns as soon as the SIGTERMs are away: activation must
  // not wait out a fixture teardown.
  //
  // A signalled record is kept until the process is positively gone, so a
  // survivor is retried by the next activation rather than forgotten.
  reapOrphans(opts: ReapOptions): ReapResult {
    const signalled: ProcessRecord[] = [];
    const vanished: ProcessRecord[] = [];

    for (const [key, { record }] of [...this.previous]) {
      if (identify(record.pid, record.startToken) !== "same-process") {
        // Gone, replaced, or unverifiable — either way, not something we may
        // signal. A false positive kills a stranger's process.
        this.previous.delete(key);
        vanished.push(record);
        continue;
      }
      try {
        // The group, not the process: an orphaned `go run` has its own
        // orphaned child holding the real work.
        opts.kill(-record.pid, "SIGTERM");
        signalled.push(record);
      } catch {
        this.previous.delete(key);
        vanished.push(record);
      }
    }

    if (signalled.length > 0) {
      this.escalation = setTimeout(() => {
        this.escalation = undefined;
        this.escalate(opts.kill);
      }, opts.graceMs);
    }

    this.persist();
    return { signalled, vanished };
  }

  // escalate is the second half of termination. SIGTERM alone is a request; a
  // child that ignores it — or whose teardown is wedged — is still running, and
  // the rest of this extension has exactly one rule about that.
  private escalate(kill: (pid: number, signal: NodeJS.Signals) => void): void {
    for (const [key, { record }] of [...this.previous]) {
      if (identify(record.pid, record.startToken) !== "same-process") {
        this.previous.delete(key);
        continue;
      }
      try {
        kill(-record.pid, "SIGKILL");
      } catch {
        // Gone between the check and the signal.
      }
      // Kept if it is somehow still there: an unkillable process is worth one
      // more attempt next activation, not silent forgetting.
      if (identify(record.pid, record.startToken) !== "same-process") {
        this.previous.delete(key);
      }
    }
    this.persist();
  }

  flush(): Promise<void> {
    return this.saveChain;
  }

  dispose(): void {
    clearTimeout(this.escalation);
  }

  private ownFile(): string {
    return `${PREFIX}${this.sessionId}${SUFFIX}`;
  }

  private persist(): void {
    this.saveChain = this.saveChain
      .then(() => this.writeToDisk())
      .catch((err) => {
        // A lost record costs us one orphan at worst; it must never take the
        // spawn that triggered it down with it.
        reportStoreError("write process registry", err);
      });
  }

  private async writeToDisk(): Promise<void> {
    if (!this.storageDir) return;
    const hostToken = readProcessStartToken(this.hostPid);
    // Without a host token nobody could ever tell whether this session is still
    // alive, and an unverifiable owner is exactly what must not be signalled.
    if (!hostToken) return;

    const data: SessionFile = {
      version: 2,
      sessionId: this.sessionId,
      hostPid: this.hostPid,
      hostToken,
      processes: [...this.current.values()],
    };
    await atomicWrite(
      path.join(this.storageDir, this.ownFile()),
      JSON.stringify(data),
    );

    await this.reconcileInherited();
  }

  // Inherited files shrink as their processes are confirmed gone, and disappear
  // when nothing in them is left. Rewriting rather than deleting outright keeps
  // a survivor visible to the next session.
  private async reconcileInherited(): Promise<void> {
    if (!this.storageDir) return;
    const byFile = new Map<string, ProcessRecord[]>();
    for (const { record, file } of this.previous.values()) {
      const list = byFile.get(file) ?? [];
      list.push(record);
      byFile.set(file, list);
    }

    for (const file of [...this.inheritedFiles]) {
      const remaining = byFile.get(file) ?? [];
      try {
        if (remaining.length === 0) {
          await unlink(file);
          this.inheritedFiles.delete(file);
          continue;
        }
        const existing = JSON.parse(
          await readFile(file, "utf-8"),
        ) as SessionFile;
        await atomicWrite(
          file,
          JSON.stringify({ ...existing, processes: remaining }),
        );
      } catch {
        // Housekeeping; a failure here costs one retry next activation.
      }
    }
  }
}
