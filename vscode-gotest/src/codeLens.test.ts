import * as path from "node:path";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => {
  class Position {
    constructor(
      public line: number,
      public character: number,
    ) {}
  }
  class Range {
    start: Position;
    end: Position;
    constructor(sl: number, sc: number, el: number, ec: number) {
      this.start = new Position(sl, sc);
      this.end = new Position(el, ec);
    }
  }
  class CodeLens {
    constructor(
      public range: Range,
      public command: { title: string; command: string; arguments?: unknown[] },
    ) {}
  }
  class EventEmitter {
    private listeners: (() => void)[] = [];
    event = (l: () => void) => {
      this.listeners.push(l);
      return { dispose: () => {} };
    };
    fire() {
      for (const l of this.listeners) l();
    }
    dispose() {}
  }
  return {
    Position,
    Range,
    CodeLens,
    EventEmitter,
    workspace: { getConfiguration: () => ({ get: () => true }) },
  };
});

import { DiscoveryCache } from "./discovery.js";
import { GoTestCodeLensProvider } from "./codeLens.js";

describe("CodeLens on a fuzz method", () => {
  it("offers Run beside Fuzz and Debug Seeds, running the target's seeds like a test", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "gotest-lens-"));
    const file = path.join(dir, "suite_test.go");
    writeFileSync(file, "package fuzzing\n");
    const cache = new DiscoveryCache();
    const flags = {
      parallel: false,
      focused: false,
      excluded: false,
      file: "suite_test.go",
      col: 1,
    };
    cache.update(
      [
        {
          importPath: "example.com/fuzzing",
          dir,
          suites: [
            {
              name: "FuzzingTestSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "suite_test.go",
              line: 11,
              col: 6,
              lifecycle: [],
              fixtures: [],
              methods: [{ ...flags, name: "TestTrim", line: 13 }],
              benchmarks: [],
              fuzzers: [{ ...flags, name: "FuzzTrimIdempotent", line: 19 }],
            },
          ],
        } as never,
      ],
      true,
      dir,
    );
    const provider = new GoTestCodeLensProvider(cache);
    const document = {
      fileName: file,
      getText: () => "package fuzzing\n",
      lineCount: 30,
      offsetAt: () => 0,
    };

    const lenses = await provider.provideCodeLenses(
      document as never,
      {} as never,
    );

    const atFuzzer = lenses
      .filter((l) => l.range.start.line === 18)
      .map((l) => [l.command!.title, l.command!.command, l.command!.arguments]);
    expect(atFuzzer).toContainEqual([
      "▶ Run",
      "gotest.runTest",
      ["example.com/fuzzing/FuzzingTestSuite/FuzzTrimIdempotent"],
    ]);
    expect(atFuzzer.map((l) => l[0])).toEqual([
      "▶ Run",
      "▶ Fuzz",
      "Debug Seeds",
    ]);
  });
});
