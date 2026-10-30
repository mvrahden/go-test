package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PoolFixture struct{}

func (f *PoolFixture) BeforeAll(ctx context.Context) error { return nil }

// Named like suites, but without a case: each is refused.

type HelperTestSuite struct{ Prefix string }

func (s *HelperTestSuite) name(n string) string { return s.Prefix + n }

type HookedTestSuite struct{}

func (s *HookedTestSuite) BeforeAll(t *gotest.T) {}
func (s *HookedTestSuite) AfterAll(t *gotest.T)  {}

type BoundTestSuite struct {
	PoolFixture *PoolFixture
}

type X_ExcludedTestSuite struct{}

type F_FocusedTestSuite struct{}

// The generic is no suite until an alias instantiates it.
type ContractTestSuite[T any] struct{}

type IntContractTestSuite = ContractTestSuite[int]

// Declared cases count, excluded ones included.

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) {
	h := HelperTestSuite{Prefix: "x-"}
	gotest.Equal(t, "x-a", h.name("a"))
}

type AllExcludedTestSuite struct{}

func (s *AllExcludedTestSuite) X_TestOne(t *gotest.T) {}

type ExcludedBenchTestSuite struct{}

func (s *ExcludedBenchTestSuite) X_BenchmarkOne(b *gotest.B) {}

type FuzzOnlyTestSuite struct{}

func (s *FuzzOnlyTestSuite) FuzzOne(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in string) {})
}
