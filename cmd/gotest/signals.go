package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
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
//
// control, when not nil, carries stop and force lines too (see readControl).
func shutdownContext(control io.Reader) (context.Context, context.CancelFunc) {
	return shutdownContextThen(control, func(hurry <-chan struct{}) {
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
func shutdownContextThen(control io.Reader, last func(hurry <-chan struct{})) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	// Caught from the start to the process's end, so no signal falls through
	// to the default disposition; one past the run is dropped.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, shutdownSignals...)
	events := make(chan stopEvent)
	go func() {
		for range sigs {
			events <- aStop
		}
	}()
	if control != nil {
		go readControl(control, events)
	}
	done := make(chan struct{})
	go func() {
		forced, ok := firstStop(events, done)
		if !ok {
			return
		}
		cancel()
		second, further := secondStop(events, done, forced)
		if !second {
			return
		}
		hurry := make(chan struct{})
		if further {
			close(hurry)
		} else {
			go closeOnFurther(events, hurry, time.Now())
		}
		forcedStop(hurry, done, last, endBound)
	}()
	var once sync.Once
	return ctx, func() {
		cancel()
		once.Do(func() { close(done) })
	}
}

// stopEvent is one event of the merged source: a signal or a control line.
type stopEvent uint8

const (
	aStop    stopEvent = iota // a signal or a stop line
	aForce                    // a force line: the second stop, never a repeat of the first
	inputEnd                  // the control input ended: a stop when none is under way
)

// readControl sends the stop and force lines of control to events, and its
// end; any other line is ignored. A trailing \r is tolerated.
func readControl(control io.Reader, events chan<- stopEvent) {
	lines := bufio.NewScanner(control)
	for lines.Scan() {
		switch strings.TrimSuffix(lines.Text(), "\r") {
		case "stop":
			events <- aStop
		case "force":
			events <- aForce
		}
	}
	events <- inputEnd
}

// firstStop waits for the first stop, forced when it is a force line: false
// when the run ends first.
func firstStop(events <-chan stopEvent, done <-chan struct{}) (forced, ok bool) {
	select {
	case e := <-events:
		return e == aForce, true
	case <-done:
		return false, false
	}
}

// secondStop waits for the second stop, past the window of a repeat of the
// first: false when the run ends first. A force counts at once, the first
// one included, but acts only once the window is over, so the first stop
// has reached the suites by then; further reports a force after it.
func secondStop(events <-chan stopEvent, done <-chan struct{}, forced bool) (second, further bool) {
	window := time.NewTimer(duplicateWindow)
	defer window.Stop()
	over := false
	for !forced || !over {
		select {
		case e := <-events:
			switch {
			case e == aForce:
				further = forced
				forced = true
			case e == inputEnd:
				// The input ending changes no stop under way.
			case over:
				return true, false
			}
			// Else the first signal, delivered once more.
		case <-window.C:
			over = true
		case <-done:
			return false, false
		}
	}
	return true, further
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
// sent at second, delivered once more, or on a further force line.
func closeOnFurther(events <-chan stopEvent, hurry chan<- struct{}, second time.Time) {
	for e := range events {
		if e == aForce || (e == aStop && time.Since(second) >= duplicateWindow) {
			close(hurry)
			return
		}
	}
}
