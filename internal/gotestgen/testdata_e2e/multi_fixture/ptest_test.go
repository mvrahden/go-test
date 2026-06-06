package multifixture

import "github.com/mvrahden/go-test/pkg/gotest"

type MultiFixtureTestSuite struct {
	DB    *DatabaseFixture
	Cache *CacheFixture
}

func (s *MultiFixtureTestSuite) TestDBReady(t *gotest.T) {
	gotest.False(t, DatabaseTornDown.Load())
}

func (s *MultiFixtureTestSuite) TestCacheReady(t *gotest.T) {
	gotest.False(t, DatabaseTornDown.Load())
}

// ServiceTestSuite runs second — it must see fixtures still alive.
type ServiceTestSuite struct {
	Svc *ServiceFixture
}

func (s *ServiceTestSuite) TestServiceReady(t *gotest.T) {
	gotest.False(t, DatabaseTornDown.Load())
}
