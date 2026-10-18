// Package fixatail has an exclusive suite whose shared fixture starts at the
// barrier after the bulk suites and hangs there, ignoring its context. It
// declares a two-second Timeout, or none with FIXA_UNBOUNDED=1.
package fixatail

import (
	"context"
	"fmt"
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

// HangSharedFixture never comes up.
type HangSharedFixture struct {
	State string
}

func (f *HangSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	if os.Getenv("FIXA_UNBOUNDED") == "1" {
		return gotest.FixtureConfig{Timeout: gotest.NoDeadline}
	}
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}

func (f *HangSharedFixture) BeforeAll(ctx context.Context) error {
	mark("hang.BeforeAll")
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	time.Sleep(10 * time.Minute)
	f.State = "up"
	return nil
}

func (f *HangSharedFixture) AfterAll(ctx context.Context) error { return nil }
