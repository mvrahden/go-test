package gotestruntime_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// StopTestSuite: once the run is asked to stop, no method that has not started
// runs, and a context built after the stop is already over.
// Sequential: the stop is process-wide.
type StopTestSuite struct{}

func (s *StopTestSuite) AfterEach(t *gotest.T) { gotestruntime.ExportResetStop() }

func (s *StopTestSuite) TestAStopSkipsTheMethodsNotStarted(t *gotest.T) {
	var ran []string
	method := func(name string, body func()) gotestruntime.Method {
		return gotestruntime.Method{Name: name, Run: func(tt *gotest.T, run func(func())) {
			run(func() {
				ran = append(ran, name)
				body()
			})
		}}
	}
	cfg := gotest.DefaultSuiteConfig()
	t.T().Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the methods run as subtests of a test of this suite's own
		gotestruntime.RunMethods(tt, cfg, cfg, gotestruntime.Methods{Cases: []gotestruntime.Method{
			method("TestFirst", runstate.Stop),
			method("TestSecond", func() {}),
		}})
	})
	t.When("the run is asked to stop while a method runs", func(w *gotest.T) {
		w.It("runs no method that had not started", func(it *gotest.T) {
			gotest.Equal(it, []string{"TestFirst"}, ran)
		})
	})
}

func (s *StopTestSuite) TestAStopCancelsANewContextAtOnce(t *gotest.T) {
	runstate.Stop()
	tt := gotestruntime.TestT(t.T(), 0)
	t.When("a method's context is built after the stop", func(w *gotest.T) {
		w.It("is already cancelled, with no goroutine to wait for", func(it *gotest.T) {
			gotest.Error(it, tt.Context().Err())
		})
	})
}

func (s *StopTestSuite) TestASuiteOpenedAfterTheStopIsSkipped(t *gotest.T) {
	runstate.Stop()
	var calls []string
	var inner *testing.T
	t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // observing testing.T skip semantics
		inner = tt
		gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			Guard:     func() string { calls = append(calls, "guard"); return "" },
			Fixtures:  func(testing.TB) { calls = append(calls, "fixtures") },
			BeforeAll: func(*gotest.T) { calls = append(calls, "beforeall") },
			AfterAll:  func(*gotest.T) { calls = append(calls, "afterall") },
		})
	})
	t.When("a suite opens after the run was stopped", func(w *gotest.T) {
		w.It("is skipped as run stopping before its guard, fixtures and BeforeAll", func(it *gotest.T) {
			gotest.True(it, inner.Skipped())
			gotest.Empty(it, calls)
		})
	})
}

func (s *StopTestSuite) TestAStopLeavesASlowAfterEachItsTurn(t *gotest.T) {
	const grace = 100 * time.Millisecond
	var mu sync.Mutex
	var order []string
	mark := func(m string) { mu.Lock(); order = append(order, m); mu.Unlock() }
	started := make(chan struct{})
	settled := make(chan bool, 1)
	var stderr bytes.Buffer
	go func() {
		<-started
		settled <- gotestruntime.ExportSettle(t.T(), &stderr, grace)
	}()
	t.T().Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the suite runs as a subtest of a test of this suite's own
		gotestruntime.HoldFixtures(tt, func() gotestruntime.MainConfig {
			return gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{{
				Name:      "DB",
				BeforeAll: func(context.Context) error { return nil },
				AfterAll:  func(context.Context) error { mark("fixture AfterAll"); return nil },
			}}}
		})
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {},
			AfterAll:  func(*gotest.T) { mark("suite AfterAll") },
		})
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
			Name: "TestHolds",
			Run: func(ttt *gotest.T, run func(func())) {
				defer func() {
					time.Sleep(3 * grace)
					mark("AfterEach")
				}()
				run(func() {
					close(started)
					<-ttt.Context().Done()
					mark("body")
				})
			},
		}}})
	})
	t.When("the run is stopped and a method's AfterEach outlasts the grace its body gets", func(w *gotest.T) {
		w.It("lets the run finish: AfterEach, then AfterAll, then the fixture release", func(it *gotest.T) {
			gotest.True(it, <-settled, stderr.String())
			gotest.Equal(it, []string{"body", "AfterEach", "suite AfterAll", "fixture AfterAll"}, order)
		})
	})
}
