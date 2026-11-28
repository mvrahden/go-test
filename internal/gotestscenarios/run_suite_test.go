package gotestscenarios_test

import (
	"github.com/mvrahden/go-test/internal/gotestscenarios"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScenariosRunVerdictTestSuite: the exit code of the run --verify made is
// judged beside the scenarios.
type ScenariosRunVerdictTestSuite struct{}

func (s *ScenariosRunVerdictTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func code(n int) *int { return &n }

func (s *ScenariosRunVerdictTestSuite) TestTheRunsExit(t *gotest.T) {
	for row, r := range gotest.Each(t, []struct {
		Desc              string
		ev                string
		code              *int
		failed            bool
		byDefault, notRun int
		closing           string
	}{
		{Desc: "its own run failed", ev: gotestscenarios.EvidenceRun, code: code(1), failed: true, byDefault: 1,
			closing: "gotest scenarios: FAIL exit 1 | scenarios: 0 | evidence: run, exited 1 | fail-on: ambiguous,failing"},
		{Desc: "its own run was stopped", ev: gotestscenarios.EvidenceRun, code: code(130), failed: true, byDefault: 1,
			closing: "gotest scenarios: FAIL exit 1 | scenarios: 0 | evidence: run, exited 130 | fail-on: ambiguous,failing"},
		{Desc: "its own run passed", ev: gotestscenarios.EvidenceRun, code: code(0),
			closing: "gotest scenarios: PASS exit 0 | scenarios: 0 | evidence: run | fail-on: ambiguous,failing"},
		{Desc: "a saved stream of a failed run", ev: gotestscenarios.EvidenceInput, code: code(1),
			closing: "gotest scenarios: PASS exit 0 | scenarios: 0 | evidence: input | fail-on: ambiguous,failing"},
		{Desc: "a stream the run did not close", ev: gotestscenarios.EvidenceRun,
			closing: "gotest scenarios: PASS exit 0 | scenarios: 0 | evidence: run | fail-on: ambiguous,failing"},
	}) {
		doc := gotestscenarios.Bind(nil, nil, &gotestscenarios.Options{Evidence: r.ev, Run: &gotestspec.RunInfo{ExitCode: r.code}})
		gotest.NotZero(row, doc.Run)
		gotest.Equal(row, r.failed, doc.Run.Failed)
		gotest.Equal(row, r.byDefault, gotestscenarios.ExitCode(&doc, gotestscenarios.DefaultFailOn))
		gotest.Equal(row, r.notRun, gotestscenarios.ExitCode(&doc, []string{gotestscenarios.StatusNotRun}))
		gotest.Equal(row, r.closing, gotestscenarios.ClosingLine(&doc, gotestscenarios.Verdict{ExitCode: r.byDefault, FailOn: gotestscenarios.DefaultFailOn}))
	}
}
