import type * as vscode from "vscode";

const WRITES = new Set([
  "trace",
  "debug",
  "info",
  "warn",
  "error",
  "append",
  "appendLine",
  "replace",
  "clear",
  "show",
  "hide",
]);

// guardedLog wraps a log channel so a write once it is closed is dropped, not
// thrown. A window reload disposes the channel before the services that still
// log while they stop, and a throw there aborts the rest of their dispose.
export function guardedLog(
  channel: vscode.LogOutputChannel,
): vscode.LogOutputChannel {
  let closed = false;
  return new Proxy(channel, {
    get(target, key, receiver) {
      const value = Reflect.get(target, key, receiver);
      if (key === "dispose") {
        return () => {
          if (closed) return;
          closed = true;
          try {
            target.dispose();
          } catch {
            // Already closed by the host.
          }
        };
      }
      if (typeof key !== "string" || !WRITES.has(key)) {
        return typeof value === "function" ? value.bind(target) : value;
      }
      return (...args: unknown[]) => {
        if (closed) return;
        try {
          (value as (...a: unknown[]) => void).apply(target, args);
        } catch {
          // The host closed the channel first: nothing is left to write to.
        }
      };
    },
  });
}
