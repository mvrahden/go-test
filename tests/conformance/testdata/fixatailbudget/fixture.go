// Package fixatailbudget has a shared fixture started late, for the exclusive tail,
// that declares a two-second Timeout and whose AfterAll never returns,
// ignoring its context and the process's shutdown.
package fixatailbudget

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

// LateSharedFixture comes up at once and never finishes releasing.
type LateSharedFixture struct {
	State string
}

func (f *LateSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}

func (f *LateSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *LateSharedFixture) AfterAll(ctx context.Context) error {
	mark("late.AfterAll")
	time.Sleep(10 * time.Minute)
	return nil
}
