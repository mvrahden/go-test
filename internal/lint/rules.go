package lint

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
)

// String is the tier as the rules document names it.
func (t Tier) String() string {
	switch t {
	case TierIntegrity:
		return "integrity"
	case TierExpressiveness:
		return "expressiveness"
	case TierMigration:
		return "migration"
	}
	return "unknown"
}

// String is the scope as the rules document names it.
func (s Scope) String() string {
	switch s {
	case ScopeEverywhere:
		return "everywhere"
	case ScopeGotestFiles:
		return "gotest-files"
	case ScopeSuites:
		return "suites"
	case ScopePollCallbacks:
		return "poll-callbacks"
	}
	return "unknown"
}

// Description is the rule's one-line description.
func Description(r Rule) string {
	return strings.Join(ruleMeta[r].Help, " ")
}

// WriteRuleTable writes the rule list gotest help lint prints: each ID in a
// column of 22, its description beside it as the registry wraps it, and an
// ID too long for the column on a line of its own.
func WriteRuleTable(w io.Writer) {
	const column = 22
	indent := strings.Repeat(" ", 2+column)
	for _, r := range registry {
		id := string(r.Rule)
		if len(id) > column-2 {
			fmt.Fprintf(w, "  %s\n%s%s\n", id, indent, r.Help[0])
		} else {
			fmt.Fprintf(w, "  %-*s%s\n", column, id, r.Help[0])
		}
		for _, line := range r.Help[1:] {
			fmt.Fprintf(w, "%s%s\n", indent, line)
		}
	}
}

// rulesDocument is what gotest lint -rules -json prints. Version opens it:
// the CLI that produced the document.
type rulesDocument struct {
	Version string         `json:"version"`
	Rules   []ruleDocument `json:"rules"`
}

type ruleDocument struct {
	ID    string `json:"id"`
	Tier  string `json:"tier"`
	Scope string `json:"scope"`
	// Skippable says -skip-<rule> and lint.skip accept the rule; the tier
	// decides it.
	Skippable   bool   `json:"skippable"`
	Fixable     bool   `json:"fixable"`
	Description string `json:"description"`
}

// WriteRulesJSON writes every rule once, in the order the help lists them.
func WriteRulesJSON(w io.Writer) error {
	doc := rulesDocument{Version: about.ResolvedVersion(), Rules: make([]ruleDocument, 0, len(registry))}
	for _, r := range registry {
		doc.Rules = append(doc.Rules, ruleDocument{
			ID:          string(r.Rule),
			Tier:        r.Tier.String(),
			Scope:       r.Scope.String(),
			Skippable:   SkippableRules[r.Rule],
			Fixable:     r.Fixable,
			Description: Description(r.Rule),
		})
	}
	return json.NewEncoder(w).Encode(doc)
}
