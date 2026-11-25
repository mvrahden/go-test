package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIVacuousTestSuite: a test that passed without an assertion is named in
// every run, fails a run under CI mode, and is counted where the run closes.
type CLIVacuousTestSuite struct{}

func (s *CLIVacuousTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

const (
	vacuousPkg  = "../testdata/assertcount/"
	vacuousTest = "assertcount TestCountTestSuite/TestNothing\n"
	// assertingOnly selects the package's tests that assert or declare how.
	assertingOnly = "^TestCountTestSuite$/^(TestTwo|TestEverywhere|TestDeclared)$"
)

func (s *CLIVacuousTestSuite) TestAVacuousPassUnderCIMode(t *gotest.T) {
	t.When("--ci runs a package whose only problem is a test that passed without an assertion", func(w *gotest.T) {
		w.It("fails with exit 1, naming the test on stderr, in the -json mode and under spec", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), vacuousPkg, "--ci")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Contains(it, run.Stderr, "FAIL: 5 test(s) passed without an assertion")
			gotest.Contains(it, run.Stderr, vacuousTest)
			spec, err := testkit.RunCLI(it.Context(), ".", "spec", "--ci", "--no-color", vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, spec.Exit, "stderr: %s", spec.Stderr)
			gotest.Contains(it, spec.Stderr, vacuousTest)
		})
		w.It("exits 0 for a package whose tests all assert or declare how", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), vacuousPkg, "--ci", "-run", assertingOnly)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.NotContains(it, run.Stderr, "passed without an assertion")
		})
	})
}

func (s *CLIVacuousTestSuite) TestAVacuousPassOutsideCIMode(t *gotest.T) {
	t.When("a run outside CI mode holds a test that passed without an assertion", func(w *gotest.T) {
		w.It("notes on stderr how many passed without an assertion, naming the test, and exits 0", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `(?m)^note: 5 test\(s\) passed without an assertion \(\S+assertcount TestCountTestSuite/TestNothing, `, run.Stderr)
			gotest.NotContains(it, run.Stdout, "passed without an assertion")
		})
	})
}

func (s *CLIVacuousTestSuite) TestARunThatCountsVacuousPasses(t *gotest.T) {
	t.When("a -json run holds tests that passed without an assertion", func(w *gotest.T) {
		w.It("closes the stream with their count as Vacuous", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), vacuousPkg)
			gotest.NoError(it, err)
			_, last := streamEnds(it, run.Stdout)
			gotest.NotZero(it, last.Result)
			gotest.Equal(it, 5, *last.Result.Vacuous)
		})
		w.It("a run whose tests reported no count closes it with no Vacuous", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/declaredguard/")
			gotest.NoError(it, err)
			_, last := streamEnds(it, run.Stdout)
			gotest.NotZero(it, last.Result)
			gotest.Zero(it, last.Result.Vacuous)
		})
	})
}
