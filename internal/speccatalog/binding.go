package speccatalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ConformanceDir holds one conformance package per capability.
const ConformanceDir = "tests/conformance"

// Clause is an It label under the innermost When label above it, both as
// written in the source, in the conformance package of its capability.
type Clause struct {
	Capability string
	When       string
	It         string
}

// ClausesOf lists the catalog's clauses as label pairs, in catalog order.
// The capability is the package's directory under tests/conformance.
func ClausesOf(js []byte) ([]Clause, error) {
	var doc struct {
		Packages []struct {
			Path  string `json:"path"`
			Nodes []node `json:"nodes"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, fmt.Errorf("catalog json: %w", err)
	}
	var out []Clause
	for _, p := range doc.Packages {
		_, capability, _ := strings.Cut(p.Path, "/"+ConformanceDir+"/")
		for _, n := range p.Nodes {
			walk(&n, nil, func(when, it *node) {
				out = append(out, Clause{Capability: capability, When: whenLabel(when.Display, when.Name), It: strings.TrimSpace(it.Display)})
			})
		}
	}
	return out, nil
}

// whenLabel undoes the renderer's "when " connective. The subtest name keeps
// the label's own first word, so a label that opens with "when" keeps it.
func whenLabel(display, name string) string {
	label := display
	if !strings.HasPrefix(name, "when_") {
		label = strings.TrimPrefix(label, "when ")
	}
	return strings.TrimSpace(label)
}

// Binding names what fails to bind. Unverified holds scenario text no clause
// states; Unspecified holds clauses no scenario states.
type Binding struct {
	Unverified  []string
	Unspecified []string
}

// Bind joins the specs with the clauses per capability: each scenario's
// WHEN and THEN/AND bullets must equal the When label and It labels of a
// clause, and each clause must be in a scenario.
func Bind(specs []Spec, clauses []Clause) Binding {
	its := map[string]map[string]map[string]bool{}
	for _, c := range clauses {
		if its[c.Capability] == nil {
			its[c.Capability] = map[string]map[string]bool{}
		}
		if its[c.Capability][c.When] == nil {
			its[c.Capability][c.When] = map[string]bool{}
		}
		its[c.Capability][c.When][c.It] = true
	}

	var b Binding
	stated := map[Clause]bool{}
	hasSpec := map[string]string{}
	for _, s := range specs {
		hasSpec[s.Capability] = s.File
		pkg := ConformanceDir + "/" + s.Capability
		whens, ok := its[s.Capability]
		if !ok {
			b.Unverified = append(b.Unverified, fmt.Sprintf("%s: %s declares no clauses", s.File, pkg))
			continue
		}
		seen := map[string]int{}
		for _, r := range s.Requirements {
			for _, sc := range r.Scenarios {
				if len(sc.Whens) != 1 {
					continue
				}
				when := sc.Whens[0]
				at := fmt.Sprintf("%s:%d: scenario %q", s.File, sc.Line, sc.Title)
				if first, dup := seen[when]; dup {
					b.Unverified = append(b.Unverified, fmt.Sprintf("%s: WHEN %q is already the WHEN of the scenario at line %d", at, when, first))
					continue
				}
				seen[when] = sc.Line
				known, ok := whens[when]
				if !ok {
					b.Unverified = append(b.Unverified, fmt.Sprintf("%s: WHEN %q is no When label in %s", at, when, pkg))
					continue
				}
				for _, then := range sc.Thens {
					if !known[then] {
						b.Unverified = append(b.Unverified, fmt.Sprintf("%s: THEN %q is no It under When %q in %s", at, then, when, pkg))
						continue
					}
					stated[Clause{Capability: s.Capability, When: when, It: then}] = true
				}
			}
		}
	}

	reported := map[Clause]bool{}
	for _, c := range sortedClauses(clauses) {
		if stated[c] || reported[c] {
			continue
		}
		reported[c] = true
		pkg := ConformanceDir + "/" + c.Capability
		if file, ok := hasSpec[c.Capability]; ok {
			b.Unspecified = append(b.Unspecified, fmt.Sprintf("%s: When %q → It %q is in no scenario of %s", pkg, c.When, c.It, file))
		} else {
			b.Unspecified = append(b.Unspecified, fmt.Sprintf("%s: When %q → It %q: %s does not exist", pkg, c.When, c.It, specFile(c.Capability)))
		}
	}
	return b
}

// sortedClauses orders clauses by capability, keeping catalog order within one.
func sortedClauses(clauses []Clause) []Clause {
	out := append([]Clause(nil), clauses...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Capability < out[j].Capability })
	return out
}

// node is the part of a catalog node ClausesOf reads.
type node struct {
	Display  string `json:"display"`
	Name     string `json:"name"`
	Vocab    string `json:"vocab"`
	Excluded bool   `json:"excluded"`
	Children []node `json:"children"`
}

// walk calls fn for every It below a When, with the innermost When above it.
// Nothing under an excluded node is visited.
func walk(n, when *node, fn func(when, it *node)) {
	if n.Excluded {
		return
	}
	switch n.Vocab {
	case "when":
		when = n
	case "it":
		if when != nil {
			fn(when, n)
		}
	}
	for i := range n.Children {
		walk(&n.Children[i], when, fn)
	}
}
