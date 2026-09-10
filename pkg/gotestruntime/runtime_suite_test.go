// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestruntime_test //nolint:fail-guard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// RuntimeTestSuite covers the fixture runtime: lifecycle order, exit-code
// forwarding, retries, timeouts, the fixture tree and DAG, budget files and
// teardown failures. Sequential: it sets environment variables.
type RuntimeTestSuite struct{}

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	copy(out, r.events)
	return out
}

func (s *RuntimeTestSuite) TestSingleRoot_LifecycleOrder(t *gotest.T) {
	rec := &recorder{}

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustEqual(t, []string{"root.init", "root.beforeAll", "m.run", "root.afterAll"}, rec.names())
}

func (s *RuntimeTestSuite) TestSingleRoot_ExitCodeForwarded(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll:  func(ctx context.Context) error { return nil },
	}

	exitCode := gotestruntime.ExportRun(func() int {
		return 42
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 42, exitCode)
}

func (s *RuntimeTestSuite) TestSingleRoot_AfterAllCalledOnNonZeroExit(t *gotest.T) {
	rec := &recorder{}

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		return 1
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 1, exitCode)
	mustContain(t, rec.names(), "root.afterAll")
}

func (s *RuntimeTestSuite) TestSingleRoot_NilAfterAll(t *gotest.T) {
	rec := &recorder{}

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: nil,
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustEqual(t, []string{"root.init", "root.beforeAll", "m.run"}, rec.names())
}

func (s *RuntimeTestSuite) TestSingleRoot_NilInit(t *gotest.T) {
	rec := &recorder{}

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   nil,
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustEqual(t, []string{"root.beforeAll", "m.run", "root.afterAll"}, rec.names())
}

func (s *RuntimeTestSuite) TestRetry_SucceedsOnSecondAttempt(t *gotest.T) {
	rec := &recorder{}
	attempts := 0

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute, Retries: 2, RetryDelay: 10 * time.Millisecond},
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return errors.New("transient failure")
			}
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustEqual(t, 2, attempts)
	mustEqual(t, []string{"root.init", "root.beforeAll", "m.run", "root.afterAll"}, rec.names())
}

func (s *RuntimeTestSuite) TestRetry_ExhaustedRetriesReturnsExitCode2(t *gotest.T) {
	rec := &recorder{}
	attempts := 0

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute, Retries: 1},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			attempts++
			return errors.New("permanent failure")
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}

	mRunCalled := false
	exitCode := gotestruntime.ExportRun(func() int {
		mRunCalled = true
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 2, exitCode)
	mustEqual(t, 2, attempts)
	mustFalse(t, mRunCalled)
	mustEqual(t, []string{}, rec.names())
}

func (s *RuntimeTestSuite) TestRetry_DelayObservedBetweenAttempts(t *gotest.T) {
	attempts := 0
	var timestamps []time.Time

	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute, Retries: 1, RetryDelay: 50 * time.Millisecond},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			timestamps = append(timestamps, time.Now())
			attempts++
			if attempts < 2 {
				return errors.New("transient")
			}
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustLen(t, timestamps, 2)
	elapsed := timestamps[1].Sub(timestamps[0])
	mustGreaterOrEqual(t, elapsed, 40*time.Millisecond)
}

func (s *RuntimeTestSuite) TestTimeout_BeforeAllExceedsTimeout(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 50 * time.Millisecond},
		Budget: 50 * time.Millisecond,
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		},
		AfterAll: func(ctx context.Context) error { return nil },
	}

	mRunCalled := false
	exitCode := gotestruntime.ExportRun(func() int {
		mRunCalled = true
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 2, exitCode)
	mustFalse(t, mRunCalled)
}

func (s *RuntimeTestSuite) TestTimeout_UndeclaredBudgetIsNotEnforced(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 20 * time.Millisecond},
		// Budget deliberately zero: the fixture declared no config of its own,
		// so the default bounds its context but never fails it.
		Init: func() {},
		BeforeAll: func(ctx context.Context) error {
			time.Sleep(60 * time.Millisecond) // ignores ctx, outlives the timeout
			return nil
		},
		AfterAll: func(ctx context.Context) error { return nil },
	}

	mRunCalled := false
	exitCode := gotestruntime.ExportRun(func() int {
		mRunCalled = true
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustTrue(t, mRunCalled, "an undeclared budget must not stop the tests from running")
}

func (s *RuntimeTestSuite) TestTimeout_BeforeAllCompletesWithinTimeout(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 500 * time.Millisecond},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return nil
			}
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestTimeout_DisabledWithNegativeOne(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: -1},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			deadline, hasDeadline := ctx.Deadline()
			_ = deadline
			mustFalse(t, hasDeadline)
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})
	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestChildren_SetupOrder(t *gotest.T) {
	rec := &recorder{}

	childA := &gotestruntime.FixtureNode{
		Name:   "ChildA",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("childA.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("childA.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("childA.afterAll")
			return nil
		},
	}
	childB := &gotestruntime.FixtureNode{
		Name:   "ChildB",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("childB.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("childB.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("childB.afterAll")
			return nil
		},
	}

	root := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
		Children: []*gotestruntime.FixtureNode{childA, childB},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{root}})

	mustEqual(t, 0, exitCode)

	events := rec.names()

	// Root must come before any child
	rootInitIdx := indexOf(events, "root.init")
	rootBeforeAllIdx := indexOf(events, "root.beforeAll")
	mustGreaterOrEqual(t, rootInitIdx, 0)
	mustGreaterOrEqual(t, rootBeforeAllIdx, 0)
	mustLess(t, rootInitIdx, rootBeforeAllIdx)

	// Children must come after root.beforeAll
	for _, ev := range []string{"childA.init", "childA.beforeAll", "childB.init", "childB.beforeAll"} {
		idx := indexOf(events, ev)
		mustGreater(t, idx, rootBeforeAllIdx, "expected %s after root.beforeAll", ev)
	}

	// m.run must come after all children setup
	mRunIdx := indexOf(events, "m.run")
	for _, ev := range []string{"childA.beforeAll", "childB.beforeAll"} {
		idx := indexOf(events, ev)
		mustLess(t, idx, mRunIdx, "expected %s before m.run", ev)
	}

	// Root AfterAll must come after children AfterAll
	rootAfterAllIdx := indexOf(events, "root.afterAll")
	for _, ev := range []string{"childA.afterAll", "childB.afterAll"} {
		idx := indexOf(events, ev)
		mustLess(t, idx, rootAfterAllIdx, "expected %s before root.afterAll", ev)
	}
}

func (s *RuntimeTestSuite) TestChildren_ConcurrentSetup(t *gotest.T) {
	childAStarted := make(chan struct{})
	childBStarted := make(chan struct{})

	childA := &gotestruntime.FixtureNode{
		Name:   "ChildA",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(childAStarted)
			// Wait for B to also start (proves concurrency)
			select {
			case <-childBStarted:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	childB := &gotestruntime.FixtureNode{
		Name:   "ChildB",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(childBStarted)
			// Wait for A to also start (proves concurrency)
			select {
			case <-childAStarted:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	root := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children:  []*gotestruntime.FixtureNode{childA, childB},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{root}})
	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestChildFailure_CancelsSiblings(t *gotest.T) {
	rec := &recorder{}
	childAStarted := make(chan struct{})

	childA := &gotestruntime.FixtureNode{
		Name:   "ChildA",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(childAStarted)
			return errors.New("childA fails")
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("childA.afterAll")
			return nil
		},
	}
	childB := &gotestruntime.FixtureNode{
		Name:   "ChildB",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			// Wait for A to have started (and likely failed)
			<-childAStarted
			// Give time for cancellation to propagate
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				rec.record("childB.beforeAll.completed")
				return nil
			}
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("childB.afterAll")
			return nil
		},
	}

	root := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
		Children: []*gotestruntime.FixtureNode{childA, childB},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{root}})

	mustEqual(t, 2, exitCode)
	events := rec.names()
	// ChildA.AfterAll NOT called (never succeeded)
	mustNotContain(t, events, "childA.afterAll")
	// ChildB.AfterAll NOT called (cancelled before success)
	mustNotContain(t, events, "childB.afterAll")
	// ChildB.BeforeAll should have been cancelled, not completed
	mustNotContain(t, events, "childB.beforeAll.completed")
	// Root.AfterAll IS called (root succeeded)
	mustContain(t, events, "root.afterAll")
}

func (s *RuntimeTestSuite) TestChildFailure_SucceededSiblingGetsAfterAll(t *gotest.T) {
	rec := &recorder{}
	childBReady := make(chan struct{})

	childA := &gotestruntime.FixtureNode{
		Name:   "ChildA",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			// Wait for B to succeed first
			<-childBReady
			return errors.New("childA fails after B succeeded")
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("childA.afterAll")
			return nil
		},
	}
	childB := &gotestruntime.FixtureNode{
		Name:   "ChildB",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(childBReady)
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("childB.afterAll")
			return nil
		},
	}

	root := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
		Children: []*gotestruntime.FixtureNode{childA, childB},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{root}})

	mustEqual(t, 2, exitCode)
	events := rec.names()
	// ChildA never succeeded
	mustNotContain(t, events, "childA.afterAll")
	// ChildB succeeded → AfterAll must be called
	mustContain(t, events, "childB.afterAll")
	// Root succeeded → AfterAll must be called
	mustContain(t, events, "root.afterAll")
}

func (s *RuntimeTestSuite) TestTreeDepth_ThreeLevels(t *gotest.T) {
	rec := &recorder{}

	grandchild := &gotestruntime.FixtureNode{
		Name:   "Grandchild",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("grandchild.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("grandchild.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("grandchild.afterAll")
			return nil
		},
	}

	child := &gotestruntime.FixtureNode{
		Name:   "Child",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("child.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("child.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("child.afterAll")
			return nil
		},
		Children: []*gotestruntime.FixtureNode{grandchild},
	}

	root := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
		Children: []*gotestruntime.FixtureNode{child},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{root}})

	mustEqual(t, 0, exitCode)
	events := rec.names()

	// Setup order: root → child → grandchild
	rootBA := indexOf(events, "root.beforeAll")
	childBA := indexOf(events, "child.beforeAll")
	grandchildBA := indexOf(events, "grandchild.beforeAll")
	mRun := indexOf(events, "m.run")

	mustLess(t, rootBA, childBA)
	mustLess(t, childBA, grandchildBA)
	mustLess(t, grandchildBA, mRun)

	// Teardown order: grandchild → child → root
	grandchildAA := indexOf(events, "grandchild.afterAll")
	childAA := indexOf(events, "child.afterAll")
	rootAA := indexOf(events, "root.afterAll")

	mustLess(t, mRun, grandchildAA)
	mustLess(t, grandchildAA, childAA)
	mustLess(t, childAA, rootAA)
}

func (s *RuntimeTestSuite) TestMultipleRoots_ConcurrentSetup(t *gotest.T) {
	rootAStarted := make(chan struct{})
	rootBStarted := make(chan struct{})

	rootA := &gotestruntime.FixtureNode{
		Name:   "RootA",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(rootAStarted)
			select {
			case <-rootBStarted:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		AfterAll: func(ctx context.Context) error { return nil },
	}
	rootB := &gotestruntime.FixtureNode{
		Name:   "RootB",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(rootBStarted)
			select {
			case <-rootAStarted:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		AfterAll: func(ctx context.Context) error { return nil },
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{rootA, rootB}})
	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestMultipleRoots_OneFailsCancelsOther(t *gotest.T) {
	rec := &recorder{}

	rootA := &gotestruntime.FixtureNode{
		Name:   "RootA",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return errors.New("rootA fails immediately")
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("rootA.afterAll")
			return nil
		},
	}
	rootB := &gotestruntime.FixtureNode{
		Name:   "RootB",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			// Block until cancelled
			<-ctx.Done()
			return ctx.Err()
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("rootB.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{rootA, rootB}})

	mustEqual(t, 2, exitCode)
	events := rec.names()
	mustNotContain(t, events, "rootA.afterAll")
	mustNotContain(t, events, "rootB.afterAll")
}

func (s *RuntimeTestSuite) TestMultipleRoots_ConcurrentTeardown(t *gotest.T) {
	rootATeardownStarted := make(chan struct{})
	rootBTeardownStarted := make(chan struct{})

	rootA := &gotestruntime.FixtureNode{
		Name:      "RootA",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			close(rootATeardownStarted)
			<-rootBTeardownStarted
			return nil
		},
	}
	rootB := &gotestruntime.FixtureNode{
		Name:      "RootB",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			close(rootBTeardownStarted)
			<-rootATeardownStarted
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{rootA, rootB}})
	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestBudgetFile_WrittenCorrectly(t *gotest.T) {
	budgetFile := filepath.Join(t.TempDir(), "budget")
	t.Setenv(protocol.EnvTeardownBudgetFile, budgetFile)

	root := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children: []*gotestruntime.FixtureNode{
			{
				Name:      "Child",
				Config:    gotest.FixtureConfig{Timeout: 1 * time.Minute},
				Init:      func() {},
				BeforeAll: func(ctx context.Context) error { return nil },
			},
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{
		Roots:                []*gotestruntime.FixtureNode{root},
		MaxSuiteSetupTimeout: 30 * time.Second,
	})

	mustEqual(t, 0, exitCode)

	data, err := os.ReadFile(budgetFile)
	mustNoError(t, err)

	// Budget = max tree path (2m root + 1m child) + max suite setup (30s) + 30s
	expected := (2*time.Minute + 1*time.Minute + 30*time.Second + 30*time.Second).String()
	mustEqual(t, expected, string(data))
}

func (s *RuntimeTestSuite) TestBudgetFile_ZeroTimeoutIsNotZeroBudget(t *gotest.T) {
	budgetFile := filepath.Join(t.TempDir(), "budget")
	t.Setenv(protocol.EnvTeardownBudgetFile, budgetFile)

	// Under literal config a zero Timeout is the spelling of "no deadline", not
	// "takes no time". Reading it as zero would hand the supervisor a budget short
	// enough to force-kill a teardown still releasing resources — and a signalled
	// process reports no meaningful exit status, so the run would still be green.
	root := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.FixtureConfig{Timeout: 0},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children: []*gotestruntime.FixtureNode{
			{
				Name:      "Child",
				Config:    gotest.FixtureConfig{Timeout: 0},
				Init:      func() {},
				BeforeAll: func(ctx context.Context) error { return nil },
			},
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{
		Roots:                []*gotestruntime.FixtureNode{root},
		MaxSuiteSetupTimeout: 30 * time.Second,
	})
	mustEqual(t, 0, exitCode)

	data, err := os.ReadFile(budgetFile)
	mustNoError(t, err)

	// Each unbounded stage falls back to the 2m default floor: 2m + 2m + 30s + 30s.
	expected := (2*time.Minute + 2*time.Minute + 30*time.Second + 30*time.Second).String()
	mustEqual(t, expected, string(data))
}

func (s *RuntimeTestSuite) TestBudgetFile_TreeAndDAGAgreeOnUndeclaredTimeouts(t *gotest.T) {
	// computeMaxTreePath (Roots) and computeMaxDAGPath (Fixtures) must read the
	// same declared value the same way. They drifted once: the DAG floored a
	// non-positive Timeout while the tree mapped it to zero.
	for _, timeout := range []time.Duration{0, -1, 90 * time.Second} {
		tree := gotestruntime.ExportComputeMaxTreePath([]*gotestruntime.FixtureNode{{
			Name:     "Root",
			Config:   gotest.FixtureConfig{Timeout: timeout},
			Children: []*gotestruntime.FixtureNode{{Name: "Child", Config: gotest.FixtureConfig{Timeout: timeout}}},
		}})
		dag := gotestruntime.ExportComputeMaxDAGPath([]*gotestruntime.FixtureNode{
			{Name: "Root", Config: gotest.FixtureConfig{Timeout: timeout}},
			{Name: "Child", Config: gotest.FixtureConfig{Timeout: timeout}, DependsOn: []string{"Root"}},
		})
		mustEqual(t, dag, tree, "tree and DAG must agree for a declared Timeout of %s", timeout)
		mustGreater(t, tree, time.Duration(0), "a fixture always gets supervisor headroom")
	}
}

func (s *RuntimeTestSuite) TestBudgetFile_NotWrittenWhenEnvUnset(t *gotest.T) {
	t.Setenv(protocol.EnvTeardownBudgetFile, "")

	root := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{root}})
	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestBudgetFile_MultipleRootsUsesMax(t *gotest.T) {
	budgetFile := filepath.Join(t.TempDir(), "budget")
	t.Setenv(protocol.EnvTeardownBudgetFile, budgetFile)

	rootA := &gotestruntime.FixtureNode{
		Name:      "RootA",
		Config:    gotest.FixtureConfig{Timeout: 1 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
	}
	rootB := &gotestruntime.FixtureNode{
		Name:      "RootB",
		Config:    gotest.FixtureConfig{Timeout: 3 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children: []*gotestruntime.FixtureNode{
			{
				Name:      "ChildB1",
				Config:    gotest.FixtureConfig{Timeout: 2 * time.Minute},
				Init:      func() {},
				BeforeAll: func(ctx context.Context) error { return nil },
			},
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{
		Roots:                []*gotestruntime.FixtureNode{rootA, rootB},
		MaxSuiteSetupTimeout: 45 * time.Second,
	})

	mustEqual(t, 0, exitCode)

	data, err := os.ReadFile(budgetFile)
	mustNoError(t, err)

	// Max tree path: max(1m, 3m+2m) = 5m; + 45s suite + 30s headroom
	expected := (5*time.Minute + 45*time.Second + 30*time.Second).String()
	mustEqual(t, expected, string(data))
}

func (s *RuntimeTestSuite) TestTeardownFailure_SetsExitCode1WhenTestsPassed(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			return errors.New("teardown exploded")
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})
	mustEqual(t, 1, exitCode)
}

func (s *RuntimeTestSuite) TestTeardownFailure_PreservesNonZeroExitCode(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			return errors.New("teardown exploded")
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 3 }, gotestruntime.MainConfig{Roots: []*gotestruntime.FixtureNode{node}})
	mustEqual(t, 3, exitCode)
}

func (s *RuntimeTestSuite) TestDAG_LinearChain(t *gotest.T) {
	rec := &recorder{}

	root := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}
	mid := &gotestruntime.FixtureNode{
		Name:      "Mid",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"Root"},
		Init:      func() { rec.record("mid.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("mid.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("mid.afterAll")
			return nil
		},
	}
	leaf := &gotestruntime.FixtureNode{
		Name:      "Leaf",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"Mid"},
		Init:      func() { rec.record("leaf.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("leaf.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("leaf.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{root, mid, leaf}})

	mustEqual(t, 0, exitCode)

	events := rec.names()

	rootBA := indexOf(events, "root.beforeAll")
	midBA := indexOf(events, "mid.beforeAll")
	leafBA := indexOf(events, "leaf.beforeAll")
	mRun := indexOf(events, "m.run")

	mustLess(t, rootBA, midBA, "root.beforeAll must precede mid.beforeAll")
	mustLess(t, midBA, leafBA, "mid.beforeAll must precede leaf.beforeAll")
	mustLess(t, leafBA, mRun, "leaf.beforeAll must precede m.run")

	leafAA := indexOf(events, "leaf.afterAll")
	midAA := indexOf(events, "mid.afterAll")
	rootAA := indexOf(events, "root.afterAll")

	mustLess(t, mRun, leafAA, "m.run must precede leaf.afterAll")
	mustLess(t, leafAA, midAA, "leaf.afterAll must precede mid.afterAll")
	mustLess(t, midAA, rootAA, "mid.afterAll must precede root.afterAll")
}

func (s *RuntimeTestSuite) TestDAG_IndependentFixtures(t *gotest.T) {
	aStarted := make(chan struct{})
	bStarted := make(chan struct{})

	fixtureA := &gotestruntime.FixtureNode{
		Name:   "A",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(aStarted)
			select {
			case <-bStarted:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	fixtureB := &gotestruntime.FixtureNode{
		Name:   "B",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			close(bStarted)
			select {
			case <-aStarted:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{fixtureA, fixtureB}})
	mustEqual(t, 0, exitCode)
}

func (s *RuntimeTestSuite) TestDAG_DiamondDependency(t *gotest.T) {
	rec := &recorder{}

	db := &gotestruntime.FixtureNode{
		Name:   "DB",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("db.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("db.afterAll")
			return nil
		},
	}
	repoA := &gotestruntime.FixtureNode{
		Name:      "RepoA",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"DB"},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("repoA.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("repoA.afterAll")
			return nil
		},
	}
	repoB := &gotestruntime.FixtureNode{
		Name:      "RepoB",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"DB"},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("repoB.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("repoB.afterAll")
			return nil
		},
	}
	service := &gotestruntime.FixtureNode{
		Name:      "Service",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"RepoA", "RepoB"},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("service.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("service.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{db, repoA, repoB, service}})

	mustEqual(t, 0, exitCode)

	events := rec.names()

	dbBA := indexOf(events, "db.beforeAll")
	repoABA := indexOf(events, "repoA.beforeAll")
	repoBBA := indexOf(events, "repoB.beforeAll")
	serviceBA := indexOf(events, "service.beforeAll")
	mRun := indexOf(events, "m.run")

	mustLess(t, dbBA, repoABA, "DB must set up before RepoA")
	mustLess(t, dbBA, repoBBA, "DB must set up before RepoB")
	mustLess(t, repoABA, serviceBA, "RepoA must set up before Service")
	mustLess(t, repoBBA, serviceBA, "RepoB must set up before Service")
	mustLess(t, serviceBA, mRun, "Service must set up before m.run")

	serviceAA := indexOf(events, "service.afterAll")
	repoAAA := indexOf(events, "repoA.afterAll")
	repoBAA := indexOf(events, "repoB.afterAll")
	dbAA := indexOf(events, "db.afterAll")

	mustLess(t, mRun, serviceAA, "m.run must precede service.afterAll")
	mustLess(t, serviceAA, repoAAA, "service.afterAll must precede repoA.afterAll")
	mustLess(t, serviceAA, repoBAA, "service.afterAll must precede repoB.afterAll")
	mustLess(t, repoAAA, dbAA, "repoA.afterAll must precede db.afterAll")
	mustLess(t, repoBAA, dbAA, "repoB.afterAll must precede db.afterAll")
}

func (s *RuntimeTestSuite) TestDAG_DependencyFailure_SkipsDependents(t *gotest.T) {
	rec := &recorder{}

	root := &gotestruntime.FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return errors.New("root fails")
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
	}
	child := &gotestruntime.FixtureNode{
		Name:      "Child",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"Root"},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("child.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("child.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{root, child}})

	mustEqual(t, 2, exitCode)
	events := rec.names()
	mustNotContain(t, events, "child.beforeAll")
	mustNotContain(t, events, "child.afterAll")
	mustNotContain(t, events, "root.afterAll")
}

func (s *RuntimeTestSuite) TestDAG_DependencyFailure_PartialTeardown(t *gotest.T) {
	rec := &recorder{}

	aReady := make(chan struct{})

	a := &gotestruntime.FixtureNode{
		Name:   "A",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("a.beforeAll")
			close(aReady)
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("a.afterAll")
			return nil
		},
	}
	b := &gotestruntime.FixtureNode{
		Name:   "B",
		Config: gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			<-aReady
			return errors.New("B fails")
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("b.afterAll")
			return nil
		},
	}
	c := &gotestruntime.FixtureNode{
		Name:      "C",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"A"},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error {
			rec.record("c.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("c.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{a, b, c}})

	mustEqual(t, 2, exitCode)
	events := rec.names()
	mustContain(t, events, "a.beforeAll")
	mustContain(t, events, "a.afterAll")
	mustNotContain(t, events, "b.afterAll")
}

func (s *RuntimeTestSuite) TestDAG_ComputeMaxPath(t *gotest.T) {
	fixtures := []*gotestruntime.FixtureNode{
		{Name: "A", Config: gotest.FixtureConfig{Timeout: 1 * time.Minute}},
		{Name: "B", Config: gotest.FixtureConfig{Timeout: 3 * time.Minute}, DependsOn: []string{"A"}},
		{Name: "C", Config: gotest.FixtureConfig{Timeout: 2 * time.Minute}, DependsOn: []string{"A"}},
	}

	// Longest path: A(1m) + B(3m) = 4m
	result := gotestruntime.ExportComputeMaxDAGPath(fixtures)
	mustEqual(t, 4*time.Minute, result)
}

func (s *RuntimeTestSuite) TestDAG_InvalidDependency(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:      "Orphan",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"DoesNotExist"},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{node}})
	mustEqual(t, 2, exitCode)
}

func (s *RuntimeTestSuite) TestDAG_SharedStateNode(t *gotest.T) {
	rec := &recorder{}

	stateJSON := `{"ConnStr":"postgres://test"}`
	stateFile := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(stateFile, fmt.Appendf(nil, `{"pkg.PostgresSharedFixture":%s}`, stateJSON), 0600)
	t.Setenv("GOTEST_SHARED_STATE_FILE", stateFile)

	type pg struct{ ConnStr string }
	pgTarget := &pg{}

	pgNode := &gotestruntime.FixtureNode{
		Name: "PostgresSharedFixture",
		SharedState: &gotestruntime.SharedStateNode{
			StateKey: "pkg.PostgresSharedFixture",
			Target:   pgTarget,
			Hydrate: func(ctx context.Context) error {
				rec.record("pg.hydrate")
				return nil
			},
			Dehydrate: func(ctx context.Context) error {
				rec.record("pg.dehydrate")
				return nil
			},
		},
	}

	apiNode := &gotestruntime.FixtureNode{
		Name:      "APIFixture",
		Config:    gotest.DefaultFixtureConfig(),
		DependsOn: []string{"PostgresSharedFixture"},
		Init: func() {
			rec.record("api.init")
			mustEqual(t, "postgres://test", pgTarget.ConnStr)
		},
		BeforeAll: func(ctx context.Context) error {
			rec.record("api.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("api.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{pgNode, apiNode}})

	mustEqual(t, 0, exitCode)

	events := rec.names()
	hydrateIdx := indexOf(events, "pg.hydrate")
	initIdx := indexOf(events, "api.init")
	beforeAllIdx := indexOf(events, "api.beforeAll")
	mRunIdx := indexOf(events, "m.run")
	afterAllIdx := indexOf(events, "api.afterAll")
	dehydrateIdx := indexOf(events, "pg.dehydrate")

	mustGreaterOrEqual(t, hydrateIdx, 0, "hydrate should be called")
	mustLess(t, hydrateIdx, initIdx, "hydrate before api.init")
	mustLess(t, initIdx, beforeAllIdx, "api.init before api.beforeAll")
	mustLess(t, beforeAllIdx, mRunIdx, "api.beforeAll before m.run")
	mustLess(t, mRunIdx, afterAllIdx, "m.run before api.afterAll")
	mustLess(t, afterAllIdx, dehydrateIdx, "api.afterAll before pg.dehydrate")
}

func (s *RuntimeTestSuite) TestDAG_SharedStateNode_HydrateTimeout(t *gotest.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(stateFile, []byte(`{"pkg.PGSharedFixture":{"ConnStr":"x"}}`), 0600)
	t.Setenv("GOTEST_SHARED_STATE_FILE", stateFile)

	type pg struct{ ConnStr string }
	var hydrateHasDeadline, dehydrateHasDeadline bool
	node := &gotestruntime.FixtureNode{
		Name:   "PGSharedFixture",
		Config: gotest.FixtureConfig{Timeout: 5 * time.Minute},
		SharedState: &gotestruntime.SharedStateNode{
			StateKey: "pkg.PGSharedFixture",
			Target:   &pg{},
			Hydrate: func(ctx context.Context) error {
				_, hydrateHasDeadline = ctx.Deadline()
				return nil
			},
			Dehydrate: func(ctx context.Context) error {
				_, dehydrateHasDeadline = ctx.Deadline()
				return nil
			},
		},
	}

	exitCode := gotestruntime.ExportRun(func() int { return 0 }, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{node}})

	mustEqual(t, 0, exitCode)
	mustTrue(t, hydrateHasDeadline, "Hydrate should run under the fixture's configured timeout")
	mustFalse(t, dehydrateHasDeadline, "Dehydrate runs on context.Background")
}

func (s *RuntimeTestSuite) TestDAG_SharedStateChain(t *gotest.T) {
	rec := &recorder{}

	stateFile := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(stateFile, []byte(`{
		"pkg.Postgres": {"ConnStr":"postgres://test"},
		"pkg.Schema":   {"Version":"v42"}
	}`), 0600)
	t.Setenv("GOTEST_SHARED_STATE_FILE", stateFile)

	type pg struct{ ConnStr string }
	type schema struct{ Version string }
	pgTarget := &pg{}
	schemaTarget := &schema{}

	pgNode := &gotestruntime.FixtureNode{
		Name: "Postgres",
		SharedState: &gotestruntime.SharedStateNode{
			StateKey: "pkg.Postgres",
			Target:   pgTarget,
			Hydrate: func(ctx context.Context) error {
				rec.record("pg.hydrate")
				return nil
			},
		},
	}
	schemaNode := &gotestruntime.FixtureNode{
		Name:      "Schema",
		DependsOn: []string{"Postgres"},
		SharedState: &gotestruntime.SharedStateNode{
			StateKey: "pkg.Schema",
			Target:   schemaTarget,
			Hydrate: func(ctx context.Context) error {
				rec.record("schema.hydrate")
				mustEqual(t, "postgres://test", pgTarget.ConnStr)
				return nil
			},
		},
		Init: func() { rec.record("schema.init") },
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{pgNode, schemaNode}})

	mustEqual(t, 0, exitCode)

	events := rec.names()
	mustLess(t, indexOf(events, "pg.hydrate"), indexOf(events, "schema.init"))
	mustLess(t, indexOf(events, "schema.init"), indexOf(events, "schema.hydrate"))
	mustLess(t, indexOf(events, "schema.hydrate"), indexOf(events, "m.run"))
}

func (s *RuntimeTestSuite) TestDAG_SharedStateNode_MissingStateFile(t *gotest.T) {
	rec := &recorder{}

	t.Setenv("GOTEST_SHARED_STATE_FILE", "")

	type pg struct{ ConnStr string }
	pgTarget := &pg{}

	pgNode := &gotestruntime.FixtureNode{
		Name: "PostgresSharedFixture",
		SharedState: &gotestruntime.SharedStateNode{
			StateKey: "pkg.PostgresSharedFixture",
			Target:   pgTarget,
			Hydrate: func(ctx context.Context) error {
				rec.record("pg.hydrate")
				return nil
			},
			Dehydrate: func(ctx context.Context) error {
				rec.record("pg.dehydrate")
				return nil
			},
		},
	}

	plainNode := &gotestruntime.FixtureNode{
		Name:   "PlainFixture",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("plain.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("plain.beforeAll")
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("plain.afterAll")
			return nil
		},
	}

	exitCode := gotestruntime.ExportRun(func() int {
		rec.record("m.run")
		return 0
	}, gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{pgNode, plainNode}})

	mustEqual(t, 0, exitCode)

	events := rec.names()
	mustNotContain(t, events, "pg.hydrate")
	mustNotContain(t, events, "pg.dehydrate")
	mustContain(t, events, "plain.init")
	mustContain(t, events, "plain.beforeAll")
	mustContain(t, events, "m.run")
	mustContain(t, events, "plain.afterAll")
}

func (s *RuntimeTestSuite) TestBeforeAllError_IncludesFixtureName(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:   "Database",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return errors.New("connection refused")
		},
	}

	err := gotestruntime.ExportRunBeforeAllWithRetry(context.Background(), node)

	mustErrorContain(t, err, "Database.BeforeAll")
	mustErrorContain(t, err, "connection refused")
}

func (s *RuntimeTestSuite) TestBeforeAllError_WrapsOriginalError(t *gotest.T) {
	sentinel := errors.New("sentinel")
	node := &gotestruntime.FixtureNode{
		Name:   "Cache",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return sentinel
		},
	}

	err := gotestruntime.ExportRunBeforeAllWithRetry(context.Background(), node)

	mustErrorIs(t, err, sentinel)
}

func (s *RuntimeTestSuite) TestBeforeAllError_ContextCancelIncludesFixtureName(t *gotest.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	node := &gotestruntime.FixtureNode{
		Name:   "Slow",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return nil
		},
	}

	err := gotestruntime.ExportRunBeforeAllWithRetry(ctx, node)

	mustErrorContain(t, err, "Slow.BeforeAll")
	mustErrorIs(t, err, context.Canceled)
}

func (s *RuntimeTestSuite) TestDAGSetupError_IncludesFixtureName(t *gotest.T) {
	node := &gotestruntime.FixtureNode{
		Name:   "Redis",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			return errors.New("dial tcp: connection refused")
		},
	}

	tracker := gotestruntime.ExportNewNodeTracker()
	err := gotestruntime.ExportSetupDAG(context.Background(), []*gotestruntime.FixtureNode{node}, nil, tracker)

	mustErrorContain(t, err, "Redis.BeforeAll")
	mustErrorContain(t, err, "dial tcp: connection refused")
}

// A setup that overran its declared budget completed its work: the resources
// exist, so the fixture is torn down like a success even though the run fails.
func (s *RuntimeTestSuite) TestDAG_OverrunSetupIsStillTornDown(t *gotest.T) {
	rec := &recorder{}

	node := &gotestruntime.FixtureNode{
		Name:   "Slow",
		Config: gotest.FixtureConfig{Timeout: 20 * time.Millisecond},
		Budget: 20 * time.Millisecond,
		BeforeAll: func(ctx context.Context) error {
			time.Sleep(60 * time.Millisecond)
			return nil
		},
		AfterAll: func(ctx context.Context) error {
			rec.record("slow.afterAll")
			return nil
		},
	}

	// gotestruntime.SetupFixtureDAG tears down its partial progress on a setup error; with the
	// overrun marked succeeded, that pass must include this fixture's AfterAll.
	_, err := gotestruntime.SetupFixtureDAG(context.Background(), gotestruntime.MainConfig{Fixtures: []*gotestruntime.FixtureNode{node}})

	mustErrorIs(t, err, gotestruntime.ErrSetupOverran)
	mustEqual(t, []string{"slow.afterAll"}, rec.names(),
		"an overrun-but-successful setup created real resources; skipping its AfterAll leaks them")
}

// A dependent that was merely waiting on the fixture that failed must not
// eclipse the causal error in the report.
func (s *RuntimeTestSuite) TestDAG_CausalErrorPreferredOverVictims(t *gotest.T) {
	boom := errors.New("no route to host")
	victim := &gotestruntime.FixtureNode{
		Name:      "Victim",
		Config:    gotest.DefaultFixtureConfig(),
		BeforeAll: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	}
	culprit := &gotestruntime.FixtureNode{
		Name:      "Culprit",
		Config:    gotest.DefaultFixtureConfig(),
		BeforeAll: func(ctx context.Context) error { return boom },
	}

	tracker := gotestruntime.ExportNewNodeTracker()
	err := gotestruntime.ExportSetupDAG(context.Background(), []*gotestruntime.FixtureNode{victim, culprit}, nil, tracker)

	mustErrorContain(t, err, "Culprit.BeforeAll",
		"the victim's cancellation names nothing an author can act on")
	mustErrorContain(t, err, "no route to host")
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return -1
}
