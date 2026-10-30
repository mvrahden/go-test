package gotestspec

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
