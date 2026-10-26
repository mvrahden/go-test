// Package fixaslowrelease releases a bulk suite.s shared fixture slowly while
// the exclusive tail starts a fixture of its own and runs.
package fixaslowrelease

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

// SlowSharedFixture is the bulk suite's; its release takes 3s.
type SlowSharedFixture struct {
	State string
}

func (f *SlowSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *SlowSharedFixture) AfterAll(ctx context.Context) error {
	mark("slow.AfterAll begins")
	time.Sleep(3 * time.Second)
	mark("slow.AfterAll ends")
	return nil
}

// TailSharedFixture is the exclusive suite's.
type TailSharedFixture struct {
	State string
}

func (f *TailSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *TailSharedFixture) AfterAll(ctx context.Context) error { return nil }
