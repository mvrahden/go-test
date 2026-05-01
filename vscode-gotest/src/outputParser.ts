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

  for (const line of lines) {
    const match = pattern.exec(line);
    if (match) {
      let file = match[1];
      const lineNum = parseInt(match[2], 10);
      const message = match[3];

      // Prepend pkgDir to relative file paths
      if (!file.startsWith("/")) {
        file = `${pkgDir}/${file}`;
      }

      messages.push({ file, line: lineNum, message });
    }
  }

  return messages;
}
