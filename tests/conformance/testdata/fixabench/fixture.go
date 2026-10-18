// Package fixabench has a benchmark whose shared fixture declares a
// two-second Timeout and then hangs in BeforeAll, ignoring its context, until
// its process is told to shut down.
package fixabench

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
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
	hangUntilShutdown()
	f.State = "up"
	return nil
}

func (f *HangSharedFixture) AfterAll(ctx context.Context) error { return nil }

// hangUntilShutdown ignores the setup's context and returns once the process
// is signalled to shut down, after the run has given up on the fixture, so
// its teardown is not left to the force-kill.
func hangUntilShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)
	select {
	case <-stop:
	case <-time.After(10 * time.Minute):
	}
}
