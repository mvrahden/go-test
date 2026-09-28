package badfixture

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *StoreFixture) BeforeEach(ctx context.Context) error { return nil }

type StoreTestSuite struct {
	Store *StoreFixture
}

func (s *StoreTestSuite) BenchmarkRead(b *gotest.B) {}
