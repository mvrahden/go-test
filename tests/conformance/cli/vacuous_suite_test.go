package cli_test

import (
	"strings"
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
		w.It("fails with exit 1, naming the test on stderr, in a text run with and without -v", func(it *gotest.T) {
			for sub, args := range gotest.Each(it, []struct {
				Desc string
				args []string
			}{
				{Desc: "without -v", args: []string{"--ci", vacuousPkg}},
				{Desc: "with -v", args: []string{"--ci", "-v", vacuousPkg}},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", inCache(sub.TempDir()), args.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 1, run.Exit, "stderr: %s", run.Stderr)
				gotest.Contains(sub, run.Stderr, "FAIL: 5 test(s) passed without an assertion")
				gotest.Contains(sub, run.Stderr, vacuousTest)
				gotest.NotContains(sub, run.Stdout, "passed without an assertion")
			}
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
		w.It("a text run without -v notes so too", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", inCache(it.TempDir()), vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `(?m)^note: 5 test\(s\) passed without an assertion \(\S+assertcount TestCountTestSuite/TestNothing, `, run.Stderr)
			gotest.Regexp(it, `^ok  \t\S+/testdata/assertcount\t\d+\.\d{3}s\n$`, run.Stdout)
		})
	})
}

func (s *CLIVacuousTestSuite) TestARunWithVacuousPasses(t *gotest.T) {
	t.When("a run of a package with 5 tests that passed without an assertion ends", func(w *gotest.T) {
		w.It("a text run ends stderr with the closing line naming PASS, exit 0 and 5 tests vacuous", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", inCache(it.TempDir()), vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, "gotest: PASS exit 0 | tests: 5 vacuous | packages: 1 passed, 0 failed, 0 skipped", lastLine(run.Stderr))
		})
		w.It("under agent output with --ci ends stdout with the closing line naming FAIL, exit 1 and 5 vacuous after the tests not run", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", inCache(it.TempDir(), "CLAUDECODE=1"), "--ci", vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, "gotest: FAIL exit 1 | tests: 18 passed, 0 failed, 1 skipped, 0 not run, 5 vacuous | packages: 1 passed, 0 failed, 0 skipped | rerun: go run github.com/mvrahden/go-test/cmd/gotest --failed --ci", lastLine(run.Stdout))
		})
	})
}

func (s *CLIVacuousTestSuite) TestRerunningAfterVacuousPasses(t *gotest.T) {
	env := inCache(t.TempDir())
	first, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "--ci", vacuousPkg)
	gotest.NoError(t, err)
	gotest.Equal(t, 1, first.Exit, "stderr: %s", first.Stderr)
	rerun, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "--failed", "--ci", "-v")
	gotest.NoError(t, err)

	t.When("--failed follows a --ci run that failed only on tests that passed without an assertion", func(w *gotest.T) {
		w.It("runs only their methods and exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, rerun.Exit, "stderr: %s", rerun.Stderr)
			var methods []string
			for line := range strings.Lines(rerun.Stdout) {
				if name, ok := strings.CutPrefix(line, "=== RUN   "); ok && strings.Count(name, "/") == 1 {
					methods = append(methods, strings.TrimSpace(name))
				}
			}
			gotest.Equal(it, []string{"TestCountTestSuite/TestNothing", "TestCountTestSuite/TestBehaviors", "TestCountTestSuite/TestRows", "TestCountTestSuite/TestNoRows"}, methods)
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
