package setupbudget

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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

// BeforeAll marks when it began, in Unix nanoseconds, so the wait is timed
// from the setup's start, not from a build before it.
func (f *SlowSharedFixture) BeforeAll(ctx context.Context) error {
	mark("began=" + strconv.FormatInt(time.Now().UnixNano(), 10))
	time.Sleep(15 * time.Second)
	f.State = "late"
	return nil
}

func (f *SlowSharedFixture) AfterAll(ctx context.Context) error { return nil }

func mark(event string) {
	file, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(event + "\n")
}
