package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureDependencyTestSuite: a shared fixture started late comes up after
// the dependencies it starts with.
type FixtureDependencyTestSuite struct{}

func (s *FixtureDependencyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	// The run takes seconds; a dependent started before its dependency hangs.
	cfg.Timeout = time.Minute
	return cfg
}

func (s *FixtureDependencyTestSuite) TestATailOnlyDependency(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixataildep/")
	gotest.NoError(t, err)
	t.When("a shared fixture started for the exclusive tail depends on a shared fixture only the tail needs", func(w *gotest.T) {
		w.It("both come up, the dependency first, and the suite reading them passes", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestTailTestSuite"), "stdout: %s", run.Stdout)
			gotest.True(it, run.MarkerOrder("bulk ran", "db.BeforeAll", "app.BeforeAll", "tail ran"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
		})
		w.It("both are torn down, the dependent first", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("tail ran", "app.AfterAll", "db.AfterAll"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
		})
	})
}
