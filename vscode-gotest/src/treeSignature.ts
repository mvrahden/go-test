import type { DiscoverBehavior, DiscoverPackage } from "./types.js";

// treeSignature captures the shape the stored results were applied against.
// Every level that contributes to an item id is in it — package, suite, method
// and the declared behaviors below them — because a restore has to run again
// exactly when an id appears that was not there before. Leaving behaviors out
// would make that claim false for the deepest ids in the tree.
//
// Folded into a hash rather than kept as a string: on a workspace with a few
// thousand behaviors the joined form is hundreds of kilobytes, built twice, on
// the activation path the snapshot exists to keep short.
export function treeSignature(packages: DiscoverPackage[]): string {
  let hash = 0x811c9dc5;
  const feed = (s: string) => {
    for (let i = 0; i < s.length; i++) {
      hash ^= s.charCodeAt(i);
      hash = Math.imul(hash, 0x01000193);
    }
    // Separator, so "ab" + "c" and "a" + "bc" do not collide.
    hash ^= 0x2c;
    hash = Math.imul(hash, 0x01000193);
  };
  const feedBehaviors = (behaviors: DiscoverBehavior[] | undefined) => {
    for (const behavior of behaviors ?? []) {
      feed(behavior.name);
      feedBehaviors(behavior.children);
    }
  };
  for (const pkg of [...packages].sort((a, b) =>
    a.importPath < b.importPath ? -1 : a.importPath > b.importPath ? 1 : 0,
  )) {
    feed(pkg.importPath);
    for (const suite of pkg.suites ?? []) {
      feed(suite.name);
      for (const method of suite.methods ?? []) {
        feed(method.name);
        feedBehaviors(method.behaviors);
      }
    }
  }
  return (hash >>> 0).toString(16);
}
