package speccatalog

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestscenarios"
)

// ConformanceDir holds one conformance package per capability.
const ConformanceDir = "tests/conformance"

// NotBound names each scenario the product does not call bound, as its text
// report lists it.
func NotBound(doc *gotestscenarios.Document) []string {
	var out []string
	for i := range doc.Scenarios {
		sc := &doc.Scenarios[i]
		if sc.Status != gotestscenarios.StatusBound {
			out = append(out, fmt.Sprintf("%s %s:%d %s / %s", sc.Status, sc.File, sc.Line, sc.Requirement, sc.Title))
		}
	}
	return out
}

// Report is the product's text report of doc, every class at fault.
func Report(doc *gotestscenarios.Document) string {
	var b bytes.Buffer
	gotestscenarios.RenderText(&b, doc, gotestscenarios.Verdict{ExitCode: 1, FailOn: gotestscenarios.Classes})
	return b.String()
}

// OutsideCapability names each It a scenario binds outside the conformance
// package of its capability: specs/<capability>/spec.md is verified by
// tests/conformance/<capability> alone.
func OutsideCapability(doc *gotestscenarios.Document) []string {
	var out []string
	for i := range doc.Scenarios {
		sc := &doc.Scenarios[i]
		want := ConformanceDir + "/" + sc.Capability
		for _, c := range sc.Clauses {
			for _, p := range c.Its {
				if !strings.HasSuffix(p.Package, "/"+want) {
					out = append(out, fmt.Sprintf("%s:%d: scenario %q: %s %q binds the It at %s:%d, outside %s", sc.File, sc.Line, sc.Title, c.Keyword, c.Text, p.File, p.Line, want))
				}
			}
		}
	}
	return out
}

// Inexact names each bound clause whose It, or whose When, is not labeled
// with the scenario's text exactly as written: the product binds labels
// that differ in case, backticks or a trailing period, this repository
// keeps one spelling.
func Inexact(doc *gotestscenarios.Document) []string {
	var out []string
	for i := range doc.Scenarios {
		sc := &doc.Scenarios[i]
		when := ""
		for _, c := range sc.Conditions {
			if c.Keyword == gotestscenarios.When {
				when = c.Text
			}
		}
		for _, c := range sc.Clauses {
			for _, p := range c.Its {
				at := fmt.Sprintf("%s:%d: scenario %q", sc.File, sc.Line, sc.Title)
				if n := len(p.Path); n >= 2 && p.Path[n-2] != when {
					out = append(out, fmt.Sprintf("%s: WHEN %q is labeled %q at %s:%d", at, when, p.Path[n-2], p.File, p.Line))
				}
				if n := len(p.Path); n >= 1 && p.Path[n-1] != c.Text {
					out = append(out, fmt.Sprintf("%s: %s %q is labeled %q at %s:%d", at, c.Keyword, c.Text, p.Path[n-1], p.File, p.Line))
				}
			}
		}
	}
	return out
}

// UnboundWhens names each conformance When holding an It directly that no
// scenario's WHEN binds, with those Its: a behavior no spec states. The
// product's unstated list covers the Its under a When a scenario binds.
// Excluded nodes are skipped.
func UnboundWhens(r *Reading) []string {
	bound := map[string]bool{}
	for i := range r.Scenarios.Scenarios {
		for _, w := range r.Scenarios.Scenarios[i].Whens {
			bound[placeKey(w.Package, path.Base(w.File), w.Line)] = true
		}
	}
	specs := map[string]bool{}
	for _, f := range r.Scenarios.Specs {
		specs[f] = true
	}
	var out []string
	for i := range r.Packages {
		p := &r.Packages[i]
		capability := p.capability()
		pkg := ConformanceDir + "/" + capability
		file := specFile(capability)
		missing := ""
		if !specs[file] {
			missing = ", which does not exist"
		}
		visitWhens(p, func(suite, method, when *node, its []*node) {
			if bound[placeKey(p.Path, when.File, when.Line)] {
				return
			}
			for _, it := range its {
				out = append(out, fmt.Sprintf("%s: %s.%s: When %q → It %q (%s:%d) is in no scenario of %s%s",
					pkg, suite.Display, method.Display, whenLabel(when.Display, when.Name), strings.TrimSpace(it.Display), when.File, it.Line, file, missing))
			}
		})
	}
	return out
}

func placeKey(pkg, file string, line int) string {
	return fmt.Sprintf("%s\x00%s:%d", pkg, file, line)
}

// visitWhens calls fn for every When with an It directly under it, with
// those Its. Nothing under an excluded node is visited.
func visitWhens(p *Package, fn func(suite, method, when *node, its []*node)) {
	for s := range p.Nodes {
		suite := &p.Nodes[s]
		if suite.Excluded {
			continue
		}
		for m := range suite.Children {
			method := &suite.Children[m]
			var visit func(n *node)
			visit = func(n *node) {
				if n.Excluded {
					return
				}
				if n.Vocab == "when" {
					var its []*node
					for k := range n.Children {
						if c := &n.Children[k]; c.Vocab == "it" && !c.Excluded {
							its = append(its, c)
						}
					}
					if len(its) > 0 {
						fn(suite, method, n, its)
					}
				}
				for k := range n.Children {
					visit(&n.Children[k])
				}
			}
			visit(method)
		}
	}
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

// node is the part of a spec --static node the checks read.
type node struct {
	Display  string `json:"display"`
	Name     string `json:"name"`
	Vocab    string `json:"vocab"`
	Excluded bool   `json:"excluded"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Children []node `json:"children"`
}

// MisplacedIts names each labeled It that is not a direct child of a When:
// under Each rows, under another It, or with no When above it. None of them
// is a clause. Excluded nodes are skipped.
func MisplacedIts(r *Reading) []string {
	var out []string
	for i := range r.Packages {
		p := &r.Packages[i]
		pkg := ConformanceDir + "/" + p.capability()
		for _, suite := range p.Nodes {
			if suite.Excluded {
				continue
			}
			for _, method := range suite.Children {
				var visit func(n, parent *node, underWhen bool)
				visit = func(n, parent *node, underWhen bool) {
					if n.Excluded {
						return
					}
					if label := strings.TrimSpace(n.Display); n.Vocab == "it" && label != "" && parent.Vocab != "when" {
						at := fmt.Sprintf("%s: %s.%s: It %q", pkg, suite.Display, method.Display, label)
						switch {
						case !underWhen:
							out = append(out, at+" has no When above it")
						case parent.Vocab == "each":
							out = append(out, fmt.Sprintf("%s is under Each row %q", at, parent.Display))
						case parent.Vocab == "it":
							out = append(out, fmt.Sprintf("%s is under It %q", at, parent.Display))
						default:
							out = append(out, fmt.Sprintf("%s is under %q, not a When", at, parent.Display))
						}
					}
					underWhen = underWhen || n.Vocab == "when"
					for i := range n.Children {
						visit(&n.Children[i], n, underWhen)
					}
				}
				for i := range method.Children {
					visit(&method.Children[i], &method, false)
				}
			}
		}
	}
	return out
}

// UnstatedTables names each When whose children include Each rows but no It:
// its table is a behavior no clause states. Excluded nodes are skipped.
func UnstatedTables(r *Reading) []string {
	var out []string
	for i := range r.Packages {
		p := &r.Packages[i]
		pkg := ConformanceDir + "/" + p.capability()
		for _, suite := range p.Nodes {
			if suite.Excluded {
				continue
			}
			for _, method := range suite.Children {
				var visit func(n *node)
				visit = func(n *node) {
					if n.Excluded {
						return
					}
					if n.Vocab == "when" && holdsTableOnly(n) {
						out = append(out, fmt.Sprintf("%s: %s.%s: When %q", pkg, suite.Display, method.Display, whenLabel(n.Display, n.Name)))
					}
					for i := range n.Children {
						visit(&n.Children[i])
					}
				}
				visit(&method)
			}
		}
	}
	return out
}

// holdsTableOnly reports whether n has Each children and no It child.
func holdsTableOnly(n *node) bool {
	var each, it bool
	for _, c := range n.Children {
		switch c.Vocab {
		case "each":
			each = true
		case "it":
			it = true
		}
	}
	return each && !it
}
