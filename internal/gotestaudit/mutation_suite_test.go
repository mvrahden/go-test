package gotestaudit_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditMutationTestSuite: a mutation section built from mutants judged by
// hand, its counts and order, and how text and Markdown write it.
type AuditMutationTestSuite struct{}

func (s *AuditMutationTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

const suite = "m/calc TestCalcTestSuite"

// result is one mutant of m/calc's calc.go.
func result(line int, fn, op, orig, mut, verdict string) gotestaudit.MutantResult {
	r := gotestaudit.MutantResult{Module: "m", File: "calc/calc.go", Package: "m/calc", Func: fn, Line: line, Col: 2,
		LineText: "line " + string(rune('a'+line%26)), Operator: op, Original: orig, Mutant: mut, Verdict: verdict}
	if verdict != gotestaudit.MutantNotCovered && verdict != gotestaudit.MutantIgnored {
		r.Suites = []string{suite}
	}
	if verdict == gotestaudit.MutantKilled {
		r.By = suite + "/TestX"
	}
	return r
}

// judged is a change of one file: a survivor beside a kill, a timeout, a
// line killed whole, one no test ran and one a comment keeps.
func judged() []gotestaudit.MutantResult {
	ignored := result(23, "Sum", "boundary", "i > 0", "i >= 0", gotestaudit.MutantIgnored)
	ignored.Reason = "adds nothing at 0"
	return []gotestaudit.MutantResult{
		result(6, "Grade", "negate-compare", "s >= 50", "s < 50", gotestaudit.MutantKilled),
		result(14, "Discount", "boundary", "t > 100", "t >= 100", gotestaudit.MutantSurvived),
		result(14, "Discount", "negate-compare", "t > 100", "t <= 100", gotestaudit.MutantKilled),
		result(25, "Sum", "delete-stmt", "i--", "", gotestaudit.MutantTimeout),
		result(32, "Unused", "literal", "0", "1", gotestaudit.MutantNotCovered),
		ignored,
	}
}

func document(results []gotestaudit.MutantResult, base gotestaudit.MutationBaseline) *gotestaudit.Document {
	d := &gotestaudit.Document{Modules: []string{"m"}}
	gotestaudit.AddMutation(d, results, &gotestaudit.MutationRun{Budget: 3 * time.Minute, Spent: 41234 * time.Millisecond, Jobs: 3,
		Operators: []string{"boundary"}, Baseline: base})
	return d
}

func passed() gotestaudit.MutationBaseline {
	return gotestaudit.MutationBaseline{Passed: true, Suites: 1}
}

func (s *AuditMutationTestSuite) TestTheSection(t *gotest.T) {
	d := document(judged(), passed())
	m := d.Mutation
	t.When("mutants of a file are added", func(w *gotest.T) {
		w.It("counts them by verdict and the lines with a survivor as the survived class", func(it *gotest.T) {
			want := gotestaudit.MutationCounts{Mutants: 6, Killed: 2, Survived: 1, Timeout: 1, NotCovered: 1, Ignored: 1, Lines: 5, SurvivedLines: 1}
			gotest.Equal(it, want, m.Counts)
			gotest.Equal(it, &want, d.Summary.Mutation)
			gotest.Equal(it, 1, d.Summary.Classes[gotestaudit.ClassSurvived])
			gotest.Equal(it, []string{"3m0s", "41.234s"}, []string{m.Budget, m.Spent})
		})
		w.It("orders the lines survived, timeout, not covered, ignored, killed and numbers them", func(it *gotest.T) {
			var got []string
			for i := range m.Lines {
				l := &m.Lines[i]
				gotest.Equal(it, i, l.Order)
				got = append(got, strings.Join([]string{l.Verdict, l.Group}, " @ "))
			}
			gotest.Equal(it, []string{
				"survived @ calc/calc.go Discount",
				"timeout @ calc/calc.go Sum",
				"not covered @ calc/calc.go Unused",
				"ignored @ calc/calc.go Sum",
				"killed @ calc/calc.go Grade",
			}, got)
		})
		w.It("puts a line's survivor first and gives each line and mutant an id of its own", func(it *gotest.T) {
			discount := m.Lines[0]
			gotest.Equal(it, []string{gotestaudit.MutantSurvived, gotestaudit.MutantKilled}, []string{discount.Mutants[0].Verdict, discount.Mutants[1].Verdict})
			gotest.Equal(it, []int{1, 1}, []int{discount.Survived, discount.Killed})
			ids := map[string]bool{}
			for i := range m.Lines {
				l := &m.Lines[i]
				ids[l.ID] = true
				for _, mu := range l.Mutants {
					ids[mu.ID] = true
				}
			}
			gotest.Len(it, ids, 11)
			gotest.Equal(it, "adds nothing at 0", m.Lines[3].Reason)
			gotest.Empty(it, m.Lines[2].Suites)
			gotest.NotNil(it, m.Lines[2].Suites)
		})
	})
	t.When("a line moves", func(w *gotest.T) {
		w.It("keeps its id", func(it *gotest.T) {
			moved := judged()
			for i := range moved {
				moved[i].Line += 3
			}
			gotest.Equal(it, m.Lines[0].ID, document(moved, passed()).Mutation.Lines[0].ID)
		})
	})
	t.When("two lines of one function hold the same text", func(w *gotest.T) {
		w.It("tells them apart", func(it *gotest.T) {
			a, b := result(3, "F", "literal", "0", "1", gotestaudit.MutantKilled), result(9, "F", "literal", "0", "1", gotestaudit.MutantKilled)
			b.LineText = a.LineText
			lines := document([]gotestaudit.MutantResult{a, b}, passed()).Mutation.Lines
			gotest.NotEqual(it, lines[0].ID, lines[1].ID)
		})
	})
	t.When("no mutant survives", func(w *gotest.T) {
		w.It("counts no survived class", func(it *gotest.T) {
			d := document(judged()[:1], passed())
			_, has := d.Summary.Classes[gotestaudit.ClassSurvived]
			gotest.False(it, has)
			gotest.True(it, gotestaudit.IsProof(gotestaudit.ClassSurvived))
		})
	})
}

func render(d *gotestaudit.Document, verbose bool) (text, md string) {
	var tb, mb bytes.Buffer
	gotestaudit.RenderText(&tb, d, gotestaudit.Verdict{Verbose: verbose})
	gotestaudit.RenderMarkdown(&mb, d, gotestaudit.Verdict{})
	return tb.String(), mb.String()
}

// section is the text from the mutation heading to the blank line after.
func section(text string) string {
	_, rest, _ := strings.Cut(text, "\nmutation: ")
	rest, _, _ = strings.Cut(rest, "\n\n")
	return "mutation: " + rest
}

func (s *AuditMutationTestSuite) TestTheRenderings(t *gotest.T) {
	d := document(judged(), passed())
	text, md := render(d, false)
	t.When("the section is written as text", func(w *gotest.T) {
		w.It("lists per file the lines to act on, names their shared suites once and counts the rest", func(it *gotest.T) {
			gotest.Equal(it, strings.Join([]string{
				"mutation: 1 survived on 1 line, 2 killed, 1 timeout, 0 not judged, 1 not covered, 1 ignored",
				"  6 mutants on 5 lines; budget 3m0s, 41.234s spent, 3 at a time",
				"  calc/calc.go · ran by ./calc TestCalcTestSuite",
				"    14    survived   t > 100 → t >= 100 (boundary); 1 killed · Discount",
				"    25    timeout    i-- removed (delete-stmt) · Sum",
				"    3 more lines: 1 killed, 1 not covered, 1 ignored",
			}, "\n"), section(text))
		})
		w.It("ends with a closing line counting the mutants", func(it *gotest.T) {
			gotest.Equal(it, "gotest audit: PASS exit 0 | weakenings: 0 | behaviors: 0 added, 0 removed, 0 reworded, 0 moved, 0 changed | mutation: 1 survived on 1 line, 2 killed, 1 timeout, 0 not judged",
				gotestaudit.ClosingLine(d, gotestaudit.Verdict{}))
		})
	})
	t.When("every line is asked for with -v", func(w *gotest.T) {
		w.It("lists each line, the suites on each when they differ", func(it *gotest.T) {
			verbose, _ := render(d, true)
			gotest.Equal(it, strings.Join([]string{
				"mutation: 1 survived on 1 line, 2 killed, 1 timeout, 0 not judged, 1 not covered, 1 ignored",
				"  6 mutants on 5 lines; budget 3m0s, 41.234s spent, 3 at a time",
				"  calc/calc.go",
				"    14    survived   t > 100 → t >= 100 (boundary); 1 killed · Discount · ran by ./calc TestCalcTestSuite",
				"    25    timeout    i-- removed (delete-stmt) · Sum · ran by ./calc TestCalcTestSuite",
				"    32    not covered 1 mutants · Unused",
				"    23    ignored    adds nothing at 0 · Sum",
				"    6     killed     1 mutants · Grade · ran by ./calc TestCalcTestSuite",
			}, "\n"), section(verbose))
		})
	})
	t.When("the section is written as Markdown", func(w *gotest.T) {
		w.It("tables the lines to act on and counts the others", func(it *gotest.T) {
			gotest.Contains(it, md, "#### Mutation\n\n1 survived on 1 line, 2 killed, 1 timeout, 0 not judged, 1 not covered, 1 ignored\n\n6 mutants on 5 lines; budget 3m0s, 41.234s spent, 3 at a time\n\n"+
				"| File | Line | Verdict | Mutants | Function | Ran by | Killed |\n|---|---|---|---|---|---|---|\n"+
				"| calc/calc.go | 14 | survived | t &gt; 100 → t &gt;= 100 (boundary) | Discount | ./calc TestCalcTestSuite | 1 |\n"+
				"| calc/calc.go | 25 | timeout | i-- removed (delete-stmt) | Sum | ./calc TestCalcTestSuite | 0 |\n\n"+
				"3 other lines: every mutant killed, not covered or ignored.\n")
		})
	})
	t.When("no mutant was judged for a failing baseline", func(w *gotest.T) {
		failed := document([]gotestaudit.MutantResult{result(6, "Grade", "literal", "0", "1", gotestaudit.MutantNotJudged)},
			gotestaudit.MutationBaseline{Failed: []string{suite}})
		text, md := render(failed, false)
		w.It("says which test failed instead of listing lines", func(it *gotest.T) {
			gotest.Equal(it, "mutation: 0 survived on 0 lines, 0 killed, 0 timeout, 1 not judged\n  1 mutants on 1 lines; budget 3m0s, 41.234s spent, 3 at a time\n"+
				"  no mutant judged: m/calc TestCalcTestSuite failed before any mutant was switched on", section(text))
			gotest.Contains(it, md, "\nNo mutant judged: m/calc TestCalcTestSuite failed before any mutant was switched on.\n")
			gotest.NotContains(it, md, "| File | Line |")
		})
	})
	t.When("a line holds an uncompilable mutant and one not judged", func(w *gotest.T) {
		d := document([]gotestaudit.MutantResult{
			result(4, "", "literal", "0", "1", gotestaudit.MutantUncompilable),
			result(8, "F", "literal", "1", "0", gotestaudit.MutantNotJudged),
		}, passed())
		text, _ := render(d, false)
		w.It("lists both, the one not judged first", func(it *gotest.T) {
			gotest.Equal(it, []int{1, 1}, []int{d.Mutation.Counts.Uncompilable, d.Mutation.Counts.NotJudged})
			gotest.Contains(it, section(text), "mutation: 0 survived on 0 lines, 0 killed, 0 timeout, 1 not judged, 1 uncompilable\n")
			gotest.Contains(it, section(text), "    8     not judged 1 → 0 (literal) · F\n    4     uncompilable 0 → 1 (literal)")
		})
	})
}
