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
)

type Node struct {
	Name     string
	Display  string
	Kind     NodeKind
	Status   Status
	Duration time.Duration
	Output   []string
	Children []*Node
	Focused  bool
	Excluded bool
}

type Package struct {
	Path     string
	Status   Status
	Duration time.Duration
	Nodes    []*Node
}

type Stats struct {
	Suites  int
	Passed  int
	Failed  int
	Skipped int
}

func (s Stats) Total() int {
	return s.Passed + s.Failed + s.Skipped
}

func BuildTree(events []TestEvent) []*Package {
	pkgs := map[string]*Package{}
	nodes := map[string]map[string]*Node{}

	for _, ev := range events {
		pkg := pkgs[ev.Package]
		if pkg == nil {
			pkg = &Package{Path: ev.Package}
			pkgs[ev.Package] = pkg
			nodes[ev.Package] = map[string]*Node{}
		}

		if ev.Test == "" {
			if ev.Action == ActionPass || ev.Action == ActionFail {
				pkg.Status = statusFrom(ev.Action)
				pkg.Duration = elapsed(ev.Elapsed)
			}
			continue
		}

		segments := strings.Split(ev.Test, "/")
		nmap := nodes[ev.Package]

		for i := range segments {
			path := strings.Join(segments[:i+1], "/")
			if nmap[path] != nil {
				continue
			}
			n := &Node{Name: segments[i]}
			nmap[path] = n
			if i == 0 {
				pkg.Nodes = append(pkg.Nodes, n)
			} else {
				parent := nmap[strings.Join(segments[:i], "/")]
				parent.Children = append(parent.Children, n)
			}
		}

		node := nmap[ev.Test]
		switch ev.Action {
		case ActionOutput:
			node.Output = append(node.Output, ev.Output)
		case ActionPass, ActionFail, ActionSkip:
			node.Status = statusFrom(ev.Action)
			node.Duration = elapsed(ev.Elapsed)
		}
	}

	for _, pkg := range pkgs {
		for _, n := range pkg.Nodes {
			classify(n, true)
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

func CollectStats(packages []*Package) Stats {
	var s Stats
	for _, pkg := range packages {
		for _, n := range pkg.Nodes {
			collectStats(n, &s)
		}
	}
	return s
}

func collectStats(n *Node, s *Stats) {
	if n.Kind == KindSuite {
		s.Suites++
	}
	if len(n.Children) == 0 {
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
		collectStats(c, s)
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
		} else if strings.HasSuffix(raw, "TestSuiteParallel") {
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(raw, "TestSuiteParallel")
		} else if strings.HasSuffix(raw, "TestSuite") {
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(raw, "TestSuite")
		} else {
			n.Kind = KindMethod
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
		} else if strings.HasSuffix(name, "TestSuiteParallel") {
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(name, "TestSuiteParallel")
		} else if strings.HasSuffix(name, "TestSuite") {
			n.Kind = KindSuite
			n.Display = strings.TrimSuffix(name, "TestSuite")
		} else if strings.HasPrefix(name, "TestParallel") {
			n.Kind = KindMethod
			n.Display = strings.TrimPrefix(name, "TestParallel")
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
