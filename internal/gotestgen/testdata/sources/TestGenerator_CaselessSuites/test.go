package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PoolFixture struct{}

func (f *PoolFixture) BeforeAll(ctx context.Context) error { return nil }

// Standalone suites, one per kind of test-run presence.

type BenchOnlyTestSuite struct{}

func (s *BenchOnlyTestSuite) BenchmarkNoop(b *gotest.B) {
	for b.Loop() {
	}
}

type FuzzOnlyTestSuite struct{}

func (s *FuzzOnlyTestSuite) FuzzEcho(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in string) {})
}

type AllExcludedTestSuite struct{}

func (s *AllExcludedTestSuite) X_TestOne(t *gotest.T) {}

type ExcludedWithFuzzTestSuite struct{}

func (s *ExcludedWithFuzzTestSuite) X_TestOne(t *gotest.T) {}
func (s *ExcludedWithFuzzTestSuite) FuzzEcho(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in string) {})
}

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}

// Fixture-bound suites render through the fixture template.

type BoundBenchTestSuite struct {
	*PoolFixture
}

func (s *BoundBenchTestSuite) BenchmarkNoop(b *gotest.B) {
	for b.Loop() {
	}
}

type BoundExcludedTestSuite struct {
	*PoolFixture
}

func (s *BoundExcludedTestSuite) X_TestOne(t *gotest.T) {}
