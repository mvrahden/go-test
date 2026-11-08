package main_test

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ControlInputTestSuite: the stop and force lines --control reads, counted
// with the signals as one source. The run's processes are given up by a
// stand-in, and the exit it asks for is observed.
// Sequential: the exit it observes is the process's.
type ControlInputTestSuite struct {
	run *controlledRun
}

func (s *ControlInputTestSuite) AfterEach(t *gotest.T) {
	if s.run != nil {
		s.run.end()
	}
}

// controlledRun is one shutdown context fed by a pipe.
type controlledRun struct {
	ctx     context.Context
	input   *io.PipeWriter
	lastRan atomic.Bool
	exits   chan int
	ended   func()
	restore func()
}

func (s *ControlInputTestSuite) start(t *gotest.T) *controlledRun {
	r, w := io.Pipe()
	run := &controlledRun{input: w, exits: make(chan int, 4)}
	run.restore = main.ExportObserveExit(func(code int, _ <-chan struct{}) { run.exits <- code })
	run.ctx, run.ended = main.ExportShutdownContextThen(r, func(<-chan struct{}) { run.lastRan.Store(true) })
	s.run = run
	return run
}

// send writes lines to the run's control input.
func (r *controlledRun) send(t *gotest.T, lines ...string) {
	for _, l := range lines {
		_, err := io.WriteString(r.input, l)
		gotest.NoError(t, err)
	}
}

// end ends the run and lets a forced stop exit.
func (r *controlledRun) end() {
	r.ended()
	_ = r.input.Close()
	r.restore()
}

func stopped(t *gotest.T, r *controlledRun) {
	select {
	case <-r.ctx.Done():
	case <-time.After(3 * time.Second):
		gotest.Fail(t, "the run was not stopped")
	}
}

func notStopped(t *gotest.T, r *controlledRun, d time.Duration) {
	gotest.Consistently(t, d, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.NoError(poll, r.ctx.Err())
	})
}

func forced(t *gotest.T, r *controlledRun) {
	gotest.Eventually(t, 3*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, r.lastRan.Load())
	})
}

func notForced(t *gotest.T, r *controlledRun, d time.Duration) {
	gotest.Consistently(t, d, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.False(poll, r.lastRan.Load())
	})
}

func (s *ControlInputTestSuite) TestAStopLine(t *gotest.T) {
	r := s.start(t)
	r.send(t, "status\n", "stop\r\n")
	stopped(t, r)
	t.When("a stop line comes, after a line it does not know", func(w *gotest.T) {
		w.It("stops the run without forcing it", func(it *gotest.T) {
			notForced(it, r, 2*main.ExportDuplicateWindow)
		})
	})
}

func (s *ControlInputTestSuite) TestTheInputEnds(t *gotest.T) {
	r := s.start(t)
	gotest.NoError(t, r.input.Close())
	stopped(t, r)
	t.When("the input ends with no stop under way", func(w *gotest.T) {
		w.It("stops the run without forcing it", func(it *gotest.T) {
			notForced(it, r, 2*main.ExportDuplicateWindow)
		})
	})
}

func (s *ControlInputTestSuite) TestAnUnknownLine(t *gotest.T) {
	r := s.start(t)
	r.send(t, "stopp\n", "STOP\n", " force\n")
	t.When("only lines it does not know come", func(w *gotest.T) {
		w.It("leaves the run alone", func(it *gotest.T) {
			notStopped(it, r, 200*time.Millisecond)
		})
	})
}

func (s *ControlInputTestSuite) TestAForceLineAfterAStop(t *gotest.T) {
	r := s.start(t)
	r.send(t, "stop\n", "force\n")
	t.When("a force line follows a stop line at once", func(w *gotest.T) {
		w.It("forces the stop, never taken for a repeat of the first", func(it *gotest.T) {
			stopped(it, r)
			forced(it, r)
		})
	})
}

func (s *ControlInputTestSuite) TestAForceLineAlone(t *gotest.T) {
	r := s.start(t)
	r.send(t, "force\n")
	t.When("a force line comes with no stop under way", func(w *gotest.T) {
		w.It("stops the run and forces the stop", func(it *gotest.T) {
			stopped(it, r)
			forced(it, r)
		})
	})
}

func (s *ControlInputTestSuite) TestARepeatedStopLine(t *gotest.T) {
	r := s.start(t)
	r.send(t, "stop\n", "stop\n")
	stopped(t, r)
	t.When("a second stop line comes within the window of a repeat", func(w *gotest.T) {
		w.It("is the same stop", func(it *gotest.T) {
			notForced(it, r, 2*main.ExportDuplicateWindow)
		})
	})
	t.When("a further stop line comes past the window", func(w *gotest.T) {
		w.It("forces the stop", func(it *gotest.T) {
			r.send(it, "stop\n")
			forced(it, r)
		})
	})
}

func (s *ControlInputTestSuite) TestTheInputEndsDuringAStop(t *gotest.T) {
	r := s.start(t)
	r.send(t, "stop\n")
	stopped(t, r)
	gotest.NoError(t, r.input.Close())
	t.When("the input ends while a stop is under way", func(w *gotest.T) {
		w.It("changes nothing", func(it *gotest.T) {
			notForced(it, r, 2*main.ExportDuplicateWindow)
		})
	})
}

func (s *ControlInputTestSuite) TestAFurtherForceLine(t *gotest.T) {
	r := s.start(t)
	r.send(t, "force\n")
	forced(t, r)
	r.ended()
	t.When("a further force line comes once the run ended on its own", func(w *gotest.T) {
		w.It("exits 130 at once", func(it *gotest.T) {
			r.send(it, "force\n")
			select {
			case code := <-r.exits:
				gotest.Equal(it, 130, code)
			case <-time.After(time.Second):
				gotest.Fail(it, "a further force line did not exit")
			}
		})
	})
}
