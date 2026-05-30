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
    modulePath?: string;
    suites?: any[];
  }>,
): DiscoveryCache {
  const pkgMap = new Map<string, any>();
  const wsDirs = new Map<string, string>();
  const pkgModules = new Map<string, string>();
  const moduleDirs = new Map<string, string>();

  for (const pkg of packages) {
    pkgMap.set(pkg.importPath, {
      importPath: pkg.importPath,
      dir: pkg.dir,
      suites: pkg.suites ?? [],
    });
    wsDirs.set(pkg.importPath, pkg.wsDir);
    if (pkg.modulePath) {
      pkgModules.set(pkg.importPath, pkg.modulePath);
      if (!moduleDirs.has(pkg.modulePath)) {
        const suffix = pkg.importPath.slice(pkg.modulePath.length);
        const moduleDir = suffix
          ? pkg.dir.slice(0, -suffix.length).replace(/[/\\]+$/, "")
          : pkg.dir;
        moduleDirs.set(pkg.modulePath, moduleDir);
      }
    }
  }

  return {
    get packages() {
      return Array.from(pkgMap.values());
    },
    getPackage: (ip: string) => pkgMap.get(ip),
    getWorkspaceDir: (ip: string) => wsDirs.get(ip),
    getModulePath: (ip: string) => pkgModules.get(ip),
    getModuleDir: (mp: string) => moduleDirs.get(mp),
    getModules: (wsDir: string) => {
      const modules = new Set<string>();
      for (const [ip, wd] of wsDirs) {
        if (wd === wsDir) {
          const mod = pkgModules.get(ip);
          if (mod) modules.add(mod);
        }
      }
      return Array.from(modules);
    },
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

function makeSuite(name: string): any {
  return {
    name,
    parallel: false,
    focused: false,
    excluded: false,
    guarded: false,
    file: `${name.toLowerCase()}_test.go`,
    line: 1,
    col: 1,
    lifecycle: [],
    fixtures: [],
    methods: [
      {
        name: "TestOne",
        parallel: false,
        focused: false,
        excluded: false,
        file: `${name.toLowerCase()}_test.go`,
        line: 10,
        col: 1,
      },
    ],
  };
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

  describe("go.work multi-module", () => {
    it("inserts module nodes when workspace has multiple modules", () => {
      const cache = createMockCache([
        {
          importPath: "example.com/proj/internal/auth",
          dir: "/ws/internal/auth",
          wsDir: "/ws",
          modulePath: "example.com/proj",
          suites: [makeSuite("AuthSuite")],
        },
        {
          importPath: "example.com/proj/examples/cart",
          dir: "/ws/examples/cart",
          wsDir: "/ws",
          modulePath: "example.com/proj/examples",
          suites: [makeSuite("CartSuite")],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      const rootIds = collectIds(ctrl.testController.items);
      expect(rootIds).toContain("module:example.com/proj");
      expect(rootIds).toContain("module:example.com/proj/examples");

      const modItem = (ctrl.testController.items as any)._map.get(
        "module:example.com/proj",
      );
      expect(modItem.tags.some((t: any) => t.id === "module")).toBe(true);
      expect(modItem.description).toBe("module");
    });

    it("omits module nodes for single-module workspace", () => {
      const cache = createMockCache([
        {
          importPath: "example.com/proj/internal/auth",
          dir: "/ws/internal/auth",
          wsDir: "/ws",
          modulePath: "example.com/proj",
          suites: [makeSuite("AuthSuite")],
        },
        {
          importPath: "example.com/proj/internal/db",
          dir: "/ws/internal/db",
          wsDir: "/ws",
          modulePath: "example.com/proj",
          suites: [makeSuite("DBSuite")],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      const rootIds = collectIds(ctrl.testController.items);
      expect(rootIds).not.toContain("module:example.com/proj");
      // Should have directory nodes directly
      expect(rootIds.some((id: string) => id.startsWith("dir:"))).toBe(true);
    });

    it("nests packages under the correct module node", () => {
      const cache = createMockCache([
        {
          importPath: "example.com/proj/pkg/a",
          dir: "/ws/pkg/a",
          wsDir: "/ws",
          modulePath: "example.com/proj",
          suites: [makeSuite("ASuite")],
        },
        {
          importPath: "example.com/proj/examples/auth",
          dir: "/ws/examples/auth",
          wsDir: "/ws",
          modulePath: "example.com/proj/examples",
          suites: [makeSuite("ExAuthSuite")],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      // Check packages are under the right module
      const projModule = (ctrl.testController.items as any)._map.get(
        "module:example.com/proj",
      );
      expect(projModule).toBeDefined();

      const exModule = (ctrl.testController.items as any)._map.get(
        "module:example.com/proj/examples",
      );
      expect(exModule).toBeDefined();

      // pkg/a should be under proj module
      const projChildren: string[] = [];
      projModule.children.forEach((c: any) => projChildren.push(c.id));
      expect(
        projChildren.some(
          (id: string) =>
            id.includes("pkg/a") || id === "example.com/proj/pkg/a",
        ),
      ).toBe(true);

      // examples/auth should be under examples module
      const exChildren: string[] = [];
      exModule.children.forEach((c: any) => exChildren.push(c.id));
      expect(
        exChildren.some(
          (id: string) =>
            id.includes("auth") || id === "example.com/proj/examples/auth",
        ),
      ).toBe(true);
    });

    it("uses module-relative paths within module subtrees", () => {
      const cache = createMockCache([
        {
          importPath: "example.com/proj/examples/demo/foo",
          dir: "/ws/examples/demo/foo",
          wsDir: "/ws",
          modulePath: "example.com/proj/examples",
          suites: [makeSuite("FooSuite")],
        },
        {
          importPath: "example.com/proj/examples/demo/bar",
          dir: "/ws/examples/demo/bar",
          wsDir: "/ws",
          modulePath: "example.com/proj/examples",
          suites: [makeSuite("BarSuite")],
        },
        {
          importPath: "example.com/proj/lib",
          dir: "/ws/lib",
          wsDir: "/ws",
          modulePath: "example.com/proj",
          suites: [makeSuite("LibSuite")],
        },
      ]);

      const ctrl = createController(cache);
      ctrl.rebuild();

      // The examples module should have demo/foo and demo/bar nested under "demo" dir,
      // NOT "examples/demo/foo"
      const exModule = (ctrl.testController.items as any)._map.get(
        "module:example.com/proj/examples",
      );
      expect(exModule).toBeDefined();

      const exChildIds: string[] = [];
      exModule.children.forEach((c: any) => exChildIds.push(c.id));
      // Should NOT have "examples" as a directory prefix within the module subtree
      expect(
        exChildIds.some((id: string) => id === "dir:examples"),
      ).toBe(false);
    });
  });
});
