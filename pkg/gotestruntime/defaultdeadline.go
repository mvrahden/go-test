package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

// defaultDeadlineOut is where the default-deadline line goes: the test's own
// output under test2json, which streams it with its test, and stderr,
// unbuffered, otherwise.
var defaultDeadlineOut = func(tb testing.TB) io.Writer {
	if test2json() {
		return tb.Output()
	}
	return os.Stderr
}

// noteDefaultDeadline says once, naming who, when ctx ends at the default
// timeout of a suite that declares no field: nothing fails, so a phase that
// waited on its context would otherwise see only the error it returned. A
// stop or the phase's end cancels ctx instead and says nothing. The returned
// call, made when the phase returns, ends the note.
func noteDefaultDeadline(tb testing.TB, ctx context.Context, who, field string, timeout time.Duration) (end func()) {
	say := func() {
		if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			fmt.Fprintf(defaultDeadlineOut(tb), "%s: t.Context() ended at the default %s deadline; the suite declares no %s\n", who, timeout, field)
		}
	}
	said := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(said)
		say()
	})
	// A phase that returns the moment its context ends gets here before the
	// line is written. Either it waits for the callback, as the test's output
	// panics once the test has completed, or it stops the callback before it
	// started, the context already done: then it says so itself.
	return sync.OnceFunc(func() {
		if stop() {
			say()
			return
		}
		<-said
	})
}

// defaultApplies reports whether timeout is the default standing in for a
// budget the suite did not declare.
func defaultApplies(timeout, budget time.Duration) bool {
	return budget == 0 && timeout > 0
}
