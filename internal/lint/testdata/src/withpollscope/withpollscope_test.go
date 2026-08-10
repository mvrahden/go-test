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
