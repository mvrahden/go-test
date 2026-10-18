package guardedshared

import (
	"context"
	"errors"
)

// TrapSharedFixture never comes up.
type TrapSharedFixture struct {
	State string
}

func (f *TrapSharedFixture) BeforeAll(ctx context.Context) error {
	return errors.New("the trap would not come up")
}

func (f *TrapSharedFixture) AfterAll(ctx context.Context) error { return nil }
