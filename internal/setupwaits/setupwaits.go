// Package setupwaits holds the waits of the goroutines a suite's BeforeAll
// starts with gotest.Go, so they can be waited for after AfterAll, which is
// what stops them. pkg/gotestruntime opens and drains a suite's set and
// pkg/gotest registers into it: pkg/gotestruntime imports pkg/gotest, so
// neither can hold it for the other.
package setupwaits

import (
	"context"
	"sync"
	"testing"
)

// Set is one suite's waits.
type Set struct {
	// TB is the suite's, the one a wait reports to: a goroutine may outlive
	// the subtest it was started from.
	TB testing.TB

	mu     sync.Mutex
	waits  []func()
	closed bool
}

type ctxKey struct{}

var (
	mu   sync.Mutex
	open = map[testing.TB]*Set{}
)

// Open starts tb's set and returns ctx carrying it, for the T BeforeAll gets:
// every T derived from that one finds the set through its context.
func Open(ctx context.Context, tb testing.TB) context.Context {
	s := &Set{TB: tb}
	mu.Lock()
	open[tb] = s
	mu.Unlock()
	return With(ctx, s)
}

// With returns ctx carrying s.
func With(ctx context.Context, s *Set) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// Of returns the set a T with this context and testing.TB registers into: the
// one its context carries, or else the one opened for tb itself, which is what
// a T wrapped around a hook's *testing.T finds. Nil for neither.
func Of(ctx context.Context, tb testing.TB) *Set {
	if s, ok := ctx.Value(ctxKey{}).(*Set); ok {
		return s
	}
	mu.Lock()
	defer mu.Unlock()
	return open[tb]
}

// Add registers wait, and reports false when the set was drained already:
// nothing is left to wait for it, so the caller waits itself.
func (s *Set) Add(wait func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.waits = append(s.waits, wait)
	return true
}

// Close ends tb's set and returns it, nil when none is open: from here on
// only [Set.Take] reaches it.
func Close(tb testing.TB) *Set {
	mu.Lock()
	defer mu.Unlock()
	s := open[tb]
	delete(open, tb)
	return s
}

// Take hands over the waits registered so far, and closes the set when there
// are none: a wait registered while the caller waits for the last batch is
// in the next.
func (s *Set) Take() []func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	waits := s.waits
	s.waits = nil
	s.closed = len(waits) == 0
	return waits
}
