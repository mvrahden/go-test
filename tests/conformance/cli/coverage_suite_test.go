package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLICoverageProfileTestSuite: a coverage profile is found in every spelling
// go test accepts, -coverprofile=file and -coverprofile file alike, by the
// run's --min gate and by summary.
type CLICoverageProfileTestSuite struct{}

func (s *CLICoverageProfileTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
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

func (s *CLICoverageProfileTestSuite) TestABlockTwoSuitesRun(t *gotest.T) {
	profile := filepath.Join(t.TempDir(), "c.out")
	run, err := testkit.RunCLI(t.Context(), ".", "-covermode=count", "-coverprofile="+profile, "../testdata/clicovercount/")
	gotest.NoError(t, err)
	data, readErr := os.ReadFile(profile)
	t.When("two suites of a package each run a block once under -covermode=count", func(w *gotest.T) {
		w.It("the profile counts the block twice", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.NoError(it, readErr)
			gotest.Regexp(it, `(?m)clicovercount\.go:\d+\.\d+,\d+\.\d+ 1 2$`, string(data))
			// Each suite process initialises the package: go test's one process would read 1.
			gotest.Regexp(it, `(?m)clicovercount_init\.go:4\.\d+,\d+\.\d+ 1 2$`, string(data))
		})
	})
}
