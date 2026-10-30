// Reading `gotest lint -json`: the go/analysis driver's document, one entry
// per package variant, findings positioned in bytes. Nothing here knows the
// editor; positions come out in the UTF-16 units it counts in.

export interface LintEdit {
  filename: string;
  // Byte offsets into the file as the lint read it.
  start: number;
  end: number;
  new: string;
}

export interface LintFix {
  message: string;
  edits: LintEdit[];
}

export interface LintFinding {
  // The rule that reported it.
  category: string;
  message: string;
  file: string;
  // 1-based line; 1-based byte column, as go/token counts.
  line: number;
  col: number;
  endLine: number;
  endCol: number;
  fixes: LintFix[];
}

interface WireDiagnostic {
  category?: string;
  posn?: string;
  end?: string;
  message?: string;
  suggested_fixes?: { message?: string; edits?: LintEdit[] }[];
}

// parsePosn reads go/token's "file:line:col". The file may itself hold
// colons (a Windows drive), so the numbers are read from the end.
export function parsePosn(
  posn: string,
): { file: string; line: number; col: number } | undefined {
  const m = /^(.*):(\d+):(\d+)$/.exec(posn);
  if (!m) return undefined;
  return { file: m[1], line: Number(m[2]), col: Number(m[3]) };
}

// parseLintJson reads the document into findings. A finding in a file both
// the package and its test variant compile is reported under each; it is
// one finding.
export function parseLintJson(stdout: string): LintFinding[] {
  const trimmed = stdout.trim();
  if (!trimmed) return [];
  const doc = JSON.parse(trimmed) as Record<
    string,
    Record<string, WireDiagnostic[] | { error?: string }>
  >;
  const seen = new Set<string>();
  const findings: LintFinding[] = [];
  for (const analyzers of Object.values(doc)) {
    for (const diags of Object.values(analyzers ?? {})) {
      if (!Array.isArray(diags)) continue;
      for (const d of diags) {
        const at = d.posn ? parsePosn(d.posn) : undefined;
        if (!at || !d.category || !d.message) continue;
        const key = `${d.posn}\u0000${d.category}\u0000${d.message}`;
        if (seen.has(key)) continue;
        seen.add(key);
        const end = (d.end ? parsePosn(d.end) : undefined) ?? at;
        findings.push({
          category: d.category,
          message: d.message,
          file: at.file,
          line: at.line,
          col: at.col,
          endLine: end.line,
          endCol: end.col,
          fixes: (d.suggested_fixes ?? []).map((f) => ({
            message: f.message ?? d.message!,
            edits: f.edits ?? [],
          })),
        });
      }
    }
  }
  return findings;
}

// utf16Column converts a 1-based byte column on a line to the 0-based
// UTF-16 column the editor counts in.
export function utf16Column(lineText: string, byteCol: number): number {
  return utf16Offset(lineText, byteCol - 1);
}

// utf16Offset converts a byte offset into text to a UTF-16 offset.
export function utf16Offset(text: string, byteOffset: number): number {
  return Buffer.from(text, "utf8").subarray(0, byteOffset).toString("utf8")
    .length;
}

// positionAt is the 0-based line and UTF-16 character of a UTF-16 offset.
export function positionAt(
  text: string,
  offset: number,
): { line: number; character: number } {
  let line = 0;
  let lineStart = 0;
  for (let i = 0; i < offset && i < text.length; i++) {
    if (text.charCodeAt(i) === 10) {
      line++;
      lineStart = i + 1;
    }
  }
  return { line, character: offset - lineStart };
}

// lineOf returns the 1-based line of text without its line break.
export function lineOf(text: string, line: number): string {
  return text.split("\n")[line - 1]?.replace(/\r$/, "") ?? "";
}
