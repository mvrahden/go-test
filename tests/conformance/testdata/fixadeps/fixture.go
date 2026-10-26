// Package fixadeps binds a shared fixture directly in a fast suite and
// through a dependent in a slower one.
package fixadeps

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


type ParentSharedFixture struct {
	State string
}

func (f *ParentSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *ParentSharedFixture) AfterAll(ctx context.Context) error {
	mark("parent.AfterAll")
	return nil
}

// ChildSharedFixture depends on Parent.
type ChildSharedFixture struct {
	Parent *ParentSharedFixture
	State  string
}

func (f *ChildSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = f.Parent.State
	return nil
}

func (f *ChildSharedFixture) AfterAll(ctx context.Context) error {
	mark("child.AfterAll")
	return nil
}
