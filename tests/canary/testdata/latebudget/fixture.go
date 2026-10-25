// Package latebudget has a shared fixture started late, for the exclusive
// tail, that declares a 2s Timeout and whose AfterAll never returns, ignoring
// its context and the process's shutdown.
package latebudget

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LateSharedFixture comes up at once and never finishes releasing.
type LateSharedFixture struct {
	State string
}

func (f *LateSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}

func (f *LateSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *LateSharedFixture) AfterAll(ctx context.Context) error {
	time.Sleep(10 * time.Minute)
	return nil
}
