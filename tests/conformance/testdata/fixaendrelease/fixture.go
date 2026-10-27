// Package fixaendrelease ends its run while one shared fixture's release still
// runs and another is still held.
package fixaendrelease

import (
	"context"
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

// AlphaSharedFixture is the quick suite's; its release takes 3s.
type AlphaSharedFixture struct {
	State string
}

func (f *AlphaSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *AlphaSharedFixture) AfterAll(ctx context.Context) error {
	mark("a.AfterAll begins")
	time.Sleep(3 * time.Second)
	mark("a.AfterAll ends")
	return nil
}

// BetaSharedFixture is the slower suite's; its release takes 3s too.
type BetaSharedFixture struct {
	State string
}

func (f *BetaSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *BetaSharedFixture) AfterAll(ctx context.Context) error {
	mark("b.AfterAll begins")
	time.Sleep(3 * time.Second)
	mark("b.AfterAll ends")
	return nil
}
