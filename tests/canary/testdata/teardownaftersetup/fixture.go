package teardownaftersetup

import (
	"context"
	"errors"
)

// DownSharedFixture never comes up, so the suite that reads it is given up.
type DownSharedFixture struct {
	State string
}

func (f *DownSharedFixture) BeforeAll(ctx context.Context) error {
	return errors.New("the fixture would not come up")
}

func (f *DownSharedFixture) AfterAll(ctx context.Context) error { return nil }

// ReleaseSharedFixture comes up and then refuses to release what it holds.
// Its failure happens in the same run as its sibling's setup failure; the
// stream must carry both, since both left something behind.
type ReleaseSharedFixture struct {
	State string
}

func (f *ReleaseSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *ReleaseSharedFixture) AfterAll(ctx context.Context) error {
	return errors.New("the fixture would not release what it holds")
}
