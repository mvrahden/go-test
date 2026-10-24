// Package stopping is a fixture-bound suite whose first method holds until
// its context ends; a run of it is stopped from outside.
package stopping

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

// HoldFixture records when it is up and when it is released, and the process
// it lives in.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error {
	mark("fixture.BeforeAll")
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	return nil
}

// AfterAll marks its start and its end; STOPPING_SLOW holds it open in
// between for the duration it names, long enough for a second signal to land
// while it runs and for a teardown nobody waited for to show as one.
func (f *HoldFixture) AfterAll(ctx context.Context) error {
	mark("fixture.AfterAll")
	if d, err := time.ParseDuration(os.Getenv("STOPPING_SLOW")); err == nil {
		time.Sleep(d)
	}
	mark("teardown done")
	return nil
}
