// Package relay has suites whose output the runner relays while something
// goes wrong beside it: a stop the channel cannot carry, a converter that
// dies, a signal from outside, a process that crashes under its method and
// an -exec wrapper that exits before the suite does.
package relay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// mark appends one line to the markers file the canary reads back.
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

// HoldFixture records its setup, its release and the process it lives in.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error {
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	return nil
}

func (f *HoldFixture) AfterAll(ctx context.Context) error {
	mark("fixture.AfterAll")
	return nil
}
