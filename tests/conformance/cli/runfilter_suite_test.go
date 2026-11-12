package cli_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRunFilterTestSuite: a -run pattern reaches each suite process narrowed
// to that suite's own functions.
type CLIRunFilterTestSuite struct{}

func (s *CLIRunFilterTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLIRunFilterTestSuite) TestASubtestPatternOverTwoSuites(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/clirunfilter/", "-run", "(Alpha|Beta)TestSuite/TestOne")
	gotest.NoError(t, err)
	t.When("-run names a method under a first level that matches two suites of a package", func(w *gotest.T) {
		w.It("runs each suite's hooks and the method once, each suite in its own process", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			count, pids := map[string]int{}, map[string]string{}
			for _, m := range run.Markers {
				name, pid, _ := strings.Cut(m, " pid=")
				count[name]++
				pids[strings.SplitN(name, ".", 2)[0]] = pid
			}
			gotest.Equal(it, map[string]int{
				"Alpha.BeforeAll TestAlphaTestSuite": 1, "Alpha.BeforeAll FuzzAlphaTestSuite_FuzzSeed": 1,
				"Alpha.TestOne":                     1,
				"Alpha.AfterAll TestAlphaTestSuite": 1, "Alpha.AfterAll FuzzAlphaTestSuite_FuzzSeed": 1,
				"Beta.BeforeAll": 1, "Beta.TestOne": 1,
			}, count)
			gotest.NotEqual(it, pids["Alpha"], pids["Beta"])
		})
	})
}

func (s *CLIRunFilterTestSuite) TestASubtestPatternBesideASharedFixture(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/clirunfilter/", "-run", "/TestState")
	gotest.NoError(t, err)
	t.When("-run /TestState runs a package holding a suite that binds a shared fixture beside one that binds none", func(w *gotest.T) {
		w.It("passes, the fixture's suite running only in the process given its state", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestStateTestSuite/TestState"))
			gotest.NotContains(it, run.Stdout+run.Stderr, "whose state only the gotest runner provides")
		})
	})
}

func (s *CLIRunFilterTestSuite) TestAPatternNamingOnlyTheTestFunction(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/clirunfilter/", "-run", "^TestAlphaTestSuite$")
	gotest.NoError(t, err)
	t.When("-run names only the Test function of a suite that also has a fuzz method", func(w *gotest.T) {
		w.It("runs the test methods and replays no fuzz seed, as go test does", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestAlphaTestSuite/TestOne"))
			gotest.Empty(it, run.Verdict("FuzzAlphaTestSuite_FuzzSeed"))
		})
	})
}
