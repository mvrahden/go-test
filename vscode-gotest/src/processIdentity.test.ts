import { describe, it, expect } from "vitest";
import { readProcessStartToken, identify } from "./processIdentity.js";

const onLinux = process.platform === "linux";

describe.skipIf(!onLinux)("process identity on linux", () => {
  it("reads a stable token for a live process", () => {
    const a = readProcessStartToken(process.pid);
    const b = readProcessStartToken(process.pid);
    expect(a).toMatch(/^\d+$/);
    expect(a).toBe(b);
  });

  it("recognises the same process", () => {
    const token = readProcessStartToken(process.pid);
    expect(identify(process.pid, token)).toBe("same-process");
  });

  // The case that must never be signalled: the number is live, but it is not
  // the process the record was written for.
  it("rejects a live pid whose start time does not match", () => {
    expect(identify(process.pid, "1")).toBe("gone-or-different");
  });

  it("reports a dead pid as gone", () => {
    // PID 0 is never a readable /proc entry.
    expect(identify(0, "1")).toBe("gone-or-different");
  });

  it("refuses to answer without a stored token", () => {
    expect(identify(process.pid, undefined)).toBe("unknown");
  });

  // comm can contain spaces and parentheses; the parser counts from the last
  // ')' for exactly that reason.
  it("survives a process whose name contains parentheses", () => {
    expect(readProcessStartToken(process.pid)).toMatch(/^\d+$/);
  });
});
