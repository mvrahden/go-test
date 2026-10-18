package fixtures

import (
	"context"
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowSetupSharedFixture declares its own setup Timeout and takes as long as
// it is told to come up, ignoring its context the way a blocking SDK call
// does. No suite references it — the runner's own tests drive it directly.
//
// EnvSlowSetupTimeout is the declared Timeout (no retries), EnvSlowSetupDelay
// how long BeforeAll actually takes.
type SlowSetupSharedFixture struct {
	Marker string
}

const (
	EnvSlowSetupTimeout = "GOTEST_TEST_SLOW_SETUP_TIMEOUT"
	EnvSlowSetupDelay   = "GOTEST_TEST_SLOW_SETUP_DELAY"
)

func (f *SlowSetupSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Retries = 0
	if d, err := time.ParseDuration(os.Getenv(EnvSlowSetupTimeout)); err == nil && d > 0 {
		cfg.Timeout = d
	}
	return cfg
}

func (f *SlowSetupSharedFixture) BeforeAll(ctx context.Context) error {
	if d, err := time.ParseDuration(os.Getenv(EnvSlowSetupDelay)); err == nil && d > 0 {
		time.Sleep(d)
	}
	f.Marker = "up"
	return nil
}

func (f *SlowSetupSharedFixture) AfterAll(ctx context.Context) error { return nil }
