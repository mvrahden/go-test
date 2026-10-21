import * as path from "node:path";

export interface TestEvent {
  Time: string;
  Action:
    | "start"
    | "run"
    | "pass"
    | "fail"
    | "skip"
    | "output"
    | "pause"
    | "cont"
    | "build-output"
    | "build-fail";
  Package: string;
  Test?: string;
  Output?: string;
  Elapsed?: number;
  // A build event (go 1.24+) carries the ImportPath of what was built and
  // no Package; the package that build failed names it in FailedBuild on
  // its own verdict.
  ImportPath?: string;
  FailedBuild?: string;
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

  for (let i = 0; i < lines.length; i++) {
    const match = pattern.exec(lines[i]);
    if (!match) continue;

    let file = match[1];
    const lineNum = parseInt(match[2], 10);
    let message = match[3];

    if (!path.isAbsolute(file)) {
      file = path.join(pkgDir, file);
    }

    // go test indents a message's continuation lines deeper than its
    // first, so a deeper line belongs to it even when it reads like file:line:.
    const header = indentOf(lines[i]);
    while (i + 1 < lines.length) {
      const next = lines[i + 1];
      const trimmed = next.trim();
      if (!trimmed || /^(===\s|---\s)/.test(trimmed)) break;
      if (indentOf(next) <= header && pattern.test(next)) break;
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

export function isPackageSummaryLine(s: string): boolean {
  const trimmed = s.replace(/[\n\r]+$/, "");
  return (
    trimmed === "PASS" ||
    trimmed === "FAIL" ||
    trimmed.startsWith("ok  \t") ||
    trimmed.startsWith("FAIL\t") ||
    trimmed.startsWith("?   \t")
  );
}

export interface ExpectedActual {
  expected: string;
  actual: string;
  // The message is only its first line and the two single-line values.
  compact: boolean;
}

const sideLabel = /^(\s*)(expected|actual):( +)(.*)$/;
// The heading of a long string shown only around its first difference.
const windowHeading = /differ at byte \d+ \(expected \d+ bytes, actual \d+\)$/;

/**
 * Read the expected and actual values from an assertion message: each
 * value's whole rendering, its continuation lines (indented deeper than its
 * label) shifted back by the value column. Undefined when the message shows
 * no complete pair, or only an excerpt of a value.
 */
export function parseExpectedActual(
  message: string,
): ExpectedActual | undefined {
  const lines = message.split("\n");
  const body = Math.min(
    ...lines
      .slice(1)
      .filter((l) => l.trim())
      .map(indentOf),
  );
  let oneLine = false;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // Labels sit at the message's top level, never inside a value or diff.
    if (i > 0 && indentOf(line) !== body) continue;
    // After this heading each value is one line, and a caret line follows.
    if (windowHeading.test(line)) {
      oneLine = true;
      continue;
    }
    const expected = readSide(lines, i, "expected", oneLine);
    if (!expected) continue;
    const actual = readSide(lines, expected.next, "actual", oneLine);
    if (!actual) return undefined;
    const excerpt = (v: string) => /^…"/.test(v) || /"…$/.test(v);
    if (excerpt(expected.value) || excerpt(actual.value)) return undefined;
    // Only a single-line pair is the whole message: a longer one is
    // followed by its diff.
    const compact = i === 1 && actual.next === lines.length;
    return { expected: expected.value, actual: actual.value, compact };
  }
  return undefined;
}

function readSide(
  lines: string[],
  at: number,
  label: string,
  oneLine: boolean,
): { value: string; next: number } | undefined {
  const m = at < lines.length ? sideLabel.exec(lines[at]) : null;
  if (!m || m[2] !== label) return undefined;
  const indent = m[1].length;
  const column = indent + label.length + 1 + m[3].length;
  const value = [m[4]];
  let next = at + 1;
  while (
    !oneLine &&
    next < lines.length &&
    lines[next].trim() &&
    indentOf(lines[next]) > indent
  ) {
    const l = lines[next];
    value.push(l.slice(Math.min(column, indentOf(l))));
    next++;
  }
  return { value: value.join("\n"), next };
}

function indentOf(line: string): number {
  return line.length - line.trimStart().length;
}
