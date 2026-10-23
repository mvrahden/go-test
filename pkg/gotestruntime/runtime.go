package gotestruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

func setupDAG(ctx context.Context, fixtures []*FixtureNode, sharedState map[string]json.RawMessage, tracker *nodeTracker) error {
	byName := make(map[string]*FixtureNode, len(fixtures))
	for _, f := range fixtures {
		byName[f.Name] = f
	}

	for _, f := range fixtures {
		for _, dep := range f.DependsOn {
			if _, ok := byName[dep]; !ok {
				return fmt.Errorf("fixture %q depends on %q, which does not exist", f.Name, dep)
			}
		}
	}

	done := make(map[string]chan struct{}, len(fixtures))
	for _, f := range fixtures {
		done[f.Name] = make(chan struct{})
	}

	// A failure cancels the siblings with itself as the cause, so a victim
	// records why it was cancelled, not that it was.
	childCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	errs := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, f := range fixtures {
		wg.Add(1)
		go func(node *FixtureNode) {
			defer wg.Done()
			defer close(done[node.Name])
			defer func() {
				if r := recover(); r != nil {
					err := fmt.Errorf("%s: panic: %v", node.Name, r)
					mu.Lock()
					errs[node.Name] = err
					mu.Unlock()
					cancel(err)
				}
			}()

			for _, dep := range node.DependsOn {
				select {
				case <-done[dep]:
					mu.Lock()
					depErr := errs[dep]
					mu.Unlock()
					if depErr != nil {
						err := fmt.Errorf("skipped: dependency %q failed", dep)
						mu.Lock()
						errs[node.Name] = err
						mu.Unlock()
						cancel(err)
						return
					}
				case <-childCtx.Done():
					mu.Lock()
					errs[node.Name] = context.Cause(childCtx)
					mu.Unlock()
					return
				}
			}

			if err := setupNodeDAG(childCtx, node, sharedState, tracker); err != nil {
				mu.Lock()
				errs[node.Name] = err
				mu.Unlock()
				cancel(err)
			}
		}(f)
	}
	wg.Wait()

	// Prefer the causal error over a victim's: cancellation recorded on a node
	// that was merely waiting for the one that actually failed names nothing an
	// author can act on, and the real failure is in the map too.
	var firstErr error
	for _, f := range fixtures {
		if err, ok := errs[f.Name]; ok && err != nil {
			if !errors.Is(err, context.Canceled) {
				return err
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func setupNodeDAG(ctx context.Context, node *FixtureNode, sharedState map[string]json.RawMessage, tracker *nodeTracker) error {
	// Handle shared state nodes (unmarshal + hydrate)
	if node.SharedState != nil {
		// The file names exactly the fixtures this suite requires; the CLI
		// refuses to write one with a required key missing. A node it does
		// not name belongs to a sibling suite of the package and is left
		// alone: hydrating it from a zero value would fail far from the cause.
		raw, ok := sharedState[node.SharedState.StateKey]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(raw, node.SharedState.Target); err != nil {
			return fmt.Errorf("unmarshal shared fixture %q: %w", node.SharedState.StateKey, err)
		}
		if node.Init != nil {
			node.Init()
		}
		if node.SharedState.Hydrate != nil { //nolint:nestif // linear hydrate sequence
			// Dehydrated once Hydrate is entered, however it comes out: a
			// failed or panicked one may hold what it reached.
			tracker.setAttempted(node, true)
			hctx := ctx
			var cancel context.CancelFunc
			if node.Config.Timeout > 0 {
				hctx, cancel = context.WithTimeout(ctx, node.Config.Timeout)
			}
			err := node.SharedState.Hydrate(hctx)
			if cancel != nil {
				cancel()
			}
			if err != nil {
				return fmt.Errorf("hydrate shared fixture %q: %w", node.SharedState.StateKey, err)
			}
		}
		tracker.setAttempted(node, true)
		return nil
	}

	if node.Init != nil {
		node.Init()
	}

	// Attempted once BeforeAll is entered: whatever it reaches, a failed,
	// panicked or cancelled setup included, is released by AfterAll in the
	// teardown pass. A setup cancelled before then has nothing to release,
	// and neither has an attempt already released between retries.
	if err := runBeforeAllWithRetry(ctx, node, func(held bool) { tracker.setAttempted(node, held) }); err != nil {
		return err
	}
	return nil
}

func teardownDAG(fixtures []*FixtureNode, tracker *nodeTracker) bool {
	dependents := make(map[string][]string, len(fixtures))
	for _, f := range fixtures {
		for _, dep := range f.DependsOn {
			dependents[dep] = append(dependents[dep], f.Name)
		}
	}

	done := make(map[string]chan struct{}, len(fixtures))
	for _, f := range fixtures {
		done[f.Name] = make(chan struct{})
	}

	failed := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, f := range fixtures {
		wg.Add(1)
		go func(node *FixtureNode) {
			defer wg.Done()
			defer close(done[node.Name])

			for _, dep := range dependents[node.Name] {
				<-done[dep]
			}

			if node.SharedState != nil {
				if tracker.isAttempted(node) && runDehydrate(node) {
					mu.Lock()
					failed[node.Name] = true
					mu.Unlock()
				}
				return // shared state nodes don't have AfterAll in test process
			}
			// A fixture whose BeforeAll ran is released whether it succeeded
			// or not; one that never started has nothing to release.
			if tracker.isAttempted(node) && runAfterAll(node) {
				mu.Lock()
				failed[node.Name] = true
				mu.Unlock()
			}
		}(f)
	}
	wg.Wait()

	for _, f := range failed {
		if f {
			return true
		}
	}
	return false
}

// runBeforeAllWithRetry adapts a DAG node onto RunFixtureSetup, the single
// policy the generated shared-fixture subprocess runs BeforeAll under too, and
// names the fixture in whatever comes back. held is FixtureSetup.Held.
func runBeforeAllWithRetry(ctx context.Context, node *FixtureNode, held func(bool)) error {
	wrapErr := func(err error) error {
		if err == nil {
			return nil
		}
		return fmt.Errorf("%s.BeforeAll: %w", node.Name, err)
	}

	return wrapErr(RunFixtureSetup(ctx, FixtureSetup{
		Name:       node.Name,
		Timeout:    node.Config.Timeout,
		Budget:     node.Budget,
		Retries:    node.Config.Retries,
		RetryDelay: node.Config.RetryDelay,
		BeforeAll:  node.BeforeAll,
		AfterAll:   node.AfterAll,
		Held:       held,
	}))
}

// runAfterAll adapts a DAG node onto RunFixtureTeardown, the single policy the
// generated shared-fixture subprocess runs AfterAll under too.
func runAfterAll(node *FixtureNode) bool {
	return RunFixtureTeardown(context.Background(), FixtureTeardown{
		Name:     node.Name,
		Timeout:  node.Config.Timeout,
		Budget:   node.Budget,
		AfterAll: node.AfterAll,
	})
}

// runDehydrate mirrors runAfterAll for shared-state nodes.
func runDehydrate(node *FixtureNode) (failed bool) {
	if node.SharedState == nil || node.SharedState.Dehydrate == nil {
		return false
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "%s: dehydrate panicked: %v\n\n%s\n", node.Name, r, debug.Stack())
			failed = true
		}
	}()
	if err := node.SharedState.Dehydrate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%s: dehydrate failed: %v\n", node.Name, err)
		return true
	}
	return false
}

// writeBudgetFile advertises how long this process may take to release its
// fixtures: the largest over every DAG it set up, so the DAG of the other test
// package does not shrink it.
func writeBudgetFile(cfg MainConfig) {
	budget := recordBudget(computeMaxDAGPath(cfg.Fixtures) + cfg.MaxSuiteSetupTimeout + 30*time.Second)
	if budgetFile == "" {
		return
	}
	_ = os.WriteFile(budgetFile, []byte(budget.String()), 0600)
}

func computeMaxDAGPath(fixtures []*FixtureNode) time.Duration {
	byName := make(map[string]*FixtureNode, len(fixtures))
	for _, f := range fixtures {
		byName[f.Name] = f
	}

	cache := make(map[string]time.Duration)
	visiting := make(map[string]bool)
	var longestPath func(name string) time.Duration
	longestPath = func(name string) time.Duration {
		if d, ok := cache[name]; ok {
			return d
		}
		if visiting[name] {
			fmt.Fprintf(os.Stderr, "WARN: cycle detected in fixture DAG at %q, budget may be inaccurate\n", name)
			return 0
		}
		visiting[name] = true
		node := byName[name]
		own := SupervisorBudget(node.Config.Timeout)
		var maxDep time.Duration
		for _, dep := range node.DependsOn {
			depPath := longestPath(dep)
			if depPath > maxDep {
				maxDep = depPath
			}
		}
		result := own + maxDep
		cache[name] = result
		return result
	}

	var maxPath time.Duration
	for _, f := range fixtures {
		p := longestPath(f.Name)
		if p > maxPath {
			maxPath = p
		}
	}
	return maxPath
}

func loadSharedState(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the runner names the file
	if err != nil {
		return nil, fmt.Errorf("read shared state file: %w", err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal shared state: %w", err)
	}
	return state, nil
}

// RunnerRequiredError refuses a suite that binds a shared fixture outside the
// gotest runner, the only writer of the fixture's state.
type RunnerRequiredError struct {
	Fixture string
}

func (e *RunnerRequiredError) Error() string {
	return "suite binds shared fixture " + e.Fixture + ", whose state only the gotest runner provides; run via go tool gotest"
}

// explained holds the fixtures whose refusal this process explained in full.
var explained sync.Map

// report is the refusal as one test reports it: in full the first time the
// process refuses the fixture, briefly after that.
func (e *RunnerRequiredError) report() string {
	if _, seen := explained.LoadOrStore(e.Fixture, true); seen {
		return "suite binds shared fixture " + e.Fixture + "; run via go tool gotest"
	}
	return e.Error()
}

// sharedNode returns the first shared fixture among fixtures, or nil.
func sharedNode(fixtures []*FixtureNode) *FixtureNode {
	for _, f := range fixtures {
		if f.SharedState != nil {
			return f
		}
	}
	return nil
}

// nodeTracker records the nodes the teardown pass releases.
type nodeTracker struct {
	mu        sync.Mutex
	attempted map[*FixtureNode]bool
}

// setAttempted records whether node's BeforeAll, or a shared node's Hydrate,
// ran with nothing released since, whatever came of it.
func (t *nodeTracker) setAttempted(node *FixtureNode, attempted bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.attempted == nil {
		t.attempted = map[*FixtureNode]bool{}
	}
	t.attempted[node] = attempted
}

func (t *nodeTracker) isAttempted(node *FixtureNode) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.attempted[node]
}

func SetupFixtureDAG(ctx context.Context, cfg MainConfig) (*FixtureDAG, error) {
	return setupFixtureDAG(ctx, cfg, nil)
}

// setupFixtureDAG sets the DAG up and hands it to up, if given, while the
// setup still counts as in flight: a stop that waits for setups then finds
// the DAG to release.
func setupFixtureDAG(ctx context.Context, cfg MainConfig, up func(*FixtureDAG)) (*FixtureDAG, error) {
	// A stop cancels setup in flight and waits for it; a canceled setup tears
	// down what came up.
	ctx, done, err := beginSetup(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	tracker := &nodeTracker{}

	// Only the runner writes a shared fixture's state; without the file the
	// suite is running under plain go test and has nothing to hydrate from.
	var sharedState map[string]json.RawMessage
	if shared := sharedNode(cfg.Fixtures); shared != nil {
		path := os.Getenv(protocol.EnvSharedStateFile)
		if path == "" {
			return nil, &RunnerRequiredError{Fixture: shared.Name}
		}
		var err error
		sharedState, err = loadSharedState(path)
		if err != nil {
			return nil, fmt.Errorf("load shared state: %w", err)
		}
	}

	if len(cfg.Fixtures) > 0 {
		if err := setupDAG(ctx, cfg.Fixtures, sharedState, tracker); err != nil {
			_ = teardownDAG(cfg.Fixtures, tracker)
			return nil, err
		}
	}

	writeBudgetFile(cfg)

	dag := &FixtureDAG{cfg: cfg, tracker: tracker}
	if up != nil {
		up(dag)
	}
	return dag, nil
}

func (d *FixtureDAG) Teardown() bool {
	d.torn.Do(func() {
		d.failed = teardownDAG(d.cfg.Fixtures, d.tracker)
	})
	return d.failed
}
