import { describe, it, expect } from "vitest";
import { parseTestEvents, extractTestMessages } from "./outputParser.js";

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
});
