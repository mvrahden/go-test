package cart_test

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CatalogExtFixture struct {
	Items map[string]float64
}

func (f *CatalogExtFixture) BeforeAll(_ context.Context) error {
	f.Items = map[string]float64{"Widget": 9.99}
	return nil
}

func (f *CatalogExtFixture) AfterAll(_ context.Context) error {
	f.Items = nil
	return nil
}

type PricingExtTestSuite struct {
	*CatalogExtFixture
}

func (s *PricingExtTestSuite) TestLookupExtPrice(t *gotest.T) {
	t.It("returns the external catalog price", func(it *gotest.T) {
		gotest.Equal(it, 9.99, s.Items["Widget"])
	})
}
