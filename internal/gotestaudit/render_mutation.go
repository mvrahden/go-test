package gotestaudit

import (
	"fmt"
	"io"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// IgnoreNote names the comment that keeps a line from being mutated; the
// renderings write it once, under a survivor.
const IgnoreNote = "a survivor no test can tell apart from the line (an equivalent mutant) takes //gotest:mutate-ignore <reason> on its line or above its function; the reason is mandatory, and audit lists each comment added as a suppression"

// mutationCounts is the mutation section's line of counts.
func mutationCounts(m *Mutation) string {
	c := &m.Counts
	text := fmt.Sprintf("%d survived on %d line%s, %d killed, %d timeout, %d not judged", c.Survived, c.SurvivedLines, plural(c.SurvivedLines), c.Killed, c.Timeout, c.NotJudged)
	for _, part := range []struct {
		n    int
		what string
	}{{c.NotCovered, "not covered"}, {c.Ignored, "ignored"}, {c.Uncompilable, "uncompilable"}} {
		if part.n > 0 {
			text += fmt.Sprintf(", %d %s", part.n, part.what)
		}
	}
	return text
}

// mutationRun says what the mutants ran under.
func mutationRun(m *Mutation) string {
	return fmt.Sprintf("%d mutants on %d lines; budget %s, %s spent, %d at a time", m.Counts.Mutants, m.Counts.Lines, m.Budget, m.Spent, m.Jobs)
}

// actionable reports a line a reader looks at: one that is not all
// killed, not covered or ignored.
func actionable(l *MutatedLine) bool {
	switch l.Verdict {
	case MutantSurvived, MutantTimeout, MutantNotJudged, MutantUncompilable:
		return true
	}
	return false
}

// writeMutation writes the mutation section: per file the lines a reader
// acts on, survivors first, and a line counting the rest; -v lists every
// line. The suites that ran a file's lines are named once when they are
// the same for each.
func writeMutation(w io.Writer, d *Document, verbose bool) {
	m := d.Mutation
	fmt.Fprintf(w, "\nmutation: %s\n  %s\n", mutationCounts(m), mutationRun(m))
	if !m.Baseline.Passed {
		fmt.Fprintf(w, "  no mutant judged: %s failed before any mutant was switched on\n", strings.Join(m.Baseline.Failed, ", "))
		return
	}
	var files []string
	byFile := map[string][]*MutatedLine{}
	for i := range m.Lines {
		l := &m.Lines[i]
		if byFile[l.File] == nil {
			files = append(files, l.File)
		}
		byFile[l.File] = append(byFile[l.File], l)
	}
	for _, f := range files {
		var shown []*MutatedLine
		rest := map[string]int{}
		for _, l := range byFile[f] {
			if verbose || actionable(l) {
				shown = append(shown, l)
				continue
			}
			rest[l.Verdict]++
		}
		shared := sharedSuites(d, shown)
		header := "  " + f
		if shared != "" {
			header += " · ran by " + shared
		}
		fmt.Fprintln(w, header)
		for _, l := range shown {
			text := lineText(l)
			if suites := suitesText(d, l.Suites); suites != "" && shared == "" {
				text += " · ran by " + suites
			}
			fmt.Fprintf(w, "    %-5d %-10s %s\n", l.Line, l.Verdict, text)
		}
		var parts []string
		n := 0
		for _, v := range []string{MutantKilled, MutantNotCovered, MutantIgnored} {
			if k := rest[v]; k > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", k, v))
				n += k
			}
		}
		if n > 0 {
			fmt.Fprintf(w, "    %d %sline%s: %s\n", n, more(len(shown)), plural(n), strings.Join(parts, ", "))
		}
	}
}

func more(shown int) string {
	if shown > 0 {
		return "more "
	}
	return ""
}

// suitesText names suites by their package in its module and their name.
func suitesText(d *Document, suites []string) string {
	out := make([]string, 0, len(suites))
	for _, s := range suites {
		pkg, name, _ := strings.Cut(s, " ")
		out = append(out, shortPackage(d, pkg)+" "+name)
	}
	return strings.Join(out, ", ")
}

// sharedSuites names the suites every line ran by, "" when they differ or
// a line has none.
func sharedSuites(d *Document, lines []*MutatedLine) string {
	shared := ""
	for i, l := range lines {
		text := suitesText(d, l.Suites)
		if text == "" || (i > 0 && text != shared) {
			return ""
		}
		shared = text
	}
	return shared
}

// lineText is a mutated line's mutants a reader acts on and its function.
func lineText(l *MutatedLine) string {
	var parts []string
	for _, mu := range l.Mutants {
		if mu.Verdict != l.Verdict || l.Verdict == MutantKilled {
			continue
		}
		parts = append(parts, mutantText(&mu))
	}
	text := strings.Join(parts, "; ")
	if l.Verdict == MutantKilled || l.Verdict == MutantNotCovered {
		text = fmt.Sprintf("%d mutants", len(l.Mutants))
	}
	if l.Verdict == MutantIgnored {
		text = l.Reason
	}
	if l.Killed > 0 && l.Verdict != MutantKilled {
		text += fmt.Sprintf("; %d killed", l.Killed)
	}
	if l.Func != "" {
		text += " · " + l.Func
	}
	return text
}

// mutantText is one mutant: its original and mutated text and operator.
func mutantText(mu *MutantRow) string {
	if mu.Mutant == "" {
		return fmt.Sprintf("%s removed (%s)", mu.Original, mu.Operator)
	}
	return fmt.Sprintf("%s → %s (%s)", mu.Original, mu.Mutant, mu.Operator)
}

// mutationSegment is the closing line's mutation segment.
func mutationSegment(m *Mutation) string {
	c := &m.Counts
	return fmt.Sprintf("mutation: %d survived on %d line%s, %d killed, %d timeout, %d not judged", c.Survived, c.SurvivedLines, plural(c.SurvivedLines), c.Killed, c.Timeout, c.NotJudged)
}

// writeMutationMarkdown writes the mutation section as a table of the
// lines a reader acts on.
func writeMutationMarkdown(w io.Writer, d *Document) {
	m := d.Mutation
	fmt.Fprintf(w, "\n#### Mutation\n\n%s\n\n%s\n", gotestspec.EscapeLabel(mutationCounts(m)), gotestspec.EscapeLabel(mutationRun(m)))
	if !m.Baseline.Passed {
		fmt.Fprintf(w, "\nNo mutant judged: %s failed before any mutant was switched on.\n", gotestspec.EscapeLabel(strings.Join(m.Baseline.Failed, ", ")))
		return
	}
	header := false
	others := 0
	for i := range m.Lines {
		l := &m.Lines[i]
		if !actionable(l) {
			others++
			continue
		}
		if !header {
			fmt.Fprint(w, "\n| File | Line | Verdict | Mutants | Function | Ran by | Killed |\n|---|---|---|---|---|---|---|\n")
			header = true
		}
		var parts []string
		for _, mu := range l.Mutants {
			if mu.Verdict == l.Verdict {
				parts = append(parts, mutantText(&mu))
			}
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %d |\n", mdCell(l.File), l.Line, l.Verdict, mdCell(strings.Join(parts, "; ")), mdCell(l.Func), mdCell(suitesText(d, l.Suites)), l.Killed)
	}
	if others > 0 {
		fmt.Fprintf(w, "\n%d other line%s: every mutant killed, not covered or ignored.\n", others, plural(others))
	}
}
