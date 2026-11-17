// Package aftereachdefault binds a fixture whose AfterEach returns its
// context's error to a suite that declares no Timeout and whose method runs
// past the default deadline (scaled down by the run's GOTEST_TIMEOUT_SCALE).
package aftereachdefault

import "context"

// ReleaseFixture's AfterEach stands for a release that honours its context,
// such as a TRUNCATE through a pool.
type ReleaseFixture struct{}

func (f *ReleaseFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *ReleaseFixture) AfterEach(ctx context.Context) error { return ctx.Err() }
