package teardownfailing

import (
	"context"
	"errors"
)

// ReleaseSharedFixture starts cleanly and refuses to release what it holds,
// so the run's only failure lands after its last suite verdict.
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
