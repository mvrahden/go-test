package gotestscenarios

import (
	"fmt"
	"io"
	"strings"
)

// countOrder is the order the closing line counts statuses in.
var countOrder = []string{StatusBound, StatusVerified, StatusFailing, StatusNotRun, StatusVacuous, StatusUnbound, StatusAmbiguous}

// RenderText writes the scenarios that are not bound or verified, each
// with the clauses at fault, then the unstated Its, what could not be
// read, and the closing line.
func RenderText(w io.Writer, d *Document, v Verdict) {
	for _, status := range Classes {
		for i := range d.Scenarios {
			sc := &d.Scenarios[i]
			if sc.Status != status {
				continue
			}
			fmt.Fprintf(w, "%-10s %s:%d  %s / %s\n", status, sc.File, sc.Line, sc.Requirement, sc.Title)
			for _, why := range faults(sc) {
				fmt.Fprintf(w, "             %s\n", why)
			}
		}
	}
	for _, p := range d.Unstated {
		fmt.Fprintf(w, "%-10s %s:%d  %s\n", ClassUnstated, p.File, p.Line, strings.Join(p.Path, " / "))
	}
	if n := len(d.Incomplete); n > 0 {
		fmt.Fprintf(w, "incomplete %d methods declare behaviors source cannot list; a scenario they state reads as unbound (--format=json names them)\n", n)
	}
	for _, pkg := range d.Unreadable {
		fmt.Fprintf(w, "unreadable %s\n", pkg)
	}
	fmt.Fprintln(w, ClosingLine(d, v))
}

// faults says, per clause at fault, what is wrong with it.
func faults(sc *Scenario) []string {
	if sc.Problem != "" {
		return []string{"cannot bind: " + sc.Problem}
	}
	if len(sc.Whens) == 0 {
		var conds []string
		for _, c := range sc.Conditions {
			conds = append(conds, c.Keyword+" "+c.Text)
		}
		return []string{strings.Join(conds, ", ") + ": no When with an It directly under it says it"}
	}
	var out []string
	for _, c := range sc.Clauses {
		head := c.Keyword + " " + c.Text + ": "
		switch c.Status {
		case StatusUnbound:
			out = append(out, head+"no It under the When says it")
		case StatusAmbiguous:
			if len(c.Its) > 1 {
				out = append(out, head+fmt.Sprintf("%d Its say it: %s", len(c.Its), places(c.Its)))
			} else {
				out = append(out, head+"another scenario's clause binds the same It: "+places(c.Its))
			}
		case StatusFailing:
			out = append(out, head+"failed at "+places(c.Its))
		case StatusNotRun:
			out = append(out, head+"not run at "+places(c.Its))
		case StatusVacuous:
			out = append(out, head+"passed with no assertion at "+places(c.Its))
		}
	}
	return out
}

func places(ps []Place) string {
	var out []string
	for _, p := range ps {
		out = append(out, fmt.Sprintf("%s:%d", p.File, p.Line))
	}
	return strings.Join(out, ", ")
}

// ClosingLine is the text format's last line:
//
//	gotest scenarios: FAIL exit 1 | scenarios: 3 (1 verified, 1 failing, 1 unbound) | evidence: input | fail-on: ambiguous,failing
func ClosingLine(d *Document, v Verdict) string {
	verdict := "PASS"
	if v.ExitCode != 0 {
		verdict = "FAIL"
	}
	var counts []string
	for _, s := range countOrder {
		if n := d.Summary.Statuses[s]; n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, s))
		}
	}
	scenarios := fmt.Sprintf("scenarios: %d", d.Summary.Scenarios)
	if len(counts) > 0 {
		scenarios += " (" + strings.Join(counts, ", ") + ")"
	}
	segments := []string{fmt.Sprintf("gotest scenarios: %s exit %d", verdict, v.ExitCode), scenarios}
	if d.Summary.Unstated > 0 {
		segments = append(segments, fmt.Sprintf("unstated: %d", d.Summary.Unstated))
	}
	if n := len(d.Unreadable); n > 0 {
		segments = append(segments, fmt.Sprintf("unreadable: %d", n))
	}
	segments = append(segments, "evidence: "+d.Evidence)
	if len(v.FailOn) > 0 {
		segments = append(segments, "fail-on: "+strings.Join(v.FailOn, ","))
	}
	return strings.Join(segments, " | ")
}

// RenderMarkdown writes a heading, the closing line, and per capability a
// table of its requirements and scenarios.
func RenderMarkdown(w io.Writer, d *Document, v Verdict) {
	fmt.Fprintf(w, "## Scenarios\n\n%s\n", mdText(ClosingLine(d, v)))
	file := ""
	for i := range d.Scenarios {
		sc := &d.Scenarios[i]
		if sc.File != file {
			file = sc.File
			fmt.Fprintf(w, "\n### %s\n\n`%s`\n\n| Requirement | Scenario | Status | Its |\n|---|---|---|---|\n", mdText(sc.Capability), sc.File)
		}
		var its []string
		for _, c := range sc.Clauses {
			for _, p := range c.Its {
				its = append(its, fmt.Sprintf("`%s:%d`", p.File, p.Line))
			}
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", mdText(sc.Requirement), mdText(sc.Title), sc.Status, strings.Join(its, ", "))
	}
	if len(d.Unstated) > 0 {
		fmt.Fprintf(w, "\n### Unstated\n\n| It | Declared |\n|---|---|\n")
		for _, p := range d.Unstated {
			fmt.Fprintf(w, "| %s | `%s:%d` |\n", mdText(strings.Join(p.Path, " / ")), p.File, p.Line)
		}
	}
}

// mdText escapes what would break a table cell or start Markdown syntax.
func mdText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\`*_[]<>|#&", r) {
			b.WriteByte('\\')
		}
		if r == '\n' || r == '\r' {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}
