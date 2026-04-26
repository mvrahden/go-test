package nestedfixture

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type InfraFixture struct {
	DBValue string
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error {
	f.DBValue = "db-ready"
	return nil
}

func (f *InfraFixture) AfterAll(ctx context.Context) error { return nil }

type APIFixture struct {
	*InfraFixture
	APIValue string
}

func (f *APIFixture) BeforeAll(ctx context.Context) error {
	f.APIValue = "api-ready"
	return nil
}

type LightTestSuite struct {
	*InfraFixture
}

func (s *LightTestSuite) TestDBAccess(t *gotest.T) {}

type FullTestSuite struct {
	*APIFixture
}

func (s *FullTestSuite) TestAPIAccess(t *gotest.T) {}
func (s *FullTestSuite) TestAPIHealth(t *gotest.T) {}
