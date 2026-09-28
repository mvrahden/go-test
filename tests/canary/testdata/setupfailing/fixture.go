package setupfailing

import (
	"context"
	"errors"
)

// DownSharedFixture never comes up, so the run fails before the suite that
// reads it gets a verdict of its own.
type DownSharedFixture struct {
	State string
}

func (f *DownSharedFixture) BeforeAll(ctx context.Context) error {
	return errors.New("the fixture would not come up")
}

func (f *DownSharedFixture) AfterAll(ctx context.Context) error { return nil }
