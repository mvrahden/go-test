package gotestruntime

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// FixtureOnce and CountMatchingTests are what generated harnesses before
// HoldFixtures called. Such a harness left on disk next to a newer runtime
// still compiles, and fails its first fixture-bound test with the fix instead
// of a missing symbol: CountMatchingTests panics inside FixtureOnce.Do, which
// reports the panic as the setup failure.

// Deprecated: regenerate the harness.
type FixtureOnce struct {
	once sync.Once
	err  error
}

// Deprecated: regenerate the harness.
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

// Deprecated: regenerate the harness.
func CountMatchingTests([]string) int {
	panic("this generated harness was written by an older gotest: run `gotest generate` again, or delete the gotest_p*suite_test.go files")
}
