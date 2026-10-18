package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

type Store = *StoreFixture

type AliasPtrTestSuite struct {
	Store Store
}

func (s *AliasPtrTestSuite) TestRuns(t *gotest.T) {}
