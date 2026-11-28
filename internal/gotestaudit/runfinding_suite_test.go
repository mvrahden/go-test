package gotestaudit_test

import (
	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditRunFindingTestSuite: the exit code of the run the audit made is a
// finding of its own.
type AuditRunFindingTestSuite struct{}

func (s *AuditRunFindingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *AuditRunFindingTestSuite) TestTheRunsExit(t *gotest.T) {
	all, err := gotestaudit.ParseClasses("all")
	gotest.NoError(t, err)
	type found struct {
		Class, Subject, Head string
		Weakens              bool
	}
	failed := func(head string) []found {
		return []found{{Class: "failing", Subject: "this audit's run", Head: head}}
	}
	for row, r := range gotest.Each(t, []struct {
		Desc                 string
		ran                  bool
		exit                 int
		findings             []found
		classes              map[string]int
		failing, none, onAll int
	}{
		{Desc: "a run that passed", ran: true, exit: 0, classes: map[string]int{}},
		{Desc: "a run that failed", ran: true, exit: 1, findings: failed("exited 1"), classes: map[string]int{"failing": 1}, failing: 1},
		{Desc: "a run that could not build", ran: true, exit: 2, findings: failed("exited 2"), classes: map[string]int{"failing": 1}, failing: 1},
		{Desc: "a run that was stopped", ran: true, exit: 130, classes: map[string]int{}},
		{Desc: "a saved stream", ran: false, exit: 1, classes: map[string]int{}},
	}) {
		rec := &gotestaudit.Record{Schema: gotestaudit.Schema, Notes: []string{gotestaudit.FloorNote, gotestaudit.SkipNote},
			Packages: []gotestaudit.Package{{Path: "m/p", Module: "m"}}}
		doc := gotestaudit.Compare(rec, rec)
		in := gotestaudit.StreamInput{Side: gotestaudit.SideHead, Ran: r.ran, Exit: r.exit}
		gotestaudit.AddEvidence(&doc, rec, rec, []gotestaudit.StreamInput{in})
		var got []found
		for i := range doc.Findings {
			f := &doc.Findings[i]
			got = append(got, found{Class: f.Class, Subject: f.Subject, Head: f.Head, Weakens: f.Weakens})
		}
		gotest.Equal(row, r.findings, got)
		gotest.Equal(row, r.classes, doc.Summary.Classes)
		gotest.Equal(row, r.failing, gotestaudit.ExitCode(&doc, []string{"failing"}))
		gotest.Equal(row, r.none, gotestaudit.ExitCode(&doc, nil))
		gotest.Equal(row, r.onAll, gotestaudit.ExitCode(&doc, all))
		if r.exit == 1 && r.ran {
			gotest.Equal(row, "c4ca12b4e80c2ee8", doc.Findings[0].ID)
			gotest.Equal(row, 0, doc.Summary.Weakenings)
			gotest.Equal(row, 0, doc.Summary.Informational)
			gotest.Equal(row, "gotest audit: PASS exit 0 | weakenings: 0 | behaviors: 0 added, 0 removed, 0 reworded, 0 moved, 0 changed | evidence: 0 (failing 0, not run 0, vacuous 0, stale 0) | run: exited 1",
				gotestaudit.ClosingLine(&doc, gotestaudit.Verdict{}))
		}
	}
}
