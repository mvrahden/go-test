package gotestruntime_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// AsyncWaitTestSuite: an async method is held to a deadline only when its
// suite declared one, a stop ends the wait as a stop, not as a missed done,
// and a failure ends it only once nothing of the method's is left to call
// done.
// Sequential: one method stops the run, which is process-wide.
type AsyncWaitTestSuite struct{}

func (s *AsyncWaitTestSuite) AfterEach(t *gotest.T) { gotestruntime.ExportResetStop() }

func (s *AsyncWaitTestSuite) TestNoDeclaredBudgetMeansNoDeadlineVerdict(t *gotest.T) {
	var failed bool
	t.T().Run("method", func(mt *testing.T) { //nolint:suite-lifecycle // the wait runs as a subtest of a test of this suite's own
		defer func() { failed = mt.Failed() }()
		// The context carries the default deadline a suite without a config
		// gets, here a short one; no budget was declared.
		tt := gotestruntime.TestT(mt, 50*time.Millisecond)
		gotestruntime.AwaitDoneAt(tt, "TestLateAsync", "x_test.go:1", func(done func()) {
			go func() {
				<-tt.Context().Done()
				time.Sleep(50 * time.Millisecond)
				done()
			}()
		})
	})
	t.When("done arrives after the default context deadline and no Timeout was declared", func(w *gotest.T) {
		w.It("is not a failure", func(it *gotest.T) {
			gotest.False(it, failed)
		})
	})
}

func (s *AsyncWaitTestSuite) TestAStopEndsTheWaitAsAStop(t *gotest.T) {
	for _, declared := range []time.Duration{0, time.Minute} {
		var skipped, failed, afterEach bool
		budget := gotest.SuiteConfig{Timeout: declared}
		t.T().Run("suite", func(st *testing.T) { //nolint:suite-lifecycle // the method runs as a subtest of a test of this suite's own
			gotestruntime.RunMethods(st, gotest.DefaultSuiteConfig(), budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
				Name: "TestHoldsAsync",
				Run: func(ttt *gotest.T, run func(func())) {
					defer func() { skipped, failed = ttt.T().Skipped(), ttt.T().Failed() }()
					defer gotestruntime.AfterEach(func() { afterEach = true })
					run(func() {
						gotestruntime.AwaitDoneAt(ttt, "TestHoldsAsync", "x_test.go:2", func(done func()) {
							go func() {
								time.Sleep(50 * time.Millisecond)
								runstate.Stop()
							}()
						})
					})
				},
			}}})
		})
		gotestruntime.ExportResetStop()
		t.When(fmt.Sprintf("the run is stopped while an async method waits for done, with a declared Timeout of %s", declared), func(w *gotest.T) {
			w.It("skips the method as run stopping instead of failing it for a missed done, and runs AfterEach", func(it *gotest.T) {
				gotest.True(it, skipped)
				gotest.False(it, failed)
				gotest.True(it, afterEach)
			})
		})
	}
}

// runAsync runs one async method, with an AfterEach, under a declared
// Timeout, as a generated suite does.
func runAsync(t *testing.T, timeout time.Duration, start func(tt *gotest.T, done func()), afterEach func()) {
	budget := gotest.SuiteConfig{Timeout: timeout}
	gotestruntime.RunMethods(t, budget, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
		Name: "TestAsync",
		Run: func(ttt *gotest.T, run func(func())) {
			defer gotestruntime.AfterEach(afterEach)
			run(func() {
				gotestruntime.AwaitDoneAt(ttt, "TestAsync", "x_test.go:3", func(done func()) { start(ttt, done) })
			})
		},
	}}})
}

func (s *AsyncWaitTestSuite) TestTheTimeoutHoldsTheHooksAfterTheWait(t *gotest.T) {
	if inChild() {
		runAsync(t.T(), 300*time.Millisecond,
			func(_ *gotest.T, done func()) { done() },
			func() { time.Sleep(time.Second) })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestTheTimeoutHoldsTheHooksAfterTheWait")
	t.When("done is called in time and AfterEach then overruns the declared Timeout", func(w *gotest.T) {
		w.It("fails the method for the overrun, naming AfterEach", func(it *gotest.T) {
			gotest.Contains(it, out, "AfterEach exceeded its configured Timeout of 300ms and is still running")
			gotest.Contains(it, out, "--- FAIL: TestAsyncWaitTestSuite/TestTheTimeoutHoldsTheHooksAfterTheWait/TestAsync")
		})
	})
}

func (s *AsyncWaitTestSuite) TestAnotherGoroutinesFailureLeavesTheOwnerItsWork(t *gotest.T) {
	if inChild() {
		var finished atomic.Bool
		runAsync(t.T(), 10*time.Second,
			func(tt *gotest.T, done func()) {
				go func() { gotest.Fail(tt, "helper fails") }()
				go func() {
					time.Sleep(300 * time.Millisecond)
					finished.Store(true)
					done()
				}()
			},
			func() { fmt.Printf("AfterEach ran, owner finished: %v\n", finished.Load()) })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestAnotherGoroutinesFailureLeavesTheOwnerItsWork")
	t.When("a goroutine other than the one that calls done fails fatally", func(w *gotest.T) {
		w.It("fails the method and waits for done before AfterEach runs", func(it *gotest.T) {
			gotest.Contains(it, out, "helper fails")
			gotest.Contains(it, out, "AfterEach ran, owner finished: true")
		})
	})
}

func (s *AsyncWaitTestSuite) TestTheOwnersFailureEndsTheWait(t *gotest.T) {
	if inChild() {
		began := time.Now()
		runAsync(t.T(), 20*time.Second,
			func(tt *gotest.T, done func()) {
				go func() { gotest.Fail(tt, "owner fails") }()
			},
			// Well short of the 20s budget a wait that outlived the owner would take.
			func() { fmt.Printf("AfterEach ran, well before the budget: %v\n", time.Since(began) < 5*time.Second) })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestTheOwnersFailureEndsTheWait")
	t.When("the goroutine that would call done fails fatally and none of the method's goroutines is left", func(w *gotest.T) {
		w.It("ends the wait at once, not at the Timeout", func(it *gotest.T) {
			gotest.Contains(it, out, "owner fails")
			gotest.Contains(it, out, "AfterEach ran, well before the budget: true")
		})
	})
}

// runAsyncWith runs one async method as a generated suite does: cfg bounds
// its context, budget is the Timeout it declared, before runs where
// BeforeEach does.
func runAsyncWith(t *testing.T, cfg, budget gotest.SuiteConfig, before func(tt *gotest.T), start func(tt *gotest.T, done func()), afterEach func()) {
	gotestruntime.RunMethods(t, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
		Name: "TestAsync",
		Run: func(ttt *gotest.T, run func(func())) {
			defer gotestruntime.AfterEach(afterEach)
			if before != nil {
				before(ttt)
			}
			run(func() {
				gotestruntime.AwaitDoneAt(ttt, "TestAsync", "x_test.go:3", func(done func()) { start(ttt, done) })
			})
		},
	}}})
}

// noTimeout is the config of a suite that declares no Timeout.
var noTimeout = gotest.SuiteConfig{}

func (s *AsyncWaitTestSuite) TestAPanicInAGoGoroutineEndsTheWait(t *gotest.T) {
	if inChild() {
		runAsyncWith(t.T(), gotest.DefaultSuiteConfig(), noTimeout, nil,
			func(tt *gotest.T, done func()) {
				gotest.Go(tt, func() { panic("the owner of done panics") })
			},
			func() { fmt.Println("AfterEach ran") })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestAPanicInAGoGoroutineEndsTheWait")
	t.When("a goroutine gotest.Go started panics while the method waits for done, with no Timeout declared", func(w *gotest.T) {
		w.It("ends the wait, runs AfterEach and reports the panic with the stack from where it happened", func(it *gotest.T) {
			gotest.Contains(it, out, "AfterEach ran")
			gotest.Contains(it, out, "the owner of done panics")
			gotest.Contains(it, out, "[recovered from goroutine started by gotest.Go]")
			gotest.Regexp(it, `async_suite_test\.go:\d+`, out)
			gotest.NotContains(it, out, "test timed out")
		})
	})
}

// ownerRow hands the call of done to an owner the method's body did not
// start itself.
type ownerRow struct {
	name   string
	before func()            // runs where BeforeEach does
	start  func(call func()) // runs in the body
	after  func()            // runs in AfterEach
}

// ownerRows returns the rows; pool is a pool's queue that serves every
// method, started before any of them.
func ownerRows(pool chan<- func()) []ownerRow {
	var work chan func()
	var srv *httptest.Server
	return []ownerRow{
		{
			name: "an owner BeforeEach started",
			before: func() {
				work = make(chan func())
				go func() {
					for job := range work {
						job()
					}
				}()
			},
			start: func(call func()) { work <- func() { time.Sleep(500 * time.Millisecond); call() } },
			after: func() { close(work) },
		},
		{
			name:  "a time.AfterFunc owner",
			start: func(call func()) { time.AfterFunc(500*time.Millisecond, call) },
		},
		{
			name:  "an owner in a pool started before the method",
			start: func(call func()) { pool <- func() { time.Sleep(500 * time.Millisecond); call() } },
		},
		{
			name: "an owner that is an http handler",
			start: func(call func()) {
				srv = httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					time.Sleep(500 * time.Millisecond)
					call()
				}))
				// The request leaves no goroutine of the method's behind.
				conn, err := net.Dial("tcp", srv.Listener.Addr().String())
				if err == nil {
					_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
					_ = conn.Close()
				}
			},
			after: func() { srv.Close() },
		},
	}
}

func (s *AsyncWaitTestSuite) TestOwnersTheBodyDidNotStartKeepTheWait(t *gotest.T) {
	if inChild() {
		pool := make(chan func())
		go func() {
			for job := range pool {
				job()
			}
		}()
		for _, row := range ownerRows(pool) {
			t.T().Run(row.name, func(rt *testing.T) { //nolint:suite-lifecycle // each row runs its method as a subtest
				var finished atomic.Bool
				runAsyncWith(rt, gotest.SuiteConfig{Timeout: 10 * time.Second}, gotest.SuiteConfig{Timeout: 10 * time.Second},
					func(*gotest.T) {
						if row.before != nil {
							row.before()
						}
					},
					func(tt *gotest.T, done func()) {
						gotest.Go(tt, func() { gotest.Fail(tt, "helper fails") })
						row.start(func() { finished.Store(true); done() })
					},
					func() {
						fmt.Printf("%s: AfterEach ran, owner finished: %v\n", row.name, finished.Load())
						if row.after != nil {
							row.after()
						}
					})
			})
		}
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestOwnersTheBodyDidNotStartKeepTheWait")
	t.When("another goroutine fails while an owner of done the body did not start works on", func(w *gotest.T) {
		w.It("waits for the owner to call done before AfterEach runs", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, ownerRows(nil)) {
				gotest.Contains(sub, out, row.name+": AfterEach ran, owner finished: true")
			}
		})
	})
}

func (s *AsyncWaitTestSuite) TestALateAssertionOfAnOwnerTheBodyDidNotStart(t *gotest.T) {
	if inChild() {
		pool := make(chan func())
		go func() {
			for job := range pool {
				job()
			}
		}()
		runAsyncWith(t.T(), gotest.SuiteConfig{Timeout: 10 * time.Second}, gotest.SuiteConfig{Timeout: 10 * time.Second}, nil,
			func(tt *gotest.T, done func()) {
				gotest.Go(tt, func() { gotest.Fail(tt, "helper fails") })
				pool <- func() {
					time.Sleep(500 * time.Millisecond)
					gotest.Equal(tt, "owner's want", "owner's got")
					done()
				}
			},
			func() { fmt.Println("AfterEach ran") })
		// The run goes on past the owner's assertion, as with a later method.
		time.Sleep(time.Second)
		fmt.Println("the run went on")
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestALateAssertionOfAnOwnerTheBodyDidNotStart")
	t.When("an owner of done in a pool fails an assertion after another goroutine failed", func(w *gotest.T) {
		w.It("records the owner's failure on the method and the run goes on", func(it *gotest.T) {
			gotest.Contains(it, out, "owner's got")
			gotest.Contains(it, out, "the run went on")
			gotest.NotContains(it, out, "has completed")
		})
	})
}

func (s *AsyncWaitTestSuite) TestAParallelSiblingsGoroutinesAreNotTheMethods(t *gotest.T) {
	if inChild() {
		budget := gotest.SuiteConfig{Timeout: 10 * time.Second}
		async := func(name string, start func(tt *gotest.T, done func())) gotestruntime.Method {
			return gotestruntime.Method{Name: name, Run: func(ttt *gotest.T, run func(func())) {
				defer gotestruntime.AfterEach(func() { fmt.Println(name + " AfterEach ran") })
				run(func() {
					gotestruntime.AwaitDoneAt(ttt, name, "x_test.go:4", func(done func()) { start(ttt, done) })
				})
			}}
		}
		gotestruntime.RunMethods(t.T(), budget, budget, gotestruntime.Methods{Parallel: true, Cases: []gotestruntime.Method{
			async("TestAAsync", func(_ *gotest.T, done func()) {
				// The goroutine that calls done outlives the ones that started it.
				go func() {
					time.Sleep(100 * time.Millisecond)
					go func() {
						go func() {
							time.Sleep(2 * time.Second)
							fmt.Println("TestAAsync's orphan calls done")
							done()
						}()
					}()
				}()
			}),
			async("TestBAsync", func(tt *gotest.T, _ func()) {
				go func() {
					time.Sleep(300 * time.Millisecond)
					gotest.Fail(tt, "TestBAsync's owner fails")
				}()
			}),
		}})
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestAParallelSiblingsGoroutinesAreNotTheMethods")
	t.When("the owner of a parallel method's done fails while a sibling's goroutine whose creator ended runs on", func(w *gotest.T) {
		w.It("ends the failed method's wait without waiting for the sibling's goroutine", func(it *gotest.T) {
			b, a := strings.Index(out, "TestBAsync AfterEach ran"), strings.Index(out, "TestAAsync's orphan calls done")
			gotest.GreaterOrEqual(it, b, 0, out)
			gotest.GreaterOrEqual(it, a, 0, out)
			gotest.Less(it, b, a, out)
		})
	})
}

// serveUntilAfterEach starts a server on a goroutine of the method's that
// AfterEach stops, as a suite serving its method does.
func serveUntilAfterEach(tt *gotest.T) (stop func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	gotest.NoError(tt, err)
	srv := &http.Server{ReadHeaderTimeout: time.Second}
	gotest.Go(tt, func() { _ = srv.Serve(l) })
	return func() { _ = l.Close() }
}

func (s *AsyncWaitTestSuite) TestAFailedWaitThatLastsToTheDeadline(t *gotest.T) {
	if inChild() {
		var stop func()
		runAsyncWith(t.T(), gotest.SuiteConfig{Timeout: time.Second}, gotest.SuiteConfig{Timeout: time.Second}, nil,
			func(tt *gotest.T, done func()) {
				stop = serveUntilAfterEach(tt)
				gotest.Go(tt, func() { gotest.Fail(tt, "owner fails") })
			},
			func() { stop() })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestAFailedWaitThatLastsToTheDeadline")
	t.When("the owner of done fails while a goroutine of the method's serves on past the declared Timeout", func(w *gotest.T) {
		w.It("says the failed method's wait lasted to the deadline for that goroutine, not that done was missed", func(it *gotest.T) {
			gotest.Contains(it, out, "owner fails")
			gotest.Contains(it, out, "x_test.go:3: TestAsync failed, and its wait for done() lasted until the test deadline: a goroutine it started was still running")
			gotest.NotContains(it, out, "done() was not called before the test deadline")
		})
	})
}

func (s *AsyncWaitTestSuite) TestTheContextBoundsAFailedWaitWithNoTimeout(t *gotest.T) {
	if inChild() {
		release := make(chan struct{})
		defer close(release)
		// The context's default deadline, shortened; no Timeout declared.
		runAsyncWith(t.T(), gotest.SuiteConfig{Timeout: 300 * time.Millisecond}, noTimeout, nil,
			func(tt *gotest.T, done func()) {
				gotest.Go(tt, func() { gotest.Fail(tt, "owner fails") })
				go func() { <-release }()
			},
			func() { fmt.Println("AfterEach ran") })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestTheContextBoundsAFailedWaitWithNoTimeout")
	t.When("the owner of done fails, no Timeout was declared and a goroutine of the method's runs on", func(w *gotest.T) {
		w.It("ends the wait when the method's context ends, saying why it lasted", func(it *gotest.T) {
			gotest.Contains(it, out, "AfterEach ran")
			gotest.Contains(it, out, "x_test.go:3: TestAsync failed, and its wait for done() lasted until its context ended: a goroutine it started was still running")
		})
	})
}

func (s *AsyncWaitTestSuite) TestTheOldHarnessHoldsTheWaitToItsBudget(t *gotest.T) {
	if inChild() {
		began := time.Now()
		t.T().Run("TestOldAsync", func(mt *testing.T) { //nolint:suite-lifecycle // the old harness ran each method as a subtest
			tt := gotestruntime.TestT(mt, 300*time.Millisecond)
			gotestruntime.RunTest(tt, 300*time.Millisecond, func() {
				gotestruntime.AwaitDoneAt(tt, "TestOldAsync", "x_test.go:5", func(func()) {})
			})
		})
		fmt.Printf("the wait ended within 5s: %v\n", time.Since(began) < 5*time.Second)
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestTheOldHarnessHoldsTheWaitToItsBudget")
	t.When("a harness generated before the method loop runs an async method that never calls done", func(w *gotest.T) {
		w.It("fails it at its 300ms budget", func(it *gotest.T) {
			gotest.Contains(it, out, "x_test.go:5: TestOldAsync: done() was not called before the test deadline")
			gotest.Contains(it, out, "--- FAIL: TestAsyncWaitTestSuite/TestTheOldHarnessHoldsTheWaitToItsBudget/TestOldAsync")
			gotest.Contains(it, out, "the wait ended within 5s: true")
		})
	})
}
