// Package fixaonce has a shared fixture read by a concurrent suite and by an
// exclusive one, which runs after it.
package fixaonce

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

type OnceSharedFixture struct {
	State string
}

func (f *OnceSharedFixture) BeforeAll(ctx context.Context) error {
	mark("once.BeforeAll")
	f.State = "up"
	return nil
}

func (f *OnceSharedFixture) AfterAll(ctx context.Context) error {
	mark("once.AfterAll")
	return nil
}
