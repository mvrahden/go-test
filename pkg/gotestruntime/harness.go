package gotestruntime

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/carry"
	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/internal/schedinfo"
	"github.com/mvrahden/go-test/internal/setupwaits"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// This file holds the wiring that `gotest generate` emits into a suite harness.
// It lives here rather than in pkg/gotest so that suite authors never meet it:
// everything exported from pkg/gotest is something you would deliberately call
// from a test.

// TestCase is the shape of a generated test-method reference.
type TestCase func(*gotest.T)

// Suite is the frame of a suite as a generated wrapper hands it to
// [OpenSuite]. Guard and Config are nil when the suite declares none,
// Fixtures when it binds none.
type Suite struct {
	Guard func() string
	// Fixtures holds the suite's fixtures on tb and wires its fixture fields.
	Fixtures  func(tb testing.TB)
	Config    func() gotest.SuiteConfig
	BeforeAll func(*gotest.T)
	AfterAll  func(*gotest.T)
}

// OpenSuite runs the frame every generated Test, Benchmark and Fuzz wrapper
// shares: the guard, the fixtures, the config, BeforeAll, and AfterAll as
// tb's cleanup. A suite its guard skips sets up no fixtures; the fixtures are
// held before AfterAll is registered, so they are torn down after it. It
// returns the config the suite runs under and the budget it declared, which
// is zero without a SuiteConfig.
func OpenSuite(tb testing.TB, s Suite) (cfg, budget gotest.SuiteConfig) {
	tb.Helper()
	b, bench := tb.(*testing.B)
	if bench {
		// A benchmark's panic can be lost (see [ReleaseOnPanic]); this one,
		// from the guard to BeforeAll, fails the benchmark instead.
		defer ReleaseOnPanic(b)
	}
	// A suite opening after the run was stopped — another top-level function
	// of a stopped process — has nothing to set up.
	if runstate.StopContext().Err() != nil {
		tb.Skip("run stopping")
	}
	if s.Guard != nil {
		if reason := s.Guard(); reason != "" {
			tb.Skipf("suite guard: %s", reason)
		}
	}
	// A stop skips the methods not started and runs AfterAll, with or without
	// fixtures to hold, and waits for the suite to end; registered first, the
	// watch ends after every other cleanup of tb.
	startWatch()
	openSuites.Add(1)
	tb.Cleanup(func() {
		openSuites.Add(-1)
		stopWatch()
	})
	if s.Fixtures != nil {
		s.Fixtures(tb)
	}
	cfg = gotest.DefaultSuiteConfig()
	if s.Config != nil {
		budget = s.Config()
		cfg = WithSuiteDefaults(budget)
	}
	// Never wait on the suite's subtests here: on panic, testing runs ancestor
	// cleanups from the panicking goroutine, and the wait would deadlock.
	tb.Cleanup(func() {
		if bench {
			defer failOnPanic(b)
		}
		// The waits on goroutines BeforeAll started come after AfterAll, which
		// is what stops them; as cleanups of tb they would run first.
		defer drainSetupWaits(tb, budget.SetupTimeout)
		RunTeardown(tb, cfg.SetupTimeout, budget.SetupTimeout, s.AfterAll)
	})
	RunSetup(tb, cfg.SetupTimeout, budget.SetupTimeout, s.BeforeAll)
	return cfg, budget
}

// SetupT builds the *gotest.T handed to a suite's BeforeAll. The waits of the
// goroutines it, or any T derived from it, starts with gotest.Go are kept for
// [drainSetupWaits].
func SetupT(t testing.TB, timeout time.Duration) *gotest.T {
	scoped := testScopedT(t, timeout)
	return gotest.NewTWithContext(t, setupwaits.Open(scoped.Context(), t))
}

// drainSetupWaits waits for the goroutines tb's BeforeAll started, held to
// budget as AfterAll is.
func drainSetupWaits(tb testing.TB, budget time.Duration) {
	set := setupwaits.Close(tb)
	if set == nil {
		return
	}
	watchWhile(tb, budget, "the wait on a goroutine BeforeAll started with gotest.Go ", "SetupTimeout", func() {
		drain(set)
	})
}

// drain runs set's waits, last first, as cleanups would, until none is left.
// Each runs deferred, so one that fails or panics still leaves the others to
// be waited for.
func drain(set *setupwaits.Set) {
	waits := set.Take()
	if len(waits) == 0 {
		return
	}
	defer drain(set)
	for _, wait := range waits {
		defer wait()
	}
}

// AfterEach runs after, a test method's AfterEach, from the defer that pairs it
// with BeforeEach. A panic in BeforeEach or the method is recovered first and
// re-raised once after returns: a fatal assertion in after would otherwise
// end the goroutine and discard the panic with it.
func AfterEach(after func()) {
	if p := carry.From(recover(), "the test method, ahead of its AfterEach"); p != nil {
		defer panic(p)
	}
	enterPhase("AfterEach ")
	after()
}

// AfterEachT builds the *gotest.T handed to a test method's AfterEach from
// t, the method's: a stop does not end its context, so cleanup that honours
// its context runs to its end, but the method's deadline still does. Under a
// declared Timeout it ends as the method's own context does, once the budget
// watch has judged an overrun.
func AfterEachT(t *gotest.T) *gotest.T {
	if after, ok := afterTs.Load(t); ok {
		return after.(*gotest.T)
	}
	tb := t.T()
	if tb == nil {
		return t
	}
	parent := t.Context()
	ctx := context.WithoutCancel(parent)
	if deadline, ok := parent.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		tb.Cleanup(cancel)
	}
	return gotest.NewTWithContext(tb, ctx)
}

// SkipIfStopped skips b, a benchmark the generated code is about to start,
// once the run is stopped, with the reason "run stopping" and no location.
func SkipIfStopped(b *testing.B) {
	if runstate.StopContext().Err() != nil {
		fmt.Fprintln(b.Output(), "run stopping")
		b.SkipNow()
	}
}

// RunBody runs a benchmark's body as one a stop gives grace to.
func RunBody(b *testing.B, body func()) {
	defer runstate.EnterBody(b, "a benchmark")()
	body()
}

// TestT builds the *gotest.T handed to a single test method. The timeout only
// bounds the context here; holding the method to it is [RunMethods]' job, because
// code that ignores its context runs to completion either way.
func TestT(t *testing.T, timeout time.Duration) *gotest.T {
	return testScopedT(t, timeout)
}

// methodWatches holds, per running test method, its budget watch: an async
// method's body ends before the method does (see [AwaitDoneAt]).
var methodWatches sync.Map // *testing.T -> bodyWatch

// bodyWatch is a method's budget watch as an async method's wait sees it.
type bodyWatch interface {
	// endBody ends the watch, the body having returned, and returns the
	// budget the suite declared; zero when it declared none.
	endBody() time.Duration
	// resume holds the hooks after the wait to what is left of the budget.
	resume()
}

// endBody ends the budget watch of t's method, whose body has returned, and
// returns the budget the method was declared; zero when there is none.
func endBody(t *testing.T) time.Duration {
	if w, ok := methodWatches.Load(t); ok {
		return w.(bodyWatch).endBody()
	}
	return 0
}

// resumeHooks holds the hooks left to run after t's method, whose watch
// [endBody] ended, to what remains of the method's budget. A wait that
// ended at the deadline leaves the hooks none to be held to.
func resumeHooks(t *testing.T) {
	if w, ok := methodWatches.Load(t); ok {
		w.(bodyWatch).resume()
	}
}

// RunSetup runs a suite's BeforeAll. timeout bounds the context it receives;
// budget is the deadline it is held to by verdict, or zero for a suite that
// declared no config of its own.
func RunSetup(t testing.TB, timeout, budget time.Duration, beforeAll func(*gotest.T)) {
	tt := SetupT(t, timeout)
	if defaultApplies(timeout, budget) {
		defer noteDefaultDeadline(t, tt.Context(), t.Name()+" BeforeAll", "SetupTimeout", timeout)()
	}
	watchWhile(t, budget, "BeforeAll ", "SetupTimeout", func() {
		// A stop gives BeforeAll the grace a method body gets: AfterAll
		// cannot run before it returns.
		defer runstate.EnterBody(t, "BeforeAll")()
		beforeAll(tt)
	})
}

// RunTeardown runs a suite's AfterAll from inside t.Cleanup, on the same terms
// as [RunSetup].
func RunTeardown(t testing.TB, timeout, budget time.Duration, afterAll func(*gotest.T)) {
	tt := TeardownT(t, timeout)
	if defaultApplies(timeout, budget) {
		defer noteDefaultDeadline(t, tt.Context(), t.Name()+" AfterAll", "SetupTimeout", timeout)()
	}
	watchWhile(t, budget, "AfterAll ", "SetupTimeout", func() { afterAll(tt) })
}

// watchWhile runs a lifecycle phase and fails tb the moment timeout passes with
// that phase still running. what names the phase for the message ("BeforeAll ",
// "AfterAll ", or empty for a test method) and budget names the config field it
// came from.
//
// The alternative — checking the deadline once the phase returns — cannot judge
// a phase that never returns, which is the case that matters most. A wedged
// BeforeAll would leave nothing behind but the -timeout dump, and that names no
// budget at all.
//
// Nothing here interrupts the phase; it cannot. Go has no way to stop another
// goroutine, and running the phase on a goroutine we could abandon is worse than
// it looks: the testing package waits on subtests registered by t.Run, so a hang
// inside a nested When or It would not be bounded anyway, and the orphaned
// subtest would later signal a parent that had already finished. Killing the
// process is the only true interruption, and that is what go test -timeout is.
// What this adds is the verdict.
func watchWhile(tb testing.TB, timeout time.Duration, what, budget string, run func()) {
	// end also runs when the phase exits via Goexit.
	defer watchPhase(tb, timeout, what, budget)()
	run()
}

// watchPhase starts watching the phase the calling goroutine runs next, as
// [watchWhile] describes, and returns the call that ends the watch. Calls
// after the first do nothing.
//
// Ending waits for the watchdog to stop, which is what makes its report
// legal: the testing package panics on a log from a test that has already
// completed, and the test cannot complete before the watch has ended.
func watchPhase(tb testing.TB, timeout time.Duration, what, budget string) (end func()) {
	if timeout <= 0 {
		return func() {}
	}
	due, done, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	timer := time.AfterFunc(timeout, func() { close(due) })
	gid := goroutineID()
	go func() {
		defer close(stopped)
		watchDeadline(tb, gid, timeout, func() string { return what }, budget, due, done)
	}()
	return sync.OnceFunc(func() {
		timer.Stop()
		close(done)
		<-stopped
	})
}

// watchDeadline fails tb if the phase, running on goroutine gid, has not
// finished (done) once due is closed, and reports whether it did. what names
// the phase running then. Reporting is safe from another goroutine while the
// test is still running, which ending the watch (see [watchPhase]) guarantees.
func watchDeadline(tb testing.TB, gid uint64, timeout time.Duration, what func() string, budget string, due, done <-chan struct{}) (reported bool) {
	select {
	case <-done:
		return false
	case <-due:
	}

	// A select picks at random among the cases that are ready, so arriving here
	// does not prove the phase is still running — it may have finished in the
	// same instant the deadline passed. Re-check without blocking: finishing on
	// the deadline is not an overrun, and reporting one would be a coin flip.
	select {
	case <-done:
		return false
	default:
	}

	// The verdict belongs where the phase is running now.
	at := phaseFrame(gid)
	phase := what()
	// Written unbuffered as well as recorded on the test. A phase that never
	// returns never completes, and the testing package only flushes a test's
	// output when it does — so if the process is later killed by go test
	// -timeout, this line is the only trace that a budget was blown, and which
	// one.
	// One line of scheduling context, because this verdict has a known false
	// positive: on a saturated machine a phase can be runnable yet never
	// scheduled, and without this the dump reads like a deadlock.
	sched := schedinfo.Summary()
	suffix := "\n" + sched
	if hint := schedinfo.StarvationHint(); hint != "" {
		suffix += "\n" + hint
	}
	// test2json streams the verdict as it is written, attributed to its
	// test; a raw line there lands on whichever test printed last.
	if !test2json() {
		loc := ""
		if at != "" {
			loc = at + ": "
		}
		fmt.Fprintf(os.Stderr, "gotest: %s%s %sexceeded its configured %s of %s and is still running %s\n",
			loc, tb.Name(), phase, budget, timeout, sched)
	}
	reportAt(tb, at, fmt.Sprintf("%sexceeded its configured %s of %s and is still running%s", phase, budget, timeout, suffix))
	return true
}

// test2json reports whether the binary runs under -test.v=test2json.
func test2json() bool {
	f := flag.Lookup("test.v")
	return f != nil && f.Value.String() == "test2json"
}

// reportAt fails tb with msg located at at (file:line, or empty for none), in
// the shape Errorf gives a failure. Errorf itself would name its caller, a
// frame inside gotest, and the editor would link a file the user does not
// have. The cost: Go 1.27's test2json tags only Error/Errorf output as
// "error".
func reportAt(tb testing.TB, at, msg string) {
	if at != "" {
		msg = at + ": " + msg
	}
	// Indented past the first line, as Errorf indents continuation lines.
	fmt.Fprintln(tb.Output(), strings.ReplaceAll(msg, "\n", "\n    "))
	tb.Fail()
}

// testScopedT applies the timeout convention shared by the phases that run while
// the test is still live: a positive timeout becomes a deadline, anything else
// (including the documented -1 "disabled") leaves the context unbounded.
//
// The context also ends when the run is asked to stop, so a test blocked on a
// resource returns it before the fixtures tear down. AfterAll's context
// (TeardownT) does not: teardown has to run to the end.
func testScopedT(t testing.TB, timeout time.Duration) *gotest.T {
	ctx, cancel := context.WithCancel(t.Context())
	if timeout > 0 {
		cancel()
		ctx, cancel = context.WithTimeout(t.Context(), timeout)
	}
	// AfterFunc runs cancel on its own goroutine; a stop already in effect
	// must be seen by the first read, so it is applied here as well.
	if runstate.StopContext().Err() != nil {
		cancel()
	}
	stop := context.AfterFunc(runstate.StopContext(), cancel)
	t.Cleanup(func() {
		stop()
		cancel()
	})
	return gotest.NewTWithContext(t, ctx)
}

// TeardownT builds the *gotest.T handed to a suite's AfterAll, and must be
// called from inside t.Cleanup.
//
// It deliberately does not inherit the test's cancellation. The testing package
// cancels t.Context() immediately before it runs the cleanup functions, so a
// context derived from it would reach AfterAll already canceled and every
// context-aware teardown would fail instantly. Values carry over; cancellation
// does not; the configured setup/teardown timeout is applied on top.
func TeardownT(t testing.TB, timeout time.Duration) *gotest.T {
	ctx := context.WithoutCancel(t.Context())
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		t.Cleanup(cancel)
	}
	return gotest.NewTWithContext(t, ctx)
}
