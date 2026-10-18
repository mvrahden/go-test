// Package lcagoroutines starts goroutines with gotest.Go in BeforeAll and
// records when each wait and each fixture release happens.
package lcagoroutines

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

// FailFixture is released after FailLastTestSuite's waits.
type FailFixture struct{}

func (f *FailFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *FailFixture) AfterAll(ctx context.Context) error {
	mark("fail: fixture.AfterAll")
	return nil
}

// PanicFixture is released after PanicLastTestSuite's waits.
type PanicFixture struct{}

func (f *PanicFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PanicFixture) AfterAll(ctx context.Context) error {
	mark("panic: fixture.AfterAll")
	return nil
}
