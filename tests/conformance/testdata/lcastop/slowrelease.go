package lcastop

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowReleaseFixture takes 300ms to release, within its 1s Timeout.
type SlowReleaseFixture struct{}

func (f *SlowReleaseFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: time.Second}
}

func (f *SlowReleaseFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SlowReleaseFixture) AfterAll(ctx context.Context) error {
	mark("release began")
	time.Sleep(300 * time.Millisecond)
	mark("release ended")
	return nil
}
