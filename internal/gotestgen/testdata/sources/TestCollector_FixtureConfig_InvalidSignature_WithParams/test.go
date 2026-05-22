package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BadFixture struct{}

func (f *BadFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadFixture) FixtureConfig(x int) gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}
