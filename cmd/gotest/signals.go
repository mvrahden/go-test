package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// shutdownContext is canceled by the first shutdown signal, which starts the
// graceful shutdown: suites are stopped and fixtures torn down within their
// budgets. A second signal gives up on that teardown, the way a terminal's
// second Ctrl-C ends any other program: the suites are told to release
// their fixtures at once, what still runs abandonBound later is killed, and
// the run ends on its own path with 130, its stream and verdict complete. A
// run that has not ended endBound after that is exited 130 by the signal
// itself; a third kills it without waiting. That exit is explicit rather
// than left to the default disposition: a CLI started by a parent that
// ignores SIGINT (go tool test2json does) would inherit that and swallow
// the second signal. The suite and fixture processes run in their own
// process groups, so the terminal's signal never reaches them through the
// CLI.
//
// A signal that follows the first within duplicateWindow is the same event
// delivered twice, not a second Ctrl-C: `go tool` forwards the terminal's
// signal to the tool it runs, which is in the terminal's process group and
// received it already.
func shutdownContext() (context.Context, context.CancelFunc) {
	return shutdownContextThen(func(hurry <-chan struct{}) {
		fmt.Fprintf(os.Stderr, abandonMessage, abandonBound)
		gotestrunner.Abandon(abandonBound, hurry)
	})
}

// forcedExit holds off the second signal's exit: work that must not stop
// halfway runs holding it.
var forcedExit sync.Mutex

// abandonBound is how long a second signal waits for the suites, told to
// release their fixtures at once, before it kills what still runs.
const abandonBound = 2 * time.Second

// endBound is how long the second signal waits for the run to end on its
// own once its processes are gone, before it exits 130 itself.
const endBound = 2 * time.Second

// duplicateWindow is how long after the first signal a repeat counts as the
// same event; a person's second Ctrl-C comes later than a launcher's forward.
const duplicateWindow = 300 * time.Millisecond

// shutdownContextThen is shutdownContext whose second signal runs last,
// holding forcedExit, before it exits 130. hurry closes on a further signal,
// one past the window of a repeat of the second.
func shutdownContextThen(last func(hurry <-chan struct{})) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	again := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		// Catch the second signal before releasing the first, so none
		// falls through to the default disposition between the two.
		signal.Notify(again, shutdownSignals...)
		stop()
		if !secondSignal(again, done) {
			return
		}
		hurry := make(chan struct{})
		go closeOnFurther(again, hurry, time.Now())
		forcedStop(hurry, done, last, endBound)
	}()
	var once sync.Once
	return ctx, func() {
		stop()
		once.Do(func() { close(done) })
	}
}

// secondSignal waits for the second signal, past the window of a repeat of
// the first: false when the run ends first.
func secondSignal(again <-chan os.Signal, done <-chan struct{}) bool {
	window := time.NewTimer(duplicateWindow)
	defer window.Stop()
	over := false
	for {
		select {
		case <-again:
			if over {
				return true
			}
			// The first signal, delivered once more.
		case <-window.C:
			over = true
		case <-done:
			return false
		}
	}
}

// forcedStop is the second signal's action. last gives the run's processes
// up; the run then ends on its own, its stream and verdict complete, and
// its own exit follows. A run that has not ended within end, or a further
// signal, is exited 130 here instead, once nothing holds forcedExit.
func forcedStop(hurry, done <-chan struct{}, last func(hurry <-chan struct{}), end time.Duration) {
	last(hurry)
	timer := time.NewTimer(end)
	defer timer.Stop()
	select {
	case <-done:
		<-hurry // the run's own exit follows; a further signal still ends it at once
	case <-timer.C:
		fmt.Fprintf(os.Stderr, endMessage, end)
	case <-hurry:
	}
	forcedExit.Lock()
	defer forcedExit.Unlock()
	exit(130, hurry)
}

// endMessage says that the run did not end on its own, given endBound.
const endMessage = "the run did not end within %s of its processes ending; exiting 130\n"

// exit ends the process between two events of the stream; a test observes
// it instead.
var exit = gotestrunner.Exit

// closeOnFurther closes hurry on the first signal that is not the second,
// sent at second, delivered once more.
func closeOnFurther(again <-chan os.Signal, hurry chan<- struct{}, second time.Time) {
	for range again {
		if time.Since(second) >= duplicateWindow {
			close(hurry)
			return
		}
	}
}
