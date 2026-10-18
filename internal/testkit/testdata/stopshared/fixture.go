// Package stopshared has a suite that holds until the run is stopped and a
// shared fixture whose AfterAll takes 3s, so a run cut short at once leaves
// the fixture process behind.
package stopshared

import (
	"context"
	"fmt"
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

// SlowReleaseSharedFixture stamps its process's pid and takes 3s to release.
type SlowReleaseSharedFixture struct {
	State string
}

func (f *SlowReleaseSharedFixture) BeforeAll(ctx context.Context) error {
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	f.State = "up"
	return nil
}

func (f *SlowReleaseSharedFixture) AfterAll(ctx context.Context) error {
	time.Sleep(3 * time.Second)
	mark("fixture released")
	return nil
}
