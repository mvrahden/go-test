// Package fixtureafterfail is a fixture-bound suite whose fixture's AfterEach
// returns an error; the canary reads where the failure is reported.
package fixtureafterfail

import (
	"context"
	"errors"
)

// LeakyFixture fails every AfterEach.
type LeakyFixture struct{}

func (f *LeakyFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *LeakyFixture) AfterEach(ctx context.Context) error {
	return errors.New("the fixture did not let go")
}
