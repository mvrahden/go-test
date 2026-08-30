import { readFileSync } from "node:fs";

// A pid on its own is not an identity. Between the crash that orphaned a child
// and the activation that wants to reap it, the OS is free to hand that number
// to something else — and signalling the wrong process is far worse than
// leaving an orphan. So a record carries a token that only the same process can
// still produce.
//
// On Linux that token is field 22 of /proc/<pid>/stat, the process start time in
// clock ticks since boot. It never changes for a live process and is never
// reproduced by a later one on the same boot, so comparing it is an exact
// identity check with no clock arithmetic.
//
// Field 2 is the comm name in parentheses and may itself contain spaces or
// parentheses, so the fields are counted from the last ')' rather than by
// splitting the whole line.
export function readProcessStartToken(pid: number): string | undefined {
  if (process.platform !== "linux") return undefined;
  try {
    const stat = readFileSync(`/proc/${pid}/stat`, "utf-8");
    const afterComm = stat.slice(stat.lastIndexOf(")") + 2);
    const fields = afterComm.split(" ");
    // stat field 22 (1-based) is index 19 of what follows the comm field.
    const startTime = fields[19];
    return startTime && /^\d+$/.test(startTime) ? startTime : undefined;
  } catch {
    return undefined;
  }
}

export type IdentityVerdict = "same-process" | "gone-or-different" | "unknown";

// identify answers whether the process now at `pid` is the one that produced
// `token`. "unknown" means the platform cannot say — never a licence to signal.
export function identify(
  pid: number,
  token: string | undefined,
): IdentityVerdict {
  if (process.platform !== "linux") return "unknown";
  // A record written on a platform that could not produce a token cannot be
  // verified on one that can, either.
  if (!token) return "unknown";
  const current = readProcessStartToken(pid);
  if (current === undefined) return "gone-or-different";
  return current === token ? "same-process" : "gone-or-different";
}
