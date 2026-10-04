package gotestruntime_test

import (
	"bytes"
	"runtime"

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

func (s *TeardownRegistryTestSuite) TestTeardownIfNotDying(t *gotest.T) {
	t.When("no panic was noted", func(w *gotest.T) {
		w.It("leaves the fixtures up for the tests still to run", func(it *gotest.T) {
			ran := false
			gotestruntime.RegisterTeardown(func() bool { ran = true; return false })
			gotestruntime.TeardownIfDying()
			gotest.False(it, ran)
		})
	})
}

func (s *TeardownRegistryTestSuite) TestTeardownIfDying(t *gotest.T) {
	t.When("a panic was noted", func(w *gotest.T) {
		w.It("tears every registered DAG down, once", func(it *gotest.T) {
			runs := 0
			gotestruntime.RegisterTeardown(func() bool { runs++; return false })
			gotest.Panics(it, func() {
				defer gotestruntime.NotePanic()
				panic("boom")
			})
			gotestruntime.TeardownIfDying()
			gotestruntime.TeardownIfDying()
			gotest.Equal(it, 1, runs)
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
