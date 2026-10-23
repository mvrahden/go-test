// Package fzshared has a fuzz-only suite that binds a shared fixture, for
// gotest fuzz and gotest fuzz triage to run.
package fzshared

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// mark appends event to the markers file of the run's canary directory.
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

// StateSharedFixture hands its state to the processes that hydrate it.
type StateSharedFixture struct {
	State string
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	mark("shared up")
	f.State = "up"
	return nil
}

func (f *StateSharedFixture) AfterAll(ctx context.Context) error {
	mark("shared down")
	return nil
}
