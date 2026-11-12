//go:build orphantagsint

package orphantags

import "github.com/mvrahden/go-test/pkg/gotest"

type IntegrationTestSuite struct{ c dbChecks }

func (s *IntegrationTestSuite) TestDB(t *gotest.T) { s.c.TestRoundTrip(t) }

type pg struct{}

type PgStoreTestSuite = StoreContractTestSuite[pg]
