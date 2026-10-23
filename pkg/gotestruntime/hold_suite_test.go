package gotestruntime_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

type HoldTestSuite struct{}

func (s *HoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *HoldTestSuite) TestBuildConfig(t *gotest.T) {
	t.When("a config method panics while the hold builds its DAG", func(w *gotest.T) {
		w.It("reports the panic as a setup error instead of letting it escape", func(it *gotest.T) {
			_, err := gotestruntime.ExportBuildConfig(func() gotestruntime.MainConfig { panic("config blew up") })
			gotest.ErrorContains(it, err, "panic: config blew up")
		})
	})

	t.When("the build returns", func(w *gotest.T) {
		w.It("hands its config through", func(it *gotest.T) {
			cfg, err := gotestruntime.ExportBuildConfig(func() gotestruntime.MainConfig {
				return gotestruntime.MainConfig{MaxSuiteSetupTimeout: time.Minute}
			})
			gotest.NoError(it, err)
			gotest.Equal(it, time.Minute, cfg.MaxSuiteSetupTimeout)
		})
	})
}

// Sequential: the watcher is process-wide, and a method sets the environment.
type HoldWatchTestSuite struct{}

func (s *HoldWatchTestSuite) TestWatcher(t *gotest.T) {
	t.When("a function holds fixtures", func(w *gotest.T) {
		w.It("is served by the stop watcher exactly while the hold lasts", func(it *gotest.T) {
			before := gotestruntime.ExportWatchCount()
			var during int
			it.T().Run("holder", func(tt *testing.T) { //nolint:suite-lifecycle // the hold must end with a test
				gotestruntime.HoldFixtures(tt, func() gotestruntime.MainConfig { return gotestruntime.MainConfig{} })
				during = gotestruntime.ExportWatchCount()
			})
			gotest.True(it, gotestruntime.ExportWatching(), "the suite's own run is watched")
			gotest.Equal(it, before+1, during, "the hold is not served while it lasts")
			gotest.Equal(it, before, gotestruntime.ExportWatchCount(), "the hold's watch outlived it")
		})
	})
}

// fatalRecorder records what a hold fails with instead of failing the test.
type fatalRecorder struct {
	testing.TB
	fatals []string
}

func (r *fatalRecorder) Fatal(args ...any) { r.fatals = append(r.fatals, fmt.Sprint(args...)) }

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

func (s *HoldWatchTestSuite) TestRunnerRequired(t *gotest.T) {
	t.When("several holds of one process bind a shared fixture without the runner's state", func(w *gotest.T) {
		w.It("fails each, explaining the refusal in full only the first time and never as a setup failure", func(it *gotest.T) {
			it.Setenv(protocol.EnvSharedStateFile, "")
			rec := &fatalRecorder{TB: it.T()}
			build := func() gotestruntime.MainConfig {
				return gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{{
					Name:        "OnceSharedFixture",
					SharedState: &gotestruntime.SharedStateNode{},
				}}}
			}
			gotestruntime.HoldFixtures(rec, build)
			gotestruntime.HoldFixtures(rec, build)
			gotest.Equal(it, []string{
				"suite binds shared fixture OnceSharedFixture, whose state only the gotest runner provides; run via go tool gotest",
				"suite binds shared fixture OnceSharedFixture; run via go tool gotest",
			}, rec.fatals)
		})
	})
}

// watcherFunc is the function the stop watcher's goroutine runs, as
// goleak.IgnoreAnyFunction must name it to leave it out of VerifyNone.
const watcherFunc = "github.com/mvrahden/go-test/pkg/gotestruntime.(*watcher).loop"

func (s *HoldWatchTestSuite) TestWatcherGoroutine(t *gotest.T) {
	buf := make([]byte, 1<<20)
	stacks := string(buf[:runtime.Stack(buf, true)])
	t.When("a suite runs", func(w *gotest.T) {
		w.It("has one goroutine of the stop watcher's, in the function the docs name for goleak", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(stacks, watcherFunc+"("), stacks)
		})
	})
}
