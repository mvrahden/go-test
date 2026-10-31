// Package fix holds a package fixture other packages bind.
package fix

import "context"

// ConfiguredFixture declares a FixtureConfig that returns no FixtureConfig.
type ConfiguredFixture struct{}

func (f *ConfiguredFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *ConfiguredFixture) FixtureConfig() int { return 0 }
