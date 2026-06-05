package cart

import "context"

type CatalogFixture struct {
	Catalog map[string]float64
}

func (f *CatalogFixture) BeforeAll(_ context.Context) error {
	f.Catalog = map[string]float64{"Apple": 1.50, "Bread": 3.00, "Milk": 2.50}
	return nil
}

func (f *CatalogFixture) AfterAll(_ context.Context) error {
	f.Catalog = nil
	return nil
}
