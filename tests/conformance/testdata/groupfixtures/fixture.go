// Package groupfixtures has a suite in the default group, two in the e2e
// group, one of which reads a shared fixture only that group needs, and one
// in the load group reading a fixture of its own.
package groupfixtures

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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

// StackSharedFixture is the e2e group's.
type StackSharedFixture struct {
	State string
}

func (f *StackSharedFixture) BeforeAll(ctx context.Context) error {
	mark("setup pid " + strconv.Itoa(os.Getpid()))
	mark("stack.BeforeAll")
	f.State = "up"
	return nil
}

func (f *StackSharedFixture) AfterAll(ctx context.Context) error {
	mark("stack.AfterAll")
	return nil
}

// LoadSharedFixture is the load group's.
type LoadSharedFixture struct {
	State string
}

func (f *LoadSharedFixture) BeforeAll(ctx context.Context) error {
	mark("load.BeforeAll")
	f.State = "up"
	return nil
}

func (f *LoadSharedFixture) AfterAll(ctx context.Context) error {
	mark("load.AfterAll")
	return nil
}
