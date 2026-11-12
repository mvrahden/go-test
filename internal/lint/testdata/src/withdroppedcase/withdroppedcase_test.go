package withdroppedcase

import "github.com/mvrahden/go-test/pkg/gotest"

// OrderTests is no suite: nothing runs its case.
type OrderTests struct{}

func (o *OrderTests) TestPlaced(t *gotest.T) {} // want `TestPlaced on OrderTests never runs: OrderTests is no suite`

// probe is handed to an interface that holds its case: the generator counts
// that as reaching it, wherever the interface is called.
type probe struct{}

func (p *probe) TestProbes(t *gotest.T) {}

type prober interface{ TestProbes(t *gotest.T) }

var _ prober = (*probe)(nil)

// helper's case is called by the suite below.
type helper struct{}

func (h helper) TestReady(t *gotest.T) {}

// Named has a method that only begins like a case.
type Named struct{}

func (n *Named) Testify(t *gotest.T) {}

type CartTestSuite struct{ h helper }

func (s *CartTestSuite) TestAdds(t *gotest.T) { s.h.TestReady(t) }

func (s *CartTestSuite) Test(t *gotest.T) {} // want `Test on CartTestSuite never runs: a case needs a name after Test`

type CountTestSuite struct{}

func (s *CountTestSuite) TestCounts(t *gotest.T) {}

func (s *CountTestSuite) Test(n int) {} // want `test method CountTestSuite.Test has wrong signature`

// LoneContractTestSuite has no alias.
type LoneContractTestSuite[T any] struct{} // want `generic suite LoneContractTestSuite never runs`

func (s *LoneContractTestSuite[T]) TestHolds(t *gotest.T) {}

// AliasedContractTestSuite's alias names no suite.
type AliasedContractTestSuite[T any] struct{} // want `generic suite AliasedContractTestSuite never runs`

func (s *AliasedContractTestSuite[T]) TestHolds(t *gotest.T) {}

type IntRunner = AliasedContractTestSuite[int]
