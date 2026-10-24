package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureRetryTestSuite: a release between a retried setup's attempts that
// fails, or outlives the fixture's declared Timeout, fails the run.
type FixtureRetryTestSuite struct{}

func (s *FixtureRetryTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~68s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
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
	t.When("a package fixture's AfterAll between its attempts outlives the fixture's declared Timeout and the second attempt comes up", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fixareleaseslow/")
		gotest.NoError(w, err)
		w.It("fails the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("reports that the release exceeded the Timeout", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "SlowReleaseFixture.AfterAll failed: exceeded its configured Timeout of 200ms")
		})
	})
}
