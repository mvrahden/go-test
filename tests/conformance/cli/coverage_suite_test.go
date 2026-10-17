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
		for it, row := range gotest.Each(w, []flagRow{
			{Desc: "-coverprofile=file passes the gate", args: []string{"--min=1", "-coverprofile=PROFILE", "../testdata/covered/"}, exit: 0},
			{Desc: "-coverprofile file passes the gate", args: []string{"--min=1", "-coverprofile", "PROFILE", "../testdata/covered/"}, exit: 0},
			{Desc: "summary -coverprofile=file prints the coverage", args: []string{"summary", "-coverprofile=PROFILE", "../testdata/covered/"}, exit: 0, stdoutHas: "Coverage:"},
			{Desc: "summary -coverprofile file prints the coverage", args: []string{"summary", "-coverprofile", "PROFILE", "../testdata/covered/"}, exit: 0, stdoutHas: "Coverage:"},
		}) {
			profile := filepath.Join(it.TempDir(), "cover.out")
			args := make([]string, len(row.args))
			for i, a := range row.args {
				args[i] = strings.ReplaceAll(a, "PROFILE", profile)
			}
			run, err := testkit.RunCLI(it.Context(), ".", args...)
			gotest.NoError(it, err)
			gotest.Equal(it, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			if row.stdoutHas != "" {
				gotest.Contains(it, run.Stdout, row.stdoutHas)
			}
		}
	})
}
