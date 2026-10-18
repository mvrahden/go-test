// Package fixbsibling binds two shared fixtures: one whose setup overruns
// its 1s Timeout and is given up, and a sibling that came up and takes 8s to
// release.
package fixbsibling

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
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

// HungSharedFixture ignores its context and returns 4s in, past its 1s
// Timeout.
type HungSharedFixture struct {
	State string
}

func (f *HungSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Timeout = time.Second
	return cfg
}

func (f *HungSharedFixture) BeforeAll(ctx context.Context) error {
	time.Sleep(4 * time.Second)
	mark("hung returned")
	return nil
}

func (f *HungSharedFixture) AfterAll(ctx context.Context) error { return nil }

// HealthySharedFixture comes up at once and takes 8s to release.
type HealthySharedFixture struct {
	State string
}

func (f *HealthySharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Timeout = 2 * time.Second
	return cfg
}

func (f *HealthySharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	mark("healthy up")
	return nil
}

func (f *HealthySharedFixture) AfterAll(ctx context.Context) error {
	time.Sleep(8 * time.Second)
	mark("healthy released")
	return nil
}
