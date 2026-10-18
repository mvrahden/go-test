package speccatalog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// SpecsDir holds one spec.md per capability, relative to the repository root.
const SpecsDir = "specs"

// Shape limits, as OpenSpec's strict validation holds them.
const (
	minPurpose     = 50
	maxRequirement = 500
)

// Spec is one specs/<capability>/spec.md in OpenSpec's format.
type Spec struct {
	Capability   string
	File         string
	Title        string
	Purpose      string
	Requirements []Requirement
	Misplaced    []Stray
}

// Requirement is a "### Requirement:" block: its text and scenarios.
type Requirement struct {
	Name      string
	Line      int
	Text      string
	Scenarios []Scenario
}

// Scenario is a "####" heading under a requirement. Thens holds the THEN
// bullet and every AND after it, in order.
type Scenario struct {
	Title string
	Line  int
	Whens []string
	Thens []string
	Stray []Stray
}

// Stray is a scenario line that is no WHEN, THEN or AND bullet, or, in
// Spec.Misplaced, a heading or bullet that binds to nothing and why.
type Stray struct {
	Line int
	Text string
}

var (
	bullet  = regexp.MustCompile(`^[-*]\s+\*\*(WHEN|THEN|AND)\*\*\s+(.*)$`)
	keyword = regexp.MustCompile(`\b(SHALL|MUST)\b`)
)

// ParseSpec reads a spec file. Lines inside code fences are skipped, and
// every "####" heading under a requirement is a scenario. Shape problems are
// left to Problems.
func ParseSpec(capability, file string, data []byte) Spec {
	spec := Spec{Capability: capability, File: file}
	var (
		section string
		purpose []string
		req     *Requirement
		reqText []string
		sc      *Scenario
		fence   string
	)
	closeScenario := func() {
		if sc != nil {
			req.Scenarios = append(req.Scenarios, *sc)
			sc = nil
		}
	}
	closeRequirement := func() {
		closeScenario()
		if req != nil {
			req.Text = strings.Join(reqText, " ")
			spec.Requirements = append(spec.Requirements, *req)
			req, reqText = nil, nil
		}
	}
	misplaced := func(line int, why, text string) {
		spec.Misplaced = append(spec.Misplaced, Stray{Line: line, Text: why + ": " + text})
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if fence != "" {
			if strings.HasPrefix(line, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fence = line[:3]
			continue
		}
		switch {
		case strings.HasPrefix(line, "#### "):
			if req == nil {
				misplaced(i+1, "scenario heading belongs to no requirement", line)
				continue
			}
			closeScenario()
			title := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "#### "), "Scenario:"))
			sc = &Scenario{Title: title, Line: i + 1}
		case strings.HasPrefix(line, "### "):
			closeRequirement()
			name, ok := strings.CutPrefix(line, "### Requirement:")
			switch {
			case ok && section == "Requirements":
				req = &Requirement{Name: strings.TrimSpace(name), Line: i + 1}
			case ok:
				misplaced(i+1, "requirement heading is outside ## Requirements", line)
			case section == "Requirements":
				misplaced(i+1, `heading under ## Requirements does not start with "Requirement:"`, line)
			}
		case strings.HasPrefix(line, "## "):
			closeRequirement()
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "# "):
			closeRequirement()
			spec.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case line == "":
		case sc != nil:
			m := bullet.FindStringSubmatch(line)
			switch {
			case m == nil:
				sc.Stray = append(sc.Stray, Stray{Line: i + 1, Text: line})
			case m[1] == "WHEN":
				sc.Whens = append(sc.Whens, strings.TrimSpace(m[2]))
			case m[1] == "THEN" || len(sc.Thens) > 0:
				sc.Thens = append(sc.Thens, strings.TrimSpace(m[2]))
			default:
				sc.Stray = append(sc.Stray, Stray{Line: i + 1, Text: line})
			}
		case bullet.MatchString(line) && req == nil:
			misplaced(i+1, "bullet belongs to no requirement", line)
		case bullet.MatchString(line):
			misplaced(i+1, "bullet belongs to no scenario", line)
		case req != nil:
			reqText = append(reqText, line)
		case section == "Purpose":
			purpose = append(purpose, line)
		}
	}
	closeRequirement()
	spec.Purpose = strings.Join(purpose, " ")
	return spec
}

// Problems names every way the spec breaks the shape, in file order.
func (s *Spec) Problems() []string {
	type problem struct {
		line int
		text string
	}
	var found []problem
	add := func(line int, format string, args ...any) {
		at := s.File
		if line > 0 {
			at = fmt.Sprintf("%s:%d", s.File, line)
		}
		found = append(found, problem{line, at + ": " + fmt.Sprintf(format, args...)})
	}
	for _, m := range s.Misplaced {
		add(m.Line, "%s", m.Text)
	}
	if n := utf8.RuneCountInString(s.Purpose); n < minPurpose {
		add(0, "Purpose has %d characters, needs at least %d", n, minPurpose)
	}
	if len(s.Requirements) == 0 {
		add(0, "has no requirement")
	}
	names := map[string]int{}
	for _, r := range s.Requirements {
		if first, ok := names[r.Name]; ok {
			add(r.Line, "requirement %q: name is already used at line %d", r.Name, first)
		} else {
			names[r.Name] = r.Line
		}
		if !keyword.MatchString(r.Text) {
			add(r.Line, "requirement %q: text contains neither SHALL nor MUST", r.Name)
		}
		if n := utf8.RuneCountInString(r.Text); n > maxRequirement {
			add(r.Line, "requirement %q: text has %d characters, at most %d", r.Name, n, maxRequirement)
		}
		if len(r.Scenarios) == 0 {
			add(r.Line, "requirement %q: has no scenario", r.Name)
		}
		for _, sc := range r.Scenarios {
			switch len(sc.Whens) {
			case 0:
				add(sc.Line, "scenario %q: has no WHEN bullet", sc.Title)
			case 1:
			default:
				add(sc.Line, "scenario %q: has %d WHEN bullets, needs one", sc.Title, len(sc.Whens))
			}
			if len(sc.Thens) == 0 {
				add(sc.Line, "scenario %q: has no THEN bullet", sc.Title)
			}
			for _, l := range sc.Stray {
				add(l.Line, "scenario %q: line is not a WHEN, THEN or AND bullet: %s", sc.Title, l.Text)
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].line < found[j].line })
	var out []string
	for _, p := range found {
		out = append(out, p.text)
	}
	return out
}

// LoadSpecs reads every <capability>/spec.md under root's specs directory,
// sorted by capability. A missing specs directory holds no spec.
func LoadSpecs(root string) ([]Spec, error) {
	dir := filepath.Join(root, SpecsDir)
	var specs []Spec
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		capDir, err := filepath.Rel(dir, filepath.Dir(path))
		if err != nil {
			return err
		}
		if capDir == "." {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: the repository's own specs
		if err != nil {
			return err
		}
		capability := filepath.ToSlash(capDir)
		specs = append(specs, ParseSpec(capability, specFile(capability), data))
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Capability < specs[j].Capability })
	return specs, nil
}

func specFile(capability string) string {
	return SpecsDir + "/" + capability + "/spec.md"
}
