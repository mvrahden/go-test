// Package fixabench has a benchmark whose shared fixture declares a
// two-second Timeout and then hangs in BeforeAll, ignoring its context.
package fixabench

import (
	"context"
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

// HangSharedFixture never comes up within the budget it declares.
type HangSharedFixture struct {
	State string
}

func (f *HangSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}

func (f *HangSharedFixture) BeforeAll(ctx context.Context) error {
	mark("hang.BeforeAll")
	time.Sleep(10 * time.Minute)
	f.State = "up"
	return nil
}

func (f *HangSharedFixture) AfterAll(ctx context.Context) error { return nil }
