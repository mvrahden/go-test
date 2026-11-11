package gotestspec

import (
	"fmt"
	"strings"
	"time"
)

// Provenance is where a report's numbers come from.
type Provenance struct {
	Branch, Commit string // empty: not rendered in a checkout
	Dirty          bool
	Search         string // the panel's search, when it copies
}

// WithProvenance names the checkout and the search a report was made in.
func WithProvenance(p Provenance) RenderOption {
	return func(c *renderConfig) { c.provenance = p }
}

// WithModule is the module the report's package headings are relative to.
func WithModule(path string) RenderOption {
	return func(c *renderConfig) { c.module = path }
}

// countsPhrase is "5 passed, 1 failed": the non-zero verdicts in the order
// passed, failed, skipped, not run; "" when all are zero.
func countsPhrase(s Stats) string { //nolint:gocritic // hugeParam: stable API
	var parts []string
	for _, b := range []struct {
		n    int
		word string
	}{{s.Passed, "passed"}, {s.Failed, "failed"}, {s.Skipped, "skipped"}, {s.NotRun, "not run"}} {
		if b.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", b.n, b.word))
		}
	}
	return strings.Join(parts, ", ")
}

// stats counts packages as the rendering reads them: a tree read from source
// has no verdicts, so nothing in it counts as not run.
func (c *renderConfig) stats(packages []*Package) Stats {
	s := CollectStats(packages)
	if c.withoutVerdicts {
		s.NotRun = 0
	}
	return s
}

// nodeStats counts one node and what is beneath it.
func nodeStats(n *Node) Stats {
	var s Stats
	collectStats(n, &s, n.Kind == KindTest)
	return s
}

// packageName is a package as a report heads it: relative to the module, or
// its full path when it lies outside it (other is then true). A run-level
// package is always other.
func (c *renderConfig) packageName(pkg *Package) (name string, other bool) {
	if pkg.RunLevel || c.module == "" {
		return pkg.Path, true
	}
	if pkg.Path == c.module {
		return ".", false
	}
	if rel, ok := strings.CutPrefix(pkg.Path, c.module+"/"); ok {
		return rel, false
	}
	return pkg.Path, true
}

// attentionRow is a failure or a row not run, with its display path.
type attentionRow struct {
	Status Status // StatusFail, or StatusNone for a row not run
	Path   []string
	Output []string // a failure's own lines, filtered
}

// attention lists every shown failure, with its output, and every shown row
// not run, in tree order.
func attention(packages []*Package, cfg *renderConfig) []attentionRow {
	var rows []attentionRow
	for _, pkg := range packages {
		if !cfg.packageShown(pkg) {
			continue
		}
		name, _ := cfg.packageName(pkg)
		if PkgFailedOnItsOwn(pkg) && !cfg.hidden[StatusFail] {
			rows = append(rows, attentionRow{Status: StatusFail, Path: []string{name}, Output: diagnostic(pkg.Output)})
		}
		for _, n := range cfg.visible(pkg.Nodes) {
			rows = attentionOf(n, []string{name}, rows, cfg)
		}
	}
	return rows
}

func attentionOf(n *Node, path []string, rows []attentionRow, cfg *renderConfig) []attentionRow {
	if n.Kind != KindFixture {
		path = append(path[:len(path):len(path)], n.Display)
	}
	switch {
	case n.Kind == KindBenchmark && len(n.Children) == 0:
		if n.Status == StatusFail {
			rows = append(rows, attentionRow{Status: StatusFail, Path: path, Output: filterOutput(n.Output)})
		}
		return rows
	case noVerdict(n) && len(n.Children) == 0, n.Kind == KindFuzz && n.Status == StatusNone:
		return append(rows, attentionRow{Status: StatusNone, Path: path})
	case len(n.Children) == 0 && n.Status == StatusFail:
		rows = append(rows, attentionRow{Status: StatusFail, Path: path, Output: filterOutput(n.Output)})
	case ownFailure(n) && !cfg.hidden[StatusFail]:
		rows = append(rows, attentionRow{Status: StatusFail, Path: path, Output: diagnostic(n.Output)})
	}
	for _, c := range cfg.visible(n.Children) {
		rows = attentionOf(c, path, rows, cfg)
	}
	return rows
}

// ownFailure reports a branch whose failure no row beneath it explains.
func ownFailure(n *Node) bool { return failedOnItsOwn(n) || hasOwnDiagnostic(n) }

// diagnostic is what a failure wrote, or the note that it wrote nothing.
func diagnostic(output []string) []string {
	if lines := filterOutput(output); len(lines) > 0 {
		return lines
	}
	return []string{noDiagnosticNote}
}

// provenanceLine says where a report's numbers come from: the runs' time
// range, the checkout, what is hidden and the search. Each part is left out
// when it has nothing to say; "" when none has.
func provenanceLine(packages []*Package, cfg *renderConfig) string {
	var parts []string
	if r := runRange(packages); r != "" {
		parts = append(parts, r)
	}
	if p := cfg.provenance; p.Commit != "" {
		git := p.Commit
		if p.Branch != "" && p.Branch != "HEAD" {
			git = p.Branch + " @ " + p.Commit
		}
		if p.Dirty {
			git += ", uncommitted changes"
		}
		parts = append(parts, git)
	}
	if h := hiddenPhrase(cfg.hidden); h != "" {
		parts = append(parts, h)
	}
	if q := cfg.provenance.Search; q != "" {
		parts = append(parts, `search "`+q+`"`)
	}
	return strings.Join(parts, " · ")
}

// runRange is "Runs from 2026-11-09 18:02 to 18:25 UTC": the earliest start
// to the latest end in the tree; the date once when both ends share it.
func runRange(packages []*Package) string {
	var first, last time.Time
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if !n.Start.IsZero() && (first.IsZero() || n.Start.Before(first)) {
				first = n.Start
			}
			if n.End.After(last) {
				last = n.End
			}
			walk(n.Children)
		}
	}
	for _, pkg := range packages {
		walk(pkg.Nodes)
	}
	if first.IsZero() {
		return ""
	}
	if last.Before(first) {
		last = first
	}
	first, last = first.UTC(), last.UTC()
	end := last.Format("2006-01-02 15:04")
	if first.Format("2006-01-02") == last.Format("2006-01-02") {
		end = last.Format("15:04")
	}
	return fmt.Sprintf("Runs from %s to %s UTC", first.Format("2006-01-02 15:04"), end)
}

// hiddenPhrase is "hiding passed and skipped rows", "" when nothing is hidden.
func hiddenPhrase(hidden map[Status]bool) string {
	var words []string
	for _, s := range []struct {
		status Status
		word   string
	}{{StatusPass, "passed"}, {StatusFail, "failed"}, {StatusSkip, "skipped"}, {StatusNone, "not run"}} {
		if hidden[s.status] {
			words = append(words, s.word)
		}
	}
	switch len(words) {
	case 0:
		return ""
	case 1:
		return "hiding " + words[0] + " rows"
	}
	return "hiding " + strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1] + " rows"
}
