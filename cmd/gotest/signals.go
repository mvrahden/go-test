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
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	again := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		stop()
		signal.Notify(again, shutdownSignals...)
		select {
		case <-again:
			fmt.Fprintln(os.Stderr, "interrupted again: giving up on the teardown")
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
