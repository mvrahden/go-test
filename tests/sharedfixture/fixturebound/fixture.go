package fixturebound

import (
	"context"

	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

type InfraFixture struct {
	Alpha *fixtures.AlphaSharedFixture
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *InfraFixture) AfterAll(ctx context.Context) error  { return nil }
