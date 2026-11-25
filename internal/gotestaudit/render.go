package gotestaudit

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// Verdict is what the command made of a document: its exit code and the
// classes it failed on.
type Verdict struct {
	ExitCode int
	FailOn   []string
}

// WriteJSON writes v as one line of JSON.
func WriteJSON(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }

// RenderText writes the document for a terminal or an agent: the weakenings,
// the behaviors, what was not compared, then the closing line.
func RenderText(w io.Writer, d *Document, v Verdict) {
	fmt.Fprintf(w, "gotest audit: base %s → head %s\n", sideText(&d.Base), sideText(&d.Head))
	if len(d.Findings) > d.Summary.Informational {
		fmt.Fprintf(w, "\nweakenings: %d\n", d.Summary.Weakenings)
		for _, f := range d.Findings {
			if f.Weakens {
				fmt.Fprintf(w, "  %-11s  %s\n", f.Class, findingText(d, &f))
			}
		}
	}
	if len(d.Behaviors) > 0 {
		fmt.Fprintf(w, "\nbehaviors: %s\n", behaviorCounts(&d.Summary))
		for _, c := range d.Behaviors {
			fmt.Fprintf(w, "  %-9s  %s\n", c.Change, changeText(d, &c))
		}
	}
	for _, list := range []struct {
		title string
		items []string
	}{{"not compared", d.Incomplete}, {"notes", d.Notes}} {
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

// ClosingLine is the text format's last line:
//
//	gotest audit: FAIL exit 1 | weakenings: 2 (removed 1, focus 1) | behaviors: 0 added, 1 removed, 0 reworded, 0 moved, 0 changed | fail-on: removed
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
	if d.Summary.Incomplete > 0 {
		segments = append(segments, fmt.Sprintf("not compared: %d", d.Summary.Incomplete))
	}
	if len(v.FailOn) > 0 {
		segments = append(segments, "fail-on: "+strings.Join(v.FailOn, ","))
	}
	return strings.Join(segments, " | ")
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

// valueText is a finding's values: base → head, or the one there is.
func valueText(f *Finding) string {
	switch {
	case f.Base != "" && f.Head != "":
		return f.Base + " → " + f.Head
	case f.Head != "":
		return f.Head
	default:
		return f.Base
	}
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
	if d.Summary.Weakenings > 0 {
		fmt.Fprint(w, "\n#### Weakenings\n\n| Class | Package | Subject | Base | Head | Where |\n|---|---|---|---|---|---|\n")
		for _, f := range d.Findings {
			if !f.Weakens {
				continue
			}
			loc := ""
			if f.Location != nil && f.Location.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.Location.File, f.Location.Line)
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s |\n", f.Class, mdCell(shortPackage(d, f.Package)), mdCell(f.Subject), mdCell(f.Base), mdCell(f.Head), mdCell(loc))
		}
	}
	if len(d.Behaviors) > 0 {
		fmt.Fprint(w, "\n#### Behaviors\n\n| Change | Package | Behavior | Where |\n|---|---|---|---|\n")
		for _, c := range d.Behaviors {
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", c.Change, mdCell(shortPackage(d, c.Package)), mdCell(mdBehavior(&c)), mdCell(mdWhere(&c)))
		}
	}
	for _, list := range []struct {
		title string
		items []string
	}{{"Not compared", d.Incomplete}, {"Notes", d.Notes}} {
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
