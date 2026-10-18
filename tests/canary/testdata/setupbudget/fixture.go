package setupbudget

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowSharedFixture declares a 2s Timeout with no retries and takes 15s,
// ignoring its context: the run must give up on it at its declared budget.
type SlowSharedFixture struct {
	State string
}

func (f *SlowSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Timeout = 2 * time.Second
	cfg.Retries = 0
	return cfg
}

func (f *SlowSharedFixture) BeforeAll(ctx context.Context) error {
	time.Sleep(15 * time.Second)
	f.State = "late"
	return nil
}

func (f *SlowSharedFixture) AfterAll(ctx context.Context) error { return nil }
