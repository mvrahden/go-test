package fixturepkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type AppFixture struct{}

func (f *AppFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{}
}

func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *AppFixture) AfterAll(ctx context.Context) error  { return nil }
func (f *AppFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *AppFixture) AfterEach(ctx context.Context) error  { return nil }
