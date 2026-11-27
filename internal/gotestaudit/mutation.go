package gotestaudit

import (
	"cmp"
	"slices"
	"strconv"
	"time"
)

// ClassSurvived is the proof class of a changed line a test executed and a
// mutant of survived: no test noticed the line being wrong.
const ClassSurvived = "survived"

// Mutant verdicts.
const (
	MutantKilled       = "killed"
	MutantSurvived     = "survived"
	MutantTimeout      = "timeout"
	MutantNotJudged    = "not judged"
	MutantNotCovered   = "not covered"
	MutantIgnored      = "ignored"
	MutantUncompilable = "uncompilable"
)

// lineRank orders a line's verdicts: the one a reader acts on first.
var lineRank = []string{MutantSurvived, MutantTimeout, MutantNotJudged, MutantUncompilable, MutantNotCovered, MutantIgnored, MutantKilled}

// Mutation is what audit --mutate found of the changed lines: for each one
// holding a mutant, whether a test noticed it changed.
type Mutation struct {
	// Budget is the time the mutants were given, Spent the time they took.
	Budget string `json:"budget"`
	Spent  string `json:"spent"`
	// Jobs is how many mutants ran at a time.
	Jobs      int      `json:"jobs"`
	Operators []string `json:"operators"`
	// Baseline is what every mutant is judged against: the coverage run of
	// the tree, then its suites on the mutants' build with none on.
	Baseline MutationBaseline `json:"baseline"`
	Counts   MutationCounts   `json:"counts"`
	// Lines come in Order: survived, timed out, not judged, uncompilable,
	// not covered, ignored, then killed lines, each by file and line.
	Lines []MutatedLine `json:"lines"`
	// Ignores are the comments of the changed files that keep lines from
	// being mutated, those without a reason too, which keep none.
	Ignores []MutateIgnore `json:"ignores"`
}

// MutationBaseline is the run every mutant is judged against.
type MutationBaseline struct {
	Passed bool `json:"passed"`
	// Suites counts the suites it ran; Failed names those that failed.
	Suites int      `json:"suites"`
	Failed []string `json:"failed"`
}

// MutationCounts counts mutants by verdict, and the lines that hold them.
type MutationCounts struct {
	Mutants      int `json:"mutants"`
	Killed       int `json:"killed"`
	Survived     int `json:"survived"`
	Timeout      int `json:"timeout"`
	NotJudged    int `json:"notJudged"`
	NotCovered   int `json:"notCovered"`
	Ignored      int `json:"ignored"`
	Uncompilable int `json:"uncompilable"`
	Lines        int `json:"lines"`
	// SurvivedLines counts the lines with a survivor: the survived class.
	SurvivedLines int `json:"survivedLines"`
}

// MutatedLine is one changed line and its mutants.
type MutatedLine struct {
	// ID is made from the line's module, file, function, text and its place
	// among the function's lines of that text, never its number.
	ID      string `json:"id"`
	Module  string `json:"module,omitempty"`
	File    string `json:"file"`
	Package string `json:"package"`
	Line    int    `json:"line"`
	// Func is the function the line lies in; Group is its file and
	// function, the key a tool groups lines by.
	Func  string `json:"func,omitempty"`
	Group string `json:"group"`
	// Verdict is the line's worst: survived when any mutant survived.
	Verdict string `json:"verdict"`
	Order   int    `json:"order"`
	Killed  int    `json:"killed"`
	// Survived, Timeout, NotJudged and the rest count its mutants so.
	Survived     int `json:"survived"`
	Timeout      int `json:"timeout"`
	NotJudged    int `json:"notJudged"`
	NotCovered   int `json:"notCovered"`
	Ignored      int `json:"ignored"`
	Uncompilable int `json:"uncompilable"`
	// Suites are the suites that executed the line, as package and suite.
	Suites []string `json:"suites"`
	// Mutants come survivors first, then in the order they were judged.
	Mutants []MutantRow `json:"mutants"`
	// Reason is the ignore comment's reason, for an ignored line.
	Reason string `json:"reason,omitempty"`
}

// MutantRow is one mutant of a line.
type MutantRow struct {
	ID       string `json:"id"`
	Operator string `json:"operator"`
	Original string `json:"original"`
	// Mutant is the mutated text; empty for a statement removed.
	Mutant  string `json:"mutant"`
	Column  int    `json:"column"`
	Verdict string `json:"verdict"`
	// By is the test whose failure killed it.
	By string `json:"by,omitempty"`
}

// MutateIgnore is one //gotest:mutate-ignore comment of the tree.
type MutateIgnore struct {
	Module string `json:"module,omitempty"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	// Scope is line, statement or function; Start and End the lines kept.
	Scope string `json:"scope"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Func  string `json:"func,omitempty"`
	// Reason is mandatory: a comment without one keeps no line.
	Reason string `json:"reason"`
}

// MutantResult is one mutant as audit --mutate judged it.
type MutantResult struct {
	Module  string
	File    string // relative to the module root, slash-separated
	Package string
	Func    string
	Line    int
	Col     int
	// LineText is the line's source, for its id.
	LineText string
	Operator string
	Original string
	Mutant   string
	Verdict  string
	By       string
	// Suites executed the line; Reason is the comment that ignores it.
	Suites []string
	Reason string
}

// MutationRun is what the mutants ran under.
type MutationRun struct {
	Budget, Spent time.Duration
	Jobs          int
	Operators     []string
	Baseline      MutationBaseline
	Ignores       []MutateIgnore
}

// AddMutation sets the document's mutation section from each mutant's
// result, one entry per line, and counts the lines with a survivor as the
// survived class.
func AddMutation(d *Document, results []MutantResult, run *MutationRun) {
	m := &Mutation{Budget: run.Budget.String(), Spent: run.Spent.Round(time.Millisecond).String(), Jobs: run.Jobs,
		Operators: slices.Clone(run.Operators), Baseline: run.Baseline, Ignores: slices.Clone(run.Ignores)}
	if m.Operators == nil {
		m.Operators = []string{}
	}
	if m.Baseline.Failed == nil {
		m.Baseline.Failed = []string{}
	}
	if m.Ignores == nil {
		m.Ignores = []MutateIgnore{}
	}
	type key struct {
		module, file string
		line         int
	}
	at := map[key]int{}
	for i := range results {
		r := &results[i]
		k := key{r.Module, r.File, r.Line}
		idx, ok := at[k]
		if !ok {
			idx = len(m.Lines)
			at[k] = idx
			group := r.File
			if r.Func != "" {
				group += " " + r.Func
			}
			m.Lines = append(m.Lines, MutatedLine{Module: r.Module, File: r.File, Package: r.Package, Line: r.Line, Func: r.Func, Group: group,
				ID: stableID("mutated line", r.Module, r.File, r.Func, r.LineText), Suites: slices.Clone(r.Suites), Reason: r.Reason})
		}
		l := &m.Lines[idx]
		l.Mutants = append(l.Mutants, MutantRow{Operator: r.Operator, Original: r.Original, Mutant: r.Mutant, Column: r.Col, Verdict: r.Verdict, By: r.By})
		count(&m.Counts, l, r.Verdict)
	}
	// Lines of one text in one function are told apart by their place.
	seen := map[string]int{}
	for i := range m.Lines {
		l := &m.Lines[i]
		n := seen[l.ID]
		seen[l.ID]++
		if n > 0 {
			l.ID = stableID(l.ID, strconv.Itoa(n))
		}
		rows := map[string]int{}
		for j := range l.Mutants {
			mu := &l.Mutants[j]
			base := stableID(l.ID, mu.Operator, mu.Original, mu.Mutant)
			mu.ID = stableID(base, strconv.Itoa(rows[base]))
			rows[base]++
		}
		slices.SortStableFunc(l.Mutants, func(a, b MutantRow) int {
			return cmp.Compare(slices.Index(lineRank, a.Verdict), slices.Index(lineRank, b.Verdict))
		})
		l.Verdict = l.Mutants[0].Verdict
		if l.Suites == nil {
			l.Suites = []string{}
		}
		if l.Survived > 0 {
			m.Counts.SurvivedLines++
		}
	}
	m.Counts.Lines = len(m.Lines)
	slices.SortStableFunc(m.Lines, func(a, b MutatedLine) int {
		return cmp.Or(cmp.Compare(slices.Index(lineRank, a.Verdict), slices.Index(lineRank, b.Verdict)),
			cmp.Compare(a.Module, b.Module), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	for i := range m.Lines {
		m.Lines[i].Order = i
	}
	if m.Lines == nil {
		m.Lines = []MutatedLine{}
	}
	d.Mutation = m
	counts := m.Counts
	d.Summary.Mutation = &counts
	setClass(&d.Summary, ClassSurvived, m.Counts.SurvivedLines)
	d.normalize()
}

func count(c *MutationCounts, l *MutatedLine, verdict string) {
	c.Mutants++
	switch verdict {
	case MutantKilled:
		c.Killed++
		l.Killed++
	case MutantSurvived:
		c.Survived++
		l.Survived++
	case MutantTimeout:
		c.Timeout++
		l.Timeout++
	case MutantNotJudged:
		c.NotJudged++
		l.NotJudged++
	case MutantNotCovered:
		c.NotCovered++
		l.NotCovered++
	case MutantIgnored:
		c.Ignored++
		l.Ignored++
	case MutantUncompilable:
		c.Uncompilable++
		l.Uncompilable++
	}
}
