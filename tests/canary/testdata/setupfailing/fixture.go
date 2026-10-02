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

// UpSharedFixture comes up. Only the exclusive suite reads it, so it starts
// once the other suites are done, after DownSharedFixture has failed.
type UpSharedFixture struct {
	State string
}

func (f *UpSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *UpSharedFixture) AfterAll(ctx context.Context) error { return nil }
