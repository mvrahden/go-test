package orphantags

import "github.com/mvrahden/go-test/pkg/gotest"

// dbChecks is called only from a file behind a build tag.
type dbChecks struct{}

func (c *dbChecks) TestRoundTrip(t *gotest.T) { gotest.True(t, true) }

// StoreContractTestSuite is instantiated only in a file behind a build tag.
type StoreContractTestSuite[S any] struct{}

func (s *StoreContractTestSuite[S]) TestPut(t *gotest.T) {}

type UnitTestSuite struct{}

func (s *UnitTestSuite) TestA(t *gotest.T) {}
