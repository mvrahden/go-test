package gotestspec

import "sort"

// Overlay lays the verdicts of observed, a tree built from a stream, over
// declared, the specification read from source, and returns declared.
//
// A declared node the stream reported takes its verdict, duration, output
// and benchmark result, and keeps what the source says of it: kind, label,
// vocabulary, focus, exclusion, completeness and position. One the stream
// did not report has no verdict and is NotRun, unless the X_ prefix excludes
// it or an ancestor: the run skips those, so they read as skipped. Nodes and
// packages only the stream holds — rows of a table the source cannot list,
// packages outside the declared ones, the run's own bookings — are kept as
// observed, under their declared parent.
//
// Nodes match by package path and by name among their siblings, the n-th of
// a repeated name matching the n-th: two same-named suites of a package's
// internal and external test files read as the run's first and its second,
// as go test runs them.
func Overlay(declared, observed []*Package) []*Package {
	byPath := map[string]*Package{}
	for _, o := range observed {
		byPath[o.Path] = o
	}
	out := make([]*Package, 0, len(declared)+len(observed))
	seen := map[string]bool{}
	for _, d := range declared {
		seen[d.Path] = true
		o := byPath[d.Path]
		if o == nil {
			d.Nodes = overlayNodes(d.Nodes, nil, false)
			out = append(out, d)
			continue
		}
		d.Status, d.Duration, d.Output = o.Status, o.Duration, o.Output
		d.Nodes = overlayNodes(d.Nodes, o.Nodes, false)
		sort.SliceStable(d.Nodes, func(i, j int) bool { return d.Nodes[i].Name < d.Nodes[j].Name })
		FailBranches(d)
		out = append(out, d)
	}
	for _, o := range observed {
		if !seen[o.Path] {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// overlayNodes pairs declared with observed siblings and appends the
// observed ones the source does not declare. A suite that declares neither a
// test method nor a fuzz target (benchmarks only) states nothing a test run
// reports, so it is left out unless the stream holds it.
func overlayNodes(declared, observed []*Node, excluded bool) []*Node {
	used := make([]bool, len(observed))
	ordinal := map[string]int{}
	out := make([]*Node, 0, len(declared)+len(observed))
	for _, d := range declared {
		nth := ordinal[d.Name]
		ordinal[d.Name]++
		match := -1
		for i, o := range observed {
			if o.Name != d.Name {
				continue
			}
			if nth == 0 {
				match = i
				break
			}
			nth--
		}
		if match < 0 {
			if d.Kind == KindSuite && len(d.Children) == 0 {
				continue
			}
			notRun([]*Node{d}, excluded)
			out = append(out, d)
			continue
		}
		used[match] = true
		o := observed[match]
		d.Status, d.Duration, d.Start, d.End, d.Paused = o.Status, o.Duration, o.Start, o.End, o.Paused
		d.Output = o.Output
		d.External, d.Variant = o.External, o.Variant
		d.Iterations, d.NsPerOp, d.BytesPerOp, d.AllocsPerOp = o.Iterations, o.NsPerOp, o.BytesPerOp, o.AllocsPerOp
		d.Metrics, d.Procs = o.Metrics, o.Procs
		d.Children = overlayNodes(d.Children, o.Children, excluded || d.Excluded)
		out = append(out, d)
	}
	for i, o := range observed {
		if !used[i] {
			out = append(out, o)
		}
	}
	return out
}

// notRun marks declared nodes the stream did not report: NotRun, or skipped
// where the X_ prefix excludes them.
func notRun(nodes []*Node, excluded bool) {
	for _, n := range nodes {
		skipped := excluded || n.Excluded
		if skipped {
			n.Status = StatusSkip
		} else {
			n.NotRun = true
		}
		notRun(n.Children, skipped)
	}
}
