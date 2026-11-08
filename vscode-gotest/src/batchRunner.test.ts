import { describe, it, expect, vi } from "vitest";

vi.mock("vscode", () => ({
  TestMessage: class {},
  Uri: { file: (p: string) => ({ fsPath: p }) },
  workspace: { getConfiguration: () => ({ get: () => undefined }) },
  window: {},
}));

import { runPiece } from "./batchRunner.js";

interface Item {
  id: string;
  label: string;
  parent?: Item;
  tags: { id: string }[];
}

function item(id: string, label: string, parent?: Item): Item {
  return { id, label, parent, tags: [] };
}

const pkg: Item = {
  ...item("example.com/cart", "cart"),
  tags: [{ id: "package" }],
};
const suite = item("example.com/cart/CartTestSuite", "CartTestSuite", pkg);
const add = item("example.com/cart/CartTestSuite/TestAdd", "TestAdd", suite);

describe("runPiece", () => {
  it("says a filtered run covered exactly the test paths it asked for", () => {
    const piece = runPiece("stream", {
      pkgInfos: [
        { importPath: pkg.id, items: [add] as never, dir: "/ws/cart" },
      ],
      filter: "^TestCartTestSuite$/^TestAdd$",
      workspaceDir: "/ws",
    });

    expect(piece.stream).toBe("stream");
    expect(piece.folder).toBe("/ws");
    const covers = piece.partial?.get(pkg.id);
    expect(covers, "a one-method rerun is partial").toBeDefined();
    expect(covers!("TestCartTestSuite/TestAdd")).toBe(true);
    expect(covers!("TestCartTestSuite/TestAdd/when_empty")).toBe(true);
    expect(covers!("TestCartTestSuite/TestTotal")).toBe(false);
    expect(covers!("TestCartTestSuite")).toBe(false);
  });

  it("says an unfiltered run replaced its packages whole", () => {
    const piece = runPiece("stream", {
      pkgInfos: [
        { importPath: pkg.id, items: [pkg] as never, dir: "/ws/cart" },
      ],
      filter: undefined,
      workspaceDir: "/ws",
    });
    expect(piece.partial).toBeUndefined();
  });
});
