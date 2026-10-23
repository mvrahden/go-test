// Package fzsharedfail has a fuzz-only suite whose shared fixture never comes
// up.
package fzsharedfail

import (
	"context"
	"errors"
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

// DownSharedFixture never comes up.
type DownSharedFixture struct {
	State string
}

func (f *DownSharedFixture) BeforeAll(ctx context.Context) error {
	return errors.New("the service is down")
}

func (f *DownSharedFixture) AfterAll(ctx context.Context) error { return nil }
