package gotestrunner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureDAGTestSuite: the setup process starts and releases shared
// fixtures concurrently, each after what it depends on.
// Sequential: the fixtures are configured through the environment.
type SharedFixtureDAGTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess against wall-clock budgets is.
func (s *SharedFixtureDAGTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

const dagPkg = "github.com/mvrahden/go-test/tests/sharedfixture/fixtures"

func dagKey(name string) string { return dagPkg + "." + name + "SharedFixture" }

// dagFixtures describes the named Dag fixtures in dependency order, DagChild
// bound to DagA; deferred ones wait for a start command.
func dagFixtures(deferred bool, names ...string) []gotestgen.SharedFixtureInfo {
	infos := make([]gotestgen.SharedFixtureInfo, 0, len(names))
	for _, name := range names {
		info := gotestgen.SharedFixtureInfo{
			Identifier:    name + "SharedFixture",
			HasAfterAll:   true,
			HasConfig:     true,
			PkgPath:       dagPkg,
			PkgName:       "fixtures",
			QualifiedType: "fixtures." + name + "SharedFixture",
			Deferred:      deferred,
		}
		if name == "DagChild" {
			info.Dependencies = []string{dagKey("DagA")}
			info.DependencyFields = map[string]string{dagKey("DagA"): "Parent"}
		}
		infos = append(infos, info)
	}
	return infos
}

// startDAG boots the setup process for infos and waits for its up-front
// phase. The returned release tears it down and ends its context.
func startDAG(t *gotest.T, infos []gotestgen.SharedFixtureInfo) (proc *gotestrunner.SharedFixtureProcess, release func() error) {
	ctx, cancel := context.WithCancel(t.Context())
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), infos, 0)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	gotest.NoError(t, proc.WaitAllReady(ctx, 0), "waiting for the up-front phase")
	return proc, func() error {
		defer cancel()
		return proc.Teardown()
	}
}

func (s *SharedFixtureDAGTestSuite) TestALateStartOfIndependentFixturesTakesTheSlowest(t *gotest.T) {
	t.Setenv(fixtures.EnvDagSetupDelay, time.Second.String())
	proc, release := startDAG(t, dagFixtures(true, "DagA", "DagB"))
	began := time.Now()
	err := proc.StartKeys(t.Context(), []string{dagKey("DagA"), dagKey("DagB")}, 0)
	took := time.Since(began)
	teardownErr := release()

	t.When("two independent fixtures taking 1s each start in one late command", func(w *gotest.T) {
		w.It("acknowledges as soon as the slower is up, not after both in turn", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NoError(it, teardownErr)
			gotest.Less(it, took, 1800*time.Millisecond)
		})
	})
}

// dagEvent is one line a Dag fixture logged.
type dagEvent struct {
	name, hook, event string
	at                int64
}

// readDagLog parses the log the Dag fixtures appended to.
func readDagLog(t *gotest.T, path string) []dagEvent {
	data, err := os.ReadFile(path)
	gotest.NoError(t, err)
	var events []dagEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var ev dagEvent
		_, err := fmt.Sscanf(line, "%s %s %s %d", &ev.name, &ev.hook, &ev.event, &ev.at)
		gotest.NoError(t, err, "log line %q", line)
		events = append(events, ev)
	}
	return events
}

// dagAt is when name's hook logged event, or zero when it did not.
func dagAt(events []dagEvent, name, hook, event string) int64 {
	for _, ev := range events {
		if ev.name == name && ev.hook == hook && ev.event == event {
			return ev.at
		}
	}
	return 0
}

func (s *SharedFixtureDAGTestSuite) TestALateStartWaitsForDependencies(t *gotest.T) {
	log := filepath.Join(t.TempDir(), "dag.log")
	t.Setenv(fixtures.EnvDagLog, log)
	t.Setenv(fixtures.EnvDagSetupDelay, (300 * time.Millisecond).String())
	proc, release := startDAG(t, dagFixtures(true, "DagA", "DagChild"))
	err := proc.StartKeys(t.Context(), []string{dagKey("DagChild"), dagKey("DagA")}, 0)
	teardownErr := release()
	events := readDagLog(t, log)

	t.When("a late command names a fixture before the fixture it depends on", func(w *gotest.T) {
		w.It("starts the fixture only once its dependency is up", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NoError(it, teardownErr)
			parentUp := dagAt(events, "DagA", "BeforeAll", "end")
			childBegins := dagAt(events, "DagChild", "BeforeAll", "begin")
			gotest.NotZero(it, parentUp, "log: %v", events)
			gotest.NotZero(it, childBegins, "log: %v", events)
			gotest.Less(it, parentUp, childBegins, "log: %v", events)
		})
	})
}

func (s *SharedFixtureDAGTestSuite) TestReleasesRunConcurrentlyDependentsFirst(t *gotest.T) {
	type row struct {
		Desc    string
		release func(proc *gotestrunner.SharedFixtureProcess, ctx context.Context) error
	}
	keys := []string{dagKey("DagA"), dagKey("DagB"), dagKey("DagChild")}
	for t, r := range gotest.Each(t, []row{
		{Desc: "the teardown command", release: func(proc *gotestrunner.SharedFixtureProcess, ctx context.Context) error {
			return proc.TeardownKeys(ctx, keys, time.Minute)
		}},
		{Desc: "the terminal teardown", release: func(*gotestrunner.SharedFixtureProcess, context.Context) error { return nil }},
	}) {
		log := filepath.Join(t.TempDir(), "dag.log")
		t.Setenv(fixtures.EnvDagLog, log)
		t.Setenv(fixtures.EnvDagTeardownDelay, (500 * time.Millisecond).String())
		proc, release := startDAG(t, dagFixtures(false, "DagA", "DagB", "DagChild"))
		releaseErr := r.release(proc, t.Context())
		teardownErr := release()
		events := readDagLog(t, log)

		t.When("three fixtures are released, one depending on another", func(w *gotest.T) {
			w.It("releases the independent ones at once, a dependent before its dependency", func(it *gotest.T) {
				gotest.NoError(it, releaseErr)
				gotest.NoError(it, teardownErr)
				childDone := dagAt(events, "DagChild", "AfterAll", "end")
				parentBegins := dagAt(events, "DagA", "AfterAll", "begin")
				siblingBegins := dagAt(events, "DagB", "AfterAll", "begin")
				gotest.NotZero(it, childDone, "log: %v", events)
				gotest.NotZero(it, parentBegins, "log: %v", events)
				gotest.NotZero(it, siblingBegins, "log: %v", events)
				gotest.LessOrEqual(it, childDone, parentBegins, "log: %v", events)
				gotest.Less(it, siblingBegins, childDone, "log: %v", events)
			})
		})
	}
}

func (s *SharedFixtureDAGTestSuite) TestTheTeardownBudgetIsTheLongestChain(t *gotest.T) {
	type row struct {
		Desc     string
		names    []string
		release  []string
		budget   time.Duration
		released time.Duration
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "four independent fixtures", names: []string{"DagA", "DagB", "DagC", "DagD"}, budget: 5*time.Minute + 30*time.Second},
		{Desc: "a two-link chain", names: []string{"DagA", "DagChild"}, budget: 10*time.Minute + 30*time.Second},
		{
			Desc: "a chain and an independent fixture, the chain's dependent released", names: []string{"DagA", "DagB", "DagChild"},
			release: []string{dagKey("DagChild")}, budget: 10*time.Minute + 30*time.Second, released: 5*time.Minute + 30*time.Second,
		},
	}) {
		t.Setenv(fixtures.EnvDagTimeout, (5 * time.Minute).String())
		proc, release := startDAG(t, dagFixtures(false, r.names...))
		budget := gotestrunner.ExportTeardownBudget(proc)
		var releaseErr error
		var released time.Duration
		if len(r.release) > 0 {
			releaseErr = proc.TeardownKeys(t.Context(), r.release, time.Minute)
			released = gotestrunner.ExportTeardownBudget(proc)
		}
		teardownErr := release()

		t.When("fixtures declaring a 5m Timeout are resident", func(w *gotest.T) {
			w.It("reports 30s plus the longest chain of their shares, less what was released", func(it *gotest.T) {
				gotest.NoError(it, releaseErr)
				gotest.NoError(it, teardownErr)
				gotest.Equal(it, r.budget, budget)
				gotest.Equal(it, r.released, released)
			})
		})
	}
}

func (s *SharedFixtureDAGTestSuite) TestARequestThatCouldOnlyHangIsRefused(t *gotest.T) {
	type row struct {
		Desc     string
		deferred bool
		send     func(proc *gotestrunner.SharedFixtureProcess, ctx context.Context) error
		names    []string
	}
	for t, r := range gotest.Each(t, []row{
		{
			Desc: "a start without the dependency it waits for", deferred: true,
			send: func(proc *gotestrunner.SharedFixtureProcess, ctx context.Context) error {
				return proc.StartKeys(ctx, []string{dagKey("DagChild")}, 0)
			},
			names: []string{"DagChildSharedFixture", "DagASharedFixture"},
		},
		{
			Desc: "a release without the dependent still up",
			send: func(proc *gotestrunner.SharedFixtureProcess, ctx context.Context) error {
				return proc.TeardownKeys(ctx, []string{dagKey("DagA")}, time.Minute)
			},
			names: []string{"DagASharedFixture", "DagChildSharedFixture"},
		},
	}) {
		proc, release := startDAG(t, dagFixtures(r.deferred, "DagA", "DagChild"))
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		began := time.Now()
		err := r.send(proc, ctx)
		took := time.Since(began)
		cancel()
		gotestrunner.ExportSetTeardownTimeout(proc, 2*time.Second)
		_ = release()

		t.When("a command could only wait for a fixture nothing will start or release", func(w *gotest.T) {
			w.It("fails within a second, naming both fixtures", func(it *gotest.T) {
				gotest.Error(it, err)
				for _, name := range r.names {
					gotest.ErrorContains(it, err, name)
				}
				gotest.Less(it, took, time.Second)
			})
		})
	}
}

func (s *SharedFixtureDAGTestSuite) TestATeardownGoingWrongLetsTheOthersFinish(t *gotest.T) {
	type row struct {
		Desc string
		env  map[string]string
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "a panicking AfterAll", env: map[string]string{fixtures.EnvDagPanic: "DagChild"}},
		{Desc: "an AfterAll outliving its declared Timeout", env: map[string]string{
			fixtures.EnvDagSlow: "DagChild", fixtures.EnvDagTimeout: "1s", fixtures.EnvDagTeardownDelay: "2s",
		}},
	}) {
		log := filepath.Join(t.TempDir(), "dag.log")
		t.Setenv(fixtures.EnvDagLog, log)
		for k, v := range r.env {
			t.Setenv(k, v)
		}
		_, release := startDAG(t, dagFixtures(false, "DagA", "DagB", "DagChild"))
		teardownErr := release()
		events := readDagLog(t, log)

		t.When("one fixture's release goes wrong while the others are released with it", func(w *gotest.T) {
			w.It("lets every other fixture finish its release, and fails the run", func(it *gotest.T) {
				gotest.Error(it, teardownErr)
				gotest.NotZero(it, dagAt(events, "DagA", "AfterAll", "end"), "log: %v", events)
				gotest.NotZero(it, dagAt(events, "DagB", "AfterAll", "end"), "log: %v", events)
			})
		})
	}
}

func (s *SharedFixtureDAGTestSuite) TestAReleaseIsReportedByTheCommandThatRanIt(t *gotest.T) {
	t.Setenv(fixtures.EnvDagPanic, "DagB")
	proc, release := startDAG(t, dagFixtures(false, "DagA", "DagB"))
	first := proc.TeardownKeys(t.Context(), []string{dagKey("DagB")}, time.Minute)
	again := proc.TeardownKeys(t.Context(), []string{dagKey("DagB")}, time.Minute)
	_ = release()

	t.When("a fixture whose release failed is named in a later teardown command", func(w *gotest.T) {
		w.It("fails only the command that released it", func(it *gotest.T) {
			gotest.ErrorContains(it, first, "DagBSharedFixture: teardown failed")
			gotest.NoError(it, again)
		})
	})
}

func (s *SharedFixtureDAGTestSuite) TestAHungReleaseHoldsUpNoLaterStart(t *gotest.T) {
	t.Setenv(fixtures.EnvDagSlow, "DagA")
	t.Setenv(fixtures.EnvDagTeardownDelay, (3 * time.Second).String())
	infos := append(dagFixtures(false, "DagA"), dagFixtures(true, "DagB")...)
	proc, release := startDAG(t, infos)
	released := make(chan error, 1)
	go func() { released <- proc.TeardownKeys(t.Context(), []string{dagKey("DagA")}, time.Minute) }()
	time.Sleep(200 * time.Millisecond)
	began := time.Now()
	startErr := proc.StartKeys(t.Context(), []string{dagKey("DagB")}, 0)
	took := time.Since(began)
	releaseErr := <-released
	teardownErr := release()

	t.When("a fixture is started while another's release still runs", func(w *gotest.T) {
		w.It("comes up without waiting for the release", func(it *gotest.T) {
			gotest.NoError(it, startErr)
			gotest.NoError(it, releaseErr)
			gotest.NoError(it, teardownErr)
			gotest.Less(it, took, 1500*time.Millisecond)
		})
	})
}
