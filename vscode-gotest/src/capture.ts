import { stripGoRunExitEcho } from "./cli.js";
import { ManagedChild } from "./managedChild.js";

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
export async function captureStdout(
  bin: string,
  args: string[],
  opts: CaptureOptions = {},
): Promise<string> {
  const mc = new ManagedChild(bin, args, { cwd: opts.cwd, kind: "read" });
  const stdout: string[] = [];
  let stderr = "";
  let timedOut = false;

  const started = Date.now();
  let sawFirstByte = false;
  mc.child.stdout?.on("data", (chunk: string) => {
    if (!sawFirstByte) {
      sawFirstByte = true;
      opts.onFirstByte?.(Date.now() - started);
    }
    stdout.push(chunk);
  });
  mc.child.stderr?.on("data", (chunk: string) => {
    stderr += chunk;
  });

  // A missing binary arrives as an error event rather than an exit code, and
  // callers read `code` off it to drop a cached path — so keep the real error.
  let spawnError: Error | undefined;
  mc.child.on("error", (err: Error) => {
    spawnError = err;
  });

  // A read: no fixtures to tear down, and past the budget the output is being
  // discarded anyway, so it gets the prompt escalation.
  const budget =
    opts.timeoutSeconds === undefined
      ? undefined
      : setTimeout(() => {
          timedOut = true;
          void mc.terminate("prompt");
        }, opts.timeoutSeconds * 1000);

  try {
    const { code } = await mc.exited;
    if (spawnError) throw spawnError;
    if (timedOut) throw new CaptureTimeoutError(opts.timeoutSeconds ?? 0);
    if (code === 0) return stdout.join("");
    const detail = stripGoRunExitEcho(stderr);
    throw new Error(`exited with code ${code}${detail ? `: ${detail}` : ""}`);
  } finally {
    clearTimeout(budget);
    mc.dispose();
  }
}
