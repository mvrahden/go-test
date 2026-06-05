package gotestruntime

import (
	"context"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedStateNode describes a shared fixture node in the DAG.
// The DAG executor handles these differently: unmarshal + hydrate instead of init + beforeAll.
type SharedStateNode struct {
	StateKey  string
	Target    any
	Hydrate   func(ctx context.Context) error
	Dehydrate func(ctx context.Context) error
}

// FixtureNode describes one fixture in the dependency graph.
// Generated code populates this as a struct literal.
type FixtureNode struct {
	Name           string
	Config         gotest.FixtureConfig
	Init           func()
	BeforeAll      func(ctx context.Context) error
	AfterAll       func(ctx context.Context) error
	SharedFixtures []SharedFixtureBinding  // deprecated: use SharedStateNode as DAG node
	SharedState    *SharedStateNode        // non-nil for shared fixture nodes
	Children       []*FixtureNode          // deprecated: use DependsOn with MainConfig.Fixtures
	DependsOn      []string
}

// SharedFixtureBinding describes how to deserialize and hydrate a shared
// fixture, then assign it to the parent fixture struct.
type SharedFixtureBinding struct {
	StateKey  string
	Target    any
	Hydrate   func(ctx context.Context) error
	Dehydrate func(ctx context.Context) error
	Assign    func()
}

type MainConfig struct {
	Roots                []*FixtureNode // deprecated: use Fixtures
	Fixtures             []*FixtureNode
	MaxSuiteSetupTimeout time.Duration
}

// FixtureDAG holds the result of SetupFixtureDAG for later teardown.
type FixtureDAG struct {
	cfg     MainConfig
	tracker *nodeTracker
	torn    sync.Once
	failed  bool
}
