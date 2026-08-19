import { spawn } from "node:child_process";
import { stripGoRunExitEcho } from "./cli.js";

export interface CaptureOptions {
  cwd?: string;
  // Wall-clock budget. Omitted means none — for a command whose duration is the
  // repository's business, not ours.
  timeoutSeconds?: number;
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
    const child = spawn(bin, args, { cwd: opts.cwd });
    const stdout: string[] = [];
    let stderr = "";
    let timedOut = false;
    let settled = false;

    // Decode on the stream, not per chunk: a read ends wherever the pipe filled,
    // and a chunk-wise toString() would corrupt whatever multi-byte character it
    // lands inside. These payloads carry the developer's own text.
    child.stdout.setEncoding("utf-8");
    child.stderr.setEncoding("utf-8");
    child.stdout.on("data", (chunk: string) => stdout.push(chunk));
    child.stderr.on("data", (chunk: string) => {
      stderr += chunk;
    });

    const timer =
      opts.timeoutSeconds === undefined
        ? undefined
        : setTimeout(() => {
            timedOut = true;
            child.kill();
          }, opts.timeoutSeconds * 1000);

    const settle = (outcome: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      outcome();
    };

    // A missing binary arrives here rather than as an exit code, and callers
    // read `code` off the error to drop a cached path — so pass it through.
    child.on("error", (err: Error) => settle(() => reject(err)));

    child.on("close", (code) => {
      settle(() => {
        if (timedOut) {
          reject(new Error(`timed out after ${opts.timeoutSeconds}s`));
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
