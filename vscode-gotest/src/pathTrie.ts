export interface PathNode {
  segment: string;
  children: Map<string, PathNode>;
  importPath?: string;
}

export function buildPathTrie(
  entries: { relativePath: string; importPath: string }[],
): PathNode {
  const root: PathNode = { segment: "", children: new Map() };

  for (const entry of entries) {
    if (entry.relativePath === ".") {
      root.importPath = entry.importPath;
      continue;
    }
    const parts = entry.relativePath.split("/");
    let current = root;
    for (const part of parts) {
      let child = current.children.get(part);
      if (!child) {
        child = { segment: part, children: new Map() };
        current.children.set(part, child);
      }
      current = child;
    }
    current.importPath = entry.importPath;
  }

  return root;
}

export function collapsePathTrie(node: PathNode): void {
  const collapsed = new Map<string, PathNode>();
  for (const [, child] of node.children) {
    collapseNode(child);
    collapsed.set(child.segment, child);
  }
  node.children = collapsed;
}

function collapseNode(node: PathNode): void {
  while (node.children.size === 1 && !node.importPath) {
    const [, child] = [...node.children.entries()][0];
    node.segment = node.segment
      ? `${node.segment}/${child.segment}`
      : child.segment;
    node.importPath = child.importPath;
    node.children = child.children;
  }
  const collapsed = new Map<string, PathNode>();
  for (const [, child] of node.children) {
    collapseNode(child);
    collapsed.set(child.segment, child);
  }
  node.children = collapsed;
}
