package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureReleaseOrderTestSuite: shared fixtures are released concurrently,
// each after the fixtures depending on it, under a budget of their longest
// chain.
type FixtureReleaseOrderTestSuite struct{}

func (s *FixtureReleaseOrderTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	// The hanging release is force-killed after its 35s budget; about 50s
	// with the build, twice that cold under -race.
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *FixtureReleaseOrderTestSuite) TestReleasesRunTogether(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixadag/")
	gotest.NoError(t, err)
	t.When("two independent shared fixtures and a dependent of one are released at the end of a run", func(w *gotest.T) {
		w.It("releases the independent ones at once and the dependent before its dependency", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("tail ran", "child.AfterAll ends", "a.AfterAll begins"), "markers: %v", run.Markers)
			gotest.True(it, run.MarkerOrder("b.AfterAll begins", "child.AfterAll ends"), "markers: %v", run.Markers)
			gotest.True(it, run.MarkerOrder("child.AfterAll begins", "b.AfterAll ends"), "markers: %v", run.Markers)
		})
	})
}

func (s *FixtureReleaseOrderTestSuite) TestTheBudgetIsTheLongestChain(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"FIXADAG_HANG=1"}, "../testdata/fixadag/")
	gotest.NoError(t, err)
	t.When("shared fixtures declaring 2s, 3s on a 2s one, and a released 10s one are resident and an AfterAll never returns", func(w *gotest.T) {
		w.It("their process is force-killed after 30s plus the longest chain still resident, 35s, and the run fails", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "bulk.AfterAll ends")
			gotest.Regexp(it, `shared fixture teardown was force-killed after 35s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}
