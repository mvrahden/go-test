package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureWindowTestSuite: a shared fixture started late, for the exclusive
// tail or a bench slot, is waited for no longer than it declared.
type FixtureWindowTestSuite struct{}

func (s *FixtureWindowTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	// A fixture with a Timeout hangs until its process is told to shut down.
	// The stopped run's fixture hangs through that too, and so does a late
	// fixture's AfterAll: their processes are force-killed after the 30s or 32s
	// teardown budget they report. The bench slot waits that out twice.
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *FixtureWindowTestSuite) TestTheExclusiveTail(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixatail/")
	gotest.NoError(t, err)
	t.When("a shared fixture started for the exclusive tail hangs past its declared 2s Timeout and no --setup-timeout is given", func(w *gotest.T) {
		w.It("the wait ends at the declared budget and the run fails", func(it *gotest.T) {
			gotest.Regexp(it, `shared fixture start timed out after 2s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("never runs the suite reading it", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestTailTestSuite"), "stdout: %s", run.Stdout)
			gotest.NotContains(it, run.Markers, "tail ran")
			gotest.Contains(it, run.Markers, "bulk ran")
		})
	})
}

func (s *FixtureWindowTestSuite) TestABenchSlot(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "../testdata/fixabench/")
	gotest.NoError(t, err)
	t.When("a shared fixture started for a bench slot hangs past its declared 2s Timeout and no --setup-timeout is given", func(w *gotest.T) {
		w.It("the wait ends at the declared budget and the run fails", func(it *gotest.T) {
			gotest.Regexp(it, `shared fixture start timed out after 2s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("never runs the benchmark reading it", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "HangTestSuite never ran: shared fixture HangSharedFixture did not come up")
		})
	})
}

func (s *FixtureWindowTestSuite) TestATailFixturesTeardownBudget(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixatailbudget/")
	gotest.NoError(t, err)
	t.When("a shared fixture started for the exclusive tail declares a 2s Timeout and its AfterAll never returns", func(w *gotest.T) {
		w.It("its process is force-killed once its 32s teardown budget has passed, and the run fails", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "late.AfterAll")
			gotest.Regexp(it, `shared fixture teardown was force-killed after 32s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}

func (s *FixtureWindowTestSuite) TestABenchSlotFixturesTeardownBudget(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "../testdata/fixabenchbudget/")
	gotest.NoError(t, err)
	t.When("a shared fixture started for a bench slot declares a 2s Timeout and its AfterAll never returns", func(w *gotest.T) {
		w.It("the slot's release is given up once its 32s teardown budget has passed, and the run fails", func(it *gotest.T) {
			gotest.Regexp(it, `shared fixture teardown timed out after 32s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}
