// Package failingshared binds a shared fixture that never comes up in its two
// attempts, and whose AfterAll must run after each; see the fixtures
// conformance suite.
package failingshared

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LeakSharedFixture reserves something, then fails to come up, twice.
type LeakSharedFixture struct {
	State string
}

func (f *LeakSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Retries: 1}
}

func (f *LeakSharedFixture) BeforeAll(ctx context.Context) error {
	mark("leak.BeforeAll")
	return errors.New("the leak would not come up")
}

func (f *LeakSharedFixture) AfterAll(ctx context.Context) error {
	mark("leak.AfterAll")
	return nil
}

func mark(name string) {
	w, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = w.WriteString(name + "\n")
		_ = w.Close()
	}
}
