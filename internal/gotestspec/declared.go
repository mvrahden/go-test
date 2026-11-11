package gotestspec

import "sort"

// Overlay lays the verdicts of observed, a tree built from a stream, over
// declared, the specification read from source, and returns declared.
//
// A declared node the stream reported takes its verdict, duration, output
// and benchmark result, and keeps what the source says of it: kind, label,
// vocabulary, focus, exclusion, completeness and position. One the stream
// did not report has no verdict and is NotRun, unless the X_ prefix excludes
// it or an ancestor, or the run skipped an ancestor (a suite its guard kept
// from running): those read as skipped. Nodes and
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
//
// skipped says a declared node the stream did not report was skipped: its
// ancestor is excluded by the X_ prefix, or the run skipped it.
func overlayNodes(declared, observed []*Node, skipped bool) []*Node {
	// Twins run internal first; the n-th match below depends on that order.
	declared = internalFirst(declared)
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
			notRun([]*Node{d}, skipped)
			out = append(out, d)
			continue
		}
		used[match] = true
		o := observed[match]
		d.Status, d.Duration, d.Start, d.End, d.Paused = o.Status, o.Duration, o.Start, o.End, o.Paused
		d.Output = o.Output
		d.External, d.Variant, d.Reported = d.External || o.External, o.Variant, o.Reported
		d.Iterations, d.NsPerOp, d.BytesPerOp, d.AllocsPerOp = o.Iterations, o.NsPerOp, o.BytesPerOp, o.AllocsPerOp
		d.Metrics, d.Procs = o.Metrics, o.Procs
		d.Children = overlayNodes(d.Children, o.Children, skipped || d.Excluded || o.Status == StatusSkip)
		out = append(out, d)
	}
	for i, o := range observed {
		if !used[i] {
			out = append(out, o)
		}
	}
	return out
}

// internalFirst returns nodes with same-named ones reordered internal before
// external, each keeping the slots its name held.
func internalFirst(nodes []*Node) []*Node {
	slots := map[string][]int{}
	for i, n := range nodes {
		slots[n.Name] = append(slots[n.Name], i)
	}
	out := append([]*Node(nil), nodes...)
	for _, idx := range slots {
		group := make([]*Node, len(idx))
		for k, i := range idx {
			group[k] = nodes[i]
		}
		sort.SliceStable(group, func(a, b int) bool { return !group[a].External && group[b].External })
		for k, i := range idx {
			out[i] = group[k]
		}
	}
	return out
}

// notRun marks declared nodes the stream did not report: NotRun, or skipped
// where the X_ prefix excludes them or the run skipped an ancestor.
func notRun(nodes []*Node, skippedAbove bool) {
	for _, n := range nodes {
		skipped := skippedAbove || n.Excluded
		if skipped {
			n.Status = StatusSkip
		} else {
			n.NotRun = true
		}
		notRun(n.Children, skipped)
	}
}
