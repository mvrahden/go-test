package runstate

import (
	"sync"
	"sync/atomic"
)

// abendWatches holds, per test whose async method waits for done, whether a
// goroutine gotest.Go started for it ended in a panic or runtime.Goexit. Such
// a goroutine re-raises at the test's cleanup, after the wait: the wait learns
// of it here.
var abendWatches sync.Map // owner -> *atomic.Bool

// WatchAbends records for owner, until stop, whether such a goroutine ended.
func WatchAbends(owner any) (abended func() bool, stop func()) {
	flag := new(atomic.Bool)
	abendWatches.Store(owner, flag)
	return flag.Load, func() { abendWatches.CompareAndDelete(owner, flag) }
}

// Abend records that a goroutine gotest.Go started for owner ended in a panic
// or runtime.Goexit; nothing records it while no wait watches owner.
func Abend(owner any) {
	if flag, ok := abendWatches.Load(owner); ok {
		flag.(*atomic.Bool).Store(true)
	}
}
