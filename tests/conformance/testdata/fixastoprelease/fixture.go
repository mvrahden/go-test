// Package fixastoprelease is stopped while one shared fixture's release runs
// and another is still held.
package fixastoprelease

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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

// SlowSharedFixture is the quick suite's; its release takes 2s.
type SlowSharedFixture struct {
	State string
}

func (f *SlowSharedFixture) BeforeAll(ctx context.Context) error {
	mark("setup pid " + strconv.Itoa(os.Getpid()))
	f.State = "up"
	return nil
}

func (f *SlowSharedFixture) AfterAll(ctx context.Context) error {
	mark("slow.AfterAll begins")
	time.Sleep(2 * time.Second)
	mark("slow.AfterAll ends")
	return nil
}

// HeldSharedFixture is the long suite's, held when the run is stopped.
type HeldSharedFixture struct {
	State string
}

func (f *HeldSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *HeldSharedFixture) AfterAll(ctx context.Context) error {
	mark("held.AfterAll")
	return nil
}
