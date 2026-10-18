// Package benchpanic is a fixture-bound benchmark suite in which one benchmark
// panics; the run reads which hooks still ran and what the others reported.
package benchpanic

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

// HeldFixture marks when it is released.
type HeldFixture struct{}

func (f *HeldFixture) BeforeAll(ctx context.Context) error { mark("fixture.BeforeAll"); return nil }
func (f *HeldFixture) AfterAll(ctx context.Context) error  { mark("fixture.AfterAll"); return nil }
