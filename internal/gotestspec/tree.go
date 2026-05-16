package gotestspec

import (
	"sort"
	"strings"
	"time"
)

type Status int

const (
	StatusNone Status = iota
	StatusPass
	StatusFail
	StatusSkip
)

type NodeKind int

const (
	KindUnknown NodeKind = iota
	KindFixture
	KindSuite
	KindMethod
	KindBlock
	KindTest
)

type Node struct {
	Name      string
	Display   string
	Kind      NodeKind
	Status    Status
	Duration  time.Duration
	Output    []string
	Children  []*Node
	Focused   bool
	Excluded  bool
	Variant   int
	duplicate bool
}

type Package struct {
	Path     string
	Status   Status
	Duration time.Duration
	Nodes    []*Node
}

type Stats struct {
	Suites    int
	Behaviors int
	Tests     int
	Passed    int
	Failed    int
	Skipped   int
}

func (s Stats) Total() int {
	return s.Passed + s.Failed + s.Skipped
}

func BuildTree(events []TestEvent) []*Package {
	pkgs := map[string]*Package{}
	nodes := map[string]map[string]*Node{}
	// Track top-level test run counts per package to detect ptest/pxtest duplicates.
	topRunCount := map[string]map[string]int{}

	for _, ev := range events {
		pkg := pkgs[ev.Package]
		if pkg == nil {
			pkg = &Package{Path: ev.Package}
			pkgs[ev.Package] = pkg
			nodes[ev.Package] = map[string]*Node{}
			topRunCount[ev.Package] = map[string]int{}
		}

		if ev.Test == "" {
			if ev.Action == ActionPass || ev.Action == ActionFail {
				pkg.Status = statusFrom(ev.Action)
				pkg.Duration = elapsed(ev.Elapsed)
			}
			continue
		}

		segments := splitTestPath(ev.Test)
		nmap := nodes[ev.Package]

		// Detect duplicate top-level run (ptest/pxtest same-name suite).
		if ev.Action == ActionRun && len(segments) == 1 {
			topRunCount[ev.Package][segments[0]]++
			if topRunCount[ev.Package][segments[0]] > 1 {
				// Create a duplicate node; children with #NN suffixes will attach here.
				dup := &Node{Name: segments[0], duplicate: true}
				dupPath := segments[0] + "\x00dup"
				nmap[dupPath] = dup
				pkg.Nodes = append(pkg.Nodes, dup)
				continue
			}
		}

		// Resolve parent for children with #NN suffix (belongs to duplicate node).
		resolvedSegments := segments
		if len(segments) > 1 {
			resolvedSegments = resolveDuplicateSegments(segments, nmap)
		}

		for i := range resolvedSegments {
			path := strings.Join(resolvedSegments[:i+1], "/")
			if nmap[path] != nil {
				continue
			}
			name := resolvedSegments[i]
			// Strip #NN suffix from display for children of duplicate runs.
			cleanName := stripDuplicateSuffix(name)
			n := &Node{Name: cleanName}
			nmap[path] = n
			if i == 0 {
				pkg.Nodes = append(pkg.Nodes, n)
			} else {
				parent := nmap[strings.Join(resolvedSegments[:i], "/")]
				parent.Children = append(parent.Children, n)
			}
		}

		node := nmap[strings.Join(resolvedSegments, "/")]
		// Route top-level pass/fail/output to duplicate if original already resolved.
		if len(resolvedSegments) == 1 && node.Status != StatusNone {
			dupPath := resolvedSegments[0] + "\x00dup"
			if dup := nmap[dupPath]; dup != nil {
				node = dup
			}
		}
		switch ev.Action {
		case ActionOutput:
			node.Output = append(node.Output, ev.Output)
		case ActionPass, ActionFail, ActionSkip:
			node.Status = statusFrom(ev.Action)
			node.Duration = elapsed(ev.Elapsed)
		}
	}

	for _, pkg := range pkgs {
		seen := map[string]int{}
		for _, n := range pkg.Nodes {
			classify(n, true)
			seen[n.Name]++
			if n.duplicate {
				n.Variant = seen[n.Name]
			}
		}
	}

	result := make([]*Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if len(pkg.Nodes) == 0 {
			continue
		}
		result = append(result, pkg)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result
}

// resolveDuplicateSegments checks if a child path belongs to a duplicate
// top-level node (parent has #NN suffix pattern) and remaps it.
func resolveDuplicateSegments(segments []string, nmap map[string]*Node) []string {
	topName := segments[0]
	dupPath := topName + "\x00dup"
	if nmap[dupPath] == nil {
		return segments
	}

	// Check if any child segment has the #NN suffix indicating it belongs to
	// the duplicate run.
	for _, seg := range segments[1:] {
		if hasDuplicateSuffix(seg) {
			out := make([]string, len(segments))
			out[0] = topName + "\x00dup"
			for i := 1; i < len(segments); i++ {
				out[i] = stripDuplicateSuffix(segments[i])
			}
			return out
		}
	}
	return segments
}

func hasDuplicateSuffix(s string) bool {
	idx := strings.LastIndex(s, "#")
	if idx < 0 {
		return false
	}
	suffix := s[idx+1:]
	if len(suffix) == 0 {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func stripDuplicateSuffix(s string) string {
	idx := strings.LastIndex(s, "#")
	if idx < 0 {
		return s
	}
	suffix := s[idx+1:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return s
		}
	}
	return s[:idx]
}

func CollectStats(packages []*Package) Stats {
	var s Stats
	for _, pkg := range packages {
		for _, n := range pkg.Nodes {
			collectStats(n, &s, n.Kind == KindTest)
		}
	}
	return s
}

func collectStats(n *Node, s *Stats, inStdlib bool) {
	if n.Kind == KindSuite {
		s.Suites++
	}
	if len(n.Children) == 0 {
		if inStdlib {
			s.Tests++
		} else {
			s.Behaviors++
		}
		switch n.Status {
		case StatusPass:
			s.Passed++
		case StatusFail:
			s.Failed++
		case StatusSkip:
			s.Skipped++
		}
	}
	for _, c := range n.Children {
		collectStats(c, s, inStdlib)
	}
}

func classify(n *Node, topLevel bool) {
	name := n.Name

	if topLevel {
		raw := strings.TrimPrefix(name, "Test")

		if strings.HasPrefix(raw, "F_") {
			n.Focused = true
			raw = raw[2:]
		} else if strings.HasPrefix(raw, "X_") {
			n.Excluded = true
			raw = raw[2:]
		}

		if strings.HasPrefix(raw, "_") && strings.HasSuffix(raw, "Fixture") {
			n.Kind = KindFixture
			n.Display = strings.TrimSuffix(strings.TrimPrefix(raw, "_"), "Fixture")
		} else if strings.HasSuffix(raw, "TestSuite") {
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(raw, "TestSuite")
		} else {
			n.Kind = KindTest
			n.Display = strings.TrimPrefix(raw, "_")
		}
	} else {
		if strings.HasPrefix(name, "F_") {
			n.Focused = true
			name = name[2:]
		} else if strings.HasPrefix(name, "X_") {
			n.Excluded = true
			name = name[2:]
		}

		if strings.HasSuffix(name, "Fixture") && !strings.HasSuffix(name, "TestSuite") {
			n.Kind = KindFixture
			n.Display = strings.TrimSuffix(name, "Fixture")
		} else if strings.HasSuffix(name, "TestSuite") {
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(name, "TestSuite")
		} else if strings.HasPrefix(name, "Test") {
			n.Kind = KindMethod
			n.Display = strings.TrimPrefix(name, "Test")
		} else {
			n.Kind = KindBlock
			n.Display = strings.ReplaceAll(name, "_", " ")
		}
	}

	if n.Display == "" {
		n.Display = n.Name
	}

	for _, c := range n.Children {
		classify(c, false)
	}
}

// splitTestPath splits a go test -json Test field into subtest segments.
// Go uses "/" as the subtest separator, but description strings may contain
// "/" too (e.g. "https:// URI"). We treat consecutive slashes as literal
// characters within a segment rather than multiple separators.
func splitTestPath(path string) []string {
	var segments []string
	var cur strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == '/' && (i+1 >= len(path) || path[i+1] != '/') &&
			(i == 0 || path[i-1] != '/') {
			segments = append(segments, cur.String())
			cur.Reset()
		} else {
			cur.WriteByte(path[i])
		}
	}
	if cur.Len() > 0 {
		segments = append(segments, cur.String())
	}
	return segments
}

func statusFrom(a Action) Status {
	switch a {
	case ActionPass:
		return StatusPass
	case ActionFail:
		return StatusFail
	case ActionSkip:
		return StatusSkip
	}
	return StatusNone
}

func elapsed(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
