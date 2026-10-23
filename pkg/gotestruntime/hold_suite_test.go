package gotestruntime_test

import (
	"fmt"
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
		w.It("watches for a stop exactly while the hold lasts", func(it *gotest.T) {
			var during bool
			it.T().Run("holder", func(tt *testing.T) { //nolint:suite-lifecycle // the hold must end with a test
				gotestruntime.HoldFixtures(tt, func() gotestruntime.MainConfig { return gotestruntime.MainConfig{} })
				during = gotestruntime.ExportWatching()
			})
			gotest.True(it, during, "no watcher while the fixtures were held")
			gotest.False(it, gotestruntime.ExportWatching(), "the watcher outlived the hold: goleak would see it")
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
