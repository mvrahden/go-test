package main_test

import (
	"sync"
	"sync/atomic"
	"time"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ForcedStopTestSuite: what the second signal does once it is told apart
// from a repeat of the first. The run's processes are given up, the run
// ends on its own and its own exit follows; the second signal exits the
// process itself only as the backstop.
// Sequential: the exit it observes is the process's.
type ForcedStopTestSuite struct {
	stop *forcedStop
}

func (s *ForcedStopTestSuite) AfterEach(t *gotest.T) {
	if s.stop != nil {
		s.stop.end()
	}
}

// anExit is one exit the second signal asked for.
type anExit struct {
	code      int
	afterLast bool // the run's processes had been given up
}

// forcedStop drives one forcedStop: hurry and done are the test's to
// close, exits receives what it exited with.
type forcedStop struct {
	hurry, done chan struct{}
	lastRan     atomic.Bool
	exits       chan anExit
	atExit      func()
	further     func() // a further signal
	ended       func() // the run ended on its own
	release     func() // releases forcedExit, held by the test
	restore     func()
	wait        func()
	once        sync.Once
}

// start runs the second signal's action with the bound end on a goroutine
// of the method's own. last marks that it ran; atExit, when set, runs at
// each exit before it is recorded.
func (s *ForcedStopTestSuite) start(t *gotest.T, end time.Duration, atExit func()) *forcedStop {
	f := &forcedStop{hurry: make(chan struct{}), done: make(chan struct{}), exits: make(chan anExit, 4), atExit: atExit}
	f.further = sync.OnceFunc(func() { close(f.hurry) })
	f.ended = sync.OnceFunc(func() { close(f.done) })
	f.restore = main.ExportObserveExit(func(code int, _ <-chan struct{}) {
		if f.atExit != nil {
			f.atExit()
		}
		f.exits <- anExit{code: code, afterLast: f.lastRan.Load()}
	})
	f.wait = gotest.Go(t, func() {
		main.ExportForcedStop(f.hurry, f.done, func(<-chan struct{}) { f.lastRan.Store(true) }, end)
	})
	s.stop = f
	return f
}

// end lets the action return, however the test left it.
func (f *forcedStop) end() {
	f.once.Do(func() {
		if f.release != nil {
			f.release()
		}
		f.further()
		f.wait()
		f.restore()
	})
}

// exited is the exit the action asked for within 3s.
func exited(t *gotest.T, f *forcedStop) anExit {
	select {
	case e := <-f.exits:
		return e
	case <-time.After(3 * time.Second):
		gotest.Fail(t, "the second signal did not exit")
		return anExit{}
	}
}

// noExit asserts that the action asked for no exit within d.
func noExit(t *gotest.T, f *forcedStop, d time.Duration) {
	gotest.Consistently(t, d, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.Empty(poll, f.exits)
	})
}

func (s *ForcedStopTestSuite) TestTheRunEndsOnItsOwn(t *gotest.T) {
	f := s.start(t, 50*time.Millisecond, nil)
	f.ended()

	t.When("the run ends on its own once the second signal gave its processes up", func(w *gotest.T) {
		w.It("does not exit: the run's own exit follows", func(it *gotest.T) {
			noExit(it, f, 300*time.Millisecond)
		})
		w.It("exits 130 at once on a further signal", func(it *gotest.T) {
			f.further()
			e := exited(it, f)
			gotest.Equal(it, 130, e.code)
			gotest.True(it, e.afterLast)
		})
	})
}

func (s *ForcedStopTestSuite) TestTheRunDoesNotEnd(t *gotest.T) {
	f := s.start(t, 300*time.Millisecond, nil)

	t.When("the run has not ended once the second signal gave its processes up", func(w *gotest.T) {
		w.It("exits 130 at its bound, once its processes are given up", func(it *gotest.T) {
			noExit(it, f, 100*time.Millisecond)
			e := exited(it, f)
			gotest.Equal(it, 130, e.code)
			gotest.True(it, e.afterLast)
		})
	})
}

func (s *ForcedStopTestSuite) TestAFurtherSignal(t *gotest.T) {
	f := s.start(t, time.Minute, nil)
	f.further()

	t.When("a further signal comes while the run has not ended", func(w *gotest.T) {
		w.It("exits 130 at once", func(it *gotest.T) {
			e := exited(it, f)
			gotest.Equal(it, 130, e.code)
			gotest.True(it, e.afterLast)
		})
	})
}

func (s *ForcedStopTestSuite) TestAWriteThatMustNotStopHalfway(t *gotest.T) {
	release := sync.OnceFunc(main.ExportHoldForcedExit())
	f := s.start(t, 0, nil)
	f.release = release

	t.When("a write that must not stop halfway is under way at the bound", func(w *gotest.T) {
		w.It("holds its exit until the write is done", func(it *gotest.T) {
			noExit(it, f, 200*time.Millisecond)
			release()
			e := exited(it, f)
			gotest.Equal(it, 130, e.code)
		})
	})
}
