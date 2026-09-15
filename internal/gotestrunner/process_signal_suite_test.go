package gotestrunner_test

import (
	"errors"
	"os"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ProcessSignalTestSuite covers the guard in front of a shutdown request: a
// process that already exited is never signalled.
type ProcessSignalTestSuite struct{}

func (s *ProcessSignalTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ProcessSignalTestSuite) TestSignalIfRunning(t *gotest.T) {
	t.When("the process already exited", func(w *gotest.T) {
		var signalled []int
		err := gotestrunner.ExportSignalIfRunning(42,
			func(int) bool { return false },
			func(pid int) error { signalled = append(signalled, pid); return nil })

		w.It("does not signal it and reports it done", func(it *gotest.T) {
			gotest.ErrorIs(it, err, os.ErrProcessDone)
			gotest.Empty(it, signalled)
		})
	})

	t.When("the process is running", func(w *gotest.T) {
		var signalled []int
		failed := errors.New("signal failed")
		err := gotestrunner.ExportSignalIfRunning(42,
			func(int) bool { return true },
			func(pid int) error { signalled = append(signalled, pid); return failed })

		w.It("signals it and returns the signal's result", func(it *gotest.T) {
			gotest.Equal(it, []int{42}, signalled)
			gotest.ErrorIs(it, err, failed)
		})
	})
}
