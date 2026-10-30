// Package sharedteardownsibling binds two shared fixtures: one whose setup
// fails and a sibling that came up and whose AfterAll fails; see the fixtures
// conformance suite.
package sharedteardownsibling

import (
	"context"
	"errors"
)

// DownSharedFixture never comes up.
type DownSharedFixture struct {
	State string
}

func (f *DownSharedFixture) BeforeAll(ctx context.Context) error {
	return errors.New("the fixture would not come up")
}

// StuckSharedFixture comes up and cannot release what it holds.
type StuckSharedFixture struct {
	State string
}

func (f *StuckSharedFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *StuckSharedFixture) AfterAll(ctx context.Context) error {
	return errors.New("the volume is still attached")
}
