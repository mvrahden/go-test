package gotestruntime_test

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// The registry and the dying mark are process-wide, so this suite stays serial
// and starts every method from an empty registry.
//
// Sequential: the registry and the mark are process-wide.
type TeardownRegistryTestSuite struct{}

func (s *TeardownRegistryTestSuite) BeforeEach(t *gotest.T) { gotestruntime.ExportResetTeardowns() }
func (s *TeardownRegistryTestSuite) AfterEach(t *gotest.T)  { gotestruntime.ExportResetTeardowns() }

func (s *TeardownRegistryTestSuite) TestFinish(t *gotest.T) {
	t.When("nothing was registered", func(w *gotest.T) {
		w.It("keeps the exit code and prints nothing", func(it *gotest.T) {
			var stderr bytes.Buffer
			gotest.Equal(it, 0, gotestruntime.ExportFinish(0, &stderr))
			gotest.Equal(it, 1, gotestruntime.ExportFinish(1, &stderr))
			gotest.Empty(it, stderr.String())
		})
	})

	t.When("teardowns were registered", func(w *gotest.T) {
		w.It("runs each once, in registration order", func(it *gotest.T) {
			var order []string
			gotestruntime.RegisterTeardown(func() bool { order = append(order, "ptest"); return false })
			gotestruntime.RegisterTeardown(func() bool { order = append(order, "pxtest"); return false })
			var stderr bytes.Buffer
			gotest.Equal(it, 0, gotestruntime.ExportFinish(0, &stderr))
			gotest.Equal(it, 0, gotestruntime.ExportFinish(0, &stderr))
			gotest.Equal(it, []string{"ptest", "pxtest"}, order)
		})
	})

	t.When("a teardown failed after passing tests", func(w *gotest.T) {
		w.It("fails the package with exit 1 and says why", func(it *gotest.T) {
			ran := false
			gotestruntime.RegisterTeardown(func() bool { return true })
			gotestruntime.RegisterTeardown(func() bool { ran = true; return false })
			var stderr bytes.Buffer
			gotest.Equal(it, 1, gotestruntime.ExportFinish(0, &stderr))
			gotest.True(it, ran, "a failed teardown must not stop the next")
			gotest.Equal(it, "FAIL: fixture teardown failed\n", stderr.String())
		})
	})

	t.When("a teardown failed after failing tests", func(w *gotest.T) {
		w.It("keeps the tests' exit code", func(it *gotest.T) {
			gotestruntime.RegisterTeardown(func() bool { return true })
			var stderr bytes.Buffer
			gotest.Equal(it, 2, gotestruntime.ExportFinish(2, &stderr))
		})
	})
}

func (s *TeardownRegistryTestSuite) TestNotePanicOnPanic(t *gotest.T) {
	t.When("a panic passes", func(w *gotest.T) {
		w.It("marks the process and lets the same value go on", func(it *gotest.T) {
			got := gotest.Panics(it, func() {
				defer gotestruntime.NotePanic()
				panic("boom")
			})
			gotest.Equal(it, any("boom"), got)
			gotest.True(it, gotestruntime.ExportDying())
		})
	})
}

func (s *TeardownRegistryTestSuite) TestNotePanicOnReturn(t *gotest.T) {
	t.When("the function returns", func(w *gotest.T) {
		w.It("leaves the mark unset", func(it *gotest.T) {
			func() {
				defer gotestruntime.NotePanic()
			}()
			gotest.False(it, gotestruntime.ExportDying())
		})
	})
}

func (s *TeardownRegistryTestSuite) TestNotePanicOnGoexit(t *gotest.T) {
	t.When("the goroutine exits through runtime.Goexit, as FailNow does", func(w *gotest.T) {
		w.It("leaves the mark unset: the process goes on", func(it *gotest.T) {
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer gotestruntime.NotePanic()
				runtime.Goexit()
			}()
			<-done
			gotest.False(it, gotestruntime.ExportDying())
		})
	})
}

func (s *TeardownRegistryTestSuite) TestRequireMainOutsideTheRun(t *gotest.T) {
	t.When("the tests did not run through Main or M", func(w *gotest.T) {
		w.It("refuses fixture setup and names the fix", func(it *gotest.T) {
			err := gotestruntime.RequireMain()
			gotest.ErrorContains(it, err, "os.Exit(gotestruntime.Main(m))")
			gotest.ErrorContains(it, err, "gotestruntime.M(m)")
		})
	})
}

func (s *TeardownRegistryTestSuite) TestRequireMainDuringTheRun(t *gotest.T) {
	t.When("the tests run through Main or M", func(w *gotest.T) {
		w.It("allows fixture setup", func(it *gotest.T) {
			var during error
			var stderr bytes.Buffer
			gotestruntime.ExportRunTests(func() int {
				during = gotestruntime.RequireMain()
				return 0
			}, &stderr)
			gotest.NoError(it, during)
		})
	})
}

func (s *TeardownRegistryTestSuite) TestRequireMainAfterTheRun(t *gotest.T) {
	t.When("Main or M already tore the fixtures down", func(w *gotest.T) {
		w.It("refuses fixture setup: the tests ran again", func(it *gotest.T) {
			var stderr bytes.Buffer
			gotestruntime.ExportRunTests(func() int { return 0 }, &stderr)
			gotest.ErrorContains(it, gotestruntime.RequireMain(), "already torn down")
		})
	})
}

func (s *TeardownRegistryTestSuite) TestASecondRunIsRefused(t *gotest.T) {
	t.When("Main or M runs the tests a second time", func(w *gotest.T) {
		w.It("refuses fixture setup in the second round", func(it *gotest.T) {
			var stderr bytes.Buffer
			gotestruntime.ExportRunTests(func() int { return 0 }, &stderr)
			var second error
			gotestruntime.ExportRunTests(func() int {
				second = gotestruntime.RequireMain()
				return 0
			}, &stderr)
			gotest.ErrorContains(it, second, "already torn down")
		})
	})
}

func (s *TeardownRegistryTestSuite) TestSetupDuringAStop(t *gotest.T) {
	t.When("a stop has begun", func(w *gotest.T) {
		w.It("refuses new fixture setup", func(it *gotest.T) {
			gotestruntime.ExportStartRun()
			gotest.True(it, gotestruntime.ExportBeginStop())
			gotest.ErrorContains(it, gotestruntime.RequireMain(), "stopping")
			_, _, err := gotestruntime.ExportBeginSetup(context.Background())
			gotest.ErrorContains(it, err, "stopping")
		})
	})
}

func (s *TeardownRegistryTestSuite) TestSetupInFlightAtAStop(t *gotest.T) {
	t.When("a stop arrives while a setup runs", func(w *gotest.T) {
		w.It("cancels the setup's context", func(it *gotest.T) {
			gotestruntime.ExportStartRun()
			ctx, done, err := gotestruntime.ExportBeginSetup(context.Background())
			gotest.NoError(it, err)
			defer done()
			gotest.NoError(it, ctx.Err())
			runstate.Stop()
			gotest.Eventually(it, time.Second, time.Millisecond, func(poll *gotest.R) {
				gotest.ErrorIs(poll, ctx.Err(), context.Canceled)
			})
		})
	})
}

func (s *TeardownRegistryTestSuite) TestReleaseBound(t *gotest.T) {
	t.When("no budget was advertised", func(w *gotest.T) {
		w.It("allows two minutes", func(it *gotest.T) {
			gotest.Equal(it, 2*time.Minute, gotestruntime.ExportReleaseBound())
		})
	})
	t.When("both test packages advertised a budget", func(w *gotest.T) {
		w.It("keeps the larger, and stays inside it, ahead of the runner's kill", func(it *gotest.T) {
			gotestruntime.ExportRecordBudget(3 * time.Minute)
			gotest.Equal(it, 3*time.Minute, gotestruntime.ExportRecordBudget(time.Minute))
			gotest.Equal(it, 3*time.Minute-5*time.Second, gotestruntime.ExportReleaseBound())
		})
	})
}

func (s *TeardownRegistryTestSuite) TestAnOlderHarness(t *gotest.T) {
	t.When("a harness from before teardown-after-m.Run calls the countdown", func(w *gotest.T) {
		w.It("names the fix", func(it *gotest.T) {
			got := gotest.Panics(it, func() { gotestruntime.CountMatchingTests(nil) })
			gotest.Contains(it, fmt.Sprint(got), "gotest generate")
		})
	})
}
