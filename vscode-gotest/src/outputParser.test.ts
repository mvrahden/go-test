import { describe, it, expect } from "vitest";
import {
  parseTestEvents,
  extractTestMessages,
  extractDiagnosticLocation,
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
      file: "/abs/pkg/foo_test.go",
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

  it("collects diff blocks in continuation", () => {
    const output = [
      "    a_test.go:10: Equal failed:",
      '          expected: map[string]int{"a":1, "b":2}',
      '          actual:   map[string]int{"a":1, "b":3}',
      "          diff:",
      '            - "b": 2',
      '            + "b": 3',
    ].join("\n");

    const messages = extractTestMessages(output, "/pkg");
    expect(messages).toHaveLength(1);
    expect(messages[0].message).toContain("diff:");
    expect(messages[0].message).toContain('+ "b": 3');
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
    expect(loc).toEqual({ file: "/abs/pkg/foo.go", line: 10 });
  });

  it("returns undefined when no .go file references", () => {
    const output = "WARNING: DATA RACE\nsome text without file refs\n";
    expect(extractDiagnosticLocation(output, "/pkg")).toBeUndefined();
  });

  it("returns undefined for empty output", () => {
    expect(extractDiagnosticLocation("", "/pkg")).toBeUndefined();
  });
});

describe("parseExpectedActual", () => {
  it("extracts expected and actual values", () => {
    const message =
      "Equal failed:\n          expected: 720000000000\n          actual:   120000000000";
    const result = parseExpectedActual(message);
    expect(result).toEqual({
      expected: "720000000000",
      actual: "120000000000",
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
    });
  });
});
