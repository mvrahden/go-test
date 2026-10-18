package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PoolFixture struct{}

func (f *PoolFixture) BeforeAll(ctx context.Context) error { return nil }

// Named like suites, but without a case: helpers.

type HelperTestSuite struct{ Prefix string }

func (s *HelperTestSuite) name(n string) string { return s.Prefix + n }

type HookedHelperTestSuite struct{}

func (s *HookedHelperTestSuite) BeforeAll(t *gotest.T) {}
func (s *HookedHelperTestSuite) AfterAll(t *gotest.T)  {}

type BoundHelperTestSuite struct {
	PoolFixture *PoolFixture
}

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) {
	h := HelperTestSuite{Prefix: "x-"}
	gotest.Equal(t, "x-a", h.name("a"))
}
