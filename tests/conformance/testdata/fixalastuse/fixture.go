// Package fixalastuse has a shared fixture only a fast suite reads, beside a
// slow suite that reads none: the fixture's last use ends long before the run.
package fixalastuse

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

type ASharedFixture struct {
	State string
}

func (f *ASharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *ASharedFixture) AfterAll(ctx context.Context) error {
	mark("a.AfterAll")
	return nil
}
