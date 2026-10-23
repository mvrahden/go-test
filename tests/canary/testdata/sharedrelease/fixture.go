// Package sharedrelease has a shared fixture whose first attempt fails,
// whose release between attempts fails, and whose second attempt comes up.
package sharedrelease

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
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

// RetrySharedFixture comes up on its second attempt.
type RetrySharedFixture struct {
	State    string
	attempts int
}

func (f *RetrySharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Retries: 1}
}

func (f *RetrySharedFixture) BeforeAll(ctx context.Context) error {
	f.attempts++
	mark("retry.BeforeAll")
	if f.attempts == 1 {
		return errors.New("the first attempt failed")
	}
	f.State = "up"
	return nil
}

// AfterAll fails the release between the attempts, and only that one.
func (f *RetrySharedFixture) AfterAll(ctx context.Context) error {
	mark("retry.AfterAll")
	if f.attempts == 1 {
		return errors.New("the shared release leaked")
	}
	return nil
}
