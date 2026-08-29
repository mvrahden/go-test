import { spawn, type ChildProcess } from "node:child_process";
import { killProcessTree } from "./processTree.js";
import {
  CAPTURE_FORCE_KILL_GRACE_MS,
  forceKillTimeoutSeconds,
} from "./config.js";

// How long the child gets between SIGTERM and SIGKILL. Classified by what the
// child is doing, not by which module spawned it:
//
//   prompt   — a read (`discover`, `cover -func`, `spec`). No fixtures, nothing
//              to tear down, and its output is being discarded anyway.
//   teardown — a run or a daemon (`gotest`, `watch`, `prepare`). SIGTERM is what
//              starts fixture teardown, and the CLI allows that up to
//              GracefulShutdownDelay (5m30s), so we must outlast it or we cut
//              container teardown off and leak what it was holding.
export type TerminationGrace = "prompt" | "teardown";

export interface ManagedChildOptions {
  cwd?: string;
  env?: Record<string, string>;
  // What this child is, for the process registry. Coarse on purpose — the
  // registry only needs enough to name what it reaped in a log line.
  kind?: string;
}

// Observes every child this extension starts, so something outside can know
// what is running. Installed once at activation; a module-level hook rather
// than a constructor argument because the alternative is threading a registry
// through five call sites that have no other use for it.
export interface ChildObserver {
  spawned(pid: number | undefined, kind: string): string | undefined;
  exited(token: string | undefined): void;
}

let observer: ChildObserver | undefined;

export function setChildObserver(next: ChildObserver | undefined): void {
  observer = next;
}

export interface Exit {
  code: number | null;
  signal: NodeJS.Signals | null;
}

// ManagedChild owns the part of running a child process that nobody should
// re-derive: it is in its own process group, both streams are decoded, and
// there is exactly one idempotent termination path that always reaches a
// terminal state.
//
// It deliberately does not own reading. Line framing, JSON cycle detection and
// stdin writing differ at every call site and belong there — the four
// lifecycles in this extension (budgeted read, cancellable run, restarting
// daemon, start-and-leave-running) share a guarantee, not a call shape.
//
// The caller MUST consume stdout and stderr. An unread pipe fills, the child
// blocks on write, and `exited` never settles — the stall looks like a hang in
// the CLI rather than a missing listener here.
export class ManagedChild {
  readonly child: ChildProcess;
  readonly exited: Promise<Exit>;

  private terminating = false;
  private escalation: ReturnType<typeof setTimeout> | undefined;
  private settle!: (exit: Exit) => void;
  private settled = false;
  private registration: string | undefined;

  constructor(
    bin: string,
    args: string[],
    private readonly opts: ManagedChildOptions = {},
  ) {
    // Detached, so termination can signal the whole group. Everything here runs
    // `go run <module> ...`: the direct child is `go` and the process holding
    // the pipes is the binary it compiled. Signalling only the child leaves
    // that grandchild alive.
    this.child = spawn(bin, args, {
      cwd: opts.cwd,
      env: opts.env ? { ...process.env, ...opts.env } : undefined,
      detached: true,
    });

    // Decoded on the stream, not per chunk: a read ends wherever the pipe
    // filled, and a chunk-wise toString() would corrupt whatever multi-byte
    // character it lands inside. These payloads carry the developer's own text.
    this.child.stdout?.setEncoding("utf-8");
    this.child.stderr?.setEncoding("utf-8");

    this.registration = observer?.spawned(this.child.pid, opts.kind ?? "child");

    this.exited = new Promise<Exit>((resolve) => {
      this.settle = (exit: Exit) => {
        if (this.settled) return;
        this.settled = true;
        clearTimeout(this.escalation);
        observer?.exited(this.registration);
        this.registration = undefined;
        resolve(exit);
      };
    });

    // `close` is the honest signal while the child is running: it waits for the
    // pipes, so the caller has all the output. Once we have started killing it,
    // `exit` is enough — a surviving grandchild can hold the pipes open past the
    // child's death, and waiting for `close` there is how a budget stops
    // bounding anything.
    this.child.on("close", (code, signal) => this.settle({ code, signal }));
    this.child.on("exit", (code, signal) => {
      if (this.terminating) this.settle({ code, signal });
    });
    // A missing binary arrives here rather than as an exit code. Callers read
    // `code` off the error, so they attach their own handler; this one only
    // makes sure `exited` cannot hang.
    this.child.on("error", () => this.settle({ code: null, signal: null }));
  }

  get pid(): number | undefined {
    return this.child.pid;
  }

  // terminate is idempotent and always resolves. Calling it twice does not
  // restart the escalation, and calling it on an already-dead child is a no-op
  // that resolves immediately.
  terminate(grace: TerminationGrace): Promise<Exit> {
    if (this.settled || this.terminating) return this.exited;
    this.terminating = true;

    const graceMs =
      grace === "prompt"
        ? CAPTURE_FORCE_KILL_GRACE_MS
        : forceKillTimeoutSeconds(this.opts.cwd) * 1000;

    // Armed before the signal is sent. A child that dies synchronously inside
    // kill() settles us from within the call below, and a timer armed after
    // that would outlive the settle that was supposed to clear it — leaving a
    // SIGKILL scheduled against a finished process, or a recycled pid.
    this.escalation = setTimeout(() => {
      if (this.settled) return;
      killProcessTree(this.child, "SIGKILL");
    }, graceMs);

    killProcessTree(this.child, "SIGTERM");
    return this.exited;
  }

  dispose(): void {
    clearTimeout(this.escalation);
  }
}
