import { describe, it, expect } from "vitest";
import { SpecStore, coverOf, isRunLevelPackage } from "./specStore.js";

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

    expect(pathsOf(store.compose("/ws"), cart)).toEqual([
      "TestCartTestSuite/TestRows/#00",
      "TestCartTestSuite/TestRows",
      "TestCartTestSuite/TestAdd",
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
      stream: fullCart + stream(verdict("interrupted", "fail")),
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

  it("treats a suite that ran twice in a package as one unit", () => {
    const twice = stream(
      ...test(cart, "TestCartTestSuite/TestAdd", "pass"),
      ...test(cart, "TestCartTestSuite", "pass"),
      ...test(cart, "TestCartTestSuite/TestAdd#01", "fail"),
      ...test(cart, "TestCartTestSuite", "fail"),
      verdict(cart, "fail"),
    );
    const store = new SpecStore();
    store.merge({ stream: twice, folder: "/ws" });
    store.merge({
      stream: twice.replace(/"fail"/g, '"pass"'),
      folder: "/ws",
      partial: new Map([[cart, coverOf(["TestCartTestSuite/TestAdd"])]]),
    });

    const composed = store.compose("/ws");
    expect(composed).not.toContain('"fail"');
    expect(pathsOf(composed, cart)).toEqual([
      "TestCartTestSuite/TestAdd",
      "TestCartTestSuite",
      "TestCartTestSuite",
      "TestCartTestSuite/TestAdd#01",
    ]);
  });

  it("restores what it serialized", () => {
    const store = new SpecStore();
    store.merge({ stream: fullCart, folder: "/ws" });
    const copy = new SpecStore();
    copy.restore(JSON.parse(JSON.stringify(store.serialize())));
    expect(copy.compose("/ws")).toBe(store.compose("/ws"));
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

describe("isRunLevelPackage", () => {
  it("knows the CLI's synthetic names from import paths", () => {
    expect(isRunLevelPackage("shared fixtures")).toBe(true);
    expect(isRunLevelPackage("global --timeout")).toBe(true);
    expect(isRunLevelPackage("interrupted")).toBe(true);
    expect(isRunLevelPackage("example.com/cart")).toBe(false);
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
        verdict("interrupted", "fail"),
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
