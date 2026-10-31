// Package fix holds a package fixture other packages bind.
package fix

import "context"

// BadFixture declares a BeforeEach without the error result fixtures need.
type BadFixture struct{ Ready bool }

func (f *BadFixture) BeforeAll(ctx context.Context) error { f.Ready = true; return nil }

func (f *BadFixture) BeforeEach(ctx context.Context) {}
