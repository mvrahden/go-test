package setupwaits

import (
	"sync"
	"testing"
)

// holds maps a method's testing.TB to what holds the waits on its gotest.Go
// goroutines to the method's Timeout, while its cleanups run.
var holds sync.Map // testing.TB -> func(<-chan struct{}) func()

// Hold makes hold what a wait on one of tb's gotest.Go goroutines runs
// under, until [Unhold]: it gets the goroutine's done channel before the wait
// and returns what ends the hold after it.
func Hold(tb testing.TB, hold func(done <-chan struct{}) (release func())) {
	holds.Store(tb, hold)
}

// Unhold ends what [Hold] began for tb.
func Unhold(tb testing.TB) { holds.Delete(tb) }

// Held returns tb's hold, nil when none.
func Held(tb testing.TB) func(done <-chan struct{}) (release func()) {
	if h, ok := holds.Load(tb); ok {
		return h.(func(<-chan struct{}) func())
	}
	return nil
}
