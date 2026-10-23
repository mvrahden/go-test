// Package failingshared binds a shared fixture that never comes up, and whose
// AfterAll must still run; see the canary suite.
package failingshared

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// LeakSharedFixture reserves something, then fails to come up.
type LeakSharedFixture struct {
	State string
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
