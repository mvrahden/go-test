// Package lcsshared binds a shared fixture to suites that end a run early: a
// method that ignores the stop, one that waits for it, a FailFast suite and
// a suite that passes at once. LCS_TEARDOWN_SLEEP holds the shared fixture's
// AfterAll open for the duration it names.
package lcsshared

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// ProbeSharedFixture marks its setup, the process it lives in, and the
// start and end of its AfterAll.
type ProbeSharedFixture struct {
	State string
}

func (f *ProbeSharedFixture) BeforeAll(ctx context.Context) error {
	mark("shared.BeforeAll")
	mark(fmt.Sprintf("setup pid %d", os.Getpid()))
	f.State = "up"
	return nil
}

func (f *ProbeSharedFixture) AfterAll(ctx context.Context) error {
	mark("shared.AfterAll begins")
	if d, err := time.ParseDuration(os.Getenv("LCS_TEARDOWN_SLEEP")); err == nil {
		time.Sleep(d)
	}
	mark("shared.AfterAll ends")
	return nil
}

// HoldFixture is a package fixture: with it, a stopped method holds the suite
// process for its grace instead of letting it end at once.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *HoldFixture) AfterAll(ctx context.Context) error  { return nil }
