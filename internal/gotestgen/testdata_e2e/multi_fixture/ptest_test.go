package multifixture

import "github.com/mvrahden/go-test/pkg/gotest"

type MultiFixtureTestSuite struct {
	DB    *DatabaseFixture
	Cache *CacheFixture
}

func (s *MultiFixtureTestSuite) TestDBReady(t *gotest.T)    { DoWork() }
func (s *MultiFixtureTestSuite) TestCacheReady(t *gotest.T) { DoWork() }

type ServiceTestSuite struct {
	Svc *ServiceFixture
}

func (s *ServiceTestSuite) TestServiceReady(t *gotest.T) { DoWork() }
