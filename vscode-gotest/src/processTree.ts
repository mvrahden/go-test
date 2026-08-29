import type { ChildProcess } from "node:child_process";

// killProcessTree signals the whole process group, not just the child we
// spawned. Everything here runs `go run <module> ...`, so the direct child is
// `go` and the process actually holding the pipes is the binary it compiled.
// Signalling only the child leaves that grandchild alive.
//
// Lives in its own module so a low-level caller like capture.ts can use it
// without importing runnerUtils, which would close an import cycle.
export function killProcessTree(
  child: ChildProcess,
  signal: NodeJS.Signals = "SIGTERM",
): void {
  if (child.pid && process.platform !== "win32") {
    try {
      process.kill(-child.pid, signal);
      return;
    } catch {
      // process group already exited
    }
  }
  child.kill(signal);
}
