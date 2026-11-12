package cli_test

import (
	"path/filepath"
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

func (s *CLIFocusTestSuite) TestAFocusedMethodOutsideCIMode(t *gotest.T) {
	const note = `note: F_ narrows this run to the focused suites and methods (1, first ../testdata/focused/focused_suite_test.go:9 FocusedTestSuite.F_TestOne) — the rest is skipped; remove F_ to run everything, --ci refuses it` + "\n"
	plain, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOTEST_CI=0"}, "../testdata/focused/")
	gotest.NoError(t, err)
	stream, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOTEST_CI=0"}, "-json", "../testdata/focused/")
	gotest.NoError(t, err)
	unfocused, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOTEST_CI=0"}, "../testdata/passing/")
	gotest.NoError(t, err)
	t.When("a run outside CI mode holds a focused method", func(w *gotest.T) {
		w.It("notes on stderr that F_ narrows the run, naming the method where it is declared, in the plain and -json modes", func(it *gotest.T) {
			gotest.Contains(it, filepath.ToSlash(plain.Stderr), note)
			gotest.Contains(it, filepath.ToSlash(stream.Stderr), note)
			gotest.NotContains(it, unfocused.Stderr, "F_ narrows")
		})
		w.It("exits 0 when what ran passed", func(it *gotest.T) {
			gotest.Equal(it, 0, plain.Exit)
			gotest.Equal(it, 0, stream.Exit)
		})
	})
}
