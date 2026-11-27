package gotestaudit

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// Verdict is what the command made of a document: its exit code and the
// classes it failed on. Verbose lists every row the text format groups.
type Verdict struct {
	ExitCode int
	FailOn   []string
	Verbose  bool
}

// WriteJSON writes v as one line of JSON.
func WriteJSON(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }

// groupAt is how many rows of one class, package and suite the text format
// folds into one line.
const groupAt = 5

// RenderText writes the document for a terminal or an agent: the evidence
// read, the weakenings, the behaviors, the unproven lines, what was not
// compared, then the closing line. Rows needing a reader come first; rows
// alike are grouped unless v.Verbose.
func RenderText(w io.Writer, d *Document, v Verdict) {
	fmt.Fprintf(w, "gotest audit: base %s → head %s\n", sideText(&d.Base), sideText(&d.Head))
	writeStreams(w, d)
	ev := evidenceIndex(d)
	marks := len(ev) > 0
	folded := 0
	if len(d.Findings) > d.Summary.Informational {
		fmt.Fprintf(w, "\nweakenings: %d\n", d.Summary.Weakenings)
		var rows []textRow
		for i := range d.Findings {
			f := &d.Findings[i]
			if !f.Weakens {
				continue
			}
			e := ev[pathKey(f.Package, f.headPath)]
			rows = append(rows, textRow{head: f.Class, mark: markOf(e), pkg: f.Package, suite: first(f.headPath, f.basePath), text: findingText(d, f), value: valueText(f), fail: e != nil && e.Verdict == VerdictFail})
		}
		folded += writeRows(w, d, rows, 11, marks, v.Verbose)
	}
	if len(d.Behaviors) > 0 {
		fmt.Fprintf(w, "\nbehaviors: %s\n", behaviorCounts(&d.Summary))
		var rows []textRow
		for _, i := range behaviorOrder(d, ev) {
			c := &d.Behaviors[i]
			var e *Evidence
			p := c.Base
			if c.Head != nil {
				e, p = ev[pathKey(c.Package, c.Head.Path)], c.Head
			}
			rows = append(rows, textRow{head: c.Change, mark: markOf(e), pkg: c.Package, suite: first(p.Path), text: changeText(d, c), fail: e != nil && e.Verdict == VerdictFail})
		}
		folded += writeRows(w, d, rows, 9, marks, v.Verbose)
	}
	if d.Coverage != nil {
		writeUnproven(w, d, v.Verbose)
	}
	notes := d.Notes
	if folded > 0 {
		notes = append(slices.Clone(notes), fmt.Sprintf("%d rows alike are grouped by package and suite; -v lists every one", folded))
	}
	for _, list := range []struct {
		title string
		items []string
	}{{"information", information(d)}, {"not compared", d.Incomplete}, {"notes", notes}} {
		if len(list.items) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s:\n", list.title)
		for _, item := range list.items {
			fmt.Fprintf(w, "  %s\n", item)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, ClosingLine(d, v))
}

// textRow is one line of a text list before grouping.
type textRow struct {
	head, mark, pkg, suite, text, value string
	fail                                bool
}

// writeRows writes rows, folding each run of groupAt or more rows that share
// head, mark, package and suite into one line, a failing row never. It
// returns how many rows it folded.
func writeRows(w io.Writer, d *Document, rows []textRow, width int, marks, verbose bool) int {
	type key struct{ head, mark, pkg, suite string }
	count := map[key]int{}
	values := map[key]map[string]bool{}
	for _, r := range rows {
		k := key{r.head, r.mark, r.pkg, r.suite}
		count[k]++
		if values[k] == nil {
			values[k] = map[string]bool{}
		}
		values[k][r.value] = true
	}
	folded := 0
	done := map[key]bool{}
	for _, r := range rows {
		k := key{r.head, r.mark, r.pkg, r.suite}
		text := r.text
		if !verbose && !r.fail && count[k] >= groupAt {
			if done[k] {
				continue
			}
			done[k] = true
			folded += count[k]
			text = groupText(d, &r, count[k], values[k])
		}
		if marks {
			fmt.Fprintf(w, "  %-*s  %-12s %s\n", width, r.head, r.mark, text)
			continue
		}
		fmt.Fprintf(w, "  %-*s  %s\n", width, r.head, text)
	}
	return folded
}

func groupText(d *Document, r *textRow, n int, values map[string]bool) string {
	text := fmt.Sprintf("%d alike", n)
	if r.pkg != "" {
		text = fmt.Sprintf("%s %s: %d behaviors", shortPackage(d, r.pkg), r.suite, n)
	}
	if len(values) == 1 {
		for v := range values {
			if v != "" {
				text += " (" + v + ")"
			}
		}
	}
	return text
}

func first(paths ...[]string) string {
	for _, p := range paths {
		if len(p) > 0 {
			return p[0]
		}
	}
	return ""
}

// evidenceIndex finds an evidence entry by package and labels.
func evidenceIndex(d *Document) map[string]*Evidence {
	out := map[string]*Evidence{}
	for i := range d.Evidence {
		out[pathKey(d.Evidence[i].Package, d.Evidence[i].Path)] = &d.Evidence[i]
	}
	return out
}

// markOf is an evidence entry's mark: its verdict, the assertions a passing
// leaf made, and * when the stream is stale; "" without an entry.
func markOf(e *Evidence) string {
	if e == nil {
		return ""
	}
	m := e.Verdict
	switch {
	case e.Verdict == VerdictFail:
		m = "FAIL"
	case e.Verdict == VerdictPass && e.Vacuous:
		m = "vacuous"
	case e.Verdict == VerdictPass && e.Leaves != nil && e.Leaves.NotRun > 0:
		m = "partly run"
	case e.Verdict == VerdictPass && e.Assertions != nil && e.Leaves == nil:
		m = fmt.Sprintf("pass %d", *e.Assertions)
	}
	if e.Stream != nil && !e.Current {
		m += "*"
	}
	return m
}

// behaviorOrder lists the behaviors a reader should see first, first:
// removed, then weakened, failing, vacuous, not run and stale ones.
func behaviorOrder(d *Document, ev map[string]*Evidence) []int {
	weakened := map[string]bool{}
	for i := range d.Findings {
		if f := &d.Findings[i]; f.Weakens && f.headPath != nil {
			weakened[pathKey(f.Package, f.headPath)] = true
		}
	}
	rank := func(c *Change) int {
		if c.Head == nil {
			return 0
		}
		k := pathKey(c.Package, c.Head.Path)
		e := ev[k]
		switch {
		case weakened[k]:
			return 1
		case e == nil:
			return 6
		case e.Verdict == VerdictFail:
			return 2
		case e.Vacuous:
			return 3
		case e.notProven():
			return 4
		case !e.Current:
			return 5
		}
		return 6
	}
	order := make([]int, len(d.Behaviors))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(rank(&d.Behaviors[a]), rank(&d.Behaviors[b])) })
	return order
}

// writeStreams names the streams read under the first line, one a line.
func writeStreams(w io.Writer, d *Document) {
	if len(d.Streams) == 0 {
		return
	}
	current := 0
	for i := range d.Streams {
		if d.Streams[i].Current {
			current++
		}
	}
	head := fmt.Sprintf("evidence: %d stream%s, %d current", len(d.Streams), plural(len(d.Streams)), current)
	if current < len(d.Streams) {
		head += "; * marks a verdict of a stale one"
	}
	fmt.Fprintln(w, head)
	for i := range d.Streams {
		st := &d.Streams[i]
		state := "current"
		if !st.Current {
			state = "stale"
		}
		text := streamName(st)
		if st.Run != nil && st.Run.ExitCode != nil {
			text += fmt.Sprintf(", exit %d", *st.Run.ExitCode)
		}
		if st.Stale != "" {
			text += ": " + st.Stale
		}
		fmt.Fprintf(w, "  %-7s  %s\n", state, text)
	}
}

// streamName is a stream's file, or the audit's run, with its side and
// commit.
func streamName(st *Stream) string {
	name := st.File
	if st.Ran {
		name = "this audit's run"
	}
	if st.Side == SideBase {
		name = "base " + name
	}
	if st.Run != nil && st.Run.Commit != "" {
		name += " of " + st.Run.Commit
	}
	return name
}

// streamsText names each stream and whether it is current, on one line.
func streamsText(d *Document) string {
	parts := make([]string, 0, len(d.Streams))
	stale := false
	for i := range d.Streams {
		st := &d.Streams[i]
		name := streamName(st)
		state := "current"
		if !st.Current {
			state, stale = "stale*: "+st.Stale, true
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", name, state))
	}
	text := strings.Join(parts, "; ")
	if stale {
		text += "; * marks a verdict of a stale stream"
	}
	return text
}

// maxRanges is how many ranges of a file the text format lists unless -v.
const maxRanges = 6

func writeUnproven(w io.Writer, d *Document, verbose bool) {
	s := &d.Summary
	fmt.Fprintf(w, "\nunproven: %s\n", unprovenCounts(s))
	width := 0
	for i := range d.Unproven {
		width = max(width, len(d.Unproven[i].File))
	}
	for i := range d.Unproven {
		u := &d.Unproven[i]
		fmt.Fprintf(w, "  %-*s  %s\n", width, u.File, unprovenText(u, verbose))
	}
}

func unprovenCounts(s *Summary) string {
	text := fmt.Sprintf("%d of %d changed executable lines not executed", s.Unproven, s.ChangedLines)
	if s.NoCoverage > 0 {
		text += fmt.Sprintf(", %d file%s without coverage data", s.NoCoverage, plural(s.NoCoverage))
	}
	return text
}

func unprovenText(u *Unproven, verbose bool) string {
	if !u.Coverage {
		return "no coverage data"
	}
	shown := u.Ranges
	more := 0
	if !verbose && len(shown) > maxRanges {
		shown, more = shown[:maxRanges], len(shown)-maxRanges
	}
	parts := make([]string, 0, len(shown)+1)
	for _, r := range shown {
		if r.Line == r.EndLine {
			parts = append(parts, fmt.Sprint(r.Line))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d-%d", r.Line, r.EndLine))
	}
	if more > 0 {
		parts = append(parts, fmt.Sprintf("… %d more", more))
	}
	return fmt.Sprintf("%d of %d: %s", u.Unexecuted, u.Changed, strings.Join(parts, ", "))
}

// information are the informational findings the behaviors do not show
// already, one line each.
func information(d *Document) []string {
	var out []string
	for i := range d.Findings {
		if f := &d.Findings[i]; !f.Weakens && f.Class != ClassChanged {
			out = append(out, fmt.Sprintf("%-11s  %s", f.Class, findingText(d, f)))
		}
	}
	return out
}

// ClosingLine is the text format's last line:
//
//	gotest audit: FAIL exit 1 | weakenings: 2 (removed 1, focus 1) | behaviors: 0 added, 1 removed, 0 reworded, 0 moved, 0 changed | evidence: 3 (failing 1, not run 0, vacuous 0, stale 0) | unproven: 2 of 9 lines | fail-on: removed
func ClosingLine(d *Document, v Verdict) string {
	verdict := "PASS"
	if v.ExitCode != 0 {
		verdict = "FAIL"
	}
	weak := fmt.Sprintf("weakenings: %d", d.Summary.Weakenings)
	var classes []string
	for _, c := range Classes {
		if n := d.Summary.Classes[c]; n > 0 && Weakens(c) {
			classes = append(classes, fmt.Sprintf("%s %d", c, n))
		}
	}
	if len(classes) > 0 {
		weak += " (" + strings.Join(classes, ", ") + ")"
	}
	segments := []string{fmt.Sprintf("gotest audit: %s exit %d", verdict, v.ExitCode), weak, "behaviors: " + behaviorCounts(&d.Summary)}
	if len(d.Streams) > 0 {
		segments = append(segments, evidenceCounts(&d.Summary))
	}
	if d.Coverage != nil {
		segments = append(segments, fmt.Sprintf("unproven: %d of %d lines", d.Summary.Unproven, d.Summary.ChangedLines)+noCoverage(&d.Summary))
	}
	if d.Summary.Incomplete > 0 {
		segments = append(segments, fmt.Sprintf("not compared: %d", d.Summary.Incomplete))
	}
	if len(v.FailOn) > 0 {
		segments = append(segments, "fail-on: "+strings.Join(v.FailOn, ","))
	}
	return strings.Join(segments, " | ")
}

func evidenceCounts(s *Summary) string {
	return fmt.Sprintf("evidence: %d (failing %d, not run %d, vacuous %d, stale %d)", s.Evidence, s.Failing, s.NotRun, s.Vacuous, s.Stale)
}

func noCoverage(s *Summary) string {
	if s.NoCoverage == 0 {
		return ""
	}
	return fmt.Sprintf(", %d without coverage", s.NoCoverage)
}

func behaviorCounts(s *Summary) string {
	return fmt.Sprintf("%d added, %d removed, %d reworded, %d moved, %d changed", s.Added, s.Removed, s.Reworded, s.Moved, s.Changed)
}

func sideText(s *Side) string {
	name := s.Commit
	if s.Ref != "" && s.Ref != s.Commit {
		name = s.Ref + " " + s.Commit
	}
	if name == "" {
		name = "(no commit)"
	}
	if s.Dirty != nil && *s.Dirty {
		name += " with uncommitted changes"
	}
	return name
}

// shortPackage is a package's path in its module: "." for the module's root.
func shortPackage(d *Document, pkg string) string {
	best := ""
	for _, m := range d.Modules {
		if (pkg == m || strings.HasPrefix(pkg, m+"/")) && len(m) > len(best) {
			best = m
		}
	}
	if best == "" {
		return pkg
	}
	if pkg == best {
		return "."
	}
	return "./" + strings.TrimPrefix(pkg, best+"/")
}

// FindingText is a finding's line as the text format writes it, without
// its class.
func FindingText(d *Document, f *Finding) string { return findingText(d, f) }

// findingText is a finding's line: what it is about, its values and where.
// A file's finding names the file once, with its line.
func findingText(d *Document, f *Finding) string {
	loc := ""
	if f.Location != nil && f.Location.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.Location.File, f.Location.Line)
	}
	if f.Package == "" {
		subject := f.Subject
		if loc != "" {
			subject = loc
		}
		return subject + " " + valueText(f)
	}
	text := shortPackage(d, f.Package) + " " + f.Subject
	if f.Class == ClassRemoved {
		return fmt.Sprintf("%s (%s, %s)", text, f.Base, loc)
	}
	return fmt.Sprintf("%s: %s (%s)", text, valueText(f), loc)
}

// valueText is a finding's values: base → head, or the one there is, and
// the counts the runs reported.
func valueText(f *Finding) string {
	text := f.Base
	switch {
	case f.Base != "" && f.Head != "":
		text = f.Base + " → " + f.Head
	case f.Head != "":
		text = f.Head
	}
	if f.Runtime != nil {
		text += " (run time " + countText(f.Runtime.Base) + " → " + countText(f.Runtime.Head) + ")"
	}
	return text
}

func countText(n *int) string {
	if n == nil {
		return "?"
	}
	return fmt.Sprint(*n)
}

func changeText(d *Document, c *Change) string {
	pkg := shortPackage(d, c.Package)
	switch {
	case c.Base == nil:
		return fmt.Sprintf("%s %s (%s)%s", pkg, strings.Join(c.Head.Path, " / "), where(c.Head), leaves(c))
	case c.Head == nil:
		return fmt.Sprintf("%s %s (%s)%s", pkg, strings.Join(c.Base.Path, " / "), where(c.Base), leaves(c))
	case c.Change == ChangeChanged:
		return fmt.Sprintf("%s %s (%s)", pkg, strings.Join(c.Head.Path, " / "), where(c.Head))
	case c.Change == ChangeReworded && len(c.Base.Path) == len(c.Head.Path):
		// Siblings: the parent once, then the label as it was and is.
		last := len(c.Head.Path) - 1
		return fmt.Sprintf("%s %s%s → %s (%s)", pkg, parentText(c.Head.Path[:last]), c.Base.Path[last], c.Head.Path[last], where(c.Head))
	default:
		return fmt.Sprintf("%s %s → %s (%s → %s)", pkg, strings.Join(c.Base.Path, " / "), strings.Join(c.Head.Path, " / "), where(c.Base), where(c.Head))
	}
}

func parentText(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return strings.Join(path, " / ") + " / "
}

func where(p *Place) string { return fmt.Sprintf("%s:%d", p.File, p.Line) }

func leaves(c *Change) string {
	if c.Behaviors <= 1 {
		return ""
	}
	return fmt.Sprintf(", %d behaviors", c.Behaviors)
}

// RenderMarkdown writes the document for a pull request or a step summary.
func RenderMarkdown(w io.Writer, d *Document, v Verdict) {
	head := "no weakenings"
	if d.Summary.Weakenings > 0 {
		head = fmt.Sprintf("%d weakening%s", d.Summary.Weakenings, plural(d.Summary.Weakenings))
	}
	fmt.Fprintf(w, "### Audit: %s\n\n", head)
	fmt.Fprintf(w, "Base %s → head %s\n", gotestspec.EscapeLabel(sideText(&d.Base)), gotestspec.EscapeLabel(sideText(&d.Head)))
	if len(d.Streams) > 0 {
		fmt.Fprintf(w, "\nEvidence: %s\n", gotestspec.EscapeLabel(streamsText(d)))
	}
	ev := evidenceIndex(d)
	if d.Summary.Weakenings > 0 {
		if len(ev) > 0 {
			fmt.Fprint(w, "\n#### Weakenings\n\n| Class | Evidence | Package | Subject | Base | Head | Where |\n|---|---|---|---|---|---|---|\n")
		} else {
			fmt.Fprint(w, "\n#### Weakenings\n\n| Class | Package | Subject | Base | Head | Where |\n|---|---|---|---|---|---|\n")
		}
		for i := range d.Findings {
			f := &d.Findings[i]
			if !f.Weakens {
				continue
			}
			loc := ""
			if f.Location != nil && f.Location.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.Location.File, f.Location.Line)
			}
			base, headValue := f.Base, f.Head
			if f.Runtime != nil {
				base += " (run " + countText(f.Runtime.Base) + ")"
				headValue += " (run " + countText(f.Runtime.Head) + ")"
			}
			if len(ev) > 0 {
				fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n", f.Class, mdCell(markOf(ev[pathKey(f.Package, f.headPath)])), mdCell(shortPackage(d, f.Package)), mdCell(f.Subject), mdCell(base), mdCell(headValue), mdCell(loc))
				continue
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s |\n", f.Class, mdCell(shortPackage(d, f.Package)), mdCell(f.Subject), mdCell(base), mdCell(headValue), mdCell(loc))
		}
	}
	if len(d.Behaviors) > 0 {
		if len(ev) > 0 {
			fmt.Fprint(w, "\n#### Behaviors\n\n| Change | Evidence | Package | Behavior | Where |\n|---|---|---|---|---|\n")
		} else {
			fmt.Fprint(w, "\n#### Behaviors\n\n| Change | Package | Behavior | Where |\n|---|---|---|---|\n")
		}
		for _, i := range behaviorOrder(d, ev) {
			c := &d.Behaviors[i]
			if len(ev) > 0 {
				var e *Evidence
				if c.Head != nil {
					e = ev[pathKey(c.Package, c.Head.Path)]
				}
				fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", c.Change, mdCell(markOf(e)), mdCell(shortPackage(d, c.Package)), mdCell(mdBehavior(c)), mdCell(mdWhere(c)))
				continue
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", c.Change, mdCell(shortPackage(d, c.Package)), mdCell(mdBehavior(c)), mdCell(mdWhere(c)))
		}
	}
	if d.Coverage != nil {
		fmt.Fprintf(w, "\n#### Unproven lines\n\n%s\n", gotestspec.EscapeLabel(unprovenCounts(&d.Summary)))
		if len(d.Unproven) > 0 {
			fmt.Fprint(w, "\n| File | Package | Not executed |\n|---|---|---|\n")
			for i := range d.Unproven {
				u := &d.Unproven[i]
				fmt.Fprintf(w, "| %s | %s | %s |\n", mdCell(u.File), mdCell(shortPackage(d, u.Package)), mdCell(unprovenText(u, true)))
			}
		}
	}
	for _, list := range []struct {
		title string
		items []string
	}{{"Information", information(d)}, {"Not compared", d.Incomplete}, {"Notes", d.Notes}} {
		if len(list.items) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n#### %s\n\n", list.title)
		for _, item := range list.items {
			fmt.Fprintf(w, "- %s\n", gotestspec.EscapeLabel(item))
		}
	}
	fmt.Fprintf(w, "\n---\n%s\n", gotestspec.EscapeLabel(ClosingLine(d, v)))
}

func mdCell(s string) string {
	if s == "" {
		return "—"
	}
	return gotestspec.EscapeLabel(s)
}

func mdBehavior(c *Change) string {
	switch {
	case c.Base == nil:
		return strings.Join(c.Head.Path, " / ")
	case c.Head == nil || c.Change == ChangeChanged:
		p := c.Head
		if p == nil {
			p = c.Base
		}
		return strings.Join(p.Path, " / ")
	default:
		return strings.Join(c.Base.Path, " / ") + " → " + strings.Join(c.Head.Path, " / ")
	}
}

func mdWhere(c *Change) string {
	switch {
	case c.Base == nil:
		return where(c.Head)
	case c.Head == nil:
		return where(c.Base)
	case c.Change == ChangeChanged:
		return where(c.Head)
	default:
		return where(c.Base) + " → " + where(c.Head)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
