// Package fixareleaseslow has a package fixture whose release between its two
// setup attempts outlives the fixture's declared Timeout.
package fixareleaseslow

import (
	"context"
	"errors"
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

// SlowReleaseFixture comes up on its second attempt.
type SlowReleaseFixture struct {
	attempts int
}

func (f *SlowReleaseFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 200 * time.Millisecond, Retries: 1}
}

func (f *SlowReleaseFixture) BeforeAll(ctx context.Context) error {
	f.attempts++
	mark("slow.BeforeAll")
	if f.attempts == 1 {
		return errors.New("the first attempt failed")
	}
	return nil
}

// AfterAll ignores its context between the attempts, and only there.
func (f *SlowReleaseFixture) AfterAll(ctx context.Context) error {
	mark("slow.AfterAll")
	if f.attempts == 1 {
		time.Sleep(time.Second)
	}
	return nil
}
