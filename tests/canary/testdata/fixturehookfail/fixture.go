// Package fixturehookfail is a fixture-bound suite whose fixture's BeforeEach
// fails for the first method; the canary reads which hooks ran after it.
package fixturehookfail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var (
	markMu sync.Mutex
	calls  int
)

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

// FlakyFixture fails its first BeforeEach and marks every hook.
type FlakyFixture struct{}

func (f *FlakyFixture) BeforeAll(ctx context.Context) error { mark("fixture.BeforeAll"); return nil }
func (f *FlakyFixture) AfterAll(ctx context.Context) error  { mark("fixture.AfterAll"); return nil }

func (f *FlakyFixture) BeforeEach(ctx context.Context) error {
	mark("fixture.BeforeEach")
	markMu.Lock()
	calls++
	first := calls == 1
	markMu.Unlock()
	if first {
		return errors.New("the fixture is not ready")
	}
	return nil
}

func (f *FlakyFixture) AfterEach(ctx context.Context) error { mark("fixture.AfterEach"); return nil }
