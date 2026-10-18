package gotestrunner_test

import (
	"context"
	"encoding/json"
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
