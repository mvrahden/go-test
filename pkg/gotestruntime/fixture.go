package gotestruntime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/callsite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedStateNode describes a shared fixture node in the DAG.
// The DAG executor handles these differently: unmarshal + hydrate instead of init + beforeAll.
type SharedStateNode struct {
	StateKey  string
	Target    any
	Hydrate   func(ctx context.Context) error
	Dehydrate func(ctx context.Context) error
	// HydrateAt and DehydrateAt locate the hooks as FixtureNode.BeforeAllAt
	// does.
	HydrateAt   string
	DehydrateAt string
}

// FixtureNode describes one fixture in the dependency graph.
// Generated code populates this as a struct literal.
type FixtureNode struct {
	Name string
	// Config is the fixture's literal configuration: its own FixtureConfig()
	// verbatim, or the defaults when it has no marker method. Timeout bounds
	// the context each lifecycle call receives.
	Config gotest.FixtureConfig
	// Budget is the deadline BeforeAll and AfterAll are each held to by
	// verdict, or zero when the fixture declared no config of its own. A
	// fixture is never failed against a number its author did not write.
	Budget      time.Duration
	Init        func()
	BeforeAll   func(ctx context.Context) error
	AfterAll    func(ctx context.Context) error
	SharedState *SharedStateNode // non-nil for shared fixture nodes
	DependsOn   []string
	// BeforeAllAt and AfterAllAt locate the hooks (file:line, the file
	// relative to the package directory); a failure is reported there. Empty,
	// as from a harness generated before them, reports it unlocated.
	BeforeAllAt string
	AfterAllAt  string
}

type MainConfig struct {
	Fixtures             []*FixtureNode
	MaxSuiteSetupTimeout time.Duration
}

// FixtureDAG holds the result of SetupFixtureDAG for later teardown.
type FixtureDAG struct {
	cfg     MainConfig
	tracker *nodeTracker
	torn    sync.Once
	failed  bool
	// failures are the hooks the teardown saw fail, in the fixtures' order.
	failures []*hookFailure
}

// hookFailure is a fixture hook that failed: the fixture, the hook, where
// it is declared (file:line, or empty) and its error, whose text it keeps.
type hookFailure struct {
	fixture, hook, at string
	err               error
}

func (f *hookFailure) Error() string { return f.err.Error() }
func (f *hookFailure) Unwrap() error { return f.err }

// where is where the hook is declared, in testing's form, or empty.
func (f *hookFailure) where() string { return callsite.Declared(f.at) }

// failureAt is where the hook that failed in err is declared, in testing's
// form, or empty when err names no hook or its hook no place.
func failureAt(err error) string {
	var f *hookFailure
	if !errors.As(err, &f) {
		return ""
	}
	return f.where()
}
