package gotestaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

// node is a suite or a behavior of one record, as the comparison sees it.
type node struct {
	kind    string
	key     string // its identity among its siblings
	pkg     string
	mod     string
	names   []string // the levels go test reports, from the suite down
	bench   bool     // a benchmark or fuzz target, or under one
	path    []string
	loc     Location
	hash    string // its own body's
	tree    string // its body's and every behavior's beneath it
	suite   *Suite
	b       *Behavior
	parent  *node
	kids    []*node
	match   *node
	how     string // "", ChangeReworded or ChangeMoved, on both sides of a pair
	skipped bool   // not compared
	whole   bool   // compared as one body: its behaviors are not all named
}

func (n *node) incomplete() bool { return n.b != nil && n.b.Incomplete }

// leaves counts the leaves at and below n.
func (n *node) leaves() int {
	if len(n.kids) == 0 {
		return 1
	}
	total := 0
	for _, k := range n.kids {
		total += k.leaves()
	}
	return total
}

// suiteNodes are a package's suites as nodes.
func suiteNodes(pkg *Package) []*node {
	out := make([]*node, 0, len(pkg.Suites))
	for i := range pkg.Suites {
		s := &pkg.Suites[i]
		key := identity(s.Name)
		if s.External {
			key += " (external)"
		}
		n := &node{kind: KindSuite, key: key, pkg: pkg.Path, mod: pkg.Module, names: []string{"Test" + s.Name}, path: []string{s.Display}, loc: s.Location, suite: s}
		n.kids = behaviorNodes(s.Methods, n)
		n.tree = treeHash(n)
		out = append(out, n)
	}
	return out
}

func behaviorNodes(bs []Behavior, parent *node) []*node {
	out := make([]*node, 0, len(bs))
	for i := range bs {
		b := &bs[i]
		key := b.Name
		if b.Kind == KindTest || b.Kind == KindBenchmark || b.Kind == KindFuzz {
			key = identity(b.Name)
		}
		path := append(append([]string{}, parent.path...), b.Display)
		n := &node{kind: b.Kind, key: key, pkg: parent.pkg, mod: parent.mod, path: path, loc: b.Location, hash: b.Hash, b: b, parent: parent,
			names: append(slices.Clip(parent.names), b.Name), bench: parent.bench || b.Kind == KindBenchmark || b.Kind == KindFuzz}
		n.kids = behaviorNodes(b.Children, n)
		n.tree = treeHash(n)
		out = append(out, n)
	}
	return out
}

// treeHash folds a node's own hash with each child's name and tree.
func treeHash(n *node) string {
	h := sha256.New()
	h.Write([]byte(n.kind + "\x00" + n.hash))
	for _, k := range n.kids {
		h.Write([]byte("\x00" + k.key + "\x00" + k.tree))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// matcher pairs the nodes of two records.
type matcher struct {
	notes []string
}

// pair matches a and b and, when both declare all their behaviors, their
// children; else the children are not compared.
func (m *matcher) pair(a, b *node, how string) {
	a.match, b.match, a.how, b.how = b, a, how, how
	if a.incomplete() || b.incomplete() {
		skipAll(a.kids)
		skipAll(b.kids)
		a.whole, b.whole = true, true
		if a.tree != b.tree {
			m.notes = append(m.notes, strings.Join(b.path, " / ")+": its behaviors are not all named in source, so it is compared as one body")
		}
		return
	}
	m.level(a.kids, b.kids)
}

func skipAll(ns []*node) {
	for _, n := range ns {
		n.skipped = true
		skipAll(n.kids)
	}
}

// level pairs siblings: by identity, then a reworded one by its tree where
// exactly one base and one head sibling share it.
func (m *matcher) level(base, head []*node) {
	byKey := map[string]*node{}
	for _, h := range head {
		byKey[h.kind+"\x00"+h.key] = h
	}
	for _, b := range base {
		if h := byKey[b.kind+"\x00"+b.key]; h != nil && h.match == nil && b.match == nil && !b.skipped && !h.skipped {
			m.pair(b, h, "")
		}
	}
	key := func(n *node) string { return n.kind + "\x00" + n.tree }
	for _, p := range uniquePairs(unmatched(base), unmatched(head), key) {
		m.pair(p[0], p[1], ChangeReworded)
	}
}

// moved pairs, across the records, the unmatched nodes that share kind,
// identity and tree with exactly one node of the other side, until none is
// left to pair.
func (m *matcher) moved(base, head []*node) {
	key := func(n *node) string { return n.kind + "\x00" + n.key + "\x00" + n.tree }
	for {
		pairs := uniquePairs(open(base), open(head), key)
		if len(pairs) == 0 {
			return
		}
		// A parent comes before its children, which its pair matches.
		for _, p := range pairs {
			if p[0].match == nil && p[1].match == nil {
				m.pair(p[0], p[1], ChangeMoved)
			}
		}
	}
}

func unmatched(ns []*node) []*node {
	var out []*node
	for _, n := range ns {
		if n.match == nil && !n.skipped {
			out = append(out, n)
		}
	}
	return out
}

// open is every unmatched node at or below roots.
func open(roots []*node) []*node {
	var out []*node
	var walk func([]*node)
	walk = func(ns []*node) {
		for _, n := range ns {
			if n.skipped {
				continue
			}
			if n.match == nil {
				out = append(out, n)
			}
			walk(n.kids)
		}
	}
	walk(roots)
	return out
}

// uniquePairs pairs a base and a head node whose key no other node of either
// side has; any other key pairs nothing, for a guess is no finding.
func uniquePairs(base, head []*node, key func(*node) string) [][2]*node {
	count := func(ns []*node) (map[string]int, map[string]*node) {
		c, first := map[string]int{}, map[string]*node{}
		for _, n := range ns {
			k := key(n)
			c[k]++
			if first[k] == nil {
				first[k] = n
			}
		}
		return c, first
	}
	bc, bf := count(base)
	hc, hf := count(head)
	var out [][2]*node
	for _, b := range base {
		k := key(b)
		if bc[k] == 1 && hc[k] == 1 && bf[k] == b {
			out = append(out, [2]*node{b, hf[k]})
		}
	}
	return out
}
