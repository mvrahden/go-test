package fixtures

import (
	"context"
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BudgetedTeardownSharedFixture declares its own SharedFixtureConfig — the only
// fixture in this package that does — so the generated subprocess derives a
// config and holds AfterAll to a declared budget by verdict. Its AfterAll
// deliberately ignores its context, the way a teardown wrapping a blocking SDK
// call does. No suite references it — the runner's own tests drive it directly.
//
// It is configured through the environment because the runner starts the setup
// program as a subprocess: EnvBudgetedTeardownTimeout is the declared Timeout,
// EnvBudgetedTeardownDelay how long AfterAll actually takes.
type BudgetedTeardownSharedFixture struct {
	Marker string
}

const (
	EnvBudgetedTeardownTimeout = "GOTEST_TEST_BUDGETED_TEARDOWN_TIMEOUT"
	EnvBudgetedTeardownDelay   = "GOTEST_TEST_BUDGETED_TEARDOWN_DELAY"
)

func (f *BudgetedTeardownSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	if d, err := time.ParseDuration(os.Getenv(EnvBudgetedTeardownTimeout)); err == nil && d > 0 {
		cfg.Timeout = d
	}
	return cfg
}

func (f *BudgetedTeardownSharedFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *BudgetedTeardownSharedFixture) AfterAll(ctx context.Context) error {
	if d, err := time.ParseDuration(os.Getenv(EnvBudgetedTeardownDelay)); err == nil && d > 0 {
		time.Sleep(d)
	}
	return nil
}
