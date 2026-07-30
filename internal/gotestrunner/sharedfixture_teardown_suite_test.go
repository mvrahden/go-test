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
