import { describe, it, expect } from "vitest";
import {
  parseLintJson,
  parsePosn,
  positionAt,
  utf16Column,
  utf16Offset,
} from "./lintFindings.js";

// The shape `gotest lint -json` prints, as recorded in cli-contract.json.
const doc = {
  "example.com/p [example.com/p.test]": {
    gotestlint: [
      {
        category: "fail-guard",
        posn: "/src/p/p_test.go:23:2",
        end: "/src/p/p_test.go:23:2",
        message: "use Equal instead of if+Fatal for != comparison",
        suggested_fixes: [
          {
            message: "use Equal",
            edits: [
              {
                filename: "/src/p/p_test.go",
                start: 590,
                end: 640,
                new: "gotest.Equal(t, 1 + 1, 2)",
              },
            ],
          },
        ],
      },
      {
        category: "testify",
        posn: "/src/p/helper.go:4:1",
        end: "/src/p/helper.go:4:9",
        message: "testify import",
      },
    ],
  },
  // The package compiled without its tests reports its own files again.
  "example.com/p": {
    gotestlint: [
      {
        category: "testify",
        posn: "/src/p/helper.go:4:1",
        end: "/src/p/helper.go:4:9",
        message: "testify import",
      },
    ],
  },
};

describe("parseLintJson", () => {
  it("reads every finding once, with its rule, place and fixes", () => {
    const findings = parseLintJson(JSON.stringify(doc, null, "\t"));

    expect(findings).toHaveLength(2);
    expect(findings[0]).toMatchObject({
      category: "fail-guard",
      file: "/src/p/p_test.go",
      line: 23,
      col: 2,
      endLine: 23,
      endCol: 2,
    });
    expect(findings[0].fixes).toEqual([
      {
        message: "use Equal",
        edits: [
          {
            filename: "/src/p/p_test.go",
            start: 590,
            end: 640,
            new: "gotest.Equal(t, 1 + 1, 2)",
          },
        ],
      },
    ]);
    expect(findings[1]).toMatchObject({ category: "testify", endCol: 9 });
    expect(findings[1].fixes).toEqual([]);
  });

  it("reads a clean run as no findings", () => {
    expect(parseLintJson("{}\n")).toEqual([]);
    expect(parseLintJson("")).toEqual([]);
  });
});

describe("parsePosn", () => {
  it("reads the line and column from the end, whatever the path holds", () => {
    expect(parsePosn("C:\\src\\p_test.go:3:14")).toEqual({
      file: "C:\\src\\p_test.go",
      line: 3,
      col: 14,
    });
    expect(parsePosn("no position")).toBeUndefined();
  });
});

describe("byte positions in editor units", () => {
  // "é" is two bytes in UTF-8 and one UTF-16 unit; "𝔾" four bytes and two
  // units. Offsets past them differ between the CLI and the editor.
  const text = 'package p\n\n// café 𝔾\nfunc f() { x := "y" }\n';

  it("converts a byte offset after multi-byte characters", () => {
    const bytes = Buffer.from(text, "utf8");
    const at = bytes.indexOf(Buffer.from("x :="));
    const offset = utf16Offset(text, at);

    expect(text.slice(offset, offset + 4)).toBe("x :=");
    expect(offset).toBe(text.indexOf("x :="));
    expect(offset).toBeLessThan(at);
    expect(positionAt(text, offset)).toEqual({ line: 3, character: 11 });
  });

  it("converts a byte column on a line", () => {
    const line = "// café 𝔾 marker";
    const byteCol = Buffer.from("// café 𝔾 ", "utf8").length + 1;
    expect(line.slice(utf16Column(line, byteCol))).toBe("marker");
  });
});
