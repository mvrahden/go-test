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
		in                   gotestaudit.StreamInput
		findings             []found
		classes              map[string]int
		failing, none, onAll int
	}{
		{Desc: "a run that passed", in: gotestaudit.StreamInput{Ran: true, Exit: 0}, classes: map[string]int{}},
		{Desc: "a run that failed", in: gotestaudit.StreamInput{Ran: true, Exit: 1}, findings: failed("exited 1"), classes: map[string]int{"failing": 1}, failing: 1},
		{Desc: "a run that could not build", in: gotestaudit.StreamInput{Ran: true, Exit: 2}, findings: failed("exited 2"), classes: map[string]int{"failing": 1}, failing: 1},
		{Desc: "a run that was stopped", in: gotestaudit.StreamInput{Ran: true, Exit: 130}, classes: map[string]int{}},
		{Desc: "a saved stream", in: gotestaudit.StreamInput{Ran: false, Exit: 1}, classes: map[string]int{}},
	}) {
		rec := &gotestaudit.Record{Schema: gotestaudit.Schema, Notes: []string{gotestaudit.FloorNote, gotestaudit.SkipNote},
			Packages: []gotestaudit.Package{{Path: "m/p", Module: "m"}}}
		doc := gotestaudit.Compare(rec, rec)
		in := r.in
		in.Side = gotestaudit.SideHead
		gotestaudit.AddEvidence(&doc, rec, rec, []gotestaudit.StreamInput{in})
		var got []found
		for _, f := range doc.Findings {
			got = append(got, found{Class: f.Class, Subject: f.Subject, Head: f.Head, Weakens: f.Weakens})
		}
		gotest.Equal(row, r.findings, got)
		gotest.Equal(row, r.classes, doc.Summary.Classes)
		gotest.Equal(row, r.failing, gotestaudit.ExitCode(&doc, []string{"failing"}))
		gotest.Equal(row, r.none, gotestaudit.ExitCode(&doc, nil))
		gotest.Equal(row, r.onAll, gotestaudit.ExitCode(&doc, all))
		if r.in.Exit == 1 && r.in.Ran {
			gotest.Equal(row, "c4ca12b4e80c2ee8", doc.Findings[0].ID)
			gotest.Equal(row, 0, doc.Summary.Weakenings)
			gotest.Equal(row, 0, doc.Summary.Informational)
			gotest.Equal(row, "gotest audit: PASS exit 0 | weakenings: 0 | behaviors: 0 added, 0 removed, 0 reworded, 0 moved, 0 changed | evidence: 0 (failing 0, not run 0, vacuous 0, stale 0) | run: exited 1",
				gotestaudit.ClosingLine(&doc, gotestaudit.Verdict{}))
		}
	}
}
