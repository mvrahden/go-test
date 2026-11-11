package gotestspec

import (
	"fmt"
	"strconv"
	"strings"
)

// WithHidden leaves out of a rendering every row whose own status is one of
// statuses and that has no shown row beneath it. It is a view, not a verdict:
// counts, durations and the exit code stay those of the whole tree.
func WithHidden(statuses ...Status) RenderOption {
	return func(c *renderConfig) {
		if len(statuses) == 0 {
			return
		}
		c.hidden = map[Status]bool{}
		for _, s := range statuses {
			c.hidden[s] = true
		}
	}
}

// WithMatch keeps the leaves whose Display holds text, case-insensitively,
// and their ancestors: the panel's search. Unlike WithHidden it is a
// selection: the counts are those of what is kept. "" keeps everything.
func WithMatch(text string) RenderOption {
	return func(c *renderConfig) { c.match = text }
}

// WithOnly renders one package, or one node of it, with its subtree. A node
// is named by its position among the shown children at each level, as the
// JSON document lists them. Unlike WithHidden it is a selection: the counts
// are those of what is kept.
func WithOnly(pkgPath string, position []int) RenderOption {
	return func(c *renderConfig) { c.only = &onlyView{pkg: pkgPath, position: position} }
}

type onlyView struct {
	pkg      string
	position []int
}

// CheckView reports whether the options' WithOnly names a node of packages.
func CheckView(packages []*Package, opts ...RenderOption) error {
	var cfg renderConfig
	for _, o := range opts {
		o(&cfg)
	}
	_, err := cfg.narrow(packages)
	return err
}

// narrow applies WithOnly and WithMatch: copies of the packages and nodes
// that are kept, the originals untouched. Without either it is packages.
func (c *renderConfig) narrow(packages []*Package) ([]*Package, error) {
	if c.only == nil && c.match == "" {
		return packages, nil
	}
	var out []*Package
	if c.only != nil {
		var pkg *Package
		for _, p := range packages {
			if p.Path == c.only.pkg {
				pkg = p
				break
			}
		}
		if pkg == nil {
			return nil, fmt.Errorf("--only: no package %s", c.only.pkg)
		}
		cp := *pkg
		nodes := pkg.Nodes
		for i, at := range c.only.position {
			shown := c.visible(nodes)
			if at < 0 || at >= len(shown) {
				return nil, fmt.Errorf("--only: no node at %s in %s", joinPosition(c.only.position[:i+1]), pkg.Path)
			}
			if i == len(c.only.position)-1 {
				cp.Nodes = []*Node{shown[at]}
			}
			nodes = shown[at].Children
		}
		restate(pkg, &cp)
		out = []*Package{&cp}
	} else {
		out = packages
	}
	if c.match == "" {
		return out, nil
	}
	var kept []*Package
	for _, pkg := range out {
		cp := *pkg
		cp.Nodes = c.matching(pkg.Nodes)
		restate(pkg, &cp)
		if len(cp.Nodes) > 0 {
			kept = append(kept, &cp)
		}
	}
	return kept, nil
}

// restate gives a package cut down from orig the verdict of what is kept
// when orig failed only through rows that were left out. A failure the
// package carries on its own stays.
func restate(orig, cut *Package) {
	if orig.Status == StatusFail && !PkgFailedOnItsOwn(orig) {
		cut.Status = worstOf(cut.Nodes)
	}
}

// worstOf is the status of rows together: fail, else none, else skip, else
// pass.
func worstOf(nodes []*Node) Status {
	worst := StatusPass
	rank := map[Status]int{StatusPass: 0, StatusSkip: 1, StatusNone: 2, StatusFail: 3}
	for _, n := range nodes {
		if rank[n.Status] > rank[worst] {
			worst = n.Status
		}
	}
	return worst
}

func joinPosition(position []int) string {
	parts := make([]string, len(position))
	for i, at := range position {
		parts[i] = strconv.Itoa(at)
	}
	return strings.Join(parts, ".")
}

// matching is the nodes that are a leaf holding the match, or have one
// beneath them.
func (c *renderConfig) matching(nodes []*Node) []*Node {
	var out []*Node
	for _, n := range nodes {
		if len(n.Children) == 0 {
			if strings.Contains(strings.ToLower(n.Display), strings.ToLower(c.match)) {
				out = append(out, n)
			}
			continue
		}
		if kids := c.matching(n.Children); len(kids) > 0 {
			cp := *n
			cp.Children = kids
			if n.Status == StatusFail && !ownFailure(n) {
				// The failure was a leaf left out: the kept rows speak for it.
				cp.Status = worstOf(kids)
			}
			out = append(out, &cp)
		}
	}
	return out
}

// WithPackageStats gives each package of a JSON document the counts of its
// own nodes, in the shape of the document's stats.
func WithPackageStats() RenderOption {
	return func(c *renderConfig) { c.packageStats = true }
}

// shown reports whether a row is rendered: its own status is not hidden, or
// a row beneath it is shown.
func (c *renderConfig) shown(n *Node) bool {
	if !c.hidden[n.Status] {
		return true
	}
	for _, child := range n.Children {
		if c.shown(child) {
			return true
		}
	}
	return false
}

// visible is the shown rows among nodes, all of them when nothing is hidden.
func (c *renderConfig) visible(nodes []*Node) []*Node {
	if len(c.hidden) == 0 {
		return nodes
	}
	var out []*Node
	for _, n := range nodes {
		if c.shown(n) {
			out = append(out, n)
		}
	}
	return out
}

// packageShown reports whether a package is rendered: its own status is not
// hidden, or one of its rows is shown.
func (c *renderConfig) packageShown(pkg *Package) bool {
	return !c.hidden[pkg.Status] || len(c.visible(pkg.Nodes)) > 0
}
