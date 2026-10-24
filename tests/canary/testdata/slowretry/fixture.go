// Package slowretry has a shared fixture with a two-second Timeout and one
// retry: its first attempt, the release after it and its second attempt each
// take 1.6s, most of the Timeout.
package slowretry

import (
	"context"
	"errors"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

const step = 1600 * time.Millisecond

// SlowSharedFixture comes up on its second attempt.
type SlowSharedFixture struct {
	State    string
	attempts int
}

func (f *SlowSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second, Retries: 1}
}

func (f *SlowSharedFixture) BeforeAll(ctx context.Context) error {
	f.attempts++
	time.Sleep(step)
	if f.attempts == 1 {
		return errors.New("the first attempt failed")
	}
	f.State = "up"
	return nil
}

func (f *SlowSharedFixture) AfterAll(ctx context.Context) error {
	time.Sleep(step)
	return nil
}
