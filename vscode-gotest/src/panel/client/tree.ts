// The behavior tree's logic: which rows show for the state, and what each
// gesture does to the state. Pure functions over plain data, embedded in the
// webview by source (see helpers.ts).
import type { TreeEntry, TreeNode } from "../protocol.js";

export type TreeLogic = ReturnType<typeof treeLogic>;

// How far the whole list opens by default; a row toggled by hand keeps
// what it was given until the next level is chosen.
export type Level = "packages" | "suites" | "behaviors";

export interface TreeState {
  open: Record<string, boolean>;
  // Packages whose suites open by default: everything below was opened.
  deep: Record<string, boolean>;
  level: Level;
  focus?: string;
}

export type RowKind =
  "dir" | "pkg" | "suite" | "method" | "when" | "leaf" | "fail" | "loading";

// A row as the tree shows it, with the rows shown under it.
export interface VRow {
  key: string;
  kind: RowKind;
  level: number;
  label: string;
  expandable: boolean;
  open: boolean;
  parent?: string;
  children: VRow[];
  pkg?: string;
  pos?: string;
  entry?: TreeEntry;
  node?: TreeNode;
  // The output of a package that failed outside any test.
  lines?: unknown[];
}

// What a slice of a package holds, as the host sent it.
export interface Slice {
  nodes: TreeNode[];
  failure?: unknown[];
}

export function treeLogic() {
  const dirKey = (path: string) => `d:${path}`;
  const pkgKey = (pkg: string) => `p:${pkg}`;
  const nodeKey = (pkg: string, pos: string) => `n:${pkg}:${pos}`;

  // A node is a branch the reader opens when it groups rows by its own
  // name: a suite, a method, a fuzz target or a stdlib test with subtests.
  // A When is a heading over its rows, open with its parent.
  const branchKinds = new Set(["suite", "method", "fuzz", "test", "fixture"]);

  const isOpen = (
    st: TreeState,
    r: { key: string; kind: RowKind; pkg?: string },
  ): boolean => {
    const set = st.open[r.key];
    if (set !== undefined) return set;
    switch (r.kind) {
      case "dir":
        return true;
      case "pkg":
        return st.level !== "packages" || !!st.deep[r.pkg!];
      case "suite":
        return st.level === "behaviors" || !!st.deep[r.pkg!];
      default:
        return true;
    }
  };

  const nodeRows = (
    st: TreeState,
    pkg: string,
    nodes: TreeNode[],
    level: number,
    parent: string,
    top: boolean,
  ): VRow[] =>
    nodes.map((n) => {
      const key = nodeKey(pkg, n.pos);
      const kids = n.c ?? [];
      const kind: RowKind =
        kids.length === 0
          ? "leaf"
          : top
            ? "suite"
            : branchKinds.has(n.k)
              ? "method"
              : "when";
      const expandable = kind === "suite" || kind === "method";
      const open =
        kind === "when" || (expandable && isOpen(st, { key, kind, pkg }));
      return {
        key,
        kind,
        level,
        label: n.d,
        expandable,
        open,
        parent,
        children: open ? nodeRows(st, pkg, kids, level + 1, key, false) : [],
        pkg,
        pos: n.pos,
        node: n,
      };
    });

  // rows is the tree as it shows: every entry, and below the open ones the
  // rows the state opens; a package without its slice shows that it loads.
  const rows = (
    tree: TreeEntry[],
    slices: Record<string, Slice | undefined>,
    st: TreeState,
  ): VRow[] => {
    const entryRow = (e: TreeEntry, level: number, parent?: string): VRow => {
      if (e.kind === "dir") {
        const key = dirKey(e.key);
        const open = isOpen(st, { key, kind: "dir" });
        return {
          key,
          kind: "dir",
          level,
          label: e.name,
          expandable: true,
          open,
          parent,
          children: open
            ? e.children.map((c) => entryRow(c, level + 1, key))
            : [],
          entry: e,
        };
      }
      const key = pkgKey(e.pkg);
      const open = !e.empty && isOpen(st, { key, kind: "pkg", pkg: e.pkg });
      let children: VRow[] = [];
      if (open) {
        const s = slices[e.pkg];
        if (!s)
          children = [
            {
              key: `l:${e.pkg}`,
              kind: "loading",
              level: level + 1,
              label: "Loading the rows…",
              expandable: false,
              open: false,
              parent: key,
              children: [],
              pkg: e.pkg,
            },
          ];
        else {
          if (s.failure && s.failure.length > 0)
            children.push({
              key: `f:${e.pkg}`,
              kind: "fail",
              level: level + 1,
              label: "The package failed",
              expandable: false,
              open: false,
              parent: key,
              children: [],
              pkg: e.pkg,
              pos: "",
              lines: s.failure,
            });
          children.push(...nodeRows(st, e.pkg, s.nodes, level + 1, key, true));
        }
      }
      return {
        key,
        kind: "pkg",
        level,
        label: e.name,
        expandable: !e.empty,
        open,
        parent,
        children,
        pkg: e.pkg,
        entry: e,
      };
    };
    return tree.map((e) => entryRow(e, 1));
  };

  // flat lists the rows a reader can move over, in the order they show.
  const flat = (list: VRow[]): VRow[] =>
    list.flatMap((r) => [r, ...flat(r.children)]);

  const find = (list: VRow[], key: string | undefined): VRow | undefined =>
    key === undefined ? undefined : flat(list).find((r) => r.key === key);

  // pkgsBelow lists the packages under an entry.
  const pkgsBelow = (e: TreeEntry): string[] =>
    e.kind === "pkg" ? [e.pkg] : e.children.flatMap(pkgsBelow);

  const dirsBelow = (e: TreeEntry): string[] =>
    e.kind === "pkg" ? [] : [e.key, ...e.children.flatMap(dirsBelow)];

  const forget = (st: TreeState, prefix: string) => {
    for (const k of Object.keys(st.open))
      if (k.startsWith(prefix)) delete st.open[k];
  };

  const toggle = (st: TreeState, r: VRow) => {
    if (r.expandable) st.open[r.key] = !r.open;
  };

  // openBelow opens a row and everything under it: directories, packages,
  // every suite through.
  const openBelow = (st: TreeState, r: VRow) => {
    if (r.entry) {
      for (const d of dirsBelow(r.entry)) st.open[dirKey(d)] = true;
      for (const p of pkgsBelow(r.entry)) {
        st.open[pkgKey(p)] = true;
        st.deep[p] = true;
        forget(st, `n:${p}:`);
      }
      if (r.entry.kind === "dir") st.open[r.key] = true;
      return;
    }
    if (r.expandable) {
      st.open[r.key] = true;
      forget(st, `${r.key}.`);
    }
  };

  // closeBelow closes everything under a row and leaves the row open at
  // its own rows.
  const closeBelow = (st: TreeState, r: VRow) => {
    if (r.entry) {
      for (const d of dirsBelow(r.entry)) st.open[dirKey(d)] = false;
      for (const p of pkgsBelow(r.entry)) {
        st.open[pkgKey(p)] = false;
        delete st.deep[p];
        forget(st, `n:${p}:`);
      }
      st.open[r.key] = true;
      return;
    }
    if (!r.expandable) return;
    st.open[r.key] = true;
    const under = (list: TreeNode[]) => {
      for (const n of list) {
        if (n.c) {
          st.open[nodeKey(r.pkg!, n.pos)] = false;
          under(n.c);
        }
      }
    };
    under(r.node?.c ?? []);
  };

  // allOpen says whether everything under a row is open, which turns its
  // action into the closing one.
  const allOpen = (r: VRow): boolean =>
    (!r.expandable || r.open) &&
    r.kind !== "loading" &&
    (r.kind === "pkg" ? r.children.length > 0 : true) &&
    r.children.every((c) => allOpen(c)) &&
    (r.entry?.kind === "dir"
      ? r.children.length === r.entry.children.length
      : true);

  // setLevel opens the whole list to a level, once: rows toggled before
  // follow it, later toggles are free.
  const setLevel = (st: TreeState, level: Level) => {
    st.level = level;
    st.open = {};
    st.deep = {};
  };

  // reveal opens the way to a row: its directories, package and the rows
  // above it; the row is the focus.
  const reveal = (
    st: TreeState,
    tree: TreeEntry[],
    pkg: string,
    pos: string,
  ) => {
    const path = (list: TreeEntry[], at: string[]): string[] | undefined => {
      for (const e of list) {
        if (e.kind === "pkg") {
          if (e.pkg === pkg) return at;
        } else {
          const p = path(e.children, [...at, e.key]);
          if (p) return p;
        }
      }
      return undefined;
    };
    for (const d of path(tree, []) ?? []) st.open[dirKey(d)] = true;
    st.open[pkgKey(pkg)] = true;
    const parts = pos ? pos.split(".") : [];
    for (let i = 1; i < parts.length; i++)
      st.open[nodeKey(pkg, parts.slice(0, i).join("."))] = true;
    st.focus = pos ? nodeKey(pkg, pos) : pkgKey(pkg);
  };

  // What a key does to the tree: a row to focus, or a change to make.
  type Op =
    | { op: "focus"; key: string }
    | {
        op: "toggle" | "openBelow" | "closeBelow" | "source" | "copy" | "menu";
        key: string;
      }
    | undefined;

  // keyOp reads a key on the focused row, as the tree pattern has it.
  const keyOp = (
    list: VRow[],
    focus: string | undefined,
    e: {
      key: string;
      shiftKey?: boolean;
      ctrlKey?: boolean;
      metaKey?: boolean;
    },
  ): Op => {
    const all = flat(list).filter((r) => r.kind !== "loading");
    if (all.length === 0) return undefined;
    const i = all.findIndex((r) => r.key === focus);
    // A focus the tree does not show moves to its first row, and acts on none.
    if (i < 0)
      return ["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)
        ? { op: "focus", key: all[0].key }
        : undefined;
    const r = all[i];
    const branch =
      r.kind === "dir" ||
      r.kind === "pkg" ||
      r.kind === "suite" ||
      r.kind === "method";
    switch (e.key) {
      case "ArrowDown":
        return { op: "focus", key: all[Math.min(all.length - 1, i + 1)].key };
      case "ArrowUp":
        return { op: "focus", key: all[Math.max(0, i - 1)].key };
      case "Home":
        return { op: "focus", key: all[0].key };
      case "End":
        return { op: "focus", key: all[all.length - 1].key };
      case "ArrowRight":
        if (e.shiftKey)
          return branch ? { op: "openBelow", key: r.key } : undefined;
        if (r.expandable && !r.open) return { op: "toggle", key: r.key };
        if (r.children.length > 0)
          return { op: "focus", key: r.children[0].key };
        return undefined;
      case "ArrowLeft":
        if (e.shiftKey)
          return branch ? { op: "closeBelow", key: r.key } : undefined;
        if (r.expandable && r.open) return { op: "toggle", key: r.key };
        return r.parent ? { op: "focus", key: r.parent } : undefined;
      case "Enter":
        if (r.expandable) return { op: "toggle", key: r.key };
        return r.node?.loc ? { op: "source", key: r.key } : undefined;
      case "ContextMenu":
      case "F10":
        if (e.key === "F10" && !e.shiftKey) return undefined;
        return { op: "menu", key: r.key };
      case "c":
      case "C":
        return (e.ctrlKey || e.metaKey) && branch
          ? { op: "copy", key: r.key }
          : undefined;
    }
    return undefined;
  };

  // A row's actions, in the order the row and its menu list them.
  type Action =
    "openBelow" | "closeBelow" | "copy" | "run" | "debug" | "source";

  const actions = (r: VRow): Action[] => {
    const out: Action[] = [];
    const id = !!r.node?.id;
    if (r.kind === "dir" || r.kind === "pkg")
      out.push(allOpen(r) ? "closeBelow" : "openBelow");
    if (
      r.kind === "dir" ||
      r.kind === "pkg" ||
      r.kind === "suite" ||
      r.kind === "method"
    )
      out.push("copy");
    if (r.kind === "dir" || r.kind === "pkg" || id) out.push("run");
    if (id && r.kind !== "dir" && r.kind !== "pkg") out.push("debug");
    if (r.node?.loc) out.push("source");
    return out;
  };

  // menuItems are a row's actions as its menu lists them: both ways of
  // opening and closing below a directory or package, closing below an
  // open suite.
  const menuItems = (r: VRow): (Action | "-")[] => {
    const own = actions(r).filter(
      (a) => a !== "openBelow" && a !== "closeBelow",
    );
    const first: Action[] =
      r.kind === "dir" || r.kind === "pkg"
        ? ["openBelow", "closeBelow"]
        : r.kind === "suite" && r.open
          ? ["closeBelow"]
          : [];
    const copy = own.filter((a) => a === "copy");
    const rest = own.filter((a) => a !== "copy");
    const head = [...first, ...copy];
    return head.length > 0 && rest.length > 0
      ? [...head, "-", ...rest]
      : [...head, ...rest];
  };

  return {
    rows,
    flat,
    find,
    pkgsBelow,
    toggle,
    openBelow,
    closeBelow,
    allOpen,
    setLevel,
    reveal,
    keyOp,
    actions,
    menuItems,
    isOpen,
    pkgKey,
    nodeKey,
    dirKey,
  };
}
