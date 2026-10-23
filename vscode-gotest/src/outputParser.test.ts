import * as path from "node:path";
import { describe, it, expect } from "vitest";
import {
  parseTestEvents,
  extractTestMessages,
  extractDiagnosticLocation,
  isPackageSummaryLine,
  parseExpectedActual,
} from "./outputParser.js";

describe("parseTestEvents", () => {
  it("parses valid JSON lines into events", () => {
    const input = [
      '{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestFoo"}',
      '{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"pkg","Test":"TestFoo","Elapsed":0.5}',
    ].join("\n");

    const events = parseTestEvents(input);
    expect(events).toHaveLength(2);
    expect(events[0].Action).toBe("run");
    expect(events[0].Test).toBe("TestFoo");
    expect(events[1].Action).toBe("pass");
    expect(events[1].Elapsed).toBe(0.5);
  });

  it("skips blank lines", () => {
    const input = '\n{"Action":"run","Package":"p","Test":"T"}\n\n';
    expect(parseTestEvents(input)).toHaveLength(1);
  });

  it("skips non-JSON lines", () => {
    const input = [
      "FAIL some/pkg",
      '{"Action":"fail","Package":"some/pkg","Test":"TestBar"}',
      "ok   some/pkg  0.5s",
    ].join("\n");

    const events = parseTestEvents(input);
    expect(events).toHaveLength(1);
    expect(events[0].Action).toBe("fail");
  });

  it("skips JSON objects without Action field", () => {
    const input = '{"Time":"2024-01-01T00:00:00Z","Package":"p"}';
    expect(parseTestEvents(input)).toHaveLength(0);
  });

  it("returns empty array for empty input", () => {
    expect(parseTestEvents("")).toHaveLength(0);
  });
});

describe("extractTestMessages", () => {
  it("extracts file:line:message patterns", () => {
    const output = "    foo_test.go:42: expected 1, got 2\n";
    const messages = extractTestMessages(output, "/abs/pkg");

    expect(messages).toHaveLength(1);
    expect(messages[0]).toEqual({
      file: path.join("/abs/pkg", "foo_test.go"),
      line: 42,
      message: "expected 1, got 2",
    });
  });

  it("preserves absolute file paths", () => {
    const output = "    /full/path/test.go:10: boom\n";
    const messages = extractTestMessages(output, "/abs/pkg");

    expect(messages).toHaveLength(1);
    expect(messages[0].file).toBe("/full/path/test.go");
  });

  it("extracts multiple messages", () => {
    const output = [
      "    a_test.go:1: first",
      "    b_test.go:2: second",
      "some unrelated output",
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(2);
    expect(messages[0].message).toBe("first");
    expect(messages[1].message).toBe("second");
  });

  it("returns empty array when no patterns match", () => {
    expect(extractTestMessages("no errors here\n", "/pkg")).toHaveLength(0);
  });

  it("returns empty array for empty input", () => {
    expect(extractTestMessages("", "/pkg")).toHaveLength(0);
  });

  it("collects continuation lines into the message", () => {
    const output = [
      "    config_suite_test.go:14: Equal failed:",
      "          expected: 720000000000",
      "          actual:   120000000000",
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toBe(
      "Equal failed:\n          expected: 720000000000\n          actual:   120000000000",
    );
  });

  it("stops continuation at the next file:line: match", () => {
    const output = [
      "    a_test.go:10: Equal failed:",
      "          expected: 1",
      "          actual:   2",
      "    a_test.go:15: NotEqual failed:",
      "          expected: 3",
      "          actual:   3",
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(2);
    expect(messages[0].message).toBe(
      "Equal failed:\n          expected: 1\n          actual:   2",
    );
    expect(messages[1].message).toBe(
      "NotEqual failed:\n          expected: 3\n          actual:   3",
    );
  });

  it("stops continuation at === RUN and --- FAIL lines", () => {
    const output = [
      "    a_test.go:10: Equal failed:",
      "          expected: 1",
      "          actual:   2",
      "--- FAIL: TestFoo (0.00s)",
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toBe(
      "Equal failed:\n          expected: 1\n          actual:   2",
    );
  });

  it("keeps a deeper line that reads like file:line: in the message", () => {
    const output = [
      "    probe_test.go:58: Equal failed:",
      "          diff (-expected +actual):",
      "            at .Address:",
      "             eqprobe.wide{",
      '               Customer: "alice smith",',
      '            -  Address: "main.go:12: boom while parsing",',
      '            +  Address: "main.go:12: bang while parsing",',
      "             }",
      "    probe_test.go:59: second",
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(2);
    expect(messages[0].line).toBe(58);
    expect(messages[0].message).toContain(
      '+  Address: "main.go:12: bang while parsing",',
    );
    expect(messages[1].message).toBe("second");
  });

  it("collects diff blocks in continuation", () => {
    const output = [
      "    a_test.go:10: Equal failed:",
      "          diff (-expected +actual):",
      '               "a": 1,',
      '            -  "b": 2,',
      '            +  "b": 3,',
      "            …",
      '               "y": 25,',
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toContain("diff (-expected +actual):");
    expect(messages[0].message).toContain('+  "b": 3,');
    expect(messages[0].message).toContain('"y": 25,');
  });
});

describe("extractDiagnosticLocation", () => {
  it("extracts location from race detector stack trace", () => {
    const output = [
      "WARNING: DATA RACE",
      "Read at 0x00c00001c0f0 by goroutine 7:",
      "  example.com/pkg.(*Foo).Bar()",
      "      /home/user/project/foo.go:42 +0x1a4",
    ].join("\n");

    const loc = extractDiagnosticLocation(output, "/home/user/project");
    expect(loc).toEqual({ file: "/home/user/project/foo.go", line: 42 });
  });

  it("extracts location from panic stack trace", () => {
    const output = [
      "panic: runtime error: index out of range",
      "",
      "goroutine 1 [running]:",
      "example.com/pkg.SomeFunc(...)",
      "\t/home/user/project/file.go:123 +0x1a4",
    ].join("\n");

    const loc = extractDiagnosticLocation(output, "/home/user/project");
    expect(loc).toEqual({ file: "/home/user/project/file.go", line: 123 });
  });

  it("skips stdlib frames", () => {
    const output = [
      "goroutine 1 [running]:",
      "runtime/pprof.writeGoroutineStacks()",
      "\t/usr/local/go/src/runtime/pprof/pprof.go:799 +0x1a4",
      "example.com/pkg.MyFunc()",
      "\t/home/user/project/my.go:10 +0x2b",
    ].join("\n");

    const loc = extractDiagnosticLocation(output, "/home/user/project");
    expect(loc).toEqual({ file: "/home/user/project/my.go", line: 10 });
  });

  it("skips testing.go frames", () => {
    const output = [
      "\ttesting.go:1234 +0x1a4",
      "\t/home/user/project/svc.go:55 +0x2b",
    ].join("\n");

    const loc = extractDiagnosticLocation(output, "/home/user/project");
    expect(loc).toEqual({ file: "/home/user/project/svc.go", line: 55 });
  });

  it("prepends pkgDir to relative paths", () => {
    const output = "      foo.go:10 +0x1a4\n";
    const loc = extractDiagnosticLocation(output, "/abs/pkg");
    expect(loc).toEqual({ file: path.join("/abs/pkg", "foo.go"), line: 10 });
  });

  it("returns undefined when no .go file references", () => {
    const output = "WARNING: DATA RACE\nsome text without file refs\n";
    expect(extractDiagnosticLocation(output, "/pkg")).toBeUndefined();
  });

  it("returns undefined for empty output", () => {
    expect(extractDiagnosticLocation("", "/pkg")).toBeUndefined();
  });
});

describe("isPackageSummaryLine", () => {
  it.each([
    ["PASS\n", true],
    ["FAIL\n", true],
    ["ok  \tpkg\t0.5s\n", true],
    ["FAIL\tpkg\t1.2s\n", true],
    ["?   \tpkg\t[no test files]\n", true],
    ["WARNING: DATA RACE\n", false],
    ["==================\n", false],
    ["Found 1 data race(s)\n", false],
    ["panic: boom\n", false],
    ["goroutine 1 [running]:\n", false],
  ])("%s → %s", (input, expected) => {
    expect(isPackageSummaryLine(input)).toBe(expected);
  });
});

// Output of gotest.Equal failures as go test prints them.
const structFailure = [
  "    probe_test.go:50: Equal failed:",
  "          expected: eqprobe.wide{",
  '                      Customer: "alice smith",',
  '                      Address: "1 main street springfield",',
  "                      Items: []string{",
  '                        "apple",',
  '                        "pear",',
  "                      },",
  "                    }",
  "          actual:   eqprobe.wide{",
  '                      Customer: "alice smith",',
  '                      Address: "2 main street springfield",',
  "                      Items: []string{",
  '                        "apple",',
  '                        "plum",',
  "                      },",
  "                    }",
  "          diff (-expected +actual):",
  "            at .Address:",
  "             eqprobe.wide{",
  '               Customer: "alice smith",',
  '            -  Address: "1 main street springfield",',
  '            +  Address: "2 main street springfield",',
  "               Items: []string{",
  '                 "apple",',
  '            -    "pear",',
  '            +    "plum",',
  "               },",
  "             }",
].join("\n");

const windowFailure = [
  "    probe_test.go:26: Equal failed:",
  "          strings differ at byte 50 (expected 101 bytes, actual 101)",
  '          expected: …"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaXbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"…',
  '          actual:   …"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaYbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"…',
  "                                                    ^",
].join("\n");

const multilineStringFailure = [
  "    probe_test.go:40: Equal failed:",
  '          expected: "a\\n" +',
  '                      "b\\n" +',
  '                      "c"',
  '          actual:   "a\\n" +',
  '                      "B\\n" +',
  '                      "c"',
  "          diff (-expected +actual):",
  '             "a\\n" +',
  '            -  "b\\n" +',
  '            +  "B\\n" +',
  '               "c"',
].join("\n");

function messageOf(output: string): string {
  const messages = extractTestMessages(output, "/pkg");
  expect(messages).toHaveLength(1);
  return messages[0].message;
}

describe("parseExpectedActual", () => {
  it("extracts expected and actual values", () => {
    const message =
      "Equal failed:\n          expected: 720000000000\n          actual:   120000000000";
    const result = parseExpectedActual(message);
    expect(result).toEqual({
      expected: "720000000000",
      actual: "120000000000",
      compact: true,
    });
  });

  it("returns undefined when no expected/actual present", () => {
    expect(parseExpectedActual("some error message")).toBeUndefined();
  });

  it("returns undefined when only expected is present", () => {
    expect(parseExpectedActual("Equal failed:\n  expected: 1")).toBeUndefined();
  });

  it("handles complex values", () => {
    const message =
      'Equal failed:\n  expected: map[string]int{"a":1, "b":2}\n  actual:   map[string]int{"a":1, "b":3}';
    const result = parseExpectedActual(message);
    expect(result).toEqual({
      expected: 'map[string]int{"a":1, "b":2}',
      actual: 'map[string]int{"a":1, "b":3}',
      compact: true,
    });
  });

  it("captures each side's whole rendering, its indent relative to the value", () => {
    const result = parseExpectedActual(messageOf(structFailure));
    expect(result).toEqual({
      expected: [
        "eqprobe.wide{",
        '  Customer: "alice smith",',
        '  Address: "1 main street springfield",',
        "  Items: []string{",
        '    "apple",',
        '    "pear",',
        "  },",
        "}",
      ].join("\n"),
      actual: [
        "eqprobe.wide{",
        '  Customer: "alice smith",',
        '  Address: "2 main street springfield",',
        "  Items: []string{",
        '    "apple",',
        '    "plum",',
        "  },",
        "}",
      ].join("\n"),
      compact: false,
    });
  });

  it("keeps a continuation line's own indent beyond the value column", () => {
    const result = parseExpectedActual(messageOf(multilineStringFailure));
    expect(result?.expected).toBe('"a\\n" +\n  "b\\n" +\n  "c"');
    expect(result?.actual).toBe('"a\\n" +\n  "B\\n" +\n  "c"');
    expect(result?.compact).toBe(false);
  });

  it("returns undefined for a windowed excerpt of a long string", () => {
    expect(parseExpectedActual(messageOf(windowFailure))).toBeUndefined();
  });

  it("reads a whole string shown with a caret, the caret left out", () => {
    const expected = '"' + "\\t".repeat(20) + "a" + "\\t".repeat(20) + '"';
    const actual = '"' + "\\t".repeat(20) + "b" + "\\t".repeat(20) + '"';
    const message = messageOf(
      [
        "    probe_test.go:75: Equal failed:",
        "          strings differ at byte 20 (expected 41 bytes, actual 41)",
        "          expected: " + expected,
        "          actual:   " + actual,
        " ".repeat(61) + "^",
      ].join("\n"),
    );
    expect(parseExpectedActual(message)).toEqual({
      expected,
      actual,
      compact: false,
    });
  });

  it("is not compact when the types differ", () => {
    const message = messageOf(
      [
        "    probe_test.go:28: Equal failed:",
        "          types differ: int vs float64",
        "          expected: int(1)",
        "          actual:   float64(1)",
      ].join("\n"),
    );
    expect(parseExpectedActual(message)).toEqual({
      expected: "int(1)",
      actual: "float64(1)",
      compact: false,
    });
  });

  it("is not compact when a user message follows", () => {
    const message = messageOf(
      [
        "    probe_test.go:41: Equal failed:",
        "          expected: 1",
        "          actual:   2",
        "          message: user msg",
      ].join("\n"),
    );
    expect(parseExpectedActual(message)).toEqual({
      expected: "1",
      actual: "2",
      compact: false,
    });
  });

  it("returns undefined when only the shared value is shown", () => {
    const message = messageOf(
      [
        "    probe_test.go:32: Equal failed:",
        "          values differ in ways their rendering does not show (NaN, a func, or state the rendering omits)",
        "          both:     NaN",
      ].join("\n"),
    );
    expect(parseExpectedActual(message)).toBeUndefined();
  });

  it("ignores fields named expected and actual inside the shared value", () => {
    const message = messageOf(
      [
        "    probe_test.go:86: Equal failed:",
        "          values differ at .expected in ways their rendering does not show (NaN, a func, or state the rendering omits)",
        "          both:     eqprobe.reading{",
        "                      expected: NaN,",
        "                      actual: 0,",
        '                      label: "a fairly long label so the rendering spans lines",',
        "                    }",
      ].join("\n"),
    );
    expect(parseExpectedActual(message)).toBeUndefined();
  });

  it("ignores fields named expected and actual inside a diff", () => {
    const message = messageOf(
      [
        "    probe_test.go:71: Equal failed:",
        "          diff (-expected +actual):",
        "            at [11].note:",
        "               eqprobe.pair{",
        "                 expected: 0,",
        "                 actual: 0,",
        '            -    note: "",',
        '            +    note: "x",',
        "               },",
        "             }",
      ].join("\n"),
    );
    expect(parseExpectedActual(message)).toBeUndefined();
  });
});
