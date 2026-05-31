package gotestruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotestruntime/coverage"
)

func run(runTests func() int, cfg MainConfig) int {
	restoreCoverage := coverage.InterceptTeardown()

	tracker := &nodeTracker{succeeded: make(map[*FixtureNode]bool)}

	var sharedState map[string]json.RawMessage
	if anyNodeHasSharedFixtures(cfg.Roots, cfg.Fixtures) {
		if os.Getenv("GOTEST_SHARED_STATE_FILE") != "" {
			var err error
			sharedState, err = loadSharedState()
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
				restoreCoverage()
				return 2
			}
		} else if anyNodeHasLegacySharedFixtures(cfg.Roots, cfg.Fixtures) {
			fmt.Fprintf(os.Stderr, "FAIL: GOTEST_SHARED_STATE_FILE not set — run via gotest CLI\n")
			restoreCoverage()
			return 2
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if len(cfg.Fixtures) > 0 {
		if err := setupDAG(ctx, cfg.Fixtures, sharedState, tracker); err != nil {
			_ = teardownDAG(cfg.Fixtures, tracker)
			restoreCoverage()
			return 2
		}

		writeBudgetFile(cfg)

		code := runTests()

		if teardownDAG(cfg.Fixtures, tracker) && code == 0 {
			code = 1
		}
		restoreCoverage()
		return code
	}

	if err := setupRoots(ctx, cfg.Roots, sharedState, tracker); err != nil {
		_ = teardownRoots(cfg.Roots, tracker)
		restoreCoverage()
		return 2
	}

	writeBudgetFile(cfg)

	code := runTests()

	if teardownRoots(cfg.Roots, tracker) && code == 0 {
		code = 1
	}
	restoreCoverage()

	return code
}

func setupRoots(ctx context.Context, roots []*FixtureNode, sharedState map[string]json.RawMessage, tracker *nodeTracker) error {
	errs := make([]error, len(roots))
	var wg sync.WaitGroup

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, root := range roots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := setupNode(childCtx, root, sharedState, tracker); err != nil {
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

func setupNode(ctx context.Context, node *FixtureNode, sharedState map[string]json.RawMessage, tracker *nodeTracker) error {
	// 1-2. Unmarshal and hydrate shared fixtures
	if len(node.SharedFixtures) > 0 {
		for _, sf := range node.SharedFixtures {
			raw, ok := sharedState[sf.StateKey]
			if !ok {
				return fmt.Errorf("shared fixture state key %q not found in state file", sf.StateKey)
			}
			if err := json.Unmarshal(raw, sf.Target); err != nil {
				return fmt.Errorf("unmarshal shared fixture %q: %w", sf.StateKey, err)
			}
			if sf.Hydrate != nil {
				if err := sf.Hydrate(ctx); err != nil {
					return fmt.Errorf("hydrate shared fixture %q: %w", sf.StateKey, err)
				}
			}
		}
	}

	// 3. Init
	if node.Init != nil {
		node.Init()
	}

	// 4. Assign shared fixtures
	for _, sf := range node.SharedFixtures {
		if sf.Assign != nil {
			sf.Assign()
		}
	}

	// 5. BeforeAll with retry/timeout
	if err := runBeforeAllWithRetry(ctx, node); err != nil {
		return err
	}

	tracker.markSucceeded(node)

	// 6. Setup children concurrently
	if len(node.Children) > 0 {
		errs := make([]error, len(node.Children))
		var wg sync.WaitGroup

		childCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for i, child := range node.Children {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := setupNode(childCtx, child, sharedState, tracker); err != nil {
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
		if node.SharedState.Hydrate != nil {
			if err := node.SharedState.Hydrate(ctx); err != nil {
				return fmt.Errorf("hydrate shared fixture %q: %w", node.SharedState.StateKey, err)
			}
		}
		tracker.markSucceeded(node)
		return nil
	}

	if len(node.SharedFixtures) > 0 {
		for _, sf := range node.SharedFixtures {
			raw, ok := sharedState[sf.StateKey]
			if !ok {
				return fmt.Errorf("shared fixture state key %q not found in state file", sf.StateKey)
			}
			if err := json.Unmarshal(raw, sf.Target); err != nil {
				return fmt.Errorf("unmarshal shared fixture %q: %w", sf.StateKey, err)
			}
			if sf.Hydrate != nil {
				if err := sf.Hydrate(ctx); err != nil {
					return fmt.Errorf("hydrate shared fixture %q: %w", sf.StateKey, err)
				}
			}
		}
	}

	if node.Init != nil {
		node.Init()
	}

	for _, sf := range node.SharedFixtures {
		if sf.Assign != nil {
			sf.Assign()
		}
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
					if node.SharedState.Dehydrate != nil {
						if err := node.SharedState.Dehydrate(context.Background()); err != nil {
							fmt.Fprintf(os.Stderr, "%s: dehydrate failed: %v\n", node.Name, err)
							mu.Lock()
							failed[node.Name] = true
							mu.Unlock()
						}
					}
					return  // shared state nodes don't have AfterAll in test process
				}

				if node.AfterAll != nil {
					ctx := context.Background()
					if node.Config.Timeout > 0 {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, node.Config.Timeout)
						defer cancel()
					}
					if err := node.AfterAll(ctx); err != nil {
						fmt.Fprintf(os.Stderr, "%s.AfterAll failed: %v\n", node.Name, err)
						mu.Lock()
						failed[node.Name] = true
						mu.Unlock()
					}
				}

				for _, sf := range node.SharedFixtures {
					if sf.Dehydrate != nil {
						if err := sf.Dehydrate(context.Background()); err != nil {
							fmt.Fprintf(os.Stderr, "%s: dehydrate failed: %v\n", node.Name, err)
							mu.Lock()
							failed[node.Name] = true
							mu.Unlock()
						}
					}
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

func runBeforeAllWithRetry(ctx context.Context, node *FixtureNode) error {
	attempts := 1 + node.Config.Retries
	var lastErr error

	for i := range attempts {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var attemptCtx context.Context
		var attemptCancel context.CancelFunc
		if node.Config.Timeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(ctx, node.Config.Timeout)
		} else {
			attemptCtx, attemptCancel = context.WithCancel(ctx)
		}

		lastErr = node.BeforeAll(attemptCtx)
		attemptCancel()

		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i < attempts-1 {
			fmt.Fprintf(os.Stderr, "%s.BeforeAll attempt %d/%d failed: %v\n", node.Name, i+1, attempts, lastErr)
			if node.Config.RetryDelay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(node.Config.RetryDelay):
				}
			}
		}
	}

	fmt.Fprintf(os.Stderr, "FAIL: %s.BeforeAll failed after %d attempt(s): %v\n", node.Name, attempts, lastErr)
	return lastErr
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

	if tracker.isSucceeded(node) {
		if node.AfterAll != nil {
			ctx := context.Background()
			if node.Config.Timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, node.Config.Timeout)
				defer cancel()
			}
			if err := node.AfterAll(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "%s.AfterAll failed: %v\n", node.Name, err)
				anyFailed = true
			}
		}

		for _, sf := range node.SharedFixtures {
			if sf.Dehydrate != nil {
				if err := sf.Dehydrate(context.Background()); err != nil {
					fmt.Fprintf(os.Stderr, "%s: dehydrate failed: %v\n", node.Name, err)
					anyFailed = true
				}
			}
		}
	}

	return anyFailed
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
	os.WriteFile(path, []byte(budget.String()), 0644)
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
			return 0
		}
		visiting[name] = true
		node := byName[name]
		own := node.Config.Timeout
		if own < 0 {
			own = 0
		}
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
	own := node.Config.Timeout
	if own < 0 {
		own = 0
	}
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
		return nil, fmt.Errorf("GOTEST_SHARED_STATE_FILE not set — run via gotest CLI")
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

func anyNodeHasSharedFixtures(roots, fixtures []*FixtureNode) bool {
	for _, f := range fixtures {
		if len(f.SharedFixtures) > 0 || f.SharedState != nil {
			return true
		}
	}
	for _, root := range roots {
		if hasSharedFixtures(root) {
			return true
		}
	}
	return false
}

func anyNodeHasLegacySharedFixtures(roots, fixtures []*FixtureNode) bool {
	for _, f := range fixtures {
		if len(f.SharedFixtures) > 0 {
			return true
		}
	}
	for _, root := range roots {
		if hasSharedFixtures(root) {
			return true
		}
	}
	return false
}

func hasSharedFixtures(node *FixtureNode) bool {
	if len(node.SharedFixtures) > 0 {
		return true
	}
	for _, child := range node.Children {
		if hasSharedFixtures(child) {
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

