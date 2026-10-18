package speccatalog

import (
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

// Clauses lists every "<when> → <then>" sentence the specs state, once each,
// in spec order: one per THEN and AND bullet, after its scenario's WHEN with
// the "when " connective in front. A WHEN that opens with "when" keeps its
// own. A scenario without exactly one WHEN states no clause.
func Clauses(specs []Spec) []string {
	var clauses []string
	seen := map[string]bool{}
	for _, s := range specs {
		for _, r := range s.Requirements {
			for _, sc := range r.Scenarios {
				if len(sc.Whens) != 1 {
					continue
				}
				when := "when " + strings.TrimPrefix(sc.Whens[0], "when ")
				for _, then := range sc.Thens {
					c := when + Arrow + then
					if !seen[c] {
						seen[c] = true
						clauses = append(clauses, c)
					}
				}
			}
		}
	}
	return clauses
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
