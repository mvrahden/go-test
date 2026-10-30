package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIFocusTestSuite: --ci refuses a run holding a focused suite or method of
// any kind.
type CLIFocusTestSuite struct{}

func (s *CLIFocusTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLIFocusTestSuite) TestFocusGuard(t *gotest.T) {
	t.When("--ci runs a package whose only focused method is a benchmark", func(w *gotest.T) {
		w.It("fails with exit 1, naming the benchmark where it is declared", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "--ci", "-json", "../testdata/focusbench/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `FAIL: focus prefix detected — remove F_ before merging:\n\s*\S*focusbench_suite_test\.go:10\s+LoopTestSuite\.F_BenchmarkLoop\n`, run.Stderr)
			gotest.Empty(it, run.Verdict("TestLoopTestSuite/TestPlain"))
		})
	})
	t.When("--ci runs a package whose only focused method is a fuzz target", func(w *gotest.T) {
		w.It("fails with exit 1, naming the fuzz target where it is declared", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "--ci", "-json", "../testdata/focusfuzz/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `FAIL: focus prefix detected — remove F_ before merging:\n\s*\S*focusfuzz_suite_test\.go:10\s+SeedsTestSuite\.F_FuzzSeeds\n`, run.Stderr)
			gotest.Empty(it, run.Verdict("TestSeedsTestSuite/TestPlain"))
		})
	})
}
