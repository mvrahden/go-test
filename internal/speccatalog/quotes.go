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
	Vocab    string `json:"vocab"`
	Children []node `json:"children"`
}

// Clauses lists every "<when> → <it>" sentence the catalog JSON holds: one per
// It block, paired with the innermost When block above it, both as rendered.
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
			collect(n, "", &clauses)
		}
	}
	return clauses, nil
}

func collect(n node, when string, out *[]string) {
	switch n.Vocab {
	case "when":
		when = n.Display
	case "it":
		if when != "" {
			*out = append(*out, when+Arrow+n.Display)
		}
	}
	for _, c := range n.Children {
		collect(c, when, out)
	}
}

// Quotes finds the blockquote lines of doc shaped like a clause: "> " followed
// by a condition, the arrow and an outcome. Line numbers start at 1.
func Quotes(file string, doc []byte) []Quote {
	var quotes []Quote
	for i, line := range strings.Split(string(doc), "\n") {
		text, ok := strings.CutPrefix(strings.TrimSpace(line), "> ")
		if !ok || !strings.Contains(text, Arrow) {
			continue
		}
		quotes = append(quotes, Quote{File: file, Line: i + 1, Text: strings.TrimSpace(text)})
	}
	return quotes
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
