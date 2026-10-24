package gotestruntime

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	"github.com/mvrahden/go-test/internal/callsite"
	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Methods is a suite's method loop as a generated Test wrapper hands it to
// [RunMethods].
type Methods struct {
	// Parallel runs every method as a parallel subtest.
	Parallel bool
	// Fixtures are the per-method hooks of the suite's package fixtures,
	// dependencies first.
	Fixtures []EachHooks
	Cases    []Method
}

// EachHooks are one package fixture's per-method hooks; either is nil when
// the fixture declares none.
type EachHooks struct {
	Name   string
	Before func(context.Context) error
	After  func(context.Context) error
	// BeforeAt and AfterAt locate the hooks (file:line, the file relative
	// to the package directory); a failure is reported there, never inside
	// the runtime's own file. Empty, as from a harness generated before
	// them, reports it unlocated.
	BeforeAt string
	AfterAt  string
}

// Method is one test method. Run calls the suite's BeforeEach, the method
// through run, and the suite's AfterEach; only it knows their static types.
type Method struct {
	Name string
	Run  func(t *gotest.T, run func(method func()))
}

// RunMethods runs each method as a subtest of t: fixture BeforeEach hooks,
// then the method's Run, then fixture AfterEach hooks in reverse. A fixture's
// turn comes once every BeforeEach before it returned without an error inside
// the budget; its AfterEach runs exactly when its turn came and its own
// BeforeEach, if any, returned without an error, so a hook never releases
// what was never acquired. cfg
// bounds each method's context and budget is the Timeout it is held to, as
// returned by [OpenSuite]; the budget covers the hooks and the body alike, and once a
// hook overran it nothing after that hook starts but the AfterEach hooks.
// With cfg.FailFast, the first method that fails stops the ones that have
// not started: each is skipped with the reason "FailFast: <that method>".
// The decision reads each method's own verdict, so a failure the suite's
// BeforeAll recorded without stopping does not trip it. Among parallel
// methods the stop is best effort: methods already running finish. Once the
// run is asked to stop, a method that has not started is skipped with the
// reason "run stopping".
func RunMethods(t *testing.T, cfg, budget gotest.SuiteConfig, m Methods) {
	var failedFirst atomic.Pointer[string]
	tripped := func() string {
		if first := failedFirst.Load(); cfg.FailFast && first != nil {
			return *first
		}
		return ""
	}
	for _, c := range m.Cases {
		if first := tripped(); first != "" {
			t.Run(c.Name, func(it *testing.T) { skipFailFast(it, first) })
			continue
		}
		passed := t.Run(c.Name, func(it *testing.T) {
			// Before any hook: goroutines BeforeEach starts are the method's.
			_, unlabel := labelMethod(it)
			defer unlabel()
			if m.Parallel {
				it.Parallel()
				// The first cleanup runs last: it sees the failures the others
				// record, the wait on a gotest.Go goroutine's among them.
				it.Cleanup(func() {
					if it.Failed() {
						failedFirst.CompareAndSwap(nil, &c.Name)
					}
				})
				if first := tripped(); first != "" {
					skipFailFast(it, first)
				}
			}
			if runstate.StopContext().Err() != nil {
				skipStopping(it)
			}
			// The budget covers the fixture hooks, the suite hooks and the body:
			// ended last, after every AfterEach registered below.
			mt, end := openMethod(it, cfg.Timeout, budget.Timeout)
			defer end()
			for _, f := range m.Fixtures {
				// After a BeforeEach that overran no later fixture's turn comes.
				if mt.overran() {
					return
				}
				if f.Before != nil {
					mt.enter(f.Name + ".BeforeEach ")
					if err := f.Before(mt.t.Context()); err != nil { //nolint:fail-guard // names the fixture and its hook, at the hook
						reportAt(it, callsite.Declared(f.BeforeAt), fmt.Sprintf("%s.BeforeEach failed: %v", f.Name, err))
						it.FailNow()
					}
				}
				if f.After != nil {
					defer func() {
						mt.enter(f.Name + ".AfterEach ")
						// Releases what BeforeEach acquired, so neither the
						// deadline nor a stop ends its context.
						if err := f.After(context.Background()); err != nil { //nolint:fail-guard // the method has run; report and go on
							reportAt(it, callsite.Declared(f.AfterAt), fmt.Sprintf("%s.AfterEach failed: %v", f.Name, err))
						}
					}()
				}
			}
			if mt.overran() {
				return
			}
			mt.enter("BeforeEach ")
			c.Run(mt.t, func(method func()) {
				if mt.overran() {
					return
				}
				mt.enter("")
				defer runstate.EnterBody(it, "a method")()
				method()
			})
		})
		if !passed {
			failedFirst.CompareAndSwap(nil, &c.Name)
		}
	}
}

// skipFailFast skips t, naming the method that stopped the suite, with no
// location: a runtime file is nothing the user can open.
func skipFailFast(t *testing.T, first string) {
	fmt.Fprintln(t.Output(), "FailFast: "+first)
	t.SkipNow()
}

// skipStopping skips t, a method a stopped run never started, with no
// location either.
func skipStopping(t *testing.T) {
	fmt.Fprintln(t.Output(), "run stopping")
	t.SkipNow()
}

// AwaitDoneAt runs an async method through start and returns once it calls
// done. at is the method's declaration (base:line), the verdict's location:
// the method has returned, so no frame of the user's is left to name.
//
// Once start returns, the method's budget watch ends: from then on the
// method is waiting for done, and the end of a budget the suite declared is
// its verdict; without one, the context's default deadline bounds the
// method's work, not the wait. The hooks after the wait are held to what
// remains of the budget. A stop ends the wait and skips the method with the
// reason "run stopping", leaving a failure it recorded its verdict.
//
// A failure recorded while waiting, or a goroutine gotest.Go started that
// panicked or exited, ends the wait once nothing can call done and no other
// goroutine of the method's is left: done is unreachable, and no goroutine
// carries the method's label. A failed method's wait that lasts on ends at
// the declared Timeout or, without one, when the method's context ends.
func AwaitDoneAt(t *gotest.T, name, at string, start func(done func())) {
	label, ok := methodLabels.Load(t.T())
	if !ok {
		// A harness generated before RunMethods labeled nothing: the body's
		// goroutines are the method's.
		l, unlabel := labelMethod(t.T())
		defer unlabel()
		label = l
	}
	abended, unwatch := runstate.WatchAbends(t.T())
	defer unwatch()
	done := make(chan struct{}, 1)
	held := handDone(start, done)
	budget := endBody(t.T())
	defer resumeHooks(t.T())
	if called(done) {
		return // called in time, even if the deadline has passed since
	}
	var deadline <-chan struct{}
	if budget > 0 {
		deadline = t.Context().Done()
	}
	// After a failure without a declared budget, the context's end bounds the
	// wait; the method is failed already.
	var settled <-chan struct{}
	// Why a failed method's wait goes on; empty until it failed.
	var left string
	poll := time.NewTimer(failedPoll)
	defer poll.Stop()
	backoff := failedPoll
	for {
		select {
		case <-done:
			return
		case <-runstate.StopContext().Done():
			if called(done) {
				return
			}
			skipStopping(t.T())
		case <-deadline:
			if called(done) {
				return
			}
			if runstate.StopContext().Err() != nil {
				skipStopping(t.T()) // the stop ended the context, not the deadline
			}
			if left != "" {
				reportAt(t.T(), callsite.Declared(at), name+" failed, and its wait for done() lasted until the test deadline: "+left)
			} else {
				reportAt(t.T(), callsite.Declared(at), name+": done() was not called before the test deadline")
			}
			t.T().FailNow() //nolint:t-escape // runtime verdict, located at the declaration
		case <-settled:
			if called(done) {
				return
			}
			if runstate.StopContext().Err() != nil {
				skipStopping(t.T())
			}
			reportAt(t.T(), callsite.Declared(at), name+" failed, and its wait for done() lasted until its context ended: "+left)
			return
		case <-poll.C:
			if !t.T().Failed() && !abended() {
				poll.Reset(failedPoll)
				continue
			}
			if deadline == nil {
				settled = t.Context().Done()
			}
			if left = leftToWaitFor(held, label.(string)); left == "" {
				return // nothing can call done, and nothing of the method's runs on
			}
			// Each look stops the world: look less often the longer it lasts.
			backoff = min(2*backoff, maxBackoff)
			poll.Reset(backoff)
		}
	}
}

// called reports whether done has come, for a wait that something else
// ended in the same instant.
func called(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

const (
	// failedPoll is how often an async wait looks for a failure recorded by
	// another goroutine; testing has no signal for one.
	failedPoll = 10 * time.Millisecond
	// maxBackoff is the longest a failed method's wait goes between looks at
	// what is left of the method.
	maxBackoff = 500 * time.Millisecond
)

// doneCall is the done an async method gets. Only that func value refers to
// it, so once it is unreachable nothing can call done.
type doneCall struct{ done chan<- struct{} }

func (d *doneCall) call() {
	select {
	case d.done <- struct{}{}:
	default:
	}
}

// handDone runs start with a done that signals on done, and returns a weak
// pointer that ends when nothing can call it any more.
//
//go:noinline
func handDone(start func(func()), done chan<- struct{}) weak.Pointer[doneCall] {
	d := &doneCall{done: done}
	start(d.call)
	return weak.Make(d)
}

// leftToWaitFor says what a failed method's wait is still for, or "" when
// nothing is left: a goroutine, timer or variable still holds done, or a
// goroutine carries the method's label besides the waiting one.
func leftToWaitFor(held weak.Pointer[doneCall], label string) string {
	runtime.GC() // ends held once nothing holds done
	if held.Value() != nil {
		return "a goroutine, timer or variable still held done()"
	}
	if labeled(label) > 1 {
		return "a goroutine it started was still running"
	}
	return ""
}

// methodLabelKey is the goroutine label that marks a test method's
// goroutines: the one it runs on and every goroutine started from it, its
// BeforeEach included, which a go statement passes on.
const methodLabelKey = "gotest.method"

var (
	labelSeq     atomic.Uint64
	methodLabels sync.Map // *testing.T -> string, its method's label
)

// labelMethod labels the calling goroutine, which runs t's method, with a
// label of the method's own; unlabel forgets it.
func labelMethod(t *testing.T) (label string, unlabel func()) {
	label = strconv.FormatUint(labelSeq.Add(1), 10)
	pprof.SetGoroutineLabels(pprof.WithLabels(context.Background(), pprof.Labels(methodLabelKey, label)))
	methodLabels.Store(t, label)
	return label, func() { methodLabels.Delete(t) }
}

// profileSize is the size of the last goroutine profile read, so the next
// read starts with room for it.
var profileSize atomic.Int64

// labeled counts the live goroutines labeled with label.
func labeled(label string) int {
	buf := bytes.NewBuffer(make([]byte, 0, profileSize.Load()))
	// Grouped by stack and labels, so 10k goroutines in one place read as one.
	_ = pprof.Lookup("goroutine").WriteTo(buf, 1)
	profileSize.Store(int64(buf.Len()))
	want := "# labels: {"
	mark := strconv.Quote(methodLabelKey) + ":" + strconv.Quote(label)
	n, count := 0, 0
	for line := range strings.Lines(buf.String()) {
		if c, _, ok := strings.Cut(line, " @ "); ok {
			count, _ = strconv.Atoi(c)
			continue
		}
		if strings.HasPrefix(line, want) && strings.Contains(line, mark) {
			n += count
		}
	}
	return n
}
