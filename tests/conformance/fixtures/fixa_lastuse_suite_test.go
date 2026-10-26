package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureLastUseTestSuite: a shared fixture is released once no planned
// suite left needs it.
type FixtureLastUseTestSuite struct{}

func (s *FixtureLastUseTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	// One run of two suites, the slower 3s; about 15s with the build, twice
	// that cold under -race.
	cfg.Timeout = time.Minute
	return cfg
}

func (s *FixtureLastUseTestSuite) TestReleasedAfterItsLastSuite(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixalastuse/")
	gotest.NoError(t, err)
	t.When("the only suite reading a shared fixture finishes while an unrelated suite still runs", func(w *gotest.T) {
		w.It("releases the fixture before the unrelated suite ends", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("fast ran", "a.AfterAll", "slow ends"), "markers: %v", run.Markers)
		})
	})
}
