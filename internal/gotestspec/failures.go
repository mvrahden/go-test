package gotestspec

// Failure is one failure a tree carries: a failed node, named by the path
// from its package's top node down to it, or a package that failed on its
// own, with no path. Lines are its diagnostic output without go test's
// framing lines. Origin says nothing under it failed: the failure is its own,
// not one it carries for a descendant beside output of its own.
type Failure struct {
	Package string
	Path    []string
	Lines   []string
	Origin  bool
}

// Failures lists what failed in packages, by the rule the summary renders
// with: a failed leaf, and an interior node with output of its own or no
// failed descendant; a failed package whose nodes all passed, by its output.
func Failures(packages []*Package) []Failure {
	var out []Failure
	for _, pkg := range packages {
		before := len(out)
		for _, n := range pkg.Nodes {
			collectFailedPaths(pkg.Path, n, nil, &out)
		}
		if len(out) == before && pkg.Status == StatusFail {
			if lines := filterOutput(pkg.Output); len(lines) > 0 {
				out = append(out, Failure{Package: pkg.Path, Lines: lines, Origin: true})
			}
		}
	}
	return out
}

func collectFailedPaths(pkgPath string, n *Node, path []string, out *[]Failure) {
	cur := append(append([]string(nil), path...), n.Name)
	if n.Status == StatusFail && (len(n.Children) == 0 || hasOwnDiagnostic(n) || failedOnItsOwn(n)) {
		*out = append(*out, Failure{Package: pkgPath, Path: cur, Lines: filterOutput(n.Output), Origin: !anyDescendantFailed(n)})
	}
	for _, c := range n.Children {
		collectFailedPaths(pkgPath, c, cur, out)
	}
}
