package gotestruntime

import (
	"fmt"
	"runtime/debug"
	"sync"
)

type FixtureOnce struct {
	once sync.Once
	err  error
}

// Do runs fn exactly once and returns its result to every caller.
//
// A panic in fn is converted to an error rather than allowed to escape. sync.Once
// marks itself done even when its body panics, so an escaping panic would leave
// err nil while the state fn was meant to build was never assigned — the next
// caller would be told setup succeeded and then dereference nothing.
func (f *FixtureOnce) Do(fn func() error) error {
	f.once.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				f.err = fmt.Errorf("panic: %v\n\n%s", r, debug.Stack())
			}
		}()
		f.err = fn()
	})
	return f.err
}
