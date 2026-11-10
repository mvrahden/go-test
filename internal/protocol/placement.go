package protocol

import "strings"

// Placement is where a go test name sits.
type Placement struct {
	Parent   string // full name of the parent; "" for a top-level test
	Own      string // own level as reported, testing's #NN included
	Written  string // Own without testing's repeat suffix
	Repeat   int    // the NN testing appended; 0 when none
	Instance int    // 1-based run of the top-level name in the process
	Guessed  bool   // placed by the fallback split, not by the rule
}

// Placer places the test names of one process's go test stream. A name's
// slashes cannot say where its parent ends, since labels may hold slashes;
// the order of the events can: a subtest starts while its parent runs.
type Placer struct {
	runs      map[string]int // top-level name -> instances seen
	openInst  map[string]int // top-level name -> its open instance; 0 when none
	instances map[instance]map[string]*placed
	used      map[string]bool // full names testing has handed out, reported or written
}

type instance struct {
	root string
	n    int
}

type placed struct {
	pl       Placement
	open     bool
	paused   bool
	children []string
}

// NewPlacer returns a Placer for one process's stream; it is not safe for
// concurrent use.
func NewPlacer() *Placer {
	return &Placer{
		runs:      map[string]int{},
		openInst:  map[string]int{},
		instances: map[instance]map[string]*placed{},
		used:      map[string]bool{},
	}
}

// Place reads one event in stream order and returns its test's placement.
// action is go test's Action; test its Test (non-empty).
func (p *Placer) Place(action, test string) Placement {
	root, _, sub := strings.Cut(test, "/")
	if action == "run" && !sub {
		p.runs[root]++
		n := p.runs[root]
		p.openInst[root] = n
		pl := Placement{Own: test, Written: test, Instance: n}
		p.nodes(instance{root, n})[test] = &placed{pl: pl, open: true}
		p.used[test] = true
		return pl
	}
	inst := instance{root, p.currentInstance(root)}
	nodes := p.nodes(inst)
	if action != "run" {
		node, ok := nodes[test]
		if !ok {
			return fallback(test, inst.n)
		}
		switch action {
		case "pause":
			node.paused = true
		case "cont":
			node.paused = false
		case "pass", "fail", "skip":
			p.close(nodes, test)
			if !sub && p.openInst[root] == inst.n {
				p.openInst[root] = 0
			}
		}
		return node.pl
	}
	pl, ok := p.underOpenParent(nodes, test, inst.n)
	if !ok {
		pl = fallback(test, inst.n)
	}
	nodes[test] = &placed{pl: pl, open: true}
	if parent, ok := nodes[pl.Parent]; ok {
		parent.children = append(parent.children, test)
	}
	p.used[test] = true
	p.used[joinName(pl.Parent, pl.Written)] = true
	return pl
}

// currentInstance is the open instance of a top-level name, else its latest.
func (p *Placer) currentInstance(root string) int {
	if n := p.openInst[root]; n > 0 {
		return n
	}
	return max(p.runs[root], 1)
}

func (p *Placer) nodes(inst instance) map[string]*placed {
	m, ok := p.instances[inst]
	if !ok {
		m = map[string]*placed{}
		p.instances[inst] = m
	}
	return m
}

// underOpenParent places test under the deepest open, unpaused test whose
// name plus a slash prefixes it.
func (p *Placer) underOpenParent(nodes map[string]*placed, test string, n int) (Placement, bool) {
	for i := strings.LastIndexByte(test, '/'); i > 0; i = strings.LastIndexByte(test[:i], '/') {
		parent := test[:i]
		node, ok := nodes[parent]
		if !ok || !node.open || node.paused {
			continue
		}
		pl := Placement{Parent: parent, Own: test[i+1:], Written: test[i+1:], Instance: n}
		if prefix, nn, ok := repeatSuffix(pl.Own); ok && p.used[joinName(parent, prefix)] {
			pl.Written, pl.Repeat = prefix, nn
		}
		return pl, true
	}
	return Placement{}, false
}

// close ends a test and every open test beneath it: sub-benchmarks never
// report a verdict of their own.
func (p *Placer) close(nodes map[string]*placed, test string) {
	node := nodes[test]
	if node == nil || !node.open {
		return
	}
	node.open = false
	for _, c := range node.children {
		p.close(nodes, c)
	}
}

// fallback splits a name at the last separator SplitTestPath finds.
func fallback(test string, n int) Placement {
	pl := Placement{Own: test, Written: test, Instance: n, Guessed: true}
	segs := SplitTestPath(test)
	if len(segs) < 2 {
		return pl
	}
	i := len(strings.Join(segs[:len(segs)-1], "/"))
	pl.Parent, pl.Own, pl.Written = test[:i], test[i+1:], test[i+1:]
	return pl
}

// repeatSuffix splits the #NN testing appends to a repeated name. Like
// testing's parseSubtestNumber, it takes two or more digits without a
// leading zero beyond the second.
func repeatSuffix(own string) (prefix string, nn int, ok bool) {
	i := strings.LastIndexByte(own, '#')
	if i < 0 {
		return "", 0, false
	}
	digits := own[i+1:]
	if len(digits) < 2 || len(digits) > 9 || (len(digits) > 2 && digits[0] == '0') {
		return "", 0, false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return "", 0, false
		}
		nn = nn*10 + int(c-'0')
	}
	return own[:i], nn, true
}

func joinName(parent, own string) string {
	if parent == "" {
		return own
	}
	return parent + "/" + own
}
