package gotestruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

func run(runTests func() int, cfg MainConfig) int {
	tracker := &nodeTracker{succeeded: make(map[*FixtureNode]bool)}

	var sharedState map[string]json.RawMessage
	if anyNodeHasSharedState(cfg.Fixtures) {
		if os.Getenv(protocol.EnvSharedStateFile) != "" {
			var err error
			sharedState, err = loadSharedState()
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
				return 2
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if len(cfg.Fixtures) > 0 {
		if err := setupDAG(ctx, cfg.Fixtures, sharedState, tracker); err != nil {
			_ = teardownDAG(cfg.Fixtures, tracker)
			return 2
		}

		writeBudgetFile(cfg)

		code := runTests()

		if teardownDAG(cfg.Fixtures, tracker) && code == 0 {
			code = 1
		}
		return code
	}

	if err := setupRoots(ctx, cfg.Roots, tracker); err != nil {
		_ = teardownRoots(cfg.Roots, tracker)
		return 2
	}

	writeBudgetFile(cfg)

	code := runTests()

	if teardownRoots(cfg.Roots, tracker) && code == 0 {
		code = 1
	}

	return code
}

func setupRoots(ctx context.Context, roots []*FixtureNode, tracker *nodeTracker) error {
	errs := make([]error, len(roots))
	var wg sync.WaitGroup

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, root := range roots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("%s: panic: %v", root.Name, r)
					cancel()
				}
			}()
			if err := setupNode(childCtx, root, tracker); err != nil {
				errs[i] = err
				cancel()
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func setupNode(ctx context.Context, node *FixtureNode, tracker *nodeTracker) error {
	if node.Init != nil {
		node.Init()
	}

	if err := runBeforeAllWithRetry(ctx, node); err != nil {
		return err
	}

	tracker.markSucceeded(node)

	if len(node.Children) > 0 {
		errs := make([]error, len(node.Children))
		var wg sync.WaitGroup

		childCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for i, child := range node.Children {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						errs[i] = fmt.Errorf("%s: panic: %v", child.Name, r)
						cancel()
					}
				}()
				if err := setupNode(childCtx, child, tracker); err != nil {
					errs[i] = err
					cancel()
				}
			}()
		}
		wg.Wait()

		for _, err := range errs {
			if err != nil {
				return err
			}
		}
	}

	return nil
}

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

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

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
					mu.Lock()
					errs[node.Name] = fmt.Errorf("%s: panic: %v", node.Name, r)
					mu.Unlock()
					cancel()
				}
			}()

			for _, dep := range node.DependsOn {
				select {
				case <-done[dep]:
					mu.Lock()
					depErr := errs[dep]
					mu.Unlock()
					if depErr != nil {
						mu.Lock()
						errs[node.Name] = fmt.Errorf("skipped: dependency %q failed", dep)
						mu.Unlock()
						cancel()
						return
					}
				case <-childCtx.Done():
					mu.Lock()
					errs[node.Name] = childCtx.Err()
					mu.Unlock()
					return
				}
			}

			if err := setupNodeDAG(childCtx, node, sharedState, tracker); err != nil {
				mu.Lock()
				errs[node.Name] = err
				mu.Unlock()
				cancel()
			}
		}(f)
	}
	wg.Wait()

	for _, f := range fixtures {
		if err, ok := errs[f.Name]; ok && err != nil {
			return err
		}
	}
	return nil
}

func setupNodeDAG(ctx context.Context, node *FixtureNode, sharedState map[string]json.RawMessage, tracker *nodeTracker) error {
	// Handle shared state nodes (unmarshal + hydrate)
	if node.SharedState != nil {
		if sharedState == nil {
			return nil
		}
		raw, ok := sharedState[node.SharedState.StateKey]
		if ok {
			if err := json.Unmarshal(raw, node.SharedState.Target); err != nil {
				return fmt.Errorf("unmarshal shared fixture %q: %w", node.SharedState.StateKey, err)
			}
		}
		if node.Init != nil {
			node.Init()
		}
		if node.SharedState.Hydrate != nil { //nolint:nestif // linear hydrate sequence
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
		tracker.markSucceeded(node)
		return nil
	}

	if node.Init != nil {
		node.Init()
	}

	if err := runBeforeAllWithRetry(ctx, node); err != nil {
		return err
	}

	tracker.markSucceeded(node)
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

			if tracker.isSucceeded(node) {
				if node.SharedState != nil {
					if runDehydrate(node) {
						mu.Lock()
						failed[node.Name] = true
						mu.Unlock()
					}
					return // shared state nodes don't have AfterAll in test process
				}

				if runAfterAll(node) {
					mu.Lock()
					failed[node.Name] = true
					mu.Unlock()
				}
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
// names the fixture in whatever comes back.
func runBeforeAllWithRetry(ctx context.Context, node *FixtureNode) error {
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
	}))
}

func teardownRoots(roots []*FixtureNode, tracker *nodeTracker) bool {
	failed := make([]bool, len(roots))
	var wg sync.WaitGroup
	for i, root := range roots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if teardownNode(root, tracker) {
				failed[i] = true
			}
		}()
	}
	wg.Wait()
	for _, f := range failed {
		if f {
			return true
		}
	}
	return false
}

func teardownNode(node *FixtureNode, tracker *nodeTracker) bool {
	var anyFailed bool

	if len(node.Children) > 0 {
		childFailed := make([]bool, len(node.Children))
		var wg sync.WaitGroup
		for i, child := range node.Children {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if teardownNode(child, tracker) {
					childFailed[i] = true
				}
			}()
		}
		wg.Wait()
		for _, f := range childFailed {
			if f {
				anyFailed = true
				break
			}
		}
	}

	if tracker.isSucceeded(node) && runAfterAll(node) {
		anyFailed = true
	}

	return anyFailed
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

func writeBudgetFile(cfg MainConfig) {
	path := os.Getenv(protocol.EnvTeardownBudgetFile)
	if path == "" {
		return
	}

	var maxPath time.Duration
	if len(cfg.Fixtures) > 0 {
		maxPath = computeMaxDAGPath(cfg.Fixtures)
	} else {
		maxPath = computeMaxTreePath(cfg.Roots)
	}
	budget := maxPath + cfg.MaxSuiteSetupTimeout + 30*time.Second
	_ = os.WriteFile(path, []byte(budget.String()), 0600)
}

func computeMaxTreePath(roots []*FixtureNode) time.Duration {
	var maxPath time.Duration
	for _, root := range roots {
		path := nodeTreePath(root)
		if path > maxPath {
			maxPath = path
		}
	}
	return maxPath
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

func nodeTreePath(node *FixtureNode) time.Duration {
	own := SupervisorBudget(node.Config.Timeout)
	var maxChild time.Duration
	for _, child := range node.Children {
		childPath := nodeTreePath(child)
		if childPath > maxChild {
			maxChild = childPath
		}
	}
	return own + maxChild
}

func loadSharedState() (map[string]json.RawMessage, error) {
	path := os.Getenv(protocol.EnvSharedStateFile)
	if path == "" {
		return nil, fmt.Errorf("%s not set — run via gotest CLI", protocol.EnvSharedStateFile)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read shared state file: %w", err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal shared state: %w", err)
	}
	return state, nil
}

func anyNodeHasSharedState(fixtures []*FixtureNode) bool {
	for _, f := range fixtures {
		if f.SharedState != nil {
			return true
		}
	}
	return false
}

type nodeTracker struct {
	mu        sync.Mutex
	succeeded map[*FixtureNode]bool
}

func (t *nodeTracker) markSucceeded(node *FixtureNode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.succeeded[node] = true
}

func (t *nodeTracker) isSucceeded(node *FixtureNode) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.succeeded[node]
}

func SetupFixtureDAG(ctx context.Context, cfg MainConfig) (*FixtureDAG, error) {
	tracker := &nodeTracker{succeeded: make(map[*FixtureNode]bool)}

	var sharedState map[string]json.RawMessage
	if anyNodeHasSharedState(cfg.Fixtures) {
		if os.Getenv(protocol.EnvSharedStateFile) != "" {
			var err error
			sharedState, err = loadSharedState()
			if err != nil {
				return nil, fmt.Errorf("load shared state: %w", err)
			}
		}
	}

	if len(cfg.Fixtures) > 0 {
		if err := setupDAG(ctx, cfg.Fixtures, sharedState, tracker); err != nil {
			_ = teardownDAG(cfg.Fixtures, tracker)
			return nil, err
		}
	} else if len(cfg.Roots) > 0 {
		if err := setupRoots(ctx, cfg.Roots, tracker); err != nil {
			_ = teardownRoots(cfg.Roots, tracker)
			return nil, err
		}
	}

	writeBudgetFile(cfg)

	return &FixtureDAG{cfg: cfg, tracker: tracker}, nil
}

func (d *FixtureDAG) Teardown() bool {
	d.torn.Do(func() {
		if len(d.cfg.Fixtures) > 0 {
			d.failed = teardownDAG(d.cfg.Fixtures, d.tracker)
		} else {
			d.failed = teardownRoots(d.cfg.Roots, d.tracker)
		}
	})
	return d.failed
}
