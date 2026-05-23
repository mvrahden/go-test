import { describe, it, expect, vi, beforeEach } from "vitest";

const mockWorkspaceFolders: Array<{ name: string; uri: { fsPath: string } }> =
  [];
const mockGetWorkspaceFolder = vi.fn();

vi.mock("vscode", () => ({
  workspace: {
    get workspaceFolders() {
      return mockWorkspaceFolders;
    },
    getWorkspaceFolder: (...args: unknown[]) => mockGetWorkspaceFolder(...args),
    getConfiguration: () => ({ get: () => undefined }),
  },
  Uri: { file: (p: string) => ({ fsPath: p }) },
  window: {},
  tests: {
    createTestController: (_id: string, _label: string) => {
      const items = createMockCollection();
      return {
        items,
        createTestItem: (id: string, label: string, uri?: unknown) => ({
          id,
          label,
          uri,
          children: createMockCollection(),
          tags: [],
          range: undefined,
          description: undefined,
        }),
        createRunProfile: () => ({}),
        dispose: vi.fn(),
      };
    },
  },
  TestRunProfileKind: { Run: 1, Debug: 2, Coverage: 3 },
  TestTag: class {
    constructor(public id: string) {}
  },
  Range: class {
    constructor(
      public start: unknown,
      public end: unknown,
    ) {}
  },
  Position: class {
    constructor(
      public line: number,
      public character: number,
    ) {}
  },
  EventEmitter: class {
    event = vi.fn();
    fire = vi.fn();
  },
}));

function createMockCollection() {
  const map = new Map<string, any>();
  return {
    get: (id: string) => map.get(id),
    add: (item: any) => map.set(item.id, item),
    delete: (id: string) => map.delete(id),
    forEach: (fn: (item: any) => void) => map.forEach(fn),
    get size() {
      return map.size;
    },
    [Symbol.iterator]: () => map.values(),
    _map: map,
  };
}

import { GoTestController } from "./testController.js";
import type { DiscoveryCache } from "./discovery.js";
import { TestResultStore } from "./testResultStore.js";

function createMockCache(
  packages: Array<{
    importPath: string;
    dir: string;
    wsDir: string;
    suites?: any[];
  }>,
): DiscoveryCache {
  const pkgMap = new Map<string, any>();
  const wsDirs = new Map<string, string>();

  for (const pkg of packages) {
    pkgMap.set(pkg.importPath, {
      importPath: pkg.importPath,
      dir: pkg.dir,
      suites: pkg.suites ?? [],
    });
    wsDirs.set(pkg.importPath, pkg.wsDir);
  }

  return {
    get packages() {
      return Array.from(pkgMap.values());
    },
    getPackage: (ip: string) => pkgMap.get(ip),
    getWorkspaceDir: (ip: string) => wsDirs.get(ip),
    onDidUpdate: vi.fn(),
  } as unknown as DiscoveryCache;
}

function createController(cache: DiscoveryCache): GoTestController {
  const resultStore = new TestResultStore(undefined);
  const outputChannel = { appendLine: vi.fn() } as any;
  const noop = async () => {};
  return new GoTestController(
    cache,
    resultStore,
    outputChannel,
    noop,
    noop,
    noop,
    noop,
  );
}

function collectIds(collection: any): string[] {
  const ids: string[] = [];
  collection.forEach((item: any) => ids.push(item.id));
  return ids;
}

describe("GoTestController.rebuild", () => {
  beforeEach(() => {
    mockWorkspaceFolders.length = 0;
    mockGetWorkspaceFolder.mockReset();
  });

  describe("multi-folder workspace", () => {
    it("creates wsFolder: root nodes and qualifies dir IDs to avoid collision", () => {
      // Two workspace folders both containing an "internal" directory
      mockWorkspaceFolders.push(
        { name: "frontend", uri: { fsPath: "/projects/frontend" } },
        { name: "backend", uri: { fsPath: "/projects/backend" } },
      );
      mockGetWorkspaceFolder.mockImplementation((uri: { fsPath: string }) => {
        if (uri.fsPath.startsWith("/projects/frontend"))
          return { name: "frontend", uri: { fsPath: "/projects/frontend" } };
        if (uri.fsPath.startsWith("/projects/backend"))
          return { name: "backend", uri: { fsPath: "/projects/backend" } };
        return undefined;
      });

      const cache = createMockCache([
        {
          importPath: "example.com/frontend/internal/foo",
          dir: "/projects/frontend/internal/foo",
          wsDir: "/projects/frontend",
          suites: [
            {
              name: "FooSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "foo_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
        {
          importPath: "example.com/backend/internal/bar",
          dir: "/projects/backend/internal/bar",
          wsDir: "/projects/backend",
          suites: [
            {
              name: "BarSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "bar_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      const rootIds = collectIds(ctrl.testController.items);

      // Should have wsFolder: root nodes, not bare dir: nodes
      expect(rootIds).toContain("wsFolder:frontend");
      expect(rootIds).toContain("wsFolder:backend");
      // Should NOT have unqualified dir:internal (which would collide)
      expect(rootIds).not.toContain("dir:internal");
    });

    it("qualifies dir IDs with folder name prefix", () => {
      mockWorkspaceFolders.push(
        { name: "alpha", uri: { fsPath: "/ws/alpha" } },
        { name: "beta", uri: { fsPath: "/ws/beta" } },
      );
      mockGetWorkspaceFolder.mockImplementation((uri: { fsPath: string }) => {
        if (uri.fsPath.startsWith("/ws/alpha"))
          return { name: "alpha", uri: { fsPath: "/ws/alpha" } };
        if (uri.fsPath.startsWith("/ws/beta"))
          return { name: "beta", uri: { fsPath: "/ws/beta" } };
        return undefined;
      });

      const cache = createMockCache([
        {
          importPath: "example.com/alpha/pkg",
          dir: "/ws/alpha/pkg",
          wsDir: "/ws/alpha",
          suites: [
            {
              name: "S",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "s_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
        {
          importPath: "example.com/beta/pkg",
          dir: "/ws/beta/pkg",
          wsDir: "/ws/beta",
          suites: [
            {
              name: "T",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "t_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      // Check that dir items inside wsFolder nodes are qualified
      const frontendFolder = ctrl.testController.items.get("wsFolder:alpha");
      expect(frontendFolder).toBeDefined();
      const alphaChildIds = collectIds(frontendFolder!.children);
      // The package is at root level under ws dir, so it should be added directly
      // (single segment "pkg" becomes a dir node with qualified id)
      expect(
        alphaChildIds.some(
          (id: string) =>
            id.startsWith("dir:alpha/") || id === "example.com/alpha/pkg",
        ),
      ).toBe(true);
    });
  });

  describe("single-folder workspace", () => {
    it("keeps flat layout with dir: nodes (no wsFolder: nodes)", () => {
      mockWorkspaceFolders.push({
        name: "myproject",
        uri: { fsPath: "/projects/myproject" },
      });
      mockGetWorkspaceFolder.mockImplementation(() => ({
        name: "myproject",
        uri: { fsPath: "/projects/myproject" },
      }));

      const cache = createMockCache([
        {
          importPath: "example.com/myproject/internal/foo",
          dir: "/projects/myproject/internal/foo",
          wsDir: "/projects/myproject",
          suites: [
            {
              name: "FooSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "foo_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
        {
          importPath: "example.com/myproject/cmd/bar",
          dir: "/projects/myproject/cmd/bar",
          wsDir: "/projects/myproject",
          suites: [
            {
              name: "BarSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "bar_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      const rootIds = collectIds(ctrl.testController.items);

      // No wsFolder: nodes in single-folder mode
      expect(
        rootIds.filter((id: string) => id.startsWith("wsFolder:")),
      ).toHaveLength(0);
      // Should have dir: nodes (flat layout)
      expect(
        rootIds.some(
          (id: string) =>
            id.startsWith("dir:") || id.startsWith("example.com/"),
        ),
      ).toBe(true);
    });

    it("uses unqualified dir IDs without folder prefix", () => {
      mockWorkspaceFolders.push({
        name: "solo",
        uri: { fsPath: "/ws/solo" },
      });
      mockGetWorkspaceFolder.mockImplementation(() => ({
        name: "solo",
        uri: { fsPath: "/ws/solo" },
      }));

      const cache = createMockCache([
        {
          importPath: "example.com/solo/internal/svc",
          dir: "/ws/solo/internal/svc",
          wsDir: "/ws/solo",
          suites: [
            {
              name: "SvcSuite",
              parallel: false,
              focused: false,
              excluded: false,
              guarded: false,
              file: "svc_test.go",
              line: 1,
              col: 1,
              lifecycle: [],
              fixtures: [],
              methods: [],
            },
          ],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      const rootIds = collectIds(ctrl.testController.items);

      // dir: IDs should not have folder prefix
      const dirIds = rootIds.filter((id: string) => id.startsWith("dir:"));
      for (const id of dirIds) {
        expect(id).not.toContain("dir:solo/");
      }
    });
  });
});
