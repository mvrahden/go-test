package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

type BrokenFixture struct{}

func (f *BrokenFixture) BeforeAll(ctx context.Context) {}

type LabelFixture string

type (
	GroupedFixture struct{}
	other          int
)

var _ other

type BoundTestSuite struct {
	Store *StoreFixture
}

func (s *BoundTestSuite) TestRuns(t *gotest.T) {}
