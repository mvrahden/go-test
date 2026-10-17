package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// shutdownContext is canceled by the first shutdown signal, which starts the
// graceful shutdown: suites are stopped and fixtures torn down within their
// budgets. A second signal ends the CLI at once with 130, abandoning what
// is left of that teardown, the way a terminal's second Ctrl-C ends any
// other program. The exit is explicit rather than left to the default
// disposition: a CLI started by a parent that ignores SIGINT (go tool
// test2json does) would inherit that and swallow the second signal. The
// suite and fixture processes run in their own process groups, so the
// terminal's signal never reaches them through the CLI.
func shutdownContext() (context.Context, context.CancelFunc) {
	return shutdownContextThen(func() {
		fmt.Fprintln(os.Stderr, "interrupted again: giving up on the teardown")
	})
}

// forcedExit holds off the second signal's exit: work that must not stop
// halfway runs holding it.
var forcedExit sync.Mutex

// shutdownContextThen is shutdownContext whose second signal runs last,
// holding forcedExit, before it exits 130.
func shutdownContextThen(last func()) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	again := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		// Catch the second signal before releasing the first, so none
		// falls through to the default disposition between the two.
		signal.Notify(again, shutdownSignals...)
		stop()
		select {
		case <-again:
			forcedExit.Lock()
			last()
			os.Exit(130)
		case <-done:
		}
	}()
	var once sync.Once
	return ctx, func() {
		stop()
		once.Do(func() { close(done) })
	}
}
