import { describe, it, expect } from "vitest";
import { SpecStore, coverOf } from "./specStore.js";

type Ev = Record<string, unknown>;

const line = (ev: Ev) => JSON.stringify(ev);
const stream = (...evs: Ev[]) => evs.map(line).join("\n") + "\n";

// The events go test writes for one test path ending in a verdict.
function test(pkg: string, name: string, action: string, output?: string) {
  const evs: Ev[] = [{ Action: "run", Package: pkg, Test: name }];
  if (output) {
    evs.push({ Action: "output", Package: pkg, Test: name, Output: output });
  }
  evs.push({ Action: action, Package: pkg, Test: name, Elapsed: 0.01 });
  return evs;
}

const verdict = (pkg: string, action: string): Ev => ({
  Action: action,
  Package: pkg,
  Elapsed: 0.02,
});

// A failure the CLI books for the run itself, as it writes one: every line
// of the package carries RunLevel.
const booked = (pkg: string, output: string): Ev[] => [
  { Action: "start", Package: pkg, RunLevel: true },
  { Action: "output", Package: pkg, Output: output, RunLevel: true },
  { Action: "fail", Package: pkg, RunLevel: true },
];

const runLevelOf = (store: SpecStore, pkg: string) =>
  store.serialize().find((r) => r.path === pkg)?.runLevel;

// The test paths of one package in the composed stream, each time it appears.
function pathsOf(composed: string, pkg: string): string[] {
  return composed
    .split("\n")
    .filter(Boolean)
    .map(
      (l) =>
        JSON.parse(l) as { Action: string; Package: string; Test?: string },
    )
    .filter((e) => e.Package === pkg && e.Test && e.Action === "run")
    .map((e) => e.Test!);
}

function outputOf(composed: string, testPath: string): string[] {
  return composed
    .split("\n")
    .filter(Boolean)
    .map(
      (l) =>
        JSON.parse(l) as { Action: string; Test?: string; Output?: string },
    )
    .filter((e) => e.Test === testPath && e.Action === "output")
    .map((e) => e.Output!);
}

const cart = "example.com/cart";
const checkout = "example.com/checkout";

const fullCart = stream(
  ...test(cart, "TestCartTestSuite/TestAdd", "pass"),
  ...test(
    cart,
    "TestCartTestSuite/TestTotal",
    "fail",
    "cart_test.go:9: wrong\n",
  ),
  ...test(cart, "TestCartTestSuite", "fail"),
  verdict(cart, "fail"),
);

describe("SpecStore", () => {
  it("keeps a package when a later run reports another one", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    store.merge({
      stream: stream(
        ...test(checkout, "TestCheckoutTestSuite", "pass"),
        verdict(checkout, "pass"),
      ),
      folder: "/ws",
    });

    const composed = store.compose("/ws");
    expect(pathsOf(composed, cart)).toContain("TestCartTestSuite/TestAdd");
    expect(pathsOf(composed, checkout)).toEqual(["TestCheckoutTestSuite"]);
  });

  it("holds a package once however many producers reported it", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    store.merge({ stream: fullCart, folder: "/ws" });

    const paths = pathsOf(store.compose("/ws"), cart);
    expect(paths).toHaveLength(3);
    expect(new Set(paths).size).toBe(3);
  });

  it("replaces a package run whole by its newest stream", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    store.merge({
      stream: stream(
        ...test(cart, "TestCartTestSuite/TestAdd", "pass"),
        ...test(cart, "TestCartTestSuite", "pass"),
        verdict(cart, "pass"),
      ),
      folder: "/ws",
    });

    // TestTotal is gone from source: a whole run is the package as it is now.
    expect(pathsOf(store.compose("/ws"), cart)).toEqual([
      "TestCartTestSuite/TestAdd",
      "TestCartTestSuite",
    ]);
  });

  it("keeps the other methods' verdicts when one method is rerun", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    store.merge({
      stream: stream(
        ...test(cart, "TestCartTestSuite/TestTotal", "pass"),
        ...test(cart, "TestCartTestSuite", "pass"),
        verdict(cart, "pass"),
      ),
      folder: "/ws",
      partial: new Map([[cart, coverOf(["TestCartTestSuite/TestTotal"])]]),
    });

    const composed = store.compose("/ws");
    expect(pathsOf(composed, cart)).toEqual([
      "TestCartTestSuite/TestAdd",
      "TestCartTestSuite/TestTotal",
      "TestCartTestSuite",
    ]);
    // The rerun's verdict, not the kept one.
    expect(outputOf(composed, "TestCartTestSuite/TestTotal")).toEqual([]);
    expect(composed).toContain(
      line({
        Action: "pass",
        Package: cart,
        Test: "TestCartTestSuite/TestTotal",
        Elapsed: 0.01,
      }),
    );
  });

  it("drops what a rerun asked for and no longer produced", () => {
    const store = new SpecStore();
    store.merge({
      stream: stream(
        ...test(cart, "TestCartTestSuite/TestRows/#00", "pass"),
        ...test(cart, "TestCartTestSuite/TestRows/#01", "fail"),
        ...test(cart, "TestCartTestSuite/TestRows", "fail"),
        ...test(cart, "TestCartTestSuite/TestAdd", "pass"),
        ...test(cart, "TestCartTestSuite", "fail"),
        verdict(cart, "fail"),
      ),
      folder: "/ws",
    });
    store.merge({
      stream: stream(
        ...test(cart, "TestCartTestSuite/TestRows/#00", "pass"),
        ...test(cart, "TestCartTestSuite/TestRows", "pass"),
        ...test(cart, "TestCartTestSuite", "pass"),
        verdict(cart, "pass"),
      ),
      folder: "/ws",
      partial: new Map([[cart, coverOf(["TestCartTestSuite/TestRows"])]]),
    });

    // The kept TestAdd was written before the rerun's lines.
    expect(pathsOf(store.compose("/ws"), cart)).toEqual([
      "TestCartTestSuite/TestAdd",
      "TestCartTestSuite/TestRows/#00",
      "TestCartTestSuite/TestRows",
      "TestCartTestSuite",
    ]);
  });

  it("composes a folder's packages only, and moves a package to the folder that ran it last", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/a" });
    store.merge({
      stream: stream(
        ...test(checkout, "TestCheckoutTestSuite", "pass"),
        verdict(checkout, "pass"),
      ),
      folder: "/b",
    });
    expect(store.folders().sort()).toEqual(["/a", "/b"]);
    expect(pathsOf(store.compose("/a"), checkout)).toEqual([]);

    store.merge({ stream: fullCart, folder: "/b" });
    expect(store.folders()).toEqual(["/b"]);
    expect(store.compose("/a")).toBe("");
  });

  it("forgets a run-level booking once the folder's next run does not report it", () => {
    const store = new SpecStore();
    store.merge({
      stream: fullCart + stream(...booked("interrupted", "FAIL: stopped\n")),
      folder: "/ws",
    });
    expect(store.packages).toContain("interrupted");

    store.merge({ stream: fullCart, folder: "/ws" });
    expect(store.packages).not.toContain("interrupted");
  });

  it("carries a failed build's diagnostics with the package that names it", () => {
    const store = new SpecStore();
    store.merge({
      stream: stream(
        {
          Action: "build-output",
          ImportPath: `${cart} [${cart}.test]`,
          Output: "cart_test.go:3:1: undefined: helper\n",
        },
        { Action: "build-fail", ImportPath: `${cart} [${cart}.test]` },
        {
          Action: "fail",
          Package: cart,
          Elapsed: 0,
          FailedBuild: `${cart} [${cart}.test]`,
        },
        { Action: "build-output", ImportPath: "unrelated", Output: "x\n" },
      ),
      folder: "/ws",
    });

    const composed = store.compose("/ws");
    expect(composed).toContain("undefined: helper");
    expect(composed).not.toContain('"unrelated"');
  });

  it("ignores gotest watch's cycle markers", () => {
    const store = new SpecStore();
    store.merge({
      stream:
        stream({ Action: "watch-start", Package: "./..." }) +
        fullCart +
        stream({ Action: "watch-end", Package: "./...", Exit: 1 }),
      folder: "/ws",
    });
    expect(store.packages).toEqual([cart]);
  });

  it("restores what it serialized", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    const copy = new SpecStore();
    copy.restore(JSON.parse(JSON.stringify(store.serialize())));
    expect(copy.compose("/ws")).toBe(store.compose("/ws"));
  });

  it("restores nothing from records whose lines carry no order", () => {
    const store = new SpecStore();
    store.restore([
      {
        path: cart,
        folder: "/ws",
        pkgLines: [line(verdict(cart, "pass"))],
        tests: [["TestCartTestSuite", [line(verdict(cart, "pass"))]]],
        runLevel: false,
      },
    ]);
    expect(store.size).toBe(0);
  });
});

describe("coverOf", () => {
  it("covers a path and what lies under it, and a suite's fuzz wrappers", () => {
    const covers = coverOf(["TestCartTestSuite"]);
    expect(covers("TestCartTestSuite")).toBe(true);
    expect(covers("TestCartTestSuite/TestAdd/when_empty")).toBe(true);
    expect(covers("FuzzCartTestSuite_FuzzParse/seed#0")).toBe(true);
    expect(covers("TestCartTestSuiteExtra")).toBe(false);
    expect(coverOf(["TestCartTestSuite/TestAdd"])("TestCartTestSuite")).toBe(
      false,
    );
  });
});

describe("the RunLevel marker", () => {
  it("makes a package the run's, whatever its name, until the folder's next run leaves it out", () => {
    const store = new SpecStore();
    store.merge({
      stream:
        fullCart +
        stream(
          ...booked("shared fixtures", "FAIL: shared fixture setup failed\n"),
          ...booked("example.com/booked", "FAIL: booked\n"),
        ),
      folder: "/ws",
    });
    expect(runLevelOf(store, "shared fixtures")).toBe(true);
    expect(runLevelOf(store, "example.com/booked")).toBe(true);
    expect(runLevelOf(store, cart)).toBe(false);

    store.merge({ stream: fullCart, folder: "/ws" });
    expect(store.packages).toEqual([cart]);
  });

  it("leaves a package whose lines carry none an ordinary one, a space in its name or not", () => {
    const store = new SpecStore();
    store.merge({
      stream: stream(
        { Action: "start", Package: "shared fixtures" },
        verdict("shared fixtures", "fail"),
      ),
      folder: "/ws",
    });
    expect(runLevelOf(store, "shared fixtures")).toBe(false);

    // Kept past a run that does not report it, as a package is.
    store.merge({ stream: fullCart, folder: "/ws" });
    expect(store.packages).toContain("shared fixtures");
  });
});

describe("SpecStore.prune", () => {
  it("drops a folder's packages discovery no longer reports and keeps the rest", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    store.merge({
      stream: stream(
        ...test(checkout, "TestCheckoutTestSuite", "pass"),
        verdict(checkout, "pass"),
        ...booked("interrupted", "FAIL: stopped\n"),
      ),
      folder: "/ws",
    });
    store.merge({
      stream: stream(...test("other.com/x", "TestXTestSuite", "pass")),
      folder: "/other",
    });

    expect(store.prune("/ws", new Set([checkout]))).toEqual([cart]);

    const composed = store.compose("/ws");
    expect(pathsOf(composed, cart)).toEqual([]);
    expect(pathsOf(composed, checkout)).toEqual(["TestCheckoutTestSuite"]);
    expect(store.packages).toContain("interrupted");
    expect(store.packages).toContain("other.com/x");
  });
});

// The lines gotest writes for a test, stamped: a run names its own level,
// and every event of a later instance of a top-level name carries it.
function begin(pkg: string, test: string, subtest: string, instance = 1): Ev {
  const stamp: Ev = { Subtest: subtest };
  if (instance >= 2) stamp.Instance = instance;
  return { Action: "run", Package: pkg, Test: test, Gotest: stamp };
}

function end(
  pkg: string,
  test: string,
  action: string,
  instance = 1,
  elapsed = 0.01,
): Ev {
  const ev: Ev = { Action: action, Package: pkg, Test: test, Elapsed: elapsed };
  if (instance >= 2) ev.Gotest = { Instance: instance };
  return ev;
}

const lines = (s: string) => s.split("\n").filter(Boolean);

describe("compose", () => {
  it("compose replays lines in the order they were written", () => {
    const suite = "TestPathsTestSuite";
    const method = suite + "/TestRead";
    const written = stream(
      begin(cart, suite, suite),
      begin(cart, method, "TestRead"),
      begin(cart, method + "/.", "."),
      end(cart, method + "/.", "pass"),
      begin(cart, method + "/./pkg", "./pkg"),
      end(cart, method + "/./pkg", "pass"),
      end(cart, method, "pass"),
      end(cart, suite, "pass"),
      verdict(cart, "pass"),
    );
    const store = new SpecStore();
    store.merge({ stream: written, folder: "/ws" });

    expect(store.compose("/ws")).toBe(written);
  });

  it("restores the order it composed", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    store.merge({
      stream: stream(
        ...test(cart, "TestCartTestSuite/TestTotal", "pass"),
        ...test(cart, "TestCartTestSuite", "pass"),
        verdict(cart, "pass"),
      ),
      folder: "/ws",
      partial: new Map([[cart, coverOf(["TestCartTestSuite/TestTotal"])]]),
    });
    const copy = new SpecStore();
    copy.restore(JSON.parse(JSON.stringify(store.serialize())));

    // A run merged after a restore is written after what was restored.
    const later = stream(
      ...test(cart, "TestCartTestSuite/TestAdd", "fail"),
      ...test(cart, "TestCartTestSuite", "fail"),
      verdict(cart, "fail"),
    );
    const partial = new Map([[cart, coverOf(["TestCartTestSuite/TestAdd"])]]);
    store.merge({ stream: later, folder: "/ws", partial });
    copy.merge({ stream: later, folder: "/ws", partial });
    expect(pathsOf(copy.compose("/ws"), cart)).toEqual([
      "TestCartTestSuite/TestTotal",
      "TestCartTestSuite/TestAdd",
      "TestCartTestSuite",
    ]);
    expect(copy.compose("/ws")).toBe(store.compose("/ws"));
  });
});

describe("a suite in both test packages", () => {
  const S = "TestTwinsTestSuite";
  // Both instances whole: the internal one with TestCreate and TestRead,
  // the external one with its TestCreate (named #01) and TestList.
  const whole = stream(
    begin(cart, S, S),
    begin(cart, `${S}/TestCreate`, "TestCreate"),
    end(cart, `${S}/TestCreate`, "pass"),
    begin(cart, `${S}/TestRead`, "TestRead"),
    end(cart, `${S}/TestRead`, "pass"),
    end(cart, S, "pass"),
    begin(cart, S, S, 2),
    begin(cart, `${S}/TestCreate#01`, "TestCreate#01", 2),
    end(cart, `${S}/TestCreate#01`, "fail", 2),
    begin(cart, `${S}/TestList`, "TestList", 2),
    end(cart, `${S}/TestList`, "pass", 2),
    end(cart, S, "fail", 2),
    verdict(cart, "fail"),
  );
  // -run ^S$/^TestCreate#01$: the internal instance runs no method.
  const rerun = stream(
    begin(cart, S, S),
    end(cart, S, "pass", 1, 0.02),
    begin(cart, S, S, 2),
    begin(cart, `${S}/TestCreate#01`, "TestCreate#01", 2),
    end(cart, `${S}/TestCreate#01`, "pass", 2, 0.02),
    end(cart, S, "pass", 2, 0.02),
    verdict(cart, "pass"),
  );

  it("rerunning one twin method keeps every other method of both twins", () => {
    const store = new SpecStore();
    store.merge({ stream: whole, folder: "/ws" });
    store.merge({
      stream: rerun,
      folder: "/ws",
      partial: new Map([[cart, coverOf([`${S}/TestCreate#01`])]]),
    });

    const w = lines(whole);
    const r = lines(rerun);
    // The kept methods in the order they ran, then the rerun as written.
    expect(lines(store.compose("/ws"))).toEqual([
      ...w.slice(1, 3),
      ...w.slice(3, 5),
      ...w.slice(9, 11),
      ...r,
    ]);
  });

  it("keeps the other twin's suite lines when a rerun stopped before it", () => {
    const store = new SpecStore();
    store.merge({ stream: whole, folder: "/ws" });
    // -run ^S$/^TestRead$, stopped after the internal instance.
    const stopped = stream(
      begin(cart, S, S),
      begin(cart, `${S}/TestRead`, "TestRead"),
      end(cart, `${S}/TestRead`, "pass", 1, 0.02),
      end(cart, S, "pass", 1, 0.02),
      verdict(cart, "fail"),
    );
    store.merge({
      stream: stopped,
      folder: "/ws",
      partial: new Map([[cart, coverOf([`${S}/TestRead`])]]),
    });

    const w = lines(whole);
    const st = lines(stopped);
    expect(lines(store.compose("/ws"))).toEqual([
      ...w.slice(1, 3),
      ...w.slice(6, 12),
      ...st,
    ]);
  });
});
