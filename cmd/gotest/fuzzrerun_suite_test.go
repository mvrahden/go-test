package main_test

import (
	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzRerunVerdictTestSuite holds triage to what a crasher re-run actually
// proved: only a process that chose its own exit status says whether the
// crasher still fails.
type FuzzRerunVerdictTestSuite struct{}

func (s *FuzzRerunVerdictTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FuzzRerunVerdictTestSuite) TestClassifyRerun(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc   string
		status int
		ran    bool
		state  string
		detail string
	}{
		{Desc: "the re-run passed", status: 0, ran: true, state: "fixed"},
		{Desc: "the re-run failed", status: 1, ran: true, state: "failing"},
		{
			Desc: "a signal killed the re-run", status: -1, ran: true,
			state: "unverified", detail: "the re-run was terminated by signal",
		},
		{
			// Windows reports the termination status itself, and read as a
			// verdict it would name a cause triage never observed.
			Desc: "the system terminated the re-run", status: 0xC000013A, ran: true,
			state: "unverified", detail: "the re-run was terminated by the system (status 0xC000013A)",
		},
		{
			Desc: "the re-run never started", ran: false,
			state: "unverified", detail: "the re-run never started",
		},
	}) {
		state, detail := main.ExportClassifyRerun(tc.status, tc.ran)
		gotest.Equal(sub, tc.state, state)
		gotest.Equal(sub, tc.detail, detail)
	}
}
