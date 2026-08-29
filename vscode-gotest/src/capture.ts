import { spawn } from "node:child_process";
import { stripGoRunExitEcho } from "./cli.js";
import { killProcessTree } from "./processTree.js";
import { CAPTURE_FORCE_KILL_GRACE_MS } from "./config.js";

// CaptureTimeoutError marks the one failure that is not worth retrying: the
// command was given its full budget and did not finish. Retrying spends the
// budget again for the same answer.
export class CaptureTimeoutError extends Error {
  constructor(seconds: number) {
    super(`timed out after ${seconds}s`);
    this.name = "CaptureTimeoutError";
  }
}

export interface CaptureOptions {
  cwd?: string;
  // Wall-clock budget. Omitted means none — for a command whose duration is the
  // repository's business, not ours.
  timeoutSeconds?: number;
  // Called once, when the child's first byte of stdout arrives. Under `go run`
  // that byte lands only after the compile, so it splits build time from the
  // work the command was asked to do — the one number that says which of the
  // two a slow run was spending its time in.
  onFirstByte?: (elapsedMs: number) => void;
}

// captureStdout runs a command and returns everything it wrote to stdout.
//
// execFile would be shorter, and that is the trap: its default 1 MiB cap is a
// ceiling on output that grows with the repository. One byte past it the child
// is killed mid-write and everything it produced is discarded — which is how a
// large project's test tree disappeared from the explorer. Nothing gotest emits
// on stdout is bounded, so nothing reading it may be.
export function captureStdout(
  bin: string,
  args: string[],
  opts: CaptureOptions = {},
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    // Own process group, so the timeout below can signal the compiled binary
    // and not just the `go run` that launched it.
    const child = spawn(bin, args, { cwd: opts.cwd, detached: true });
    const stdout: string[] = [];
    let stderr = "";
    let timedOut = false;
    let settled = false;

    // Decode on the stream, not per chunk: a read ends wherever the pipe filled,
    // and a chunk-wise toString() would corrupt whatever multi-byte character it
    // lands inside. These payloads carry the developer's own text.
    child.stdout.setEncoding("utf-8");
    child.stderr.setEncoding("utf-8");
    const started = Date.now();
    let sawFirstByte = false;
    child.stdout.on("data", (chunk: string) => {
      if (!sawFirstByte) {
        sawFirstByte = true;
        opts.onFirstByte?.(Date.now() - started);
      }
      stdout.push(chunk);
    });
    child.stderr.on("data", (chunk: string) => {
      stderr += chunk;
    });

    let forceKillTimer: ReturnType<typeof setTimeout> | undefined;
    const timer =
      opts.timeoutSeconds === undefined
        ? undefined
        : setTimeout(() => {
            timedOut = true;
            // SIGTERM the group, then escalate. Without the escalation a child
            // that ignores SIGTERM never closes its pipes, `close` never fires,
            // and this promise never settles — which wedges the caller far more
            // thoroughly than the slow command the budget was meant to bound.
            //
            // Armed before the signal, because a child that dies on SIGTERM
            // settles us from inside the call below — and a timer armed after
            // that would outlive the settle that was supposed to clear it.
            forceKillTimer = setTimeout(() => {
              if (settled) return;
              killProcessTree(child, "SIGKILL");
            }, CAPTURE_FORCE_KILL_GRACE_MS);
            killProcessTree(child, "SIGTERM");
          }, opts.timeoutSeconds * 1000);

    const settle = (outcome: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      clearTimeout(forceKillTimer);
      outcome();
    };

    // A missing binary arrives here rather than as an exit code, and callers
    // read `code` off the error to drop a cached path — so pass it through.
    child.on("error", (err: Error) => settle(() => reject(err)));

    // `close` waits for the pipes, which is what we want for the output — but a
    // surviving grandchild can hold them open forever. Once we have timed out
    // the output is being discarded anyway, so `exit` is enough to settle.
    child.on("exit", () => {
      if (!timedOut) return;
      settle(() => reject(new CaptureTimeoutError(opts.timeoutSeconds ?? 0)));
    });

    child.on("close", (code) => {
      settle(() => {
        if (timedOut) {
          reject(new CaptureTimeoutError(opts.timeoutSeconds ?? 0));
        } else if (code === 0) {
          resolve(stdout.join(""));
        } else {
          const detail = stripGoRunExitEcho(stderr);
          reject(
            new Error(`exited with code ${code}${detail ? `: ${detail}` : ""}`),
          );
        }
      });
    });
  });
}
