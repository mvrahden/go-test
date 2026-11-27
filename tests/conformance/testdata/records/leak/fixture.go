package leak

import (
	"context"
	"errors"
)

// LeakSharedFixture's teardown fails.
type LeakSharedFixture struct{ State string }

func (f *LeakSharedFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *LeakSharedFixture) AfterAll(ctx context.Context) error { return errors.New("teardown broke") }
