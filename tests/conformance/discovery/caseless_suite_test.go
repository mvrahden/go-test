package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CaselessTestSuite: a type named like a suite that declares no case is a
// helper; nothing is generated or reported for it.
type CaselessTestSuite struct{}

func (s *CaselessTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CaselessTestSuite) TestHelperSuites(t *gotest.T) {
	t.When("a suite declares no case", func(w *gotest.T) {
		w.It("the run passes with the real suite's methods", func(it *gotest.T) {
			checkShape(it, shape{pkg: "caseless", verdict: "TestRealTestSuite/TestNames"})
		})
		w.It("nothing is generated or reported for the case-less suite", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/caseless/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.NotContains(it, run.Stdout, "HelperTestSuite")
			gotest.NotContains(it, run.Stderr, "HelperTestSuite")
		})
		w.It("discover does not list the case-less suite", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "discover", "../testdata/shapes/caseless/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, "RealTestSuite")
			gotest.NotContains(it, run.Stdout, "HelperTestSuite")
		})
	})
}
