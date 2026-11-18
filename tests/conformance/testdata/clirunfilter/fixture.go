// Package clirunfilter holds two suites a -run first level can match at once,
// one with a fuzz method, and a suite that binds a shared fixture, so a run
// shows which process ran what.
package clirunfilter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// mark appends one line, naming the process, to the markers file the run
// under test reads back.
func mark(name string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(fmt.Sprintf("%s pid=%d\n", name, os.Getpid()))
}

// StateSharedFixture hands its state to the suite that binds it.
type StateSharedFixture struct {
	Value string
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	f.Value = "up"
	return nil
}

func (f *StateSharedFixture) AfterAll(ctx context.Context) error { return nil }
