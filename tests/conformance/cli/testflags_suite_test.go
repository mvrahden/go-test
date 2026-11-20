package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLITestFlagsTestSuite: the test binary's flags are the testing package's.
type CLITestFlagsTestSuite struct{}

func (s *CLITestFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLITestFlagsTestSuite) TestFlagsOfTheTestingPackage(t *gotest.T) {
	t.When("-artifacts and -mutexprofilefraction=5 are given to a run", func(w *gotest.T) {
		w.It("takes both, and the run passes with exit 0", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "-artifacts", "-mutexprofilefraction=5", "-outputdir", it.TempDir(), "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `(?m)^ok\s`, run.Stdout)
		})
	})
	t.When("-mutexprofilerate=5 is given to a run", func(w *gotest.T) {
		w.It("refuses it with exit 2 as an unknown flag, before anything is built", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "-mutexprofilerate=5", "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "unknown flag: -mutexprofilerate")
			gotest.NotContains(it, run.Stdout, "ok")
		})
	})
}
