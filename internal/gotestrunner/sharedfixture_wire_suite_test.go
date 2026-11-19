package gotestrunner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureWireTestSuite: the lines the runner and the generated setup
// program exchange, byte for byte, in both directions. A runner and a
// program from different releases meet on these lines.
// Sequential: the fixtures are configured through the environment.
type SharedFixtureWireTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess is.
func (s *SharedFixtureWireTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

// wireRecord collects the lines the runner wrote ("> ") and read ("< "),
// and the outcome of the runner's calls ("= ").
type wireRecord struct {
	mu    sync.Mutex
	lines []string
}

func (r *wireRecord) tap(dir byte, line []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, string(dir)+" "+string(line))
}

func (r *wireRecord) note(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, "= "+label+": "+fmt.Sprint(err))
}

func (r *wireRecord) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

// startTapped starts the setup program for infos with every line recorded.
func startTapped(t *gotest.T, infos []gotestgen.SharedFixtureInfo) (*gotestrunner.SharedFixtureProcess, *wireRecord) {
	rec := &wireRecord{}
	proc, err := gotestrunner.ExportStartTapped(t.Context(), t.TempDir(), infos, 0, rec.tap)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	return proc, rec
}

func panickySetupFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "PanickySetupSharedFixture",
		HasAfterAll:    true,
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.PanickySetupSharedFixture",
		TransferFields: []string{"Marker"},
	}}
}

func (s *SharedFixtureWireTestSuite) TestSetupAndStop(t *gotest.T) {
	proc, rec := startTapped(t, noisySetupFixture())
	gotest.NoError(t, proc.WaitAllReady(t.Context(), 30*time.Second))
	rec.note("teardown", proc.Teardown())

	t.When("a shared fixture with one transfer field comes up and the run ends", func(w *gotest.T) {
		w.It("the process reports its setup budget, its state and the end of setup, and is stopped by one line", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`< {"key":"_setup","fixture":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.NoisySetupSharedFixture","budget":"2m0s"}`,
				`< {"key":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.NoisySetupSharedFixture","state":{"Marker":"noisy"}}`,
				`< {"key":"_done","teardownBudget":"2m30s"}`,
				`> {"verb":"stop"}`,
				`= teardown: <nil>`,
			}, rec.all())
		})
	})
}

func (s *SharedFixtureWireTestSuite) TestHydrate(t *gotest.T) {
	proc, _ := startTapped(t, noisySetupFixture())
	gotest.NoError(t, proc.WaitAllReady(t.Context(), 30*time.Second))
	key := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.NoisySetupSharedFixture"
	path, err := proc.WriteSuiteStateFile("example.com/app/store", "StoreTestSuite", []string{key})
	gotest.NoError(t, err)
	suiteFile, suiteErr := os.ReadFile(path)
	runFile, runErr := os.ReadFile(proc.StateFile())
	gotest.NoError(t, proc.Teardown())

	t.When("a suite's state file is written for the one fixture it binds", func(w *gotest.T) {
		w.It("the file holds that fixture's state under its key, as the run's state file does", func(it *gotest.T) {
			const stateFile = `{"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.NoisySetupSharedFixture":{"Marker":"noisy"}}`
			gotest.NoError(it, suiteErr)
			gotest.NoError(it, runErr)
			gotest.Equal(it, stateFile, string(suiteFile))
			gotest.Equal(it, stateFile, string(runFile))
			gotest.Regexp(it, `^store_[0-9a-f]{8}_StoreTestSuite\.json$`, filepath.Base(path))
		})
	})
}

func (s *SharedFixtureWireTestSuite) TestFailedSetup(t *gotest.T) {
	t.Setenv(fixtures.EnvPanicSetupArm, "1")
	proc, rec := startTapped(t, panickySetupFixture())
	rec.note("setup", proc.WaitAllReady(t.Context(), 30*time.Second))
	rec.note("teardown", proc.Teardown())

	t.When("a shared fixture's setup fails", func(w *gotest.T) {
		w.It("the end of setup carries the failure, and the process is stopped by one line", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`< {"key":"_setup","fixture":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.PanickySetupSharedFixture","budget":"2m0s"}`,
				`< {"key":"_done","error":"one or more shared fixtures failed","teardownBudget":"2m30s"}`,
				`> {"verb":"stop"}`,
				`= setup: shared fixture setup failed: one or more shared fixtures failed`,
				`= teardown: <nil>`,
			}, rec.all())
		})
	})
}

func (s *SharedFixtureWireTestSuite) TestFailedTeardown(t *gotest.T) {
	t.Setenv(fixtures.EnvDagTimeout, "2s")
	t.Setenv(fixtures.EnvDagPanic, "DagA")
	proc, rec := startTapped(t, dagFixtures(false, "DagA"))
	gotest.NoError(t, proc.WaitAllReady(t.Context(), 30*time.Second))
	rec.note("teardown", proc.Teardown())

	t.When("a shared fixture's AfterAll fails", func(w *gotest.T) {
		w.It("the process names it on a teardown line after the stop", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`< {"key":"_setup","fixture":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","budget":"2s"}`,
				`< {"key":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","state":{}}`,
				`< {"key":"_done","teardownBudget":"32s"}`,
				`> {"verb":"stop"}`,
				`< {"key":"_teardown","error":"DagASharedFixture: teardown failed"}`,
				`= teardown: shared fixture teardown failed: DagASharedFixture: teardown failed`,
			}, rec.all())
		})
	})
}

func (s *SharedFixtureWireTestSuite) TestLateStartAndReleaseAtLastUse(t *gotest.T) {
	t.Setenv(fixtures.EnvDagTimeout, "2s")
	proc, rec := startTapped(t, dagFixtures(true, "DagA", "DagChild"))
	ctx := t.Context()
	a, child := dagKey("DagA"), dagKey("DagChild")
	gotest.NoError(t, proc.WaitAllReady(ctx, 0))
	gotest.NoError(t, proc.StartKeys(ctx, []string{a, child}, 0))
	rec.note("release DagA", proc.TeardownKeys(ctx, []string{a}, 30*time.Second))
	gotest.NoError(t, proc.TeardownKeys(ctx, []string{child}, 30*time.Second))
	rec.note("restart DagChild", proc.StartKeys(ctx, []string{child}, 0))
	gotest.NoError(t, proc.TeardownKeys(ctx, []string{a}, 30*time.Second))
	rec.note("teardown", proc.Teardown())

	t.When("a chain of shared fixtures held for a window is started, released and asked back", func(w *gotest.T) {
		w.It("each command is acknowledged by its id, a release ahead of a dependent and a restart are refused", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`< {"key":"_done","teardownBudget":"4m30s"}`,
				`> {"verb":"start","keys":["github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagChildSharedFixture"],"id":1}`,
				`< {"key":"_setup","fixture":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","budget":"2s"}`,
				`< {"key":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","state":{}}`,
				`< {"key":"_setup","fixture":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagChildSharedFixture","budget":"2s"}`,
				`< {"key":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagChildSharedFixture","state":{}}`,
				`< {"key":"_cmd","id":1,"error":"","teardownBudget":"34s"}`,
				`> {"verb":"teardown","keys":["github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture"],"id":2}`,
				`< {"key":"_cmd","id":2,"error":"DagASharedFixture: DagChildSharedFixture, which depends on it, is up and not released with it","teardownBudget":"34s"}`,
				`= release DagA: shared fixture teardown: DagASharedFixture: DagChildSharedFixture, which depends on it, is up and not released with it`,
				`> {"verb":"teardown","keys":["github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagChildSharedFixture"],"id":3}`,
				`< {"key":"_cmd","id":3,"error":"","teardownBudget":"32s"}`,
				`> {"verb":"start","keys":["github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagChildSharedFixture"],"id":4}`,
				`< {"key":"_cmd","id":4,"error":"DagChildSharedFixture: released earlier in this run; shared fixtures are never restarted","teardownBudget":"32s"}`,
				`= restart DagChild: shared fixture start: DagChildSharedFixture: released earlier in this run; shared fixtures are never restarted`,
				`> {"verb":"teardown","keys":["github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture"],"id":5}`,
				`< {"key":"_cmd","id":5,"error":"","teardownBudget":"30s"}`,
				`> {"verb":"stop"}`,
				`= teardown: <nil>`,
			}, rec.all())
		})
	})
}

func (s *SharedFixtureWireTestSuite) TestForce(t *gotest.T) {
	t.Setenv(fixtures.EnvDagTimeout, "2s")
	proc, rec := startTapped(t, dagFixtures(false, "DagA"))
	gotest.NoError(t, proc.WaitAllReady(t.Context(), 30*time.Second))
	gotestrunner.Abandon(5*time.Second, nil)
	rec.note("teardown", proc.Teardown())

	t.When("the runner gives up on a shared fixture process that is up", func(w *gotest.T) {
		w.It("it sends one force line and the process ends clean", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`< {"key":"_setup","fixture":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","budget":"2s"}`,
				`< {"key":"github.com/mvrahden/go-test/tests/sharedfixture/fixtures.DagASharedFixture","state":{}}`,
				`< {"key":"_done","teardownBudget":"32s"}`,
				`> {"verb":"force"}`,
				`= teardown: <nil>`,
			}, rec.all())
		})
	})
}
