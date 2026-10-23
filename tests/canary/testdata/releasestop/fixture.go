// Package releasestop has a shared fixture whose first attempt fails and
// whose release between attempts is still running when the run is stopped.
package releasestop

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var markMu sync.Mutex

func markersPath() string { return filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers") }

// mark appends one line to the markers file the canary reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(markersPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// marked reports whether event is in the markers file.
func marked(event string) bool {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.Open(markersPath())
	if err != nil {
		return false
	}
	defer f.Close()
	var lines []string
	for sc := bufio.NewScanner(f); sc.Scan(); {
		lines = append(lines, sc.Text())
	}
	return slices.Contains(lines, event)
}

// WatchSharedFixture marks when the stop reaches the fixture process.
type WatchSharedFixture struct {
	State string
}

func (f *WatchSharedFixture) BeforeAll(ctx context.Context) error {
	<-ctx.Done()
	mark("watch.stopped")
	return ctx.Err()
}

func (f *WatchSharedFixture) AfterAll(ctx context.Context) error { return nil }

// RetrySharedFixture fails its first attempt; its release between attempts
// outlasts the stop.
type RetrySharedFixture struct {
	State    string
	attempts int
}

func (f *RetrySharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Retries: 2}
}

func (f *RetrySharedFixture) BeforeAll(ctx context.Context) error {
	f.attempts++
	mark("retry.BeforeAll")
	return errors.New("not ready yet")
}

// AfterAll waits for the stop to reach the process, then says whether its own
// context saw it.
func (f *RetrySharedFixture) AfterAll(ctx context.Context) error {
	mark("retry.AfterAll")
	for deadline := time.Now().Add(time.Minute); !marked("watch.stopped") && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if ctx.Err() != nil {
		mark("retry.AfterAll cut short")
	} else {
		mark("retry.AfterAll finished")
	}
	return nil
}
