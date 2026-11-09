// Package stopatopen holds its TestMain before the tests run until the test
// driving it says go: a stop sent meanwhile waits for the first suite.
package stopatopen

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

// OpenFixture marks its setup, which a stopped run never reaches.
type OpenFixture struct{}

func (f *OpenFixture) BeforeAll(ctx context.Context) error {
	mark("fixture.BeforeAll")
	return nil
}

func (f *OpenFixture) AfterAll(ctx context.Context) error { return nil }
