package gotestruntime

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
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

// AwaitDone is what harnesses before [AwaitDoneAt] called; its verdict names
// no location.
//
// Deprecated: regenerate the harness.
func AwaitDone(t *gotest.T, name string, start func(done func())) {
	AwaitDoneAt(t, name, "", start)
}

// RunTest is what harnesses before [RunMethods] called to hold a test
// method's body to budget.
//
// Deprecated: regenerate the harness.
func RunTest(t *gotest.T, budget time.Duration, run func()) {
	end := watchPhase(t.T(), budget, "", "Timeout")
	methodWatches.Store(t.T(), end)
	defer methodWatches.Delete(t.T())
	defer end()
	defer runstate.EnterBody(t.T(), "a method")()
	run()
}

// Deprecated: regenerate the harness.
func CountMatchingTests([]string) int {
	panic("this generated harness was written by an older gotest: run `gotest generate` again, or delete the gotest_p*suite_test.go files")
}
