package cli_test

import (
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIFocusReportTestSuite: spec and summary run what a run runs, so they say
// that F_ narrows it as a run does.
type CLIFocusReportTestSuite struct{}

func (s *CLIFocusReportTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const focusNote = `note: F_ narrows this run to the focused suites and methods (1, first ../testdata/focused/focused_suite_test.go:9 FocusedTestSuite.F_TestOne) — the rest is skipped; remove F_ to run everything, --ci refuses it` + "\n"

func (s *CLIFocusReportTestSuite) TestAFocusedMethodUnderSpecAndSummary(t *gotest.T) {
	t.When("spec or summary runs, outside CI mode, a package holding a focused method", func(w *gotest.T) {
		w.It("spec notes on stderr that F_ narrows the run, naming the method where it is declared, and exits 0", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GOTEST_CI=0"}, "spec", "../testdata/focused/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, filepath.ToSlash(run.Stderr), focusNote)
		})
		w.It("summary notes on stderr that F_ narrows the run, naming the method where it is declared, and exits 0", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GOTEST_CI=0"}, "summary", "../testdata/focused/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, filepath.ToSlash(run.Stderr), focusNote)
		})
	})
}
