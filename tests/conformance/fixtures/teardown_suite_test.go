package fixtures_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureTeardownTestSuite: a fixture whose setup ran is released, however
// the setup came out.
type FixtureTeardownTestSuite struct{}

func (s *FixtureTeardownTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *FixtureTeardownTestSuite) TestFailedSharedSetupIsReleased(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failingshared/")
	gotest.NoError(t, err)
	t.When("a shared fixture's setup fails on its last attempt", func(w *gotest.T) {
		w.It("fails the run and still runs the fixture's AfterAll, releasing what the attempt reached", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("leak.BeforeAll", "leak.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
