package nestedfixture

import "github.com/mvrahden/go-test/pkg/gotest"

//go:test fixture
type InfraFixture struct {
	DBValue string
}

func (f *InfraFixture) BeforeAll(t *gotest.T) {
	f.DBValue = "db-ready"
}

func (f *InfraFixture) AfterAll(t *gotest.T) {}

//go:test fixture
type APIFixture struct {
	*InfraFixture
	APIValue string
}

func (f *APIFixture) BeforeAll(t *gotest.T) {
	f.APIValue = "api-ready"
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
