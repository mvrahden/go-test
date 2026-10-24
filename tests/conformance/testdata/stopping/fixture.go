// Package stopping is a fixture-bound suite whose first method holds until
// its context ends; a run of it is stopped from outside.
package stopping

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

// HoldFixture records when it is up and when it is released.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error {
	mark("fixture.BeforeAll")
	return nil
}

func (f *HoldFixture) AfterAll(ctx context.Context) error {
	mark("fixture.AfterAll")
	return nil
}
