import * as path from "node:path";

export interface TestEvent {
  Time: string;
  Action: "run" | "pass" | "fail" | "skip" | "output" | "pause" | "cont";
  Package: string;
  Test?: string;
  Output?: string;
  Elapsed?: number;
}

export interface TestMessage {
  file: string;
  line: number;
  message: string;
}

/**
 * Parse newline-delimited JSON into TestEvent array.
 * Skips non-JSON lines silently.
 */
export function parseTestEvents(jsonLines: string): TestEvent[] {
  const events: TestEvent[] = [];
  const lines = jsonLines.split("\n");

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }
    try {
      const event = JSON.parse(trimmed) as TestEvent;
      if (event.Action) {
        events.push(event);
      }
    } catch {
      // Skip non-JSON lines
    }
  }

  return events;
}

export function groupEventsByPackage(
  events: TestEvent[],
): Map<string, TestEvent[]> {
  const groups = new Map<string, TestEvent[]>();
  for (const event of events) {
    let list = groups.get(event.Package);
    if (!list) {
      list = [];
      groups.set(event.Package, list);
    }
    list.push(event);
  }
  return groups;
}

/**
 * Extract file:line:message patterns from test output.
 * Pattern: /^\s*(.+?):(\d+):\s*(.+)$/
 * Prepends pkgDir to relative file paths.
 */
export function extractTestMessages(
  output: string,
  pkgDir: string,
): TestMessage[] {
  const messages: TestMessage[] = [];
  const pattern = /^\s*(.+?):(\d+):\s*(.+)$/;
  const lines = output.split("\n");

  for (let i = 0; i < lines.length; i++) {
    const match = pattern.exec(lines[i]);
    if (!match) continue;

    let file = match[1];
    const lineNum = parseInt(match[2], 10);
    let message = match[3];

    if (!path.isAbsolute(file)) {
      file = path.join(pkgDir, file);
    }

    while (i + 1 < lines.length) {
      const next = lines[i + 1];
      const trimmed = next.trim();
      if (!trimmed || pattern.test(next) || /^(===\s|---\s)/.test(trimmed))
        break;
      if (!/^\s/.test(next)) break;
      message += "\n" + next;
      i++;
    }

    messages.push({ file, line: lineNum, message });
  }

  return messages;
}

export function extractDiagnosticLocation(
  output: string,
  pkgDir: string,
): { file: string; line: number } | undefined {
  const pattern = /\.go:(\d+)/;
  for (const line of output.split("\n")) {
    const match = pattern.exec(line);
    if (!match) continue;
    const raw = line.substring(0, match.index + 3).trim();
    if (isStdlibPath(raw) || raw.endsWith("testing.go")) continue;
    let file = raw;
    if (!path.isAbsolute(file)) {
      file = path.join(pkgDir, file);
    }
    return { file, line: parseInt(match[1], 10) };
  }
  return undefined;
}

function isStdlibPath(file: string): boolean {
  const idx = file.lastIndexOf("/src/");
  if (idx < 0) return false;
  const after = file.substring(idx + 5);
  const seg = after.split("/")[0];
  return seg !== "" && !seg.includes(".");
}

export function parseExpectedActual(
  message: string,
): { expected: string; actual: string } | undefined {
  const expectedMatch = /^\s*expected:\s*(.+)$/m.exec(message);
  const actualMatch = /^\s*actual:\s*(.+)$/m.exec(message);
  if (!expectedMatch || !actualMatch) return undefined;
  return { expected: expectedMatch[1], actual: actualMatch[1] };
}
