package gotestruntime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// method is one running test method: its T and, under a declared Timeout,
// the watch that holds its hooks and body to it and the phase it is in.
type method struct {
	it     *testing.T
	t      *gotest.T
	budget time.Duration
	ctx    *budgetContext // nil without a budget
	after  *budgetContext // AfterEach's: the budget's, which a stop does not end
	phase  atomic.Pointer[string]
	due    chan struct{} // closed at the deadline
	gid    uint64        // the goroutine the method runs on

	mu      sync.Mutex
	watch   *phaseWatch // the watch running now, nil when none
	expired bool        // the deadline has passed
}

// phaseWatch is one run of a method's watch: done ends it, stopped closes
// once it has.
type phaseWatch struct{ done, stopped chan struct{} }

// openMethod builds the T of the method it runs and, with a positive budget,
// starts its watch; end stops the watch. The T's context ends at timeout, or
// at the budget when there is one: then only after the watch has judged an
// overrun, so a hook waiting on that context is still waiting, and located,
// when the verdict is written.
func openMethod(it *testing.T, timeout, budget time.Duration) (m *method, end func()) {
	if budget <= 0 {
		m = &method{it: it, t: TestT(it, timeout)}
		if !defaultApplies(timeout, budget) {
			return m, func() {}
		}
		return m, noteDefaultDeadline(it, m.t.Context(), it.Name(), "Timeout", timeout)
	}
	parent, cancel := context.WithCancel(it.Context())
	// As in [testScopedT]: a stop already in effect is seen at once.
	if runstate.StopContext().Err() != nil {
		cancel()
	}
	stop := context.AfterFunc(runstate.StopContext(), cancel)
	m = &method{it: it, budget: budget, due: make(chan struct{}), gid: goroutineID()}
	deadline := time.Now().Add(budget)
	m.ctx = newBudgetContext(parent, deadline)
	m.after = newBudgetContext(context.WithoutCancel(it.Context()), deadline)
	m.t = gotest.NewTWithContext(it, m.ctx)
	afterTs.Store(m.t, gotest.NewTWithContext(it, m.after))
	m.enter("")
	// Watching before the timer starts: the deadline then always finds the
	// watch that judges it.
	m.watchOn()
	timer := time.AfterFunc(budget, m.deadlinePassed)
	it.Cleanup(func() {
		timer.Stop()
		stop()
		cancel()
	})
	methodWatches.Store(it, bodyWatch(m))
	return m, func() {
		afterTs.Delete(m.t)
		methodWatches.Delete(it)
		m.watchOff()
	}
}

// watchOn starts the method's watch unless one runs or the deadline has
// passed, which leaves nothing to hold the method to.
func (m *method) watchOn() {
	m.mu.Lock()
	if m.watch != nil || m.expired {
		m.mu.Unlock()
		return
	}
	w := &phaseWatch{done: make(chan struct{}), stopped: make(chan struct{})}
	m.watch = w
	m.mu.Unlock()
	methodsByGoroutine.Store(m.gid, m)
	budgeted.Add(1)
	go func() {
		defer close(w.stopped)
		if watchDeadline(m.it, m.gid, m.budget, m.phaseName, "Timeout", m.due, w.done) {
			m.ctx.end(context.DeadlineExceeded)
			m.after.end(context.DeadlineExceeded)
		}
	}()
}

// watchOff ends the watch running now, if any. Only the method's goroutine
// starts and ends it.
func (m *method) watchOff() {
	m.mu.Lock()
	w := m.watch
	m.mu.Unlock()
	if w == nil {
		return
	}
	methodsByGoroutine.Delete(m.gid)
	budgeted.Add(-1)
	close(w.done)
	<-w.stopped
	m.mu.Lock()
	m.watch = nil
	expired := m.expired
	m.mu.Unlock()
	if expired {
		m.ctx.end(context.DeadlineExceeded)
		m.after.end(context.DeadlineExceeded)
	}
}

// endBody ends the watch while an async method waits for done and returns
// the budget it declared.
func (m *method) endBody() time.Duration {
	m.watchOff()
	return m.budget
}

// resume holds the hooks after an async method's wait to what is left of
// the budget: the same deadline, so none is left once it has passed.
func (m *method) resume() { m.watchOn() }

// deadlinePassed ends the method's context at its deadline, or leaves that
// to the watch while it runs.
func (m *method) deadlinePassed() {
	m.mu.Lock()
	m.expired = true
	close(m.due)
	watching := m.watch != nil
	m.mu.Unlock()
	if !watching {
		m.ctx.end(context.DeadlineExceeded)
		m.after.end(context.DeadlineExceeded)
	}
}

// afterTs holds, per running budgeted method's T, the T its AfterEach gets.
var afterTs sync.Map // *gotest.T -> *gotest.T

// enter names the phase the method runs next, for an overrun's verdict:
// "BeforeEach ", "AfterEach ", "<Fixture>.BeforeEach " and the like, or
// empty for the body.
func (m *method) enter(phase string) {
	if m.budget > 0 {
		m.phase.Store(&phase)
	}
}

func (m *method) phaseName() string { return *m.phase.Load() }

// overran reports whether the method's budget has run out: the watch has
// failed it, and nothing after the phase that overran should start.
func (m *method) overran() bool {
	return m.budget > 0 && errors.Is(m.ctx.Err(), context.DeadlineExceeded)
}

// methodsByGoroutine maps the goroutine of each running budgeted method to
// it, so [AfterEach], which only the generated code calls, can name its phase.
var (
	methodsByGoroutine sync.Map // uint64 -> *method
	budgeted           atomic.Int64
)

// enterPhase names the phase of the budgeted method running on the calling
// goroutine, if any.
func enterPhase(phase string) {
	if budgeted.Load() == 0 {
		return
	}
	if m, ok := methodsByGoroutine.Load(goroutineID()); ok {
		m.(*method).enter(phase)
	}
}

// budgetContext is a method's context under a declared Timeout. Its deadline
// is the budget's, but it ends only when told to: by the method's watch, once
// the verdict is written, or when its parent ends.
type budgetContext struct {
	cancel   context.CancelCauseFunc
	values   context.Context
	deadline time.Time
	done     chan struct{}
	once     sync.Once
	err      atomic.Pointer[error]
}

func newBudgetContext(parent context.Context, deadline time.Time) *budgetContext {
	// values carries the cause for context.Cause; its own Done is never
	// handed out, so contexts derived from this one take Err from it.
	values, cancel := context.WithCancelCause(parent)
	c := &budgetContext{cancel: cancel, values: values, deadline: deadline, done: make(chan struct{})}
	context.AfterFunc(parent, func() { c.end(parent.Err()) })
	return c
}

func (c *budgetContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c *budgetContext) Done() <-chan struct{}       { return c.done }
func (c *budgetContext) Value(key any) any           { return c.values.Value(key) }

func (c *budgetContext) Err() error {
	if err := c.err.Load(); err != nil {
		return *err
	}
	return nil
}

// end ends the context with err; calls after the first do nothing.
func (c *budgetContext) end(err error) {
	c.once.Do(func() {
		c.cancel(err)
		c.err.Store(&err)
		close(c.done)
	})
}
