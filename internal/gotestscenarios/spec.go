// Package gotestscenarios reads the scenarios of OpenSpec spec files and
// binds them, by their text, to the When and It labels a module's suites
// declare; a run's verdicts on those Its judge each scenario.
package gotestscenarios

import (
	"regexp"
	"strings"
)

// Spec is one spec file's scenarios.
type Spec struct {
	Capability string
	// File is the spec file's path, module-relative and slash-separated.
	File         string
	Requirements []Requirement
}

// Requirement is a requirement heading and the scenarios under it.
type Requirement struct {
	Name      string
	Line      int
	Scenarios []SpecScenario
}

// SpecScenario is a level-4 heading under a requirement. Conditions are its
// GIVEN and WHEN clauses with the ANDs after them; Outcomes its THEN
// clause and the ANDs after it, each in written order.
type SpecScenario struct {
	Title      string
	Line       int
	Conditions []Step
	Outcomes   []Step
	// Problem says why the scenario cannot bind; empty when it can.
	Problem string
}

// Step is one clause as written, its keyword in capitals.
type Step struct {
	Keyword string
	Text    string
	Line    int
}

// Keywords a clause opens with.
const (
	Given = "GIVEN"
	When  = "WHEN"
	Then  = "THEN"
	And   = "AND"
)

var (
	listItem = regexp.MustCompile(`^ {0,3}(?:[-*+]|\d{1,9}[.)])[ \t]+(.*)$`)
	// A keyword in bold or underlined, any case, a colon inside or after.
	strongKeyword = regexp.MustCompile(`^(?i)(\*\*|__)(GIVEN|WHEN|THEN|AND)(:?)(\*\*|__):?(?:[ \t]+|$)(.*)$`)
	// A bare keyword, in capitals only, as OpenSpec's own examples write it.
	bareKeyword = regexp.MustCompile(`^(GIVEN|WHEN|THEN|AND):?(?:[ \t]+|$)(.*)$`)
	heading     = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	closingHash = regexp.MustCompile(`[ \t]+#+$`)
	// Requirement sections a main spec or a change's delta spec holds.
	readSections = map[string]bool{"requirements": true, "added requirements": true, "modified requirements": true}
)

// Parse reads a spec file's scenarios. Every level-3 heading in a
// Requirements, ADDED Requirements or MODIFIED Requirements section is a
// requirement, every level-4 heading under one a scenario; code fences are
// skipped, as OpenSpec's own parser does.
func Parse(capability, file string, data []byte) Spec {
	spec := Spec{Capability: capability, File: file}
	text := strings.TrimPrefix(string(data), string(rune(0xFEFF)))
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")

	var (
		reading bool
		req     *Requirement
		sc      *SpecScenario
		last    *Step // the clause a following text line continues
		f       fence
	)
	closeScenario := func() {
		if sc != nil {
			finish(sc)
			req.Scenarios = append(req.Scenarios, *sc)
			sc, last = nil, nil
		}
	}
	closeRequirement := func() {
		closeScenario()
		if req != nil {
			spec.Requirements = append(spec.Requirements, *req)
			req = nil
		}
	}
	for i, raw := range strings.Split(text, "\n") {
		n := i + 1
		if f.skip(raw) {
			last = nil
			continue
		}
		if m := heading.FindStringSubmatch(raw); m != nil {
			last = nil
			title := strings.TrimSpace(closingHash.ReplaceAllString(m[2], ""))
			switch level := len(m[1]); {
			case level <= 2:
				closeRequirement()
				reading = level == 2 && readSections[strings.ToLower(title)]
			case level == 3:
				closeRequirement()
				if reading {
					req = &Requirement{Name: cutLabel(title, "Requirement:"), Line: n}
				}
			case level == 4 && req != nil:
				closeScenario()
				sc = &SpecScenario{Title: cutLabel(title, "Scenario:"), Line: n}
			}
			continue
		}
		if sc == nil {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			last = nil
			continue
		}
		if m := listItem.FindStringSubmatch(raw); m != nil {
			last = nil
			keyword, rest, ok := clauseOf(m[1])
			if !ok {
				continue
			}
			step := Step{Keyword: keyword, Text: rest, Line: n}
			switch {
			case keyword == Then:
				sc.Outcomes = append(sc.Outcomes, step)
				last = &sc.Outcomes[len(sc.Outcomes)-1]
			case keyword == And && len(sc.Outcomes) > 0:
				sc.Outcomes = append(sc.Outcomes, step)
				last = &sc.Outcomes[len(sc.Outcomes)-1]
			case keyword == And && len(sc.Conditions) == 0:
				problem(sc, "an AND opens it, before any GIVEN, WHEN or THEN")
			case len(sc.Outcomes) > 0:
				problem(sc, "a "+keyword+" follows its THEN")
			default:
				sc.Conditions = append(sc.Conditions, step)
				last = &sc.Conditions[len(sc.Conditions)-1]
			}
			continue
		}
		// A line right below a clause continues it, as Markdown wraps a list item.
		if last != nil {
			last.Text += " " + strings.TrimSpace(raw)
		}
	}
	closeRequirement()
	return spec
}

// clauseOf reads a list item's text as a clause: its keyword and the rest.
func clauseOf(item string) (keyword, rest string, ok bool) {
	if m := strongKeyword.FindStringSubmatch(item); m != nil && m[1] == m[4] {
		return strings.ToUpper(m[2]), strings.TrimSpace(m[5]), true
	}
	if m := bareKeyword.FindStringSubmatch(item); m != nil {
		return m[1], strings.TrimSpace(m[2]), true
	}
	return "", "", false
}

// finish records why a scenario cannot bind, if nothing did yet.
func finish(sc *SpecScenario) {
	hasWhen := false
	for _, c := range sc.Conditions {
		hasWhen = hasWhen || c.Keyword == When
	}
	switch {
	case !hasWhen:
		problem(sc, "it has no WHEN clause")
	case len(sc.Outcomes) == 0:
		problem(sc, "it has no THEN clause")
	}
}

func problem(sc *SpecScenario, why string) {
	if sc.Problem == "" {
		sc.Problem = why
	}
}

// cutLabel drops a heading's "Requirement:" or "Scenario:", in any case.
func cutLabel(title, prefix string) string {
	if len(title) >= len(prefix) && strings.EqualFold(title[:len(prefix)], prefix) {
		title = title[len(prefix):]
	}
	return strings.TrimSpace(title)
}

// fence tracks CommonMark code fences: three or more backticks or tildes,
// closed by a run of the same character at least as long.
type fence struct {
	char byte
	size int
}

// skip reports whether line opens, closes or lies inside a fence.
func (f *fence) skip(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return f.size > 0
	}
	run := 0
	for run < len(trimmed) && (trimmed[run] == '`' || trimmed[run] == '~') && trimmed[run] == trimmed[0] {
		run++
	}
	if f.size > 0 {
		if run >= f.size && trimmed[0] == f.char && strings.TrimSpace(trimmed[run:]) == "" {
			f.size = 0
		}
		return true
	}
	if run >= 3 && (trimmed[0] != '`' || !strings.Contains(trimmed[run:], "`")) {
		f.char, f.size = trimmed[0], run
		return true
	}
	return false
}
