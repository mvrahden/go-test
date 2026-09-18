package main_test

import (
	"os"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CmdEnvTestSuite covers what the CLI reads from its own environment and what it
// leaves in the environment of the CLIs the suites in this package spawn.
// Sequential: Setenv.
//
//nolint:lifecycle-pair // BeforeAll only unsets variables that must stay unset for the whole package run
type CmdEnvTestSuite struct{}

func (s *CmdEnvTestSuite) BeforeAll(t *gotest.T) {
	testkit.ScrubActionsEnv()
}

func (s *CmdEnvTestSuite) TestDetectCIEnv(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct { //nolint:gocritic // rangeValCopy: intentional
		Desc     string
		gotestCI string
		ci       string
		expect   bool
	}{
		{Desc: "unset both", gotestCI: "", ci: "", expect: false},
		{Desc: "GOTEST_CI=1", gotestCI: "1", ci: "", expect: true},
		{Desc: "GOTEST_CI=true", gotestCI: "true", ci: "", expect: true},
		{Desc: "typo'd opt-in stays on", gotestCI: "yes", ci: "", expect: true},
		{Desc: "GOTEST_CI=0 opts out of CI env", gotestCI: "0", ci: "true", expect: false},
		{Desc: "GOTEST_CI=false opts out", gotestCI: "false", ci: "1", expect: false},
		{Desc: "CI set", gotestCI: "", ci: "true", expect: true},
		{Desc: "CI=false is not CI", gotestCI: "", ci: "false", expect: false},
		{Desc: "CI=0 is not CI", gotestCI: "", ci: "0", expect: false},
	}) {
		sub.Setenv("GOTEST_CI", tc.gotestCI)
		sub.Setenv("CI", tc.ci)
		gotest.Equal(sub, tc.expect, ExportDetectCIEnv())
	}
}

func (s *CmdEnvTestSuite) TestChildEnvironment(t *gotest.T) {
	t.It("carries no GitHub Actions variables for spawned CLIs to inherit", func(it *gotest.T) {
		_, actions := os.LookupEnv("GITHUB_ACTIONS")
		_, summary := os.LookupEnv("GITHUB_STEP_SUMMARY")

		gotest.False(it, actions, "GITHUB_ACTIONS still set")
		gotest.False(it, summary, "GITHUB_STEP_SUMMARY still set")
	})
}
