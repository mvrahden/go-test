// Package fixtures holds the shared fixture two suites read, one of whose
// packages never links.
package fixtures

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// Mark appends one line to the markers file the run under test reads back.
func Mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

type ASharedFixture struct {
	State string
}

func (f *ASharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *ASharedFixture) AfterAll(ctx context.Context) error {
	Mark("a.AfterAll")
	return nil
}
