package main_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// SpecOrderTestSuite renders one package's spec twice: a written spec must
// not churn with the order the suites happened to finish in.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type SpecOrderTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *SpecOrderTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *SpecOrderTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

func (s *SpecOrderTestSuite) TestMarkdownIsStableAcrossRuns(t *gotest.T) {
	dir := t.TempDir()
	render := func(name string) string {
		out := filepath.Join(dir, name)
		log, code := s.cli.runExit(t, "spec", "--format=md", "--output="+out, "./tests/sharedfixture/standalone/")
		gotest.Equal(t, 0, code, log)
		got, err := os.ReadFile(out)
		gotest.NoError(t, err)
		return string(got)
	}
	first := render("first.md")
	second := render("second.md")
	gotest.Equal(t, first, second)
}
