// Package elsewhere declares fixture hooks outside the package of the
// suites that bind them.
package elsewhere

import (
	"context"
	"errors"
)

// RemoteFixture is a package fixture a suite in another package binds.
type RemoteFixture struct{}

func (f *RemoteFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *RemoteFixture) BeforeEach(ctx context.Context) error {
	return errors.New("the remote fixture is not ready")
}

// Hooks lends its AfterEach to the fixture that embeds it.
type Hooks struct{}

func (h *Hooks) AfterEach(ctx context.Context) error {
	return errors.New("the embedded hook did not let go")
}
