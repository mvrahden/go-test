package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
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
	// fixture's AfterAll: their processes are force-killed after the teardown
	// budget they report, 30s or 32s for a late fixture and 2m30s for the
	// stopped run, which counts its unstarted fixture at the default.
	cfg.Timeout = 4 * time.Minute
	return cfg
}

func (s *FixtureWindowTestSuite) TestTheExclusiveTail(t *gotest.T) {
	markers := t.TempDir()
	warmPackage(t, markers, "../testdata/fixatail/")
	began := time.Now()
	run, err := testkit.RunPackage(t.Context(), markers, "../testdata/fixatail/")
	elapsed := time.Since(began)
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
		w.It("ends the run within 60s", func(it *gotest.T) {
			gotest.Less(it, elapsed, 60*time.Second, "stderr: %s", run.Stderr)
		})
	})
}

func (s *FixtureWindowTestSuite) TestABenchSlot(t *gotest.T) {
	cache := warmBench(t, "../testdata/fixabench/")
	began := time.Now()
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", cache, "bench", "../testdata/fixabench/")
	elapsed := time.Since(began)
	gotest.NoError(t, err)
	t.When("a shared fixture started for a bench slot hangs past its declared 2s Timeout and no --setup-timeout is given", func(w *gotest.T) {
		w.It("the wait ends at the declared budget and the run fails", func(it *gotest.T) {
			gotest.Regexp(it, `shared fixture start timed out after 2s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("never runs the benchmark reading it", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "HangTestSuite never ran: shared fixture HangSharedFixture did not come up")
		})
		w.It("ends the run within 60s", func(it *gotest.T) {
			gotest.Less(it, elapsed, 60*time.Second, "stderr: %s", run.Stderr)
		})
	})
}

func (s *FixtureWindowTestSuite) TestATailFixturesTeardownBudget(t *gotest.T) {
	markers := t.TempDir()
	warmPackage(t, markers, "../testdata/fixatailbudget/")
	began := time.Now()
	run, err := testkit.RunPackage(t.Context(), markers, "../testdata/fixatailbudget/")
	elapsed := time.Since(began)
	gotest.NoError(t, err)
	t.When("a shared fixture started for the exclusive tail declares a 2s Timeout and its AfterAll never returns", func(w *gotest.T) {
		w.It("its process is force-killed once its 32s teardown budget has passed, and the run fails", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "late.AfterAll")
			gotest.Regexp(it, `shared fixture teardown was force-killed after 32s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("ends the run within 60s, without a second wait for the release given up", func(it *gotest.T) {
			gotest.Less(it, elapsed, 60*time.Second, "stderr: %s", run.Stderr)
		})
	})
}

func (s *FixtureWindowTestSuite) TestABenchSlotFixturesTeardownBudget(t *gotest.T) {
	cache := warmBench(t, "../testdata/fixabenchbudget/")
	began := time.Now()
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", cache, "bench", "../testdata/fixabenchbudget/")
	elapsed := time.Since(began)
	gotest.NoError(t, err)
	t.When("a shared fixture started for a bench slot declares a 2s Timeout and its AfterAll never returns", func(w *gotest.T) {
		w.It("the slot's release is given up once its 32s teardown budget has passed, and the run fails", func(it *gotest.T) {
			gotest.Regexp(it, `shared fixture teardown timed out after 32s\b`, run.Stderr)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("ends the run within 60s, without a second wait for the release given up", func(it *gotest.T) {
			gotest.Less(it, elapsed, 60*time.Second, "stderr: %s", run.Stderr)
		})
	})
}

// warmPackage builds pkg into the binary cache of the runs that use markers,
// selecting nothing, so no fixture starts. A bound on the run that follows
// then measures the run, not how long a loaded machine takes to compile.
func warmPackage(t *gotest.T, markers, pkg string) {
	run, err := testkit.RunPackage(t.Context(), markers, pkg, "-run", "^$")
	gotest.NoError(t, err)
	gotest.Zero(t, run.Exit, "warm-up stderr: %s", run.Stderr)
}

// warmBench is warmPackage for a bench run: it returns the environment that
// points the measured run at the binary cache the warm-up filled.
func warmBench(t *gotest.T, pkg string) []string {
	cache := []string{protocol.EnvCacheDir + "=" + testkit.ChildCacheDir(t.TempDir())}
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", cache, "bench", pkg, "-bench", "^$")
	gotest.NoError(t, err)
	gotest.Zero(t, run.Exit, "warm-up stderr: %s", run.Stderr)
	return cache
}
