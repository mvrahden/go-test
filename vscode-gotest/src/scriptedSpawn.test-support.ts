// A stand-in for node:child_process' spawn, built on real streams.
//
// The code under test reads its payload in whatever pieces the pipe delivers,
// so a fake that resolves a finished string tests the half that never breaks.
// Here a scripted run writes real chunks — strings or Buffers, split wherever
// the test wants them split — into a PassThrough the caller consumes for real.

import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";

export type ScriptedRun = {
  stdout?: Array<string | Buffer>;
  stderr?: string;
  code?: number;
  error?: NodeJS.ErrnoException;
  // A child that produces its output and then never exits, for testing budgets.
  neverExits?: boolean;
  // A child that ignores SIGTERM. Only SIGKILL ends it — the case a timeout
  // with no escalation can never recover from.
  ignoresSigterm?: boolean;
  // The direct child exits but a grandchild keeps the pipes open, so `exit`
  // fires and `close` never does.
  holdsPipesAfterExit?: boolean;
};

export interface SpawnScript {
  // Consumed one per run, ahead of `always` — so a test can queue a recovery in
  // front of a standing failure.
  once: ScriptedRun[];
  always?: ScriptedRun;
  // What was spawned, in order, for tests that care about the command itself
  // or the options it was given.
  calls?: Array<{
    bin: string;
    args: string[];
    opts?: Record<string, unknown>;
  }>;
}

export interface FakeChild extends EventEmitter {
  stdout: PassThrough;
  stderr: PassThrough;
  // Only the spec render writes to stdin, but every child has one.
  stdin: PassThrough;
  kill: (signal?: NodeJS.Signals) => void;
}

export function createScriptedSpawn(
  script: SpawnScript,
  onKill: (signal?: NodeJS.Signals) => void = () => {},
): (bin: string, args: string[], opts?: Record<string, unknown>) => FakeChild {
  return (bin: string, args: string[], opts?: Record<string, unknown>) => {
    script.calls?.push({ bin, args, opts });
    const run = script.once.shift() ?? script.always ?? { stdout: [] };
    const child = new EventEmitter() as FakeChild;
    child.stdout = new PassThrough();
    child.stderr = new PassThrough();
    child.stdin = new PassThrough();
    child.stdin.resume();

    const finish = () => {
      child.stdout.end();
      child.stderr.end();
    };
    child.kill = (signal?: NodeJS.Signals) => {
      onKill(signal);
      if (run.ignoresSigterm && signal !== "SIGKILL") return;
      child.emit("exit", run.code ?? 0, signal ?? null);
      if (!run.holdsPipesAfterExit) finish();
    };

    let ended = 0;
    const closeWhenDrained = () => {
      if (++ended === 2) {
        child.emit("exit", run.error ? null : (run.code ?? 0), null);
        child.emit("close", run.error ? null : (run.code ?? 0));
      }
    };
    child.stdout.on("end", closeWhenDrained);
    child.stderr.on("end", closeWhenDrained);

    // A microtask later: the caller attaches its listeners on return.
    void Promise.resolve().then(() => {
      if (run.error) {
        child.emit("error", run.error);
        finish();
        return;
      }
      for (const chunk of run.stdout ?? []) child.stdout.write(chunk);
      if (run.stderr) child.stderr.write(run.stderr);
      if (!run.neverExits) finish();
    });

    return child;
  };
}
