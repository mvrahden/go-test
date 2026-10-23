package fixtures_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureTeardownTestSuite: a shared fixture whose setup ran is released,
// however the setup came out, and so is one whose Hydrate ran.
type FixtureTeardownTestSuite struct{}

func (s *FixtureTeardownTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *FixtureTeardownTestSuite) TestFailedSharedSetupIsReleased(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failingshared/")
	gotest.NoError(t, err)
	t.When("a shared fixture's setup fails on its last attempt", func(w *gotest.T) {
		w.It("fails the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("runs AfterAll after each of its 2 attempts, releasing what each reached before the next", func(it *gotest.T) {
			gotest.Equal(it, []string{"leak.BeforeAll", "leak.AfterAll", "leak.BeforeAll", "leak.AfterAll"}, run.Markers)
		})
	})
}

func (s *FixtureTeardownTestSuite) TestFailedHydrateIsDehydrated(t *gotest.T) {
	t.When("a shared fixture's Hydrate fails after reaching part of what it opens", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fixahydrate/")
		gotest.NoError(w, err)
		w.It("fails the run, runs no method and still runs its Dehydrate", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, []string{"pool.Hydrate", "pool.Dehydrate"}, run.Markers)
		})
	})
}
