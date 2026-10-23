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
	phase  atomic.Pointer[string]
	due    chan struct{} // closed at the deadline

	mu       sync.Mutex
	watching bool
	expired  bool // the deadline has passed
}

// openMethod builds the T of the method it runs and, with a positive budget,
// starts its watch; end stops the watch. The T's context ends at timeout, or
// at the budget when there is one: then only after the watch has judged an
// overrun, so a hook waiting on that context is still waiting, and located,
// when the verdict is written.
func openMethod(it *testing.T, timeout, budget time.Duration) (m *method, end func()) {
	if budget <= 0 {
		return &method{it: it, t: TestT(it, timeout)}, func() {}
	}
	parent, cancel := context.WithCancel(it.Context())
	stop := context.AfterFunc(runstate.StopContext(), cancel)
	m = &method{it: it, budget: budget, due: make(chan struct{}), watching: true}
	m.ctx = newBudgetContext(parent, time.Now().Add(budget))
	m.t = gotest.NewTWithContext(it, m.ctx)
	m.enter("")
	timer := time.AfterFunc(budget, m.deadlinePassed)
	it.Cleanup(func() {
		timer.Stop()
		stop()
		cancel()
	})

	gid := goroutineID()
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		if watchDeadline(it, gid, budget, m.phaseName, "Timeout", m.due, done) {
			m.ctx.end(context.DeadlineExceeded)
		}
	}()
	methodsByGoroutine.Store(gid, m)
	budgeted.Add(1)
	stopWatch := sync.OnceFunc(func() {
		methodsByGoroutine.Delete(gid)
		budgeted.Add(-1)
		close(done)
		<-stopped
		m.mu.Lock()
		m.watching = false
		expired := m.expired
		m.mu.Unlock()
		if expired {
			m.ctx.end(context.DeadlineExceeded)
		}
	})
	methodWatches.Store(it, stopWatch)
	return m, func() {
		methodWatches.Delete(it)
		stopWatch()
	}
}

// deadlinePassed ends the method's context at its deadline, or leaves that
// to the watch while it runs.
func (m *method) deadlinePassed() {
	m.mu.Lock()
	m.expired = true
	close(m.due)
	watching := m.watching
	m.mu.Unlock()
	if !watching {
		m.ctx.end(context.DeadlineExceeded)
	}
}

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
