package gotest_test

import (
	"context"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzAdapterLifecycle is a top-level stdlib fuzz target — this IS the
// legitimate integration point for gotest.Fuzz, exempt from the
// suites-only idiom. It replays two seed corpus entries and proves that
// beforeEach/afterEach interpose around EACH execution of the fuzz body
// (not just once around the whole fuzz target).
func FuzzAdapterLifecycle(f *testing.F) {
	f.Add("ab")
	f.Add("cd")
	var order []string
	gf := gotest.NewF(f,
		func(*gotest.T) { order = append(order, "before") },
		func(*gotest.T) { order = append(order, "after") })

	if gf.F() != f {
		f.Fatalf("gf.F() = %p, want %p (identity passthrough broken)", gf.F(), f)
	}

	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		order = append(order, "body:"+s)

		// This execution's before must sit immediately before this
		// execution's body entry.
		gotest.Equal(t, "before", order[len(order)-2])
		gotest.Equal(t, "body:"+s, order[len(order)-1])

		// Once a second execution has begun, the FIRST execution's
		// after hook must already have run and interposed between the
		// two executions — proving per-execution (not aggregate) hook
		// interposition. (The after hook for THIS execution hasn't run
		// yet at this point — it's deferred until fn returns — so the
		// full triple for THIS execution is checked below, after
		// gotest.Fuzz returns.)
		if len(order) > 3 {
			gotest.Equal(t, []string{"before", "body:ab", "after"}, order[:3])
		}
	})

	// gotest.Fuzz has now returned: under `go test` (seed replay, not
	// live fuzzing) testing.F.Fuzz runs synchronously over the seed
	// corpus, so both executions — including the LAST one's afterEach —
	// have completed by now.
	want := []string{"before", "body:ab", "after", "before", "body:cd", "after"}
	if len(order) != len(want) {
		f.Fatalf("order = %v, want %v", order, want)
	}
	for i, w := range want {
		if order[i] != w {
			f.Fatalf("order[%d] = %q, want %q (full: %v)", i, order[i], w, order)
		}
	}
}

// FuzzAdapterNilHooks proves nil before/after hooks don't panic.
func FuzzAdapterNilHooks(f *testing.F) {
	f.Add("x")
	gf := gotest.NewF(f, nil, nil)
	gotest.Fuzz(gf, func(t *gotest.T, s string) {
		gotest.NotZero(t, s)
	})
}

// FuzzAdapter2Args proves Fuzz2 passes both arguments through correctly.
func FuzzAdapter2Args(f *testing.F) {
	f.Add("a", 7)
	gf := gotest.NewF(f, nil, nil)
	gotest.Fuzz2(gf, func(t *gotest.T, s string, n int) {
		gotest.Equal(t, "a", s)
		gotest.Equal(t, 7, n)
	})
}

// FWrapperTestSuite is a normal gotest suite covering what's assertable
// about *gotest.F outside of a real fuzz target. *testing.F has no public
// constructor, so an actual *gotest.F can only be built inside a genuine
// fuzz target — F() identity, Add forwarding (via seed count driving
// executions), and the generic adapters are exercised end to end by
// FuzzAdapterLifecycle, FuzzAdapterNilHooks, and FuzzAdapter2Args above.
// What remains assertable here, without an instance, is the assertion
// contract *gotest.F promises to satisfy.
type FWrapperTestSuite struct{}

func (s *FWrapperTestSuite) TestAssertionContract(t *gotest.T) {
	t.It("satisfies Errorf/FailNow/Skipf/Context like B and T", func(it *gotest.T) {
		var _ interface {
			Errorf(format string, args ...any)
			FailNow()
			Skipf(format string, args ...any)
			Context() context.Context
		} = (*gotest.F)(nil)
	})
}
