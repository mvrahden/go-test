package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}
