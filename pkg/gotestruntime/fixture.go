package gotestruntime

import (
	"context"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureNode describes one fixture in the tree.
// Generated code populates this as a struct literal.
type FixtureNode struct {
	Name           string
	Config         gotest.FixtureConfig
	Init           func()
	BeforeAll      func(ctx context.Context) error
	AfterAll       func(ctx context.Context) error
	SharedFixtures []SharedFixtureBinding
	Children       []*FixtureNode
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

// MainConfig holds the configuration for RunFixtureMain.
type MainConfig struct {
	Roots                []*FixtureNode
	MaxSuiteSetupTimeout time.Duration
}

// RunFixtureMain replaces the generated ƒƒ_GOTEST_main function.
// It orchestrates fixture tree setup, m.Run(), and teardown.
func RunFixtureMain(m *testing.M, cfg MainConfig) int {
	return run(m.Run, cfg)
}
