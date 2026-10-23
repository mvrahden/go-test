// Package fixacancelshared has a shared fixture, LaterSharedFixture, whose dependency holds
// in BeforeAll until its context ends and then comes up anyway. A run of it is
// stopped meanwhile, so LaterSharedFixture's setup ends before its BeforeAll runs.
package fixacancelshared

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

// HoldSharedFixture holds until its context ends, then reports itself up.
type HoldSharedFixture struct {
	State string
}

func (f *HoldSharedFixture) BeforeAll(ctx context.Context) error {
	mark("hold.BeforeAll")
	<-ctx.Done()
	return nil
}

func (f *HoldSharedFixture) AfterAll(ctx context.Context) error {
	mark("hold.AfterAll")
	return nil
}

// LaterSharedFixture sets up after HoldSharedFixture.
type LaterSharedFixture struct {
	Hold  *HoldSharedFixture
	State string
}

func (f *LaterSharedFixture) BeforeAll(ctx context.Context) error {
	mark("later.BeforeAll")
	return nil
}

func (f *LaterSharedFixture) AfterAll(ctx context.Context) error {
	mark("later.AfterAll")
	return nil
}
