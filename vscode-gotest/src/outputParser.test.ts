import { describe, it, expect } from "vitest";
import {
  parseTestEvents,
  extractTestMessages,
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
