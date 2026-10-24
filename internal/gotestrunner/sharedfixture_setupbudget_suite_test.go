package gotestrunner_test

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureSetupBudgetTestSuite: without a --setup-timeout, the runner
// waits for a shared fixture's setup as long as the fixture declared, not for
// a number of its own.
// Sequential: the fixture is configured through the environment.
type SharedFixtureSetupBudgetTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess against wall-clock budgets is.
func (s *SharedFixtureSetupBudgetTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func slowSetupFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "SlowSetupSharedFixture",
		HasAfterAll:    true,
		HasConfig:      true,
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.SlowSetupSharedFixture",
		TransferFields: []string{"Marker"},
	}}
}

// startSlowSetup boots the subprocess with the fixture declaring timeout and
// taking delay, and waits for its setup under flat (zero: the fixtures' own
// budgets), returning how long the wait took and its error. The caller tears
// the process down and releases its context.
func startSlowSetup(t *gotest.T, timeout, delay, flat time.Duration) (proc *gotestrunner.SharedFixtureProcess, release func(), took time.Duration, err error) {
	t.Setenv(fixtures.EnvSlowSetupTimeout, timeout.String())
	t.Setenv(fixtures.EnvSlowSetupDelay, delay.String())
	ctx, cancel := context.WithCancel(t.Context())
	proc, startErr := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), slowSetupFixture(), flat)
	gotest.NoError(t, startErr, "starting the shared fixture subprocess")
	began := time.Now()
	err = proc.WaitSetup(ctx, flat)
	release = func() {
		cancel()
		_ = proc.Teardown()
	}
	return proc, release, time.Since(began), err
}

func (s *SharedFixtureSetupBudgetTestSuite) TestTheWaitIsTheDeclaredBudget(t *gotest.T) {
	_, release, took, err := startSlowSetup(t, time.Second, 15*time.Second, 0)
	defer release()
	t.When("no --setup-timeout is given and a fixture outlives the Timeout it declared", func(w *gotest.T) {
		w.It("the wait ends at the declared budget as a setup timeout", func(it *gotest.T) {
			gotest.ErrorContains(it, err, "timed out")
			gotest.Less(it, took, 10*time.Second)
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestAGivenUpSetupGetsItsBudgetToRelease(t *gotest.T) {
	proc, release, _, err := startSlowSetup(t, time.Second, 3*time.Second, 0)
	defer release()
	t.When("no --setup-timeout is given and the setup is given up before it reports a teardown budget", func(w *gotest.T) {
		w.It("the process gets the budget it overran, at least 5s, to exit before it is killed", func(it *gotest.T) {
			_ = gotest.ErrorAs[*gotestrunner.SetupTimeoutError](it, err)
			gotest.Equal(it, 5*time.Second, gotestrunner.ExportTeardownBudget(proc))
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestAnInterruptedSetupGetsTheDefaultTeardownWindow(t *gotest.T) {
	t.Setenv(fixtures.EnvSlowSetupDelay, (3 * time.Second).String())
	ctx, cancel := context.WithCancel(t.Context())
	proc, startErr := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), slowSetupFixture(), 0)
	gotest.NoError(t, startErr, "starting the shared fixture subprocess")
	defer func() { _ = proc.Teardown() }()
	defer cancel()
	time.AfterFunc(500*time.Millisecond, cancel)
	err := proc.WaitSetup(ctx, 0)
	t.When("no --setup-timeout is given and the run is stopped before the setup reports a teardown budget", func(w *gotest.T) {
		w.It("the process gets 2m to exit before it is killed", func(it *gotest.T) {
			gotest.ErrorIs(it, err, context.Canceled)
			gotest.Equal(it, 2*time.Minute, gotestrunner.ExportTeardownBudget(proc))
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestTheBudgetOutlivesTheFloor(t *gotest.T) {
	// The floor is the wait before the process reports its first budget;
	// shrunk here so the declared budget, not the floor, is what is tested.
	restore := gotestrunner.ExportSetSetupReportFloor(time.Second)
	defer restore()
	proc, release, _, err := startSlowSetup(t, 20*time.Second, 3*time.Second, 0)
	defer release()
	key := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.SlowSetupSharedFixture"
	t.When("a fixture declares a Timeout longer than the runner's floor and needs it", func(w *gotest.T) {
		w.It("the wait lasts the declared budget and the state arrives", func(it *gotest.T) {
			gotest.NoError(it, err)
			var got struct{ Marker string }
			gotest.NoError(it, json.Unmarshal(proc.State([]string{key})[key], &got))
			gotest.Equal(it, "up", got.Marker)
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestTheFloorIsThirtySeconds(t *gotest.T) {
	// The bubble's clock: the floor's real value, waited out at once.
	var err error
	var took time.Duration
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewSilentProcess()
		began := time.Now()
		err = proc.WaitSetup(context.Background(), 0)
		took = time.Since(began)
	})
	t.When("no --setup-timeout is given and the setup process never reports a budget", func(w *gotest.T) {
		w.It("the wait ends after 30s as a setup timeout", func(it *gotest.T) {
			timeout := gotest.ErrorAs[*gotestrunner.SetupTimeoutError](it, err)
			gotest.Equal(it, 30*time.Second, timeout.After)
			gotest.Equal(it, 30*time.Second, took)
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestTheFloorIsTimedFromTheSetupsStart(t *gotest.T) {
	// The bubble's clock: the setup began 10s before the runner starts
	// waiting, and no budget is ever reported.
	var err error
	var took time.Duration
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewSilentProcess()
		time.Sleep(10 * time.Second)
		began := time.Now()
		err = proc.WaitSetup(context.Background(), 0)
		took = time.Since(began)
	})
	t.When("the wait starts after the setup began and no budget is reported", func(w *gotest.T) {
		w.It("the wait ends 30s after the setup began, not after the wait began", func(it *gotest.T) {
			timeout := gotest.ErrorAs[*gotestrunner.SetupTimeoutError](it, err)
			gotest.Equal(it, 30*time.Second, timeout.After)
			gotest.Equal(it, 20*time.Second, took)
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestAFlatTimeoutIsStillTheBudget(t *gotest.T) {
	_, release, took, err := startSlowSetup(t, 20*time.Second, 5*time.Second, time.Second)
	defer release()
	t.When("--setup-timeout is given below a fixture's declared Timeout", func(w *gotest.T) {
		w.It("the flag is the budget", func(it *gotest.T) {
			gotest.ErrorContains(it, err, "timed out after 1s")
			gotest.Less(it, took, 4*time.Second)
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestTheTimeoutIsTimedFromTheSetupsStart(t *gotest.T) {
	// The bubble's clock: the fixture reports its 2s budget 500ms before the
	// runner starts waiting.
	var upFront, window error
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewSilentProcess()
		gotestrunner.ExportNoteSetupBudget(proc, 2*time.Second)
		time.Sleep(500 * time.Millisecond)
		upFront = proc.WaitSetup(context.Background(), 0)
	})
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewReportingProcess(2*time.Second, 0, 500*time.Millisecond)
		window = proc.StartKeys(context.Background(), []string{"k"}, 0)
	})
	t.When("a fixture's budget is reported before the wait for it begins", func(w *gotest.T) {
		w.It("the up-front wait says it timed out after the declared budget", func(it *gotest.T) {
			timeout := gotest.ErrorAs[*gotestrunner.SetupTimeoutError](it, upFront)
			gotest.Equal(it, 2*time.Second, timeout.After)
		})
		w.It("a window's wait says it timed out after the declared budget", func(it *gotest.T) {
			gotest.ErrorContains(it, window, "timed out after 2s")
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestTheTimeoutNamesTheBudgetHeldTo(t *gotest.T) {
	// The bubble's clock: the fixture reports its 2s budget 1.5s after its
	// setup began, as a loaded machine delays it.
	var upFront, window error
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewSilentProcess()
		time.Sleep(1500 * time.Millisecond)
		gotestrunner.ExportNoteSetupBudget(proc, 2*time.Second)
		upFront = proc.WaitSetup(context.Background(), 0)
	})
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewReportingProcess(2*time.Second, 1500*time.Millisecond, 0)
		window = proc.StartKeys(context.Background(), []string{"k"}, 0)
	})
	t.When("a fixture's budget is reported late", func(w *gotest.T) {
		w.It("the up-front wait says it timed out after the declared budget", func(it *gotest.T) {
			timeout := gotest.ErrorAs[*gotestrunner.SetupTimeoutError](it, upFront)
			gotest.Equal(it, 2*time.Second, timeout.After)
		})
		w.It("a window's wait says it timed out after the declared budget", func(it *gotest.T) {
			gotest.ErrorContains(it, window, "timed out after 2s")
		})
	})
}

func (s *SharedFixtureSetupBudgetTestSuite) TestANegativeTimeoutWaitsWithoutADeadline(t *gotest.T) {
	// The bubble's clock: a process that never reports, watched for an hour.
	var err error
	var ended bool
	synctest.Test(t.T(), func(*testing.T) {
		proc := gotestrunner.ExportNewSilentProcess()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- proc.WaitSetup(ctx, -1) }()
		select {
		case err = <-result:
			ended = true
		case <-time.After(time.Hour):
			cancel() // ends the wait, so the bubble can end
			<-result
		}
		cancel()
	})
	t.When("--setup-timeout is 0 and the setup never ends", func(w *gotest.T) {
		w.It("the wait is still going after an hour", func(it *gotest.T) {
			gotest.False(it, ended, "the wait ended: %v", err)
		})
	})
}
