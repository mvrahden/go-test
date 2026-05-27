package gotestruntime //nolint:stdlib-test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

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

func TestSingleRoot_LifecycleOrder(t *testing.T) {
	rec := &recorder{}

	node := &FixtureNode{
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

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, []string{"root.init", "root.beforeAll", "m.run", "root.afterAll"}, rec.names())
}

func TestSingleRoot_ExitCodeForwarded(t *testing.T) {
	node := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll:  func(ctx context.Context) error { return nil },
	}

	exitCode := run(func() int {
		return 42
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 42, exitCode)
}

func TestSingleRoot_AfterAllCalledOnNonZeroExit(t *testing.T) {
	rec := &recorder{}

	node := &FixtureNode{
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

	exitCode := run(func() int {
		return 1
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 1, exitCode)
	gotest.Contains(t, rec.names(), "root.afterAll")
}

func TestSingleRoot_NilAfterAll(t *testing.T) {
	rec := &recorder{}

	node := &FixtureNode{
		Name:   "Root",
		Config: gotest.DefaultFixtureConfig(),
		Init:   func() { rec.record("root.init") },
		BeforeAll: func(ctx context.Context) error {
			rec.record("root.beforeAll")
			return nil
		},
		AfterAll: nil,
	}

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, []string{"root.init", "root.beforeAll", "m.run"}, rec.names())
}

func TestSingleRoot_NilInit(t *testing.T) {
	rec := &recorder{}

	node := &FixtureNode{
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

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, []string{"root.beforeAll", "m.run", "root.afterAll"}, rec.names())
}

func TestRetry_SucceedsOnSecondAttempt(t *testing.T) {
	rec := &recorder{}
	attempts := 0

	node := &FixtureNode{
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

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, 2, attempts)
	gotest.Equal(t, []string{"root.init", "root.beforeAll", "m.run", "root.afterAll"}, rec.names())
}

func TestRetry_ExhaustedRetriesReturnsExitCode2(t *testing.T) {
	rec := &recorder{}
	attempts := 0

	node := &FixtureNode{
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
	exitCode := run(func() int {
		mRunCalled = true
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 2, exitCode)
	gotest.Equal(t, 2, attempts)
	gotest.False(t, mRunCalled)
	gotest.Equal(t, []string{}, rec.names())
}

func TestRetry_DelayObservedBetweenAttempts(t *testing.T) {
	attempts := 0
	var timestamps []time.Time

	node := &FixtureNode{
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

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, 2, len(timestamps))
	elapsed := timestamps[1].Sub(timestamps[0])
	gotest.GreaterOrEqual(t, elapsed, 40*time.Millisecond)
}

func TestTimeout_BeforeAllExceedsTimeout(t *testing.T) {
	node := &FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: 50 * time.Millisecond},
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
	exitCode := run(func() int {
		mRunCalled = true
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 2, exitCode)
	gotest.False(t, mRunCalled)
}

func TestTimeout_BeforeAllCompletesWithinTimeout(t *testing.T) {
	node := &FixtureNode{
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

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
}

func TestTimeout_DisabledWithNegativeOne(t *testing.T) {
	node := &FixtureNode{
		Name:   "Root",
		Config: gotest.FixtureConfig{Timeout: -1},
		Init:   func() {},
		BeforeAll: func(ctx context.Context) error {
			deadline, hasDeadline := ctx.Deadline()
			_ = deadline
			gotest.False(t, hasDeadline)
			return nil
		},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{node}})
	gotest.Equal(t, 0, exitCode)
}

func TestChildren_SetupOrder(t *testing.T) {
	rec := &recorder{}

	childA := &FixtureNode{
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
	childB := &FixtureNode{
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

	root := &FixtureNode{
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
		Children: []*FixtureNode{childA, childB},
	}

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{root}})

	gotest.Equal(t, 0, exitCode)

	events := rec.names()

	// Root must come before any child
	rootInitIdx := indexOf(events, "root.init")
	rootBeforeAllIdx := indexOf(events, "root.beforeAll")
	gotest.True(t, rootInitIdx >= 0)
	gotest.True(t, rootBeforeAllIdx >= 0)
	gotest.True(t, rootInitIdx < rootBeforeAllIdx)

	// Children must come after root.beforeAll
	for _, ev := range []string{"childA.init", "childA.beforeAll", "childB.init", "childB.beforeAll"} {
		idx := indexOf(events, ev)
		gotest.True(t, idx > rootBeforeAllIdx, "expected %s after root.beforeAll", ev)
	}

	// m.run must come after all children setup
	mRunIdx := indexOf(events, "m.run")
	for _, ev := range []string{"childA.beforeAll", "childB.beforeAll"} {
		idx := indexOf(events, ev)
		gotest.True(t, idx < mRunIdx, "expected %s before m.run", ev)
	}

	// Root AfterAll must come after children AfterAll
	rootAfterAllIdx := indexOf(events, "root.afterAll")
	for _, ev := range []string{"childA.afterAll", "childB.afterAll"} {
		idx := indexOf(events, ev)
		gotest.True(t, idx < rootAfterAllIdx, "expected %s before root.afterAll", ev)
	}
}

func TestChildren_ConcurrentSetup(t *testing.T) {
	childAStarted := make(chan struct{})
	childBStarted := make(chan struct{})

	childA := &FixtureNode{
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
	childB := &FixtureNode{
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

	root := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children:  []*FixtureNode{childA, childB},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{root}})
	gotest.Equal(t, 0, exitCode)
}

func TestChildFailure_CancelsSiblings(t *testing.T) {
	rec := &recorder{}
	childAStarted := make(chan struct{})

	childA := &FixtureNode{
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
	childB := &FixtureNode{
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

	root := &FixtureNode{
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
		Children: []*FixtureNode{childA, childB},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{root}})

	gotest.Equal(t, 2, exitCode)
	events := rec.names()
	// ChildA.AfterAll NOT called (never succeeded)
	gotest.NotContains(t, events, "childA.afterAll")
	// ChildB.AfterAll NOT called (cancelled before success)
	gotest.NotContains(t, events, "childB.afterAll")
	// ChildB.BeforeAll should have been cancelled, not completed
	gotest.NotContains(t, events, "childB.beforeAll.completed")
	// Root.AfterAll IS called (root succeeded)
	gotest.Contains(t, events, "root.afterAll")
}

func TestChildFailure_SucceededSiblingGetsAfterAll(t *testing.T) {
	rec := &recorder{}
	childBReady := make(chan struct{})

	childA := &FixtureNode{
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
	childB := &FixtureNode{
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

	root := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			rec.record("root.afterAll")
			return nil
		},
		Children: []*FixtureNode{childA, childB},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{root}})

	gotest.Equal(t, 2, exitCode)
	events := rec.names()
	// ChildA never succeeded
	gotest.NotContains(t, events, "childA.afterAll")
	// ChildB succeeded → AfterAll must be called
	gotest.Contains(t, events, "childB.afterAll")
	// Root succeeded → AfterAll must be called
	gotest.Contains(t, events, "root.afterAll")
}

func TestTreeDepth_ThreeLevels(t *testing.T) {
	rec := &recorder{}

	grandchild := &FixtureNode{
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

	child := &FixtureNode{
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
		Children: []*FixtureNode{grandchild},
	}

	root := &FixtureNode{
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
		Children: []*FixtureNode{child},
	}

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{root}})

	gotest.Equal(t, 0, exitCode)
	events := rec.names()

	// Setup order: root → child → grandchild
	rootBA := indexOf(events, "root.beforeAll")
	childBA := indexOf(events, "child.beforeAll")
	grandchildBA := indexOf(events, "grandchild.beforeAll")
	mRun := indexOf(events, "m.run")

	gotest.True(t, rootBA < childBA)
	gotest.True(t, childBA < grandchildBA)
	gotest.True(t, grandchildBA < mRun)

	// Teardown order: grandchild → child → root
	grandchildAA := indexOf(events, "grandchild.afterAll")
	childAA := indexOf(events, "child.afterAll")
	rootAA := indexOf(events, "root.afterAll")

	gotest.True(t, mRun < grandchildAA)
	gotest.True(t, grandchildAA < childAA)
	gotest.True(t, childAA < rootAA)
}

func TestMultipleRoots_ConcurrentSetup(t *testing.T) {
	rootAStarted := make(chan struct{})
	rootBStarted := make(chan struct{})

	rootA := &FixtureNode{
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
	rootB := &FixtureNode{
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

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{rootA, rootB}})
	gotest.Equal(t, 0, exitCode)
}

func TestMultipleRoots_OneFailsCancelsOther(t *testing.T) {
	rec := &recorder{}

	rootA := &FixtureNode{
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
	rootB := &FixtureNode{
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

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{rootA, rootB}})

	gotest.Equal(t, 2, exitCode)
	events := rec.names()
	gotest.NotContains(t, events, "rootA.afterAll")
	gotest.NotContains(t, events, "rootB.afterAll")
}

func TestMultipleRoots_ConcurrentTeardown(t *testing.T) {
	rootATeardownStarted := make(chan struct{})
	rootBTeardownStarted := make(chan struct{})

	rootA := &FixtureNode{
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
	rootB := &FixtureNode{
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

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{rootA, rootB}})
	gotest.Equal(t, 0, exitCode)
}

func TestSharedFixture_LoadAndHydrate(t *testing.T) {
	rec := &recorder{}

	type SharedDB struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	stateData := map[string]json.RawMessage{
		"example.com/fixtures.SharedDB": json.RawMessage(`{"host":"localhost","port":5432}`),
	}
	stateBytes, _ := json.Marshal(stateData)
	stateFile := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(stateFile, stateBytes, 0644)
	t.Setenv("GOTEST_SHARED_STATE_FILE", stateFile)

	var target SharedDB
	var assignedHost string

	node := &FixtureNode{
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
		SharedFixtures: []SharedFixtureBinding{
			{
				StateKey: "example.com/fixtures.SharedDB",
				Target:   &target,
				Hydrate: func(ctx context.Context) error {
					rec.record("sf.hydrate")
					return nil
				},
				Dehydrate: func(ctx context.Context) error {
					rec.record("sf.dehydrate")
					return nil
				},
				Assign: func() {
					rec.record("sf.assign")
					assignedHost = target.Host
				},
			},
		},
	}

	exitCode := run(func() int {
		rec.record("m.run")
		return 0
	}, MainConfig{Roots: []*FixtureNode{node}})

	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, "localhost", target.Host)
	gotest.Equal(t, 5432, target.Port)
	gotest.Equal(t, "localhost", assignedHost)

	events := rec.names()
	// Order: hydrate → init → assign → beforeAll → m.run → afterAll → dehydrate
	hydrateIdx := indexOf(events, "sf.hydrate")
	initIdx := indexOf(events, "root.init")
	assignIdx := indexOf(events, "sf.assign")
	beforeAllIdx := indexOf(events, "root.beforeAll")
	mRunIdx := indexOf(events, "m.run")
	afterAllIdx := indexOf(events, "root.afterAll")
	dehydrateIdx := indexOf(events, "sf.dehydrate")

	gotest.True(t, hydrateIdx < initIdx)
	gotest.True(t, initIdx < assignIdx)
	gotest.True(t, assignIdx < beforeAllIdx)
	gotest.True(t, beforeAllIdx < mRunIdx)
	gotest.True(t, mRunIdx < afterAllIdx)
	gotest.True(t, afterAllIdx < dehydrateIdx)
}

func TestSharedFixture_MissingEnvVar(t *testing.T) {
	t.Setenv("GOTEST_SHARED_STATE_FILE", "")

	node := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		SharedFixtures: []SharedFixtureBinding{
			{
				StateKey: "example.com/fixtures.SharedDB",
				Target:   &struct{}{},
			},
		},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{node}})
	gotest.Equal(t, 2, exitCode)
}

func TestSharedFixture_NilHydrateAndDehydrate(t *testing.T) {
	type SharedDB struct {
		Host string `json:"host"`
	}

	stateData := map[string]json.RawMessage{
		"key": json.RawMessage(`{"host":"db"}`),
	}
	stateBytes, _ := json.Marshal(stateData)
	stateFile := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(stateFile, stateBytes, 0644)
	t.Setenv("GOTEST_SHARED_STATE_FILE", stateFile)

	var target SharedDB
	node := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		SharedFixtures: []SharedFixtureBinding{
			{
				StateKey:  "key",
				Target:    &target,
				Hydrate:   nil,
				Dehydrate: nil,
				Assign:    func() {},
			},
		},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{node}})
	gotest.Equal(t, 0, exitCode)
	gotest.Equal(t, "db", target.Host)
}

func TestBudgetFile_WrittenCorrectly(t *testing.T) {
	budgetFile := filepath.Join(t.TempDir(), "budget")
	t.Setenv("GOTEST_TEARDOWN_BUDGET_FILE", budgetFile)

	root := &FixtureNode{
		Name:      "Root",
		Config:    gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children: []*FixtureNode{
			{
				Name:      "Child",
				Config:    gotest.FixtureConfig{Timeout: 1 * time.Minute},
				Init:      func() {},
				BeforeAll: func(ctx context.Context) error { return nil },
			},
		},
	}

	exitCode := run(func() int { return 0 }, MainConfig{
		Roots:                []*FixtureNode{root},
		MaxSuiteSetupTimeout: 30 * time.Second,
	})

	gotest.Equal(t, 0, exitCode)

	data, err := os.ReadFile(budgetFile)
	gotest.NoError(t, err)

	// Budget = max tree path (2m root + 1m child) + max suite setup (30s) + 30s
	expected := (2*time.Minute + 1*time.Minute + 30*time.Second + 30*time.Second).String()
	gotest.Equal(t, expected, string(data))
}

func TestBudgetFile_NotWrittenWhenEnvUnset(t *testing.T) {
	t.Setenv("GOTEST_TEARDOWN_BUDGET_FILE", "")

	root := &FixtureNode{
		Name:      "Root",
		Config:    gotest.FixtureConfig{Timeout: 2 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{root}})
	gotest.Equal(t, 0, exitCode)
}

func TestBudgetFile_MultipleRootsUsesMax(t *testing.T) {
	budgetFile := filepath.Join(t.TempDir(), "budget")
	t.Setenv("GOTEST_TEARDOWN_BUDGET_FILE", budgetFile)

	rootA := &FixtureNode{
		Name:      "RootA",
		Config:    gotest.FixtureConfig{Timeout: 1 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
	}
	rootB := &FixtureNode{
		Name:      "RootB",
		Config:    gotest.FixtureConfig{Timeout: 3 * time.Minute},
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		Children: []*FixtureNode{
			{
				Name:      "ChildB1",
				Config:    gotest.FixtureConfig{Timeout: 2 * time.Minute},
				Init:      func() {},
				BeforeAll: func(ctx context.Context) error { return nil },
			},
		},
	}

	exitCode := run(func() int { return 0 }, MainConfig{
		Roots:                []*FixtureNode{rootA, rootB},
		MaxSuiteSetupTimeout: 45 * time.Second,
	})

	gotest.Equal(t, 0, exitCode)

	data, err := os.ReadFile(budgetFile)
	gotest.NoError(t, err)

	// Max tree path: max(1m, 3m+2m) = 5m; + 45s suite + 30s headroom
	expected := (5*time.Minute + 45*time.Second + 30*time.Second).String()
	gotest.Equal(t, expected, string(data))
}

func TestTeardownFailure_SetsExitCode1WhenTestsPassed(t *testing.T) {
	node := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			return errors.New("teardown exploded")
		},
	}

	exitCode := run(func() int { return 0 }, MainConfig{Roots: []*FixtureNode{node}})
	gotest.Equal(t, 1, exitCode)
}

func TestTeardownFailure_PreservesNonZeroExitCode(t *testing.T) {
	node := &FixtureNode{
		Name:      "Root",
		Config:    gotest.DefaultFixtureConfig(),
		Init:      func() {},
		BeforeAll: func(ctx context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			return errors.New("teardown exploded")
		},
	}

	exitCode := run(func() int { return 3 }, MainConfig{Roots: []*FixtureNode{node}})
	gotest.Equal(t, 3, exitCode)
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return -1
}
