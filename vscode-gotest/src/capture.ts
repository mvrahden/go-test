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

export interface CaptureOutputOptions extends CaptureOptions {
  // Ends the child: a newer request has made its answer worthless.
  abort?: AbortSignal;
  // A child that writes files gets the teardown grace, so a SIGKILL never
  // lands while it is writing them.
  grace?: "prompt" | "teardown";
}

export interface CapturedOutput {
  code: number | null;
  stdout: string;
  stderr: string;
  // The abort signal ended it; what it wrote is incomplete.
  aborted: boolean;
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
  const out = await captureOutput(bin, args, opts);
  if (out.code === 0) return out.stdout;
  const detail = stripGoRunExitEcho(out.stderr);
  throw new Error(`exited with code ${out.code}${detail ? `: ${detail}` : ""}`);
}

// captureOutput is captureStdout for a command whose exit code is part of
// its answer: it returns the code with both streams instead of throwing.
export async function captureOutput(
  bin: string,
  args: string[],
  opts: CaptureOutputOptions = {},
): Promise<CapturedOutput> {
  const mc = new ManagedChild(bin, args, { cwd: opts.cwd, kind: "read" });
  const stdout: string[] = [];
  let stderr = "";
  let timedOut = false;
  let aborted = false;

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
  const grace = opts.grace ?? "prompt";
  const budget =
    opts.timeoutSeconds === undefined
      ? undefined
      : setTimeout(() => {
          timedOut = true;
          void mc.terminate(grace);
        }, opts.timeoutSeconds * 1000);
  const onAbort = () => {
    aborted = true;
    void mc.terminate(grace);
  };
  if (opts.abort?.aborted) onAbort();
  else opts.abort?.addEventListener("abort", onAbort, { once: true });

  try {
    const { code } = await mc.exited;
    if (spawnError) throw spawnError;
    if (timedOut) throw new CaptureTimeoutError(opts.timeoutSeconds ?? 0);
    return { code, stdout: stdout.join(""), stderr, aborted };
  } finally {
    clearTimeout(budget);
    opts.abort?.removeEventListener("abort", onAbort);
    mc.dispose();
  }
}
