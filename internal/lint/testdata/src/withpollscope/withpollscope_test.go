package withpollscope //nolint:stdlib-test

import (
	"errors"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestEventuallyWithWrongT(t *testing.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.Equal(t, 1, 2)    // want `use poll instead of t in poll callback passed to Eventually`
		gotest.Equal(poll, 1, 2) // ok
	})
}

func TestEventuallyWithMultipleAssertions(t *testing.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.True(t, true)         // want `use poll instead of t in poll callback passed to Eventually`
		gotest.NoError(t, nil)       // want `use poll instead of t in poll callback passed to Eventually`
		gotest.MatchSnapshot(t, "x") // want `use poll instead of t in poll callback passed to Eventually`
		gotest.True(poll, true)      // ok
	})
}

func TestConsistentlyWithWrongT(t *testing.T) {
	gotest.Consistently(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.Equal(t, 1, 2) // want `use poll instead of t in poll callback passed to Consistently`
	})
}

func TestDirectMethodCall(t *testing.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		t.Errorf("wrong") // want `t.Errorf in poll callback bypasses assertion recording — use poll`
		t.Fatal("wrong")  // want `t.Fatal in poll callback bypasses assertion recording — use poll`
		t.FailNow()       // want `t.FailNow in poll callback bypasses assertion recording — use poll`
	})
}

func TestCorrectUsage(t *testing.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.Equal(poll, 1, 2)
		gotest.True(poll, true)
	})
}

func TestCustomPollParamName(t *testing.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(r *gotest.R) {
		gotest.Equal(t, 1, 2) // want `use r instead of t in poll callback passed to Eventually`
		gotest.Equal(r, 1, 2) // ok
	})
}

func TestOutsidePollCallback(t *testing.T) {
	gotest.Equal(t, 1, 2) // ok — not inside a poll callback
}

// A guard inside a poll callback is poll-scope's finding alone — the
// expressiveness rule must stand down on the owned construct.
func TestGuardInsideCallback(t *testing.T) {
	var err error
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		if err != nil {
			t.Errorf("boom: %v", err) // want `t\.Errorf in poll callback bypasses assertion recording — use poll`
		}
	})
}

// Constructs owned by poll-scope get no expressiveness findings on top.
func TestSimplifiableInsideCallback(t *testing.T) {
	a, b := 1, 2
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.True(t, a == b) // want `use poll instead of t in poll callback passed to Eventually`
	})
}

func TestRedundantInsideCallback(t *testing.T) {
	var err error
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.Error(t, err)                  // want `use poll instead of t in poll callback passed to Eventually`
		gotest.ErrorIs(t, err, errSentinelPS) // want `use poll instead of t in poll callback passed to Eventually`
	})
}

var errSentinelPS = errors.New("sentinel")

// Foreign assertion libraries still escape the poll scope — integrity
// coverage is deliberately package-agnostic for assertion-shaped names.
func NoError(t *testing.T, err error, msgAndArgs ...any) {}

func TestForeignAssertionInsideCallback(t *testing.T) {
	var err error
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		NoError(t, err) // want `use poll instead of t in poll callback passed to Eventually`
	})
}

// Receiver methods are matched by type, not name — loggers stay silent.
type retryLogger struct{}

func (l *retryLogger) Errorf(string, ...any) {}

func TestLoggerInsideCallback(t *testing.T) {
	log := &retryLogger{}
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		log.Errorf("retrying")
	})
}

// Wrapped or re-exported polling functions are recognized by the typed
// *gotest.R callback parameter, not by the callee's name or package.
var eventually = gotest.Eventually

func TestWrappedEventually(t *testing.T) {
	eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		t.Errorf("boom") // want `t\.Errorf in poll callback bypasses assertion recording — use poll`
	})
}

// Other functions taking func(*gotest.R) callbacks (Record-style harnesses)
// are not polling contexts — only Eventually/Consistently shapes are.
func record(fn func(r *gotest.R)) {}

func TestRecordStyleCallback(t *testing.T) {
	record(func(r *gotest.R) {
		gotest.Eventually(r, time.Second, time.Millisecond, func(poll *gotest.R) {
			gotest.True(poll, true)
		})
	})
}

// A nested poll callback owns its subtree — the outer scope must not
// claim the inner poll's assertions.
func TestNestedEventually(t *testing.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(outer *gotest.R) {
		gotest.Consistently(outer, time.Millisecond, time.Millisecond, func(inner *gotest.R) {
			gotest.True(inner, true)
		})
	})
}
