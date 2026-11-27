import { describe, it, expect } from "vitest";
import { guardedLog } from "./guardedLog.js";

// A channel that throws once closed, as the extension host's does.
function hostChannel() {
  const lines: string[] = [];
  let closed = false;
  const write = (line: string) => {
    if (closed) throw new Error("Channel has been closed");
    lines.push(line);
  };
  return {
    lines,
    close: () => {
      closed = true;
    },
    channel: {
      name: "gotest",
      info: write,
      warn: write,
      error: write,
      appendLine: write,
      dispose: () => {
        if (closed) throw new Error("Channel has been closed");
        closed = true;
      },
    } as never,
  };
}

describe("guardedLog", () => {
  it("writes through while the channel is open", () => {
    const host = hostChannel();
    const log = guardedLog(host.channel);
    log.info("[watch] stopping all (0 active)");
    log.warn("w");
    expect(host.lines).toEqual(["[watch] stopping all (0 active)", "w"]);
    expect(log.name).toBe("gotest");
  });

  it("drops a write once it is disposed", () => {
    const host = hostChannel();
    const log = guardedLog(host.channel);
    log.dispose();
    expect(() => log.info("[watch] stopping all (0 active)")).not.toThrow();
    expect(() => log.dispose()).not.toThrow();
    expect(host.lines).toEqual([]);
  });

  it("drops a write the host refuses because it closed the channel first", () => {
    const host = hostChannel();
    const log = guardedLog(host.channel);
    host.close();
    expect(() => log.error("late")).not.toThrow();
    expect(() => log.dispose()).not.toThrow();
  });
});
