package cli_test

import (
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLICoverageProfileTestSuite: a coverage profile is found in every spelling
// go test accepts, -coverprofile=file and -coverprofile file alike, by the
// run's --min gate and by summary.
type CLICoverageProfileTestSuite struct{}

func (s *CLICoverageProfileTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CLICoverageProfileTestSuite) TestProfileSpellings(t *gotest.T) {
	t.When("a fully covered package runs with a minimum and a profile", func(w *gotest.T) {
		w.It("finds the profile in every spelling go test accepts, for the --min gate and for summary", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "-coverprofile=file passes the gate", args: []string{"--min=1", "-coverprofile=PROFILE", "../testdata/covered/"}, exit: 0},
				{Desc: "-coverprofile file passes the gate", args: []string{"--min=1", "-coverprofile", "PROFILE", "../testdata/covered/"}, exit: 0},
				{Desc: "summary -coverprofile=file prints the coverage", args: []string{"summary", "-coverprofile=PROFILE", "../testdata/covered/"}, exit: 0, stdoutHas: "Coverage:"},
				{Desc: "summary -coverprofile file prints the coverage", args: []string{"summary", "-coverprofile", "PROFILE", "../testdata/covered/"}, exit: 0, stdoutHas: "Coverage:"},
				{Desc: "the gate still sees the profile when words follow -args", args: []string{"--min=50", "../testdata/covered/", "-args", "extra"}, exit: 0},
			}) {
				profile := filepath.Join(sub.TempDir(), "cover.out")
				args := make([]string, len(row.args))
				for i, a := range row.args {
					args[i] = strings.ReplaceAll(a, "PROFILE", profile)
				}
				run, err := testkit.RunCLI(sub.Context(), ".", args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				if row.stdoutHas != "" {
					gotest.Contains(sub, run.Stdout, row.stdoutHas)
				}
			}
		})
	})
}
