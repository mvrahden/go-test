// Package lastattempt has a shared fixture that fails both of its attempts,
// bound by a suite that would pass without it.
package lastattempt

import (
	"context"
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DownSharedFixture never comes up.
type DownSharedFixture struct {
	State string
}

func (f *DownSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Retries: 1}
}

func (f *DownSharedFixture) BeforeAll(ctx context.Context) error {
	return errors.New("the fixture would not come up")
}

func (f *DownSharedFixture) AfterAll(ctx context.Context) error { return nil }
