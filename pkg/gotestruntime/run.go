package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/runstate"
)

// The sidecar files the runner shares with this process, read once and taken
// out of the environment: a process a test starts must not answer to them.
var (
	stopFile   = takeEnv(protocol.EnvStopFile)
	budgetFile = takeEnv(protocol.EnvTeardownBudgetFile)
)

func takeEnv(key string) string {
	v := os.Getenv(key)
	_ = os.Unsetenv(key)
	return v
}

// Teardown registry: one entry per fixture DAG that came up in this process.
var (
	teardownMu sync.Mutex
	teardowns  []func() (failed bool)
	// teardownRun serializes runs of the registry.
	teardownRun sync.Mutex
)

// RegisterTeardown queues a fixture DAG's teardown. Generated code calls it once
// its DAG came up; a DAG that never came up has nothing to release.
func RegisterTeardown(fn func() (failed bool)) {
	teardownMu.Lock()
	teardowns = append(teardowns, fn)
	teardownMu.Unlock()
}

// runTeardowns runs every registered teardown once, in registration order, and
// reports whether any failed.
func runTeardowns() (failed bool) {
	teardownRun.Lock()
	defer teardownRun.Unlock()
	teardownMu.Lock()
	fns := teardowns
	teardowns = nil
	teardownMu.Unlock()
	for _, fn := range fns {
		if fn() {
			failed = true
		}
	}
	return failed
}

// teardownWithin runs the registry, giving up waiting at deadline. It reports
// whether a teardown failed and whether all of them finished.
func teardownWithin(deadline time.Time) (failed, finished bool) {
	done := make(chan bool, 1)
	go func() { done <- runTeardowns() }()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case failed = <-done:
		return failed, true
	case <-timer.C:
		return true, false
	}
}

// Run states of the test binary, as fixture setup sees them.
const (
	runNotStarted int32 = iota // m.Run was called directly, or not yet
	runActive                  // the tests run through Main or M
	runStopping                // a stop is tearing the fixtures down
	runFinished                // Main or M tore the fixtures down
)

var runState atomic.Int32

// Main runs the tests, then tears down every fixture DAG the run set up, and
// returns the exit code. The TestMain gotest generates calls it; a TestMain of
// your own in a package whose suites bind fixtures exits with it in place of
// m.Run:
//
//	func TestMain(m *testing.M) { os.Exit(gotestruntime.Main(m)) }
//
// A teardown failure fails the package: the code becomes 1 when the tests
// passed. Exit with the code: a TestMain that returns makes Go exit with the
// tests' own code, which knows nothing of the teardown.
func Main(m *testing.M) int {
	return runTests(m.Run, os.Stderr)
}

// M wraps m for a library that runs the tests itself through Run, such as
// goleak.VerifyTestMain or testscript.Main. Its Run runs the tests and then
// tears the fixtures down, as [Main] does:
//
//	func TestMain(m *testing.M) { goleak.VerifyTestMain(gotestruntime.M(m)) }
func M(m *testing.M) interface{ Run() int } {
	return wrappedM{m}
}

type wrappedM struct{ m *testing.M }

func (w wrappedM) Run() int { return runTests(w.m.Run, os.Stderr) }

// runTests owns the span in which fixture resources can exist: the stop
// watcher lives exactly as long, and nothing of it outlives the call.
func runTests(run func() int, stderr io.Writer) int {
	if !runState.CompareAndSwap(runNotStarted, runActive) {
		// A second run: fixture setup refuses it (RequireMain).
		return run()
	}
	started := time.Now()
	w := watch(stderr)
	code := run()
	setupMu.Lock()
	finished := runState.CompareAndSwap(runActive, runFinished)
	setupMu.Unlock()
	if !finished {
		// A stop is tearing down; it ends the process by its signal.
		select {}
	}
	code = finishWithin(code, stderr, teardownDeadline(started))
	w.close()
	return code
}

func finish(code int, stderr io.Writer) int {
	if runTeardowns() && code == 0 {
		fmt.Fprintln(stderr, "FAIL: fixture teardown failed")
		return 1
	}
	return code
}

// finishWithin is finish held to the -test.timeout the tests ran under: m.Run
// stopped its own alarm when it returned, and a teardown that never returns
// must still end the run, the way the alarm would have.
func finishWithin(code int, stderr io.Writer, deadline time.Time) int {
	if deadline.IsZero() {
		return finish(code, stderr)
	}
	failed, finished := teardownWithin(deadline)
	if !finished {
		fmt.Fprintf(stderr, "panic: test timed out: fixture teardown still running at the -test.timeout deadline\n\n%s\n", allStacks())
		os.Exit(2)
	}
	if failed && code == 0 {
		fmt.Fprintln(stderr, "FAIL: fixture teardown failed")
		return 1
	}
	return code
}

// teardownDeadline is the -test.timeout deadline counted from the start of the
// run, or zero when the timeout is disabled.
func teardownDeadline(started time.Time) time.Time {
	d := flagDuration("test.timeout")
	if d <= 0 {
		return time.Time{}
	}
	return started.Add(d)
}

// RequireMain reports why fixtures cannot be set up now, or nil. Generated
// setup calls it before every fixture-bound test, benchmark or fuzz target:
// fixtures tear down when Main or M finishes, so tests that did not run
// through either would leave them up, and tests that run after it, or while a
// stop tears down, would find them released.
func RequireMain() error {
	switch runState.Load() {
	case runActive:
		return nil
	case runStopping:
		return errors.New("the run is stopping: no fixture is set up any more")
	case runFinished:
		return errors.New("fixtures were already torn down: the tests ran again after gotestruntime.Main or gotestruntime.M returned; run them once, through either")
	default:
		return errors.New("fixtures tear down after the tests only when they run through gotestruntime: in TestMain, call os.Exit(gotestruntime.Main(m)) in place of os.Exit(m.Run()), or pass gotestruntime.M(m) to a library that takes m")
	}
}

// Fixture setup in flight. A stop waits for it: a cancelled setup tears down
// what it brought up, and a setup that finished registers its teardown.
var (
	setupMu       sync.Mutex
	setupInFlight int
)

// beginSetup admits a fixture setup unless a stop has begun, and returns the
// context it runs under: canceled by a stop as well as by ctx.
func beginSetup(ctx context.Context) (context.Context, func(), error) {
	setupMu.Lock()
	defer setupMu.Unlock()
	if runState.Load() == runStopping {
		return nil, nil, errors.New("the run is stopping: no fixture is set up any more")
	}
	setupInFlight++
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(runstate.StopContext(), cancel)
	return ctx, func() {
		stop()
		cancel()
		setupMu.Lock()
		setupInFlight--
		setupMu.Unlock()
	}, nil
}

// beginStop moves an active run to stopping; false when the run is not active.
func beginStop() bool {
	setupMu.Lock()
	defer setupMu.Unlock()
	return runState.CompareAndSwap(runActive, runStopping)
}

// waitSetups waits for setups in flight, until deadline.
func waitSetups(deadline time.Time) bool {
	for {
		setupMu.Lock()
		n := setupInFlight
		setupMu.Unlock()
		if n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The teardown budget this process advertised to the runner: the longest
// fixture path plus suite setup plus headroom, over every DAG it set up.
var (
	budgetMu  sync.Mutex
	maxBudget time.Duration
)

func recordBudget(d time.Duration) time.Duration {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	if d > maxBudget {
		maxBudget = d
	}
	return maxBudget
}

// releaseBound is how long a stop or a panic may spend releasing fixtures:
// within the budget the runner waits before it kills the process, so the
// process says what it could not release instead of being killed silently.
func releaseBound() time.Duration {
	budgetMu.Lock()
	b := maxBudget
	budgetMu.Unlock()
	if b <= 0 {
		return 2 * time.Minute
	}
	if b > 10*time.Second {
		return b - 5*time.Second
	}
	return b
}

// NotePanic marks the process as dying when a panic passes, then lets it go on.
// Generated code defers it first in the closures the testing package calls, so
// it runs last and sees panics from every other deferred call as well. It must
// be deferred directly: recover only reports a panic to the function the
// runtime defers. Under GODEBUG=panicnil=1 it does not recover at all.
func NotePanic() {
	if runstate.PanicNilLegacy {
		return
	}
	if r := recover(); r != nil {
		runstate.MarkDying(r, debug.Stack())
		panic(r)
	}
}

// Guard watches a top-level generated test, benchmark or fuzz function in a
// package whose binary sets fixtures up, and releases them when a test under
// it panics: the testing package then runs the cleanups of the panicking test
// and every ancestor, and ends the process, so Main never gets to.
//
// Two signals mark that path. A panic that passed a gotest hook marks the
// process dying. A panic in a subtest gotest did not make (t.T().Run, a
// panicking cleanup) passes none, but the testing package runs the ancestors'
// cleanups while their bodies are still blocked in t.Run, and a cleanup runs
// before its body ended only on that path.
type Guard struct {
	tb    testing.TB
	ended atomic.Bool
}

// GuardBody starts a Guard for tb. Generated code calls it first, as
// defer gotestruntime.GuardBody(t).End(), so its cleanup runs after every
// cleanup the function registers later, the suite's AfterAll among them.
func GuardBody(tb testing.TB) *Guard {
	g := &Guard{tb: tb}
	tb.Cleanup(g.cleanup)
	return g
}

// End records that the body ended — by returning, or by Goexit from FailNow or
// SkipNow — and notes a panic that leaves it. It must be deferred directly.
func (g *Guard) End() {
	if runstate.PanicNilLegacy {
		g.ended.Store(true)
		return
	}
	if r := recover(); r != nil {
		runstate.MarkDying(r, debug.Stack())
		panic(r)
	}
	g.ended.Store(true)
}

func (g *Guard) cleanup() {
	if runstate.Dying() || !g.ended.Load() {
		releaseOnPanic(g.tb.Name())
	}
}

var releaseOnce sync.Once

// releaseOnPanic prints the panic that ends the process before tearing the
// fixtures down — a sibling test failing on a released fixture, or a teardown
// blocked on what a sibling holds, must not be all that is left to read — then
// tears down within the release bound.
func releaseOnPanic(test string) {
	releaseOnce.Do(func() {
		if v, stack, ok := runstate.Noted(); ok {
			fmt.Fprintf(os.Stderr, "gotest: %s panicked; releasing fixtures before the process ends:\npanic: %v\n\n%s\n", test, v, stack)
		} else {
			fmt.Fprintf(os.Stderr, "gotest: a test under %s panicked; releasing fixtures before the process ends\n", test)
		}
		if _, finished := teardownWithin(time.Now().Add(releaseBound())); !finished {
			fmt.Fprintf(os.Stderr, "gotest: fixture teardown did not finish within %s; what it holds may be left behind\n", releaseBound())
		}
	})
}

// CountMatchingTests is what generated harnesses before teardown-after-m.Run
// called. Such a harness left on disk next to a newer runtime fails its first
// fixture-bound test with the fix, instead of a missing symbol.
//
// Deprecated: regenerate the harness.
func CountMatchingTests([]string) int {
	panic("this generated harness was written by an older gotest: run `gotest generate` again, or delete the gotest_p*suite_test.go files")
}

func allStacks() []byte {
	buf := make([]byte, 1<<20)
	return buf[:runtimeStack(buf)]
}
