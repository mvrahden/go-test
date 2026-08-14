package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureTeardownTestSuite drives a real shared fixture subprocess to the
// point of shutdown.
//
// Whatever a shared fixture holds outlives every test process, so its AfterAll
// is the only thing that ever releases it. If the runner cuts that short, or
// lets it fail quietly, a green run leaves containers and volumes behind.
//
// The suite is sequential: it configures the subprocess through the environment,
// which t.Setenv forbids sharing with parallel tests.
type SharedFixtureTeardownTestSuite struct{}

// SuiteConfig: Exclusive because every method builds and force-kills real
// subprocesses against configured budgets — the one workload that must not
// share the machine with concurrent compiles.
func (s *SharedFixtureTeardownTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

// slowTeardownFixture is the fixture description the generator would produce for
// tests/sharedfixture/fixtures.SlowTeardownSharedFixture.
func slowTeardownFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "SlowTeardownSharedFixture",
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.SlowTeardownSharedFixture",
		TransferFields: []string{"Marker"},
	}}
}

// panicSetupFixtures pairs the panicking fixture with the slow-teardown one, so
// the run has a sibling that is demonstrably up and holding something when the
// panic lands.
func panicSetupFixtures() []gotestgen.SharedFixtureInfo {
	return append(slowTeardownFixture(), gotestgen.SharedFixtureInfo{
		Identifier:     "PanickySetupSharedFixture",
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.PanickySetupSharedFixture",
		TransferFields: []string{"Marker"},
	})
}

// crashTeardownFixture describes the fixture whose owned goroutine crashes the
// whole subprocess during teardown, outside every containment frame.
func crashTeardownFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "CrashTeardownSharedFixture",
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.CrashTeardownSharedFixture",
		TransferFields: []string{"Marker"},
	}}
}

// budgetedTeardownFixture describes the one fixture that declares its own
// SharedFixtureConfig, driving the declared-budget teardown verdict end to end:
// config derivation in the subprocess, the budget on RunFixtureTeardown, the
// teardown-failed exit status, and the runner's report of it.
func budgetedTeardownFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "BudgetedTeardownSharedFixture",
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.BudgetedTeardownSharedFixture",
		HasConfig:      true,
		TransferFields: []string{"Marker"},
	}}
}

// startSlowTeardown boots the shared fixture subprocess under a cancellable
// context, and returns it with the marker path its AfterAll writes once it has
// finished releasing, plus the cancel the pipeline calls on its way out.
func startSlowTeardown(t *gotest.T, delay, budget time.Duration) (proc *gotestrunner.SharedFixtureProcess, marker string, cancel context.CancelFunc) {
	marker = filepath.Join(t.TempDir(), "released")
	t.Setenv(fixtures.EnvSlowTeardownDelay, delay.String())
	t.Setenv(fixtures.EnvSlowTeardownMarker, marker)

	ctx, cancel := context.WithCancel(t.Context())
	t.T().Cleanup(cancel)

	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), slowTeardownFixture(), budget)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	gotest.NoError(t, proc.WaitAllReady(ctx, budget), "waiting for setup")
	return proc, marker, cancel
}

func (s *SharedFixtureTeardownTestSuite) TestTeardownGetsItsConfiguredBudget(t *gotest.T) {
	t.When("AfterAll runs longer than the runner's shutdown grace", func(w *gotest.T) {
		// The pipeline cancels the run context around teardown, which is what
		// arms exec's WaitDelay. Any grace shorter than the configured budget
		// silently becomes the budget: a container fixture is given minutes to
		// stop and gets killed part-way through instead, with the run still
		// reporting success.
		proc, marker, cancel := startSlowTeardown(w, 7*time.Second, 60*time.Second)
		cancel()
		err := proc.Teardown()

		w.It("lets it finish releasing", func(it *gotest.T) {
			_, statErr := os.Stat(marker)
			gotest.NoError(it, statErr,
				"AfterAll never finished: the fixture was killed part-way through and whatever it held is leaked")
			gotest.NoError(it, err)
		})
	})
}

func (s *SharedFixtureTeardownTestSuite) TestCleanTeardown(t *gotest.T) {
	t.When("AfterAll finishes normally", func(w *gotest.T) {
		proc, marker, cancel := startSlowTeardown(w, 0, 30*time.Second)
		cancel()
		err := proc.Teardown()

		w.It("reports success", func(it *gotest.T) {
			gotest.NoError(it, err)
			_, statErr := os.Stat(marker)
			gotest.NoError(it, statErr)
		})
	})
}

func (s *SharedFixtureTeardownTestSuite) TestSetupPanicDoesNotOrphanSiblings(t *gotest.T) {
	t.When("one shared fixture's BeforeAll panics while a sibling is up", func(w *gotest.T) {
		marker := filepath.Join(w.TempDir(), "released")
		// A real (non-zero) sibling teardown is the point: the old WaitAllReady
		// killed the process the moment it saw the setup error, and this test
		// used to bypass WaitAllReady entirely to dodge that. It now goes
		// through the production path — the runner requests shutdown and waits
		// the sibling's AfterAll out.
		w.Setenv(fixtures.EnvSlowTeardownDelay, "300ms")
		w.Setenv(fixtures.EnvSlowTeardownMarker, marker)
		w.Setenv(fixtures.EnvPanicSetupArm, "1")

		ctx, cancel := context.WithCancel(w.Context())

		proc, err := gotestrunner.StartSharedFixtures(ctx, w.TempDir(), panicSetupFixtures(), 30*time.Second)
		gotest.NoError(w, err, "starting the shared fixture subprocess")

		waitErr := proc.WaitAllReady(ctx, 30*time.Second)
		teardownErr := proc.Teardown()
		cancel()

		w.It("reports the failure instead of aborting the process", func(it *gotest.T) {
			gotest.ErrorContains(it, waitErr, "shared fixture setup",
				"a panicking BeforeAll must surface as a setup failure")
		})

		w.It("still releases the sibling that came up", func(it *gotest.T) {
			// The panic used to kill the subprocess outright, so the sibling's
			// AfterAll never ran and whatever it held was orphaned.
			_, statErr := os.Stat(marker)
			gotest.NoError(it, statErr, "the sibling's AfterAll never ran; its resources are leaked")
		})

		w.It("does not blame teardown for the setup failure", func(it *gotest.T) {
			gotest.NoError(it, teardownErr,
				"the sibling teardown ran to completion under the runner's shutdown request")
		})
	})
}

func (s *SharedFixtureTeardownTestSuite) TestCrashDuringTeardown(t *gotest.T) {
	t.When("a fixture-owned goroutine crashes the process mid-teardown", func(w *gotest.T) {
		w.Setenv(fixtures.EnvCrashTeardownArm, "1")

		ctx, cancel := context.WithCancel(w.Context())

		proc, err := gotestrunner.StartSharedFixtures(ctx, w.TempDir(), crashTeardownFixture(), 30*time.Second)
		gotest.NoError(w, err, "starting the shared fixture subprocess")
		gotest.NoError(w, proc.WaitAllReady(ctx, 30*time.Second), "waiting for setup")

		err = proc.Teardown()
		cancel()

		w.It("reports the death instead of passing", func(it *gotest.T) {
			// The process exits 2, not the teardown-failed status. Recognizing
			// only that one status and defaulting to green — the old shape —
			// read exactly this crash as a successful teardown.
			gotest.ErrorContains(it, err, "died during teardown",
				"an abnormal death in the teardown window must fail the run")
		})
	})
}

func (s *SharedFixtureTeardownTestSuite) TestDeclaredBudgetIsATeardownVerdict(t *gotest.T) {
	t.When("AfterAll ignores its context and outlives its declared Timeout", func(w *gotest.T) {
		w.Setenv(fixtures.EnvBudgetedTeardownTimeout, "250ms")
		w.Setenv(fixtures.EnvBudgetedTeardownDelay, "1s")

		ctx, cancel := context.WithCancel(w.Context())

		proc, err := gotestrunner.StartSharedFixtures(ctx, w.TempDir(), budgetedTeardownFixture(), 30*time.Second)
		gotest.NoError(w, err, "starting the shared fixture subprocess")
		gotest.NoError(w, proc.WaitAllReady(ctx, 30*time.Second), "waiting for setup")

		err = proc.Teardown()
		cancel()

		w.It("fails the run with the teardown verdict", func(it *gotest.T) {
			// Drives the whole declared-config chain the snapshots only parse:
			// DeriveFixtureConfig in the subprocess, the budget on
			// RunFixtureTeardown, the teardown-failed exit status, and the
			// runner's report of it.
			gotest.ErrorContains(it, err, "teardown failed; see AfterAll errors above",
				"a declared Timeout must be a verdict on teardown, not just a context it may ignore")
		})
	})
}

func (s *SharedFixtureTeardownTestSuite) TestTeardownForceKilled(t *gotest.T) {
	t.When("AfterAll outlives the teardown budget and the process is force-killed", func(w *gotest.T) {
		// The budget the subprocess reports on its _done line overwrites the one
		// StartSharedFixtures was given, so shrinking it here is what makes the
		// force-kill path reachable without waiting out the fixture's minutes.
		proc, marker, cancel := startSlowTeardown(w, 10*time.Second, 30*time.Second)
		gotestrunner.ExportSetTeardownTimeout(proc, 300*time.Millisecond)
		cancel()
		err := proc.Teardown()

		w.It("reports the cut-short teardown rather than passing", func(it *gotest.T) {
			// SIGKILL leaves no exit status to read — the process reports -1, never
			// the teardown-failed status — so nothing else in the run would say the
			// resources are still out there.
			_, statErr := os.Stat(marker)
			gotest.Error(it, statErr, "AfterAll must not have finished")
			gotest.ErrorContains(it, err, "force-killed",
				"a force-killed teardown must fail the run and name the force-kill, so the operator knows what leaked")
		})
	})
}

func (s *SharedFixtureTeardownTestSuite) TestProcessThatDiedOnItsOwn(t *gotest.T) {
	t.When("the fixture process is killed outright, as an OOM would", func(w *gotest.T) {
		proc, marker, _ := startSlowTeardown(w, 0, 30*time.Second)
		gotest.NoError(w, gotestrunner.ForceKillProcessGroup(gotestrunner.ExportProcessPID(proc)))
		select {
		case <-gotestrunner.ExportProcessDone(proc):
		case <-time.After(10 * time.Second):
			gotest.Fail(w, "the killed fixture process never exited")
		}
		err := proc.Teardown()

		w.It("reports the orphaned resources rather than passing", func(it *gotest.T) {
			// Its AfterAll never ran. Every test may still have passed, so
			// nothing else in the run will say the resources are still out there.
			_, statErr := os.Stat(marker)
			gotest.Error(it, statErr, "AfterAll must not have run")
			gotest.Error(it, err, "a fixture process that died before teardown must fail the run")
		})
	})
}
