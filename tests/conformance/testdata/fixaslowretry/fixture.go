// Package fixaslowretry has a shared fixture with a two-second Timeout and
// one retry: its first attempt, the release after it and its second attempt
// each take 1.6s, most of the Timeout.
package fixaslowretry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

const step = 1600 * time.Millisecond

// SlowSharedFixture comes up on its second attempt.
type SlowSharedFixture struct {
	State    string
	attempts int
}

func (f *SlowSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second, Retries: 1}
}

func (f *SlowSharedFixture) BeforeAll(ctx context.Context) error {
	f.attempts++
	mark("slow.BeforeAll")
	time.Sleep(step)
	if f.attempts == 1 {
		return errors.New("the first attempt failed")
	}
	f.State = "up"
	return nil
}

func (f *SlowSharedFixture) AfterAll(ctx context.Context) error {
	mark("slow.AfterAll")
	time.Sleep(step)
	return nil
}
