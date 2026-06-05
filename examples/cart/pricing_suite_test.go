package cart

import "github.com/mvrahden/go-test/pkg/gotest"

type PricingTestSuite struct {
	*CatalogFixture
}

func (s *PricingTestSuite) TestLookupPrice(t *gotest.T) {
	t.It("returns the catalog price", func(it *gotest.T) {
		gotest.Equal(it, 1.50, s.Catalog["Apple"])
	})
}
