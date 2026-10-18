package fixtures_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureRetryTestSuite: what a retried setup's releases between attempts
// count for: their failures fail the run, their time counts toward the wait.
type FixtureRetryTestSuite struct{}

func (s *FixtureRetryTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FixtureRetryTestSuite) TestAFailedReleaseBetweenAttempts(t *gotest.T) {
	t.When("a package fixture's AfterAll fails between its attempts and the second attempt comes up", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fixaretry/")
		gotest.NoError(w, err)
		w.It("fails the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("reports that AfterAll's error", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "the package release leaked")
		})
	})
	t.When("a shared fixture's AfterAll fails between its attempts and the second attempt comes up", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fixaretryshared/")
		gotest.NoError(w, err)
		w.It("fails the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("reports that AfterAll's error", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "the shared release leaked")
		})
	})
}

func (s *FixtureRetryTestSuite) TestReleasesCountTowardTheWait(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixaslowretry/")
	gotest.NoError(t, err)
	t.When("a shared fixture with a 2s Timeout and 1 retry spends 1.6s on each attempt and on the release between them, and no --setup-timeout is given", func(w *gotest.T) {
		w.It("the run passes: the wait counts the release", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSlowTestSuite/TestStateArrived"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, []string{"slow.BeforeAll", "slow.AfterAll", "slow.BeforeAll", "slow.AfterAll"}, run.Markers)
		})
	})
}
