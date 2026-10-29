// Package fix holds a package fixture other packages bind.
package fix

import "context"

// HydratedFixture declares Hydrate, which only shared fixtures have.
type HydratedFixture struct{}

func (f *HydratedFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *HydratedFixture) Hydrate(ctx context.Context) error { return nil }
