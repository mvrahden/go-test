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

// RunTest is what harnesses before [RunMethods] called to hold a test
// method's body to budget.
//
// Deprecated: regenerate the harness.
func RunTest(t *gotest.T, budget time.Duration, run func()) {
	end := watchPhase(t.T(), budget, "", "Timeout")
	defer end()
	defer runstate.EnterBody(t.T(), "a method")()
	run()
}

// AwaitDoneAt and AwaitDone are what harnesses before v1.35 called for a test
// method taking done func(). Such a harness still compiles, and the first such
// method panics with the fix, ending the suite's run: the generator now
// refuses the method.

// Deprecated: regenerate the harness.
func AwaitDoneAt(t *gotest.T, name, at string, start func(done func())) {
	panic(name + ": this generated harness was written by an older gotest and calls a test method taking done func(), which gotest no longer runs. Run `gotest generate` again, and in the method signal from the callback on a channel and wait for it, asserting on the test goroutine:\n" +
		"\tdone := make(chan struct{})\n" +
		"\tgo func() { defer close(done); /* the work that called done() */ }()\n" +
		"\tselect {\n" +
		"\tcase <-done:\n" +
		"\tcase <-t.Context().Done():\n" +
		"\t\tgotest.Fail(t, \"never finished\")\n" +
		"\t}")
}

// Deprecated: regenerate the harness.
func AwaitDone(t *gotest.T, name string, start func(done func())) {
	AwaitDoneAt(t, name, "", start)
}

// Deprecated: regenerate the harness.
func CountMatchingTests([]string) int {
	panic("this generated harness was written by an older gotest: run `gotest generate` again, or delete the gotest_p*suite_test.go files")
}
