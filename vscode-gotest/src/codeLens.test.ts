import * as path from "node:path";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
  afterEach,
  afterAll,
} from "vitest";

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
  let dir = "";

  afterEach(() => {
    if (dir) rmSync(dir, { recursive: true, force: true });
  });

  // Every dir a test made is gone once they ran.
  afterAll(() => {
    expect(dir).not.toBe("");
    expect(existsSync(dir)).toBe(false);
  });

  it("offers Run beside Fuzz and Debug Seeds, running the target's seeds like a test", async () => {
    dir = mkdtempSync(path.join(tmpdir(), "gotest-lens-"));
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

describe("Promote CodeLens", () => {
  const wrapper = "FuzzingTestSuite_FuzzTrimIdempotent";
  let dir: string;
  let cacheDir: string;
  let savedCacheDir: string | undefined;

  beforeEach(() => {
    dir = mkdtempSync(path.join(tmpdir(), "gotest-lens-"));
    cacheDir = mkdtempSync(path.join(tmpdir(), "gotest-cache-"));
    savedCacheDir = process.env.GOTEST_CACHE_DIR;
    process.env.GOTEST_CACHE_DIR = cacheDir;
    writeFileSync(path.join(dir, "suite_test.go"), "package fuzzing\n");
    mkdirSync(path.join(dir, "testdata", "fuzz", `Fuzz${wrapper}`), {
      recursive: true,
    });
  });

  afterEach(() => {
    if (savedCacheDir === undefined) delete process.env.GOTEST_CACHE_DIR;
    else process.env.GOTEST_CACHE_DIR = savedCacheDir;
    rmSync(dir, { recursive: true, force: true });
    rmSync(cacheDir, { recursive: true, force: true });
  });

  // Every dir a test made is gone once they ran.
  afterAll(() => {
    expect(existsSync(dir)).toBe(false);
    expect(existsSync(cacheDir)).toBe(false);
  });

  function crasher(name: string, input: string): string {
    const content = `go test fuzz v1\nstring(${JSON.stringify(input)})\n`;
    writeFileSync(
      path.join(dir, "testdata", "fuzz", `Fuzz${wrapper}`, name),
      content,
    );
    return createHash("sha256").update(content).digest("hex");
  }

  function session(entries: { name: string; sha256: string }[]) {
    writeFileSync(
      path.join(cacheDir, "fuzz-session.json"),
      JSON.stringify({
        crashers: entries.map((e) => ({
          dir,
          func: `Fuzz${wrapper}`,
          ...e,
        })),
      }),
    );
  }

  async function promoteLenses() {
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
              methods: [],
              benchmarks: [],
              fuzzers: [{ ...flags, name: "FuzzTrimIdempotent", line: 19 }],
            },
          ],
        } as never,
      ],
      true,
      dir,
    );
    const lenses = await new GoTestCodeLensProvider(cache).provideCodeLenses(
      {
        fileName: path.join(dir, "suite_test.go"),
        getText: () => "package fuzzing\n",
        lineCount: 30,
        offsetAt: () => 0,
      } as never,
      {} as never,
    );
    return lenses
      .filter((l) => l.command!.command === "gotest.promoteCrashers")
      .map((l) => l.command!.title);
  }

  it("is not offered for a crasher the latest fuzz session did not write", async () => {
    crasher("planted", "older");
    session([]);

    expect(await promoteLenses()).toEqual([]);
  });

  it("is not offered when there is no session record", async () => {
    crasher("planted", "older");

    expect(await promoteLenses()).toEqual([]);
  });

  it("counts the session's crashers and not the files beside them", async () => {
    const sha256 = crasher("found", "new");
    crasher("planted", "older");
    session([{ name: "found", sha256 }]);

    expect(await promoteLenses()).toEqual(["⚠ Promote 1 crasher"]);
  });

  it("is not offered once a session crasher changed, since promote refuses the run", async () => {
    const sha256 = crasher("found", "new");
    const other = crasher("second", "also new");
    crasher("found", "edited");
    session([
      { name: "found", sha256 },
      { name: "second", sha256: other },
    ]);

    expect(await promoteLenses()).toEqual([]);
  });

  it("does not count a session crasher that is gone from disk", async () => {
    const sha256 = crasher("found", "new");
    session([
      { name: "found", sha256 },
      { name: "promoted-already", sha256 },
    ]);

    expect(await promoteLenses()).toEqual(["⚠ Promote 1 crasher"]);
  });
});
