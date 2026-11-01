package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureBoundSharedTestSuite: a shared fixture bound through a package
// fixture reaches the suites of both test packages hydrated.
type FixtureBoundSharedTestSuite struct{}

func (s *FixtureBoundSharedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *FixtureBoundSharedTestSuite) TestSuitesInBothTestPackages(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixsbound/")
	gotest.NoError(t, err)

	t.When("a suite in the internal and one in the external test package each bind a package fixture that binds a shared fixture depending on another", func(w *gotest.T) {
		w.It("both suites pass, seeing the shared fixture hydrated and set up after its dependency", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestInternalTestSuite/TestSeesTheSharedFixture"), "stream:\n%s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestExternalTestSuite/TestSeesTheSharedFixture"), "stream:\n%s", run.Stdout)
		})
	})
}
