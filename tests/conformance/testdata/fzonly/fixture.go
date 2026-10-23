// Package fzonly has a suite with fuzz methods and no test method, so it has
// no Test function for -run or -skip to name.
package fzonly

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

// MarkSharedFixture marks its setup; the runner starts it only for a suite
// the run dispatches.
type MarkSharedFixture struct {
	State string
}

func (f *MarkSharedFixture) BeforeAll(ctx context.Context) error {
	mark("shared up")
	f.State = "up"
	return nil
}

func (f *MarkSharedFixture) AfterAll(ctx context.Context) error { return nil }
