package gotestruntime_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

func (s *StopTestSuite) TestAStopFileWithoutASignalStopsTheRun(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "stop")
	stopped := runstate.StopContext()
	stop := gotestruntime.ExportWatchStopFile(t.T(), path)
	gotest.NoError(t, os.WriteFile(path, nil, 0o600))
	var ended bool
	select {
	case <-stopped.Done():
		ended = true
	case <-time.After(5 * time.Second):
	}
	stop()
	t.When("the runner's stop file appears and no signal follows", func(w *gotest.T) {
		w.It("stops the run", func(it *gotest.T) {
			gotest.True(it, ended)
		})
	})
}

func (s *StopTestSuite) TestAStopTheStopFileBeganTakesTheNextSignalAsItsRequest(t *gotest.T) {
	stopWith := func(signals int) string {
		defer gotestruntime.ExportReleaseBound(time.Second, 0)()
		defer gotestruntime.ExportHoldSuiteOpen()()
		again := make(chan os.Signal, signals)
		for range signals {
			again <- os.Interrupt
		}
		var stderr bytes.Buffer
		gotestruntime.ExportStopFound(t.T(), &stderr, 5*time.Second, again)
		gotestruntime.ExportResetStop()
		return stderr.String()
	}
	one, two := stopWith(1), stopWith(2)
	t.When("the stop file began a stop and the runner's signal follows it", func(w *gotest.T) {
		w.It("takes that signal for the request, not for a second one", func(it *gotest.T) {
			gotest.NotContains(it, one, "a second stop signal")
		})
		w.It("forces the stop on the signal after it", func(it *gotest.T) {
			gotest.Contains(it, two, "a second stop signal")
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

func (s *StopTestSuite) TestASuiteThatOutlastsTheReleaseBoundIsForced(t *gotest.T) {
	defer gotestruntime.ExportReleaseBound(300*time.Millisecond, 0)()
	closeSuite := gotestruntime.ExportHoldSuiteOpen()
	defer closeSuite()
	var stderr bytes.Buffer
	settled := gotestruntime.ExportSettle(t.T(), &stderr, 100*time.Millisecond)
	t.When("the run is stopped and a suite stays open past the release bound, no body running", func(w *gotest.T) {
		w.It("does not count the run as stopped, and says it did not stop within its budget", func(it *gotest.T) {
			gotest.False(it, settled)
			gotest.Contains(it, stderr.String(), "the run did not stop within its budget")
		})
	})
}

func (s *StopTestSuite) TestAForcedStopLeavesTheReleaseItsShare(t *gotest.T) {
	var mu sync.Mutex
	var order []string
	mark := func(m string) { mu.Lock(); order = append(order, m); mu.Unlock() }
	var stderr bytes.Buffer
	var settled, released bool
	t.T().Run("holder", func(tt *testing.T) { //nolint:suite-lifecycle // the hold must end with a test
		gotestruntime.HoldFixtures(tt, func() gotestruntime.MainConfig {
			return gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{{
				Name:      "Slow",
				BeforeAll: func(context.Context) error { return nil },
				AfterAll: func(context.Context) error {
					time.Sleep(300 * time.Millisecond)
					mark("released")
					return nil
				},
			}}}
		})
		// The bound leaves the run 500ms and the release its 1s share.
		defer gotestruntime.ExportReleaseBound(1500*time.Millisecond, time.Second)()
		defer gotestruntime.ExportHoldSuiteOpen()()
		settled, released = gotestruntime.ExportStop(t.T(), &stderr, 100*time.Millisecond, nil)
		mark("stop returned")
	})
	t.When("the run is stopped, outlasts its part of the release bound, and the fixtures take 300ms to release", func(w *gotest.T) {
		w.It("forces the stop and still gives the release the time to finish", func(it *gotest.T) {
			gotest.False(it, settled)
			gotest.True(it, released, stderr.String())
			gotest.Equal(it, []string{"released", "stop returned"}, order)
		})
	})
}

func (s *StopTestSuite) TestASecondStopSignalForcesTheStopAtOnce(t *gotest.T) {
	defer gotestruntime.ExportReleaseBound(10*time.Second, 0)()
	defer gotestruntime.ExportHoldSuiteOpen()()
	again := make(chan os.Signal, 1)
	var stderr bytes.Buffer
	began := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		again <- os.Interrupt
	}()
	settled, _ := gotestruntime.ExportStop(t.T(), &stderr, 5*time.Second, again)
	took := time.Since(began)
	t.When("a second stop signal arrives while the stopped run is still going", func(w *gotest.T) {
		w.It("forces the stop at once, saying so, without waiting out the grace or the bound", func(it *gotest.T) {
			gotest.False(it, settled)
			gotest.Contains(it, stderr.String(), "a second stop signal")
			gotest.Less(it, took, 2*time.Second)
		})
	})
}

func (s *StopTestSuite) TestAStopLeavesAfterEachsContextAlone(t *gotest.T) {
	for _, budget := range []time.Duration{0, 10 * time.Second} {
		var bodyErr, afterErr error
		cfg := gotest.DefaultSuiteConfig()
		t.T().Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the methods run as subtests of a test of this suite's own
			gotestruntime.RunMethods(tt, cfg, gotest.SuiteConfig{Timeout: budget}, gotestruntime.Methods{Cases: []gotestruntime.Method{{
				Name: "TestStops",
				Run: func(ttt *gotest.T, run func(func())) {
					defer gotestruntime.AfterEach(func() { afterErr = gotestruntime.AfterEachT(ttt).Context().Err() })
					run(func() {
						runstate.Stop()
						<-ttt.Context().Done()
						bodyErr = ttt.Context().Err()
					})
				},
			}}})
		})
		gotestruntime.ExportResetStop()
		t.When("the run is stopped while a method runs, with a budget of "+budget.String(), func(w *gotest.T) {
			w.It("ends the body's context and leaves AfterEach's running", func(it *gotest.T) {
				gotest.Error(it, bodyErr)
				gotest.NoError(it, afterErr)
			})
		})
	}
}

func (s *StopTestSuite) TestAForcedReleaseThatRunsOutNamesTheTimeItGot(t *gotest.T) {
	var stderr bytes.Buffer
	var released bool
	t.T().Run("holder", func(tt *testing.T) { //nolint:suite-lifecycle // the hold must end with a test
		gotestruntime.HoldFixtures(tt, func() gotestruntime.MainConfig {
			return gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{{
				Name:      "Slow",
				BeforeAll: func(context.Context) error { return nil },
				AfterAll: func(context.Context) error {
					time.Sleep(time.Second)
					return nil
				},
			}}}
		})
		// The bound leaves the run 300ms and the release its 500ms share.
		defer gotestruntime.ExportReleaseBound(800*time.Millisecond, 500*time.Millisecond)()
		defer gotestruntime.ExportHoldSuiteOpen()()
		_, released = gotestruntime.ExportStop(t.T(), &stderr, 100*time.Millisecond, nil)
	})
	t.When("a forced stop's release outlasts the 500ms share of the bound it keeps", func(w *gotest.T) {
		w.It("says the teardown did not finish within those 500ms", func(it *gotest.T) {
			gotest.False(it, released)
			gotest.Contains(it, stderr.String(), "fixture teardown did not finish within 500ms")
		})
	})
}

func (s *StopTestSuite) TestABodyStartingAfterTheStopGetsTheGraceFromItsStart(t *gotest.T) {
	const grace = 100 * time.Millisecond
	started := make(chan struct{})
	var bodyBegan, settledAt time.Time
	settled := make(chan bool, 1)
	var stderr bytes.Buffer
	defer gotestruntime.ExportReleaseBound(2*time.Second, 0)()
	defer gotestruntime.ExportHoldSuiteOpen()() // the method's suite
	go func() {
		<-started
		ok := gotestruntime.ExportSettle(t.T(), &stderr, grace)
		settledAt = time.Now()
		settled <- ok
	}()
	cfg := gotest.DefaultSuiteConfig()
	t.T().Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the methods run as subtests of a test of this suite's own
		gotestruntime.RunMethods(tt, cfg, cfg, gotestruntime.Methods{Cases: []gotestruntime.Method{{
			Name: "TestLate",
			Run: func(ttt *gotest.T, run func(func())) {
				close(started)
				time.Sleep(3 * grace) // a BeforeEach the stop lands in
				run(func() {
					bodyBegan = time.Now()
					time.Sleep(10 * grace) // ignores its context
				})
			},
		}}})
	})
	ok := <-settled
	t.When("the run is stopped while a method's BeforeEach runs and its body then ignores its context", func(w *gotest.T) {
		w.It("cuts the body short once the grace has passed since it began, saying so", func(it *gotest.T) {
			gotest.False(it, ok)
			gotest.Contains(it, stderr.String(), "a method did not return within 100ms of its context ending")
			gotest.GreaterOrEqual(it, settledAt.Sub(bodyBegan), grace)
		})
	})
}
