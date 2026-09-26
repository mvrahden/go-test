package main_test

import (
	"os"
	"path/filepath"
	"regexp"

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
	gotest.Equal(t, withoutDurations(first), withoutDurations(second))
}

// A table row ends in a wall-clock duration, which two runs never share
// exactly (Windows rounds a short one to 1ms as easily as to <1ms); the
// order of suites and rows is what the spec must hold stable.
var durationCell = regexp.MustCompile(`(?m)^(\|.*\|) [^|]+ \|$`)

func withoutDurations(markdown string) string {
	return durationCell.ReplaceAllString(markdown, "$1")
}
