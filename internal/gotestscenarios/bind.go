package gotestscenarios

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// Options say what a binding reads besides the specs and the tree.
type Options struct {
	// ModuleRoot is the directory paths are made relative to.
	ModuleRoot string
	// Evidence is EvidenceNone when the tree holds no run's verdicts.
	Evidence string
	// Run is the run the verdicts come from, when its stream names it.
	Run *gotestspec.RunInfo
	// RunPackages are the packages the run's stream reports.
	RunPackages []string
	// Unstated lists the Its under bound Whens no clause states.
	Unstated bool
	Patterns []string
	// Incomplete are the methods whose behaviors source cannot list.
	Incomplete []string
	// Unreadable are the packages that could not be read.
	Unreadable []string
}

// it is one It declaration directly under a When: every row and run of it.
type it struct {
	key       string
	whenKey   string
	when      Place
	label     string
	chain     []string // the When labels around it, outermost first
	place     Place
	instances []*gotestspec.Node
}

// Bind joins each scenario with the Its of the tree whose labels state it.
// The tree is the source's, with a run's verdicts laid over it when
// opt.Evidence says so.
func Bind(specs []Spec, tree []*gotestspec.Package, opt *Options) Document {
	its, byWhen := collect(tree, opt.ModuleRoot)
	doc := newDocument(opt)
	for i := range specs {
		doc.Specs = append(doc.Specs, specs[i].File)
		for _, r := range specs[i].Requirements {
			for k := range r.Scenarios {
				doc.Scenarios = append(doc.Scenarios, bindScenario(&specs[i], &r, &r.Scenarios[k], byWhen))
			}
		}
	}
	claimed := markClaims(doc.Scenarios)
	evidence := opt.Evidence != EvidenceNone
	for i := range doc.Scenarios {
		judge(&doc.Scenarios[i], evidence)
	}
	if opt.Unstated {
		doc.Unstated = unstated(doc.Scenarios, its, claimed)
	}
	sort.SliceStable(doc.Scenarios, func(a, b int) bool {
		x, y := &doc.Scenarios[a], &doc.Scenarios[b]
		if x.File != y.File {
			return x.File < y.File
		}
		return x.Line < y.Line
	})
	sort.Strings(doc.Specs)
	doc.summarize()
	return doc
}

// collect indexes every It directly under a When by its innermost When's
// label. Excluded nodes and nodes the source does not declare are none.
func collect(tree []*gotestspec.Package, root string) (map[string]*it, map[string][]*it) {
	its := map[string]*it{}
	byWhen := map[string][]*it{}
	for _, pkg := range tree {
		for _, suite := range pkg.Nodes {
			var visit func(n *gotestspec.Node, ancestors []*gotestspec.Node)
			visit = func(n *gotestspec.Node, ancestors []*gotestspec.Node) {
				if n.Excluded {
					return
				}
				if parent := last(ancestors); n.Vocab == gotestspec.VocabIt && parent != nil && parent.Vocab == gotestspec.VocabWhen {
					file, whenFile := relFile(root, pkg.Dir, n.File), relFile(root, pkg.Dir, parent.File)
					key := fmt.Sprintf("%s\x00%s:%d\x00%s", pkg.Path, file, n.Line, n.SourceLabel)
					x := its[key]
					if x == nil {
						path := labels(suite, ancestors, n)
						x = &it{
							key:     key,
							whenKey: fmt.Sprintf("%s\x00%s:%d", pkg.Path, whenFile, parent.Line),
							when:    Place{Package: pkg.Path, File: whenFile, Line: parent.Line, Path: path[:len(path)-1]},
							label:   Normalize(n.SourceLabel),
							chain:   whenChain(ancestors),
							place:   Place{Package: pkg.Path, File: file, Line: n.Line, Path: path},
						}
						its[key] = x
						inner := x.chain[len(x.chain)-1]
						byWhen[inner] = append(byWhen[inner], x)
					}
					x.instances = append(x.instances, n)
				}
				for _, c := range n.Children {
					visit(c, append(ancestors[:len(ancestors):len(ancestors)], n))
				}
			}
			visit(suite, nil)
		}
	}
	return its, byWhen
}

func last(nodes []*gotestspec.Node) *gotestspec.Node {
	if len(nodes) == 0 {
		return nil
	}
	return nodes[len(nodes)-1]
}

// whenChain is the labels of the Whens directly around an It, outermost
// first, the run of Whens ending at its parent.
func whenChain(ancestors []*gotestspec.Node) []string {
	var chain []string
	for i := len(ancestors) - 1; i >= 0 && ancestors[i].Vocab == gotestspec.VocabWhen; i-- {
		chain = append([]string{Normalize(ancestors[i].SourceLabel)}, chain...)
	}
	return chain
}

// labels is an It's path: suite, method, then each label as written.
func labels(suite *gotestspec.Node, ancestors []*gotestspec.Node, n *gotestspec.Node) []string {
	var out []string
	for _, a := range append(ancestors, n) {
		switch {
		case a == suite || a.Vocab == gotestspec.VocabNone:
			out = append(out, a.Name)
		default:
			out = append(out, a.SourceLabel)
		}
	}
	return out
}

func relFile(root, dir, file string) string {
	abs := filepath.Join(dir, file)
	if rel, err := filepath.Rel(root, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}

// bindScenario finds the Its each of the scenario's outcomes states under a
// When whose chain ends with its conditions.
func bindScenario(spec *Spec, r *Requirement, sc *SpecScenario, byWhen map[string][]*it) Scenario {
	out := Scenario{
		Capability:  spec.Capability,
		File:        spec.File,
		Requirement: r.Name,
		Title:       sc.Title,
		Line:        sc.Line,
		Problem:     sc.Problem,
		Conditions:  []Clause{},
		Clauses:     []Clause{},
		Whens:       []Place{},
	}
	conds := make([]string, len(sc.Conditions))
	for i, c := range sc.Conditions {
		conds[i] = Normalize(c.Text)
		out.Conditions = append(out.Conditions, Clause{Keyword: c.Keyword, Text: c.Text, Line: c.Line})
	}
	var candidates []*it
	if sc.Problem == "" && len(conds) > 0 {
		for _, x := range byWhen[conds[len(conds)-1]] {
			if endsWith(x.chain, conds) {
				candidates = append(candidates, x)
			}
		}
	}
	whens := map[string]bool{}
	for _, x := range candidates {
		if !whens[x.whenKey] {
			whens[x.whenKey] = true
			out.Whens = append(out.Whens, x.when)
		}
	}
	for _, o := range sc.Outcomes {
		c := Clause{Keyword: o.Keyword, Text: o.Text, Line: o.Line, Its: []Place{}}
		want := Normalize(o.Text)
		for _, x := range candidates {
			if x.label == want {
				c.its = append(c.its, x)
				c.Its = append(c.Its, x.place)
			}
		}
		out.Clauses = append(out.Clauses, c)
	}
	return out
}

func endsWith(chain, conds []string) bool {
	if len(chain) < len(conds) {
		return false
	}
	off := len(chain) - len(conds)
	for i, c := range conds {
		if chain[off+i] != c {
			return false
		}
	}
	return true
}

// markClaims finds the Its the clauses of more than one scenario bind, and
// returns every It a clause binds.
func markClaims(scenarios []Scenario) map[string]bool {
	owners := map[string]map[int]bool{}
	for i := range scenarios {
		for _, c := range scenarios[i].Clauses {
			for _, x := range c.its {
				if owners[x.key] == nil {
					owners[x.key] = map[int]bool{}
				}
				owners[x.key][i] = true
			}
		}
	}
	claimed := map[string]bool{}
	for key, by := range owners {
		claimed[key] = true
		if len(by) < 2 {
			continue
		}
		for i := range by {
			for k := range scenarios[i].Clauses {
				c := &scenarios[i].Clauses[k]
				for _, x := range c.its {
					if x.key == key {
						c.shared = true
					}
				}
			}
		}
	}
	return claimed
}

// judge gives each clause and the scenario a status.
func judge(sc *Scenario, evidence bool) {
	worst := ""
	for k := range sc.Clauses {
		c := &sc.Clauses[k]
		switch {
		case sc.Problem != "" || len(sc.Whens) == 0:
			c.Status = StatusUnbound
		case len(c.its) == 0:
			c.Status = StatusUnbound
		case len(c.its) > 1 || c.shared:
			c.Status = StatusAmbiguous
		case !evidence:
			c.Status = StatusBound
		default:
			c.Status = verdict(c.its[0].instances)
			c.Its[0].Status = c.Status
			c.Its[0].Assertions = assertions(c.its[0].instances)
		}
		worst = worse(worst, c.Status)
	}
	if worst == "" {
		worst = StatusUnbound
	}
	sc.Status = worst
}

// verdict is the worst of an It's rows and runs.
func verdict(instances []*gotestspec.Node) string {
	out := StatusVerified
	for _, n := range instances {
		var s string
		switch {
		case n.Status == gotestspec.StatusFail:
			s = StatusFailing
		case n.NotRun || n.Status == gotestspec.StatusSkip || n.Status == gotestspec.StatusNone:
			s = StatusNotRun
		case n.Vacuous():
			s = StatusVacuous
		default:
			s = StatusVerified
		}
		out = worse(out, s)
	}
	return out
}

// assertions is the fewest assertions a run of the It made, nil when no
// run reported a count.
func assertions(instances []*gotestspec.Node) *int {
	var out *int
	for _, n := range instances {
		if n.Assertions != nil && (out == nil || *n.Assertions < *out) {
			v := *n.Assertions
			out = &v
		}
	}
	return out
}

// precedence orders statuses from the one that rules a scenario.
var precedence = []string{StatusAmbiguous, StatusFailing, StatusUnbound, StatusNotRun, StatusVacuous, StatusVerified, StatusBound}

func worse(a, b string) string {
	if a == "" {
		return b
	}
	for _, s := range precedence {
		if a == s || b == s {
			return s
		}
	}
	return a
}

// unstated lists the Its under a When some clause binds into that no
// clause binds.
func unstated(scenarios []Scenario, its map[string]*it, claimed map[string]bool) []Place {
	bound := map[string]bool{}
	for i := range scenarios {
		for _, c := range scenarios[i].Clauses {
			for _, x := range c.its {
				bound[x.whenKey] = true
			}
		}
	}
	out := []Place{}
	for key, x := range its {
		if bound[x.whenKey] && !claimed[key] {
			out = append(out, x.place)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Package != out[b].Package {
			return out[a].Package < out[b].Package
		}
		if out[a].File != out[b].File {
			return out[a].File < out[b].File
		}
		return out[a].Line < out[b].Line
	})
	return out
}
