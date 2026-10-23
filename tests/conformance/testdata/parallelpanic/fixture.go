// Package parallelpanic has a parallel suite whose one method panics while a
// sibling is still running; the markers record what ran after.
package parallelpanic

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// mark appends event to the markers file of the run's canary directory.
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

// TrackFixture records its own lifecycle.
type TrackFixture struct{}

func (f *TrackFixture) BeforeAll(ctx context.Context) error { mark("fixture up"); return nil }
func (f *TrackFixture) AfterAll(ctx context.Context) error  { mark("fixture down"); return nil }
