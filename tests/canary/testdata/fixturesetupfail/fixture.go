// Package fixturesetupfail is a fixture-bound suite whose fixture fails to
// come up; the canary reads whether it was still released.
package fixturesetupfail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// mark appends one line to the markers file the canary reads back.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// HalfFixture reaches part of its setup, then fails.
type HalfFixture struct{}

func (f *HalfFixture) BeforeAll(ctx context.Context) error {
	mark("fixture.BeforeAll")
	return errors.New("the second half never came up")
}

func (f *HalfFixture) AfterAll(ctx context.Context) error {
	mark("fixture.AfterAll")
	return nil
}
