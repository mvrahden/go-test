package speccatalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Arrow separates a quoted clause's condition from its outcome.
const Arrow = " → "

// Quote is a clause-shaped blockquote found in a document.
type Quote struct {
	File string
	Line int
	Text string
}

// node is the part of a catalog node the resolver reads.
type node struct {
	Display  string `json:"display"`
	Name     string `json:"name"`
	Vocab    string `json:"vocab"`
	Excluded bool   `json:"excluded"`
	Children []node `json:"children"`
}

// Clauses lists every "<when> → <it>" sentence the catalog JSON holds: one per
// It block, paired with the innermost When block above it, both as rendered.
// An excluded suite or method is parked, not promised: nothing under it is a
// clause.
func Clauses(js []byte) ([]string, error) {
	var doc struct {
		Packages []struct {
			Nodes []node `json:"nodes"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, fmt.Errorf("catalog json: %w", err)
	}
	var clauses []string
	for _, p := range doc.Packages {
		for _, n := range p.Nodes {
			walk(&n, nil, func(when, it *node) {
				clauses = append(clauses, when.Display+Arrow+it.Display)
			})
		}
	}
	return clauses, nil
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

// Quotes finds the blockquote lines of doc shaped like a clause: "> " followed
// by a condition, the arrow and an outcome. Lines inside code fences are
// skipped. Line numbers start at 1.
func Quotes(file string, doc []byte) []Quote {
	var quotes []Quote
	prose(doc, func(n int, line string) {
		text, ok := strings.CutPrefix(strings.TrimSpace(line), "> ")
		if ok && strings.Contains(text, Arrow) {
			quotes = append(quotes, Quote{File: file, Line: n, Text: strings.TrimSpace(text)})
		}
	})
	return quotes
}

// WhenBullets finds the scenario WHEN bullets of doc outside code fences.
func WhenBullets(file string, doc []byte) []Quote {
	var found []Quote
	prose(doc, func(n int, line string) {
		text := strings.TrimSpace(line)
		if m := bullet.FindStringSubmatch(text); m != nil && m[1] == "WHEN" {
			found = append(found, Quote{File: file, Line: n, Text: text})
		}
	})
	return found
}

// Unresolved names every quote that matches no clause, as "file:line: text".
func Unresolved(quotes []Quote, clauses []string) []string {
	known := make(map[string]bool, len(clauses))
	for _, c := range clauses {
		known[c] = true
	}
	var missing []string
	for _, q := range quotes {
		if !known[q.Text] {
			missing = append(missing, fmt.Sprintf("%s:%d: %s", q.File, q.Line, q.Text))
		}
	}
	return missing
}
