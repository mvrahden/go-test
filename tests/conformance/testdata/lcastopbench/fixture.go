// Package lcastopbench holds a benchmark suite and a fuzz suite, each bound to a
// fixture, whose first benchmark or seed runs until a run of them is stopped
// from outside. LCA_IGNORE=1 makes the first benchmark ignore the stop for
// a minute; otherwise it returns 2s after it began.
package lcastopbench

import (
	"context"
	"os"
	"path/filepath"
	"sync"
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

// HoldFixture records when it is released.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *HoldFixture) AfterAll(ctx context.Context) error {
	mark("fixture.AfterAll")
	return nil
}
